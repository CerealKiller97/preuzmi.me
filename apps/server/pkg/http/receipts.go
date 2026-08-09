package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/payments"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
)

// API models

type APIReceipt struct {
	Provider string `json:"provider"`
	// Account is the provider account id ("" for a solo deployment); Label is its
	// display name ("Mama"). Both are omitted for solo so the payload is unchanged
	// from before multi-account support.
	Account      string  `json:"account,omitempty"`
	Label        string  `json:"label,omitempty"`
	Period       string  `json:"period"`
	URL          string  `json:"url"`
	FileName     string  `json:"filename"`
	Currency     string  `json:"currency"`
	Status       string  `json:"status"`
	Size         int64   `json:"size"`
	Modified     int64   `json:"modified"`
	DownloadedAt int64   `json:"downloaded_at"`
	Amount       float64 `json:"amount"`
	PaidAt       int64   `json:"paid_at"`
	ConfirmedAt  int64   `json:"confirmed_at"`
	DueAt        int64   `json:"due_at"`
	Paid         bool    `json:"paid"`
	Confirmed    bool    `json:"confirmed"`
}

// collectReceipts returns the dashboard's view of every receipt, choosing the
// source that actually holds them for the configured backend.
//
// With local storage the PDFs sit under download_path, so the on-disk walk is
// the source of truth. With S3 the PDFs live in the bucket and download_path
// holds only the index, so walking disk finds nothing — the receipts database
// becomes the source instead. Both paths annotate amounts from meta.json and
// paid state from the payments store, so the UI sees an identical shape.
func collectReceipts(cfg *config.Config, rec *receipts.Repository) ([]APIReceipt, error) {
	if cfg.Storage == config.StorageS3 {
		return scanReceiptsDB(cfg, rec)
	}

	return scanReceipts(cfg, rec)
}

// CollectReceipts exposes the dashboard's receipt view to out-of-band callers
// (the receipts CLI), so a listing on the command line matches the UI exactly:
// same source per backend, same meta.json amounts, same paid state.
func CollectReceipts(cfg *config.Config, rec *receipts.Repository) ([]APIReceipt, error) {
	return collectReceipts(cfg, rec)
}

// NormalizePeriod exposes the canonical "MM-YYYY" period form used to build
// receipt keys, so the CLI and the UI agree on the paid-state key for a period.
func NormalizePeriod(p string) string {
	return normalizePeriod(p)
}

// scanReceiptsDB builds the receipt list from the database rather than the
// filesystem, for backends (S3) whose objects are not on local disk. A nil
// store yields an empty list: without the index there is nothing to enumerate,
// since the PDFs are remote.
func scanReceiptsDB(cfg *config.Config, rec *receipts.Repository) ([]APIReceipt, error) {
	if rec == nil {
		return []APIReceipt{}, nil
	}

	// meta.json (hand-curated amounts/currency) still lives on disk and is
	// optional, matching the on-disk walk's behaviour.
	meta, err := loadReceiptMeta(cfg.DownloadPath)
	if err != nil {
		return nil, err
	}
	amounts := make(map[string]ReceiptMeta, len(meta))
	for _, m := range meta {
		amounts[metaKey(m.Period, m.Provider)] = m
	}

	rows, err := rec.List(context.Background())
	if err != nil {
		return nil, err
	}

	entries := make([]APIReceipt, 0, len(rows))
	for _, row := range rows {
		// Skip paid-only stubs (see markPaidQuery): a receipt marked paid but
		// never downloaded has no PDF to show and no storage key.
		if row.StorageKey == "" {
			continue
		}

		period := normalizePeriod(row.Period)
		label := ""
		if row.Account != "" {
			label = accountLabels(cfg, row.Provider)[row.Account]
		}

		receipt := APIReceipt{
			Provider: row.Provider,
			Account:  row.Account,
			Label:    label,
			Period:   period,
			// The period column is slash-form; the route embeds it as a path
			// segment, so build the URL with the dash form.
			URL:          receiptURL(period, row.Provider, row.Account),
			FileName:     fmt.Sprintf("%s.pdf", row.Provider),
			Status:       row.Status,
			Size:         row.SizeBytes,
			Modified:     row.DownloadedAt,
			DownloadedAt: row.DownloadedAt,
			Amount:       row.Price,
			Paid:         row.PaidAt > 0,
			PaidAt:       row.PaidAt,
			Confirmed:    row.ConfirmedAt > 0,
			ConfirmedAt:  row.ConfirmedAt,
			DueAt:        row.DueAt,
		}

		// Hand-curated meta.json predates multi-account, so only a solo receipt can
		// match it (account ''); a named account keeps its database price.
		key := metaKey(row.Period, row.Provider)
		if m, ok := amounts[key]; ok && row.Account == "" {
			// Hand-curated metadata wins over the database price, mirroring the
			// on-disk walk.
			if m.Amount != 0 {
				receipt.Amount = m.Amount
			}
			receipt.Currency = m.Currency
		}

		entries = append(entries, receipt)
	}

	return entries, nil
}

// scanReceipts walks the ./receipts directory and returns all available
// receipts. A nil receipts store simply leaves every receipt unpaid and its
// provider-reported status empty.
func scanReceipts(cfg *config.Config, rec *receipts.Repository) ([]APIReceipt, error) {
	root := cfg.DownloadPath
	ctx := context.Background()

	// Index the metadata so each receipt can be annotated with its amount.
	meta, err := loadReceiptMeta(root)
	if err != nil {
		return nil, err
	}
	amounts := make(map[string]ReceiptMeta, len(meta))
	for _, m := range meta {
		amounts[metaKey(m.Period, m.Provider)] = m
	}

	// Index the database rows (keyed by provider, account and period) so each
	// receipt can carry its provider-reported status and price.
	dbByKey := map[string]receipts.Receipt{}
	if rec != nil {
		rows, listErr := rec.List(ctx)
		if listErr != nil {
			return nil, listErr
		}
		for _, row := range rows {
			dbByKey[dbKey(row.Period, row.Provider, row.Account)] = row
		}
	}

	entries := make([]APIReceipt, 0, 16)
	// Walk both the solo layout receipts/{period}/{provider}.pdf and the
	// per-account layout receipts/{period}/{provider}/{account}.pdf.
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".pdf" {
			return nil
		}

		period, provider, account, ok := receiptPathParts(root, p)
		if !ok {
			// A PDF somewhere other than a receipt path (e.g. a stray file); skip it.
			return nil
		}

		// Stat for size/mod time
		fi, statErr := os.Stat(p)
		if statErr != nil {
			return statErr
		}

		label := ""
		if account != "" {
			label = accountLabels(cfg, provider)[account]
		}

		receipt := APIReceipt{
			Provider: provider,
			Account:  account,
			Label:    label,
			Period:   normalizePeriod(period),
			URL:      receiptURL(normalizePeriod(period), provider, account),
			FileName: filepath.Base(p),
			Size:     fi.Size(),
			Modified: fi.ModTime().Unix(),
		}

		// meta.json predates multi-account, so it only annotates solo receipts.
		if account == "" {
			if m, ok := amounts[metaKey(period, provider)]; ok {
				receipt.Amount = m.Amount
				receipt.Currency = m.Currency
			}
		}

		if row, ok := dbByKey[dbKey(period, provider, account)]; ok {
			receipt.Status = row.Status
			receipt.DownloadedAt = row.DownloadedAt
			// Fall back to the database price when hand-curated metadata does
			// not carry an amount for this receipt.
			if receipt.Amount == 0 {
				receipt.Amount = row.Price
			}
			receipt.Paid = row.PaidAt > 0
			receipt.PaidAt = row.PaidAt
			receipt.Confirmed = row.ConfirmedAt > 0
			receipt.ConfirmedAt = row.ConfirmedAt
			receipt.DueAt = row.DueAt
		}

		entries = append(entries, receipt)

		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// receiptPathParts splits a walked PDF path under root into its period, provider
// and account, mirroring storage.ReceiptKey. A solo receipt sits at
// {period}/{provider}.pdf (account ""); a per-account receipt at
// {period}/{provider}/{account}.pdf. ok is false for a PDF at any other depth,
// so state files or stray PDFs are ignored rather than misread.
func receiptPathParts(root, p string) (period, provider, account string, ok bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return "", "", "", false
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	base := strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1]))

	switch len(parts) {
	case 2:
		// {period}/{provider}.pdf
		return parts[0], base, "", true
	case 3:
		// {period}/{provider}/{account}.pdf
		return parts[0], parts[1], base, true
	default:
		return "", "", "", false
	}
}

// parsePeriod parses a period like "04-2025" or "010-2025" into (month, year). Returns (0,0) on error.
func parsePeriod(p string) (int, int) {
	// Periods may arrive dash-separated ("06-2026", from folders/meta.json) or
	// slash-separated ("06/2026", from the database column); accept both.
	parts := strings.Split(strings.ReplaceAll(p, "/", "-"), "-")
	if len(parts) != 2 {
		return 0, 0
	}
	mStr := strings.TrimLeft(parts[0], "0")
	if mStr == "" {
		mStr = "0"
	}
	yStr := parts[1]
	m, err1 := strconv.Atoi(mStr)
	y, err2 := strconv.Atoi(yStr)
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return m, y
}

// normalizePeriod rewrites a period to a canonical zero-padded "MM-YYYY".
// On-disk folders are currently created as "010-2025" for October, while
// receipts/meta.json uses "10-2025"; both normalize to the same value.
func normalizePeriod(p string) string {
	m, y := parsePeriod(p)
	if m == 0 || y == 0 {
		return p
	}

	return fmt.Sprintf("%02d-%d", m, y)
}

// metaKey builds the lookup key joining a receipt to its metadata and its paid
// state. Both use the same key so the two can never drift apart.
func metaKey(period, provider string) string {
	return payments.Key(normalizePeriod(period), provider)
}

// dbKey is the account-aware lookup key joining a walked file to its database
// row, so several accounts of one provider and period stay distinct. It extends
// metaKey with the account id (empty for a solo receipt, keeping the solo key
// equal to what a lookup by (period, provider) would build).
func dbKey(period, provider, account string) string {
	if account == "" {
		return metaKey(period, provider)
	}

	return metaKey(period, provider) + "\x00" + account
}

// receiptURL builds the receipt's download URL. A solo receipt keeps the bare
// /receipt/{period}/{provider} path; a named account adds ?account=<id>, which
// every receipt handler reads (see accountParam).
func receiptURL(period, provider, account string) string {
	u := fmt.Sprintf("/receipt/%s/%s", period, provider)
	if account != "" {
		u += "?account=" + account
	}

	return u
}

// accountLabels builds an id→label map for a provider from config, so the
// listing can annotate database rows (which carry only the account id) with the
// friendly name. A solo provider yields an empty map.
func accountLabels(cfg *config.Config, provider string) map[string]string {
	out := map[string]string{}
	for _, a := range cfg.Accounts(provider) {
		if a.ID != "" {
			out[a.ID] = a.Label
		}
	}

	return out
}

type ReceiptMeta struct {
	Provider string  `json:"provider"`
	Period   string  `json:"period"`
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// loadReceiptMeta reads receipts/meta.json if present; otherwise returns empty slice.
func loadReceiptMeta(dir string) ([]ReceiptMeta, error) {
	p := path.Join(dir, "meta.json")
	b, err := os.ReadFile(p)
	if err != nil {
		// No metadata is fine
		return []ReceiptMeta{}, nil
	}
	var items []ReceiptMeta
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// providersAPIHandler returns all configured provider keys from config
func providersAPIHandler(cfg *config.Config) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		// Nothing configured yet is an ordinary state for a fresh install, not
		// a server error: answer with an empty list so the UI still renders.
		providers, err := utils.GetPairs(cfg)
		if err != nil {
			if !errors.Is(err, utils.ErrEmptyProviders) {
				log.Err(err).Msg("Error getting pairs for providers")
				http.Error(w, "could not read providers", http.StatusInternalServerError)

				return
			}

			providers = []string{}
		}

		writeJSON(w, providers)
	}
}

// receiptsAPIHandler returns the list of receipts; optional query params: provider (comma-separated), period
func receiptsAPIHandler(cfg *config.Config, receiptsStore func() *receipts.Repository) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := collectReceipts(cfg, receiptsStore())
		if err != nil {
			log.Err(err).Msg("Error scanning receipts")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Apply filters if provided
		providersParam := strings.TrimSpace(r.URL.Query().Get("provider"))
		periodParam := strings.TrimSpace(r.URL.Query().Get("period"))

		if providersParam != "" || periodParam != "" {
			allowed := map[string]struct{}{}
			if providersParam != "" {
				for _, p := range strings.Split(providersParam, ",") {
					p = strings.TrimSpace(strings.ToLower(p))
					if p != "" {
						allowed[p] = struct{}{}
					}
				}
			}
			filtered := make([]APIReceipt, 0, len(items))
			for _, it := range items {
				if len(allowed) > 0 {
					if _, ok := allowed[strings.ToLower(it.Provider)]; !ok {
						continue
					}
				}
				if periodParam != "" && !strings.EqualFold(it.Period, periodParam) {
					continue
				}
				filtered = append(filtered, it)
			}
			items = filtered
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(items); err != nil {
			log.Err(err).Msg("Error encoding receipts")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// markPaidHandler toggles the paid flag for a single receipt.
//
// Body: {"paid": true|false}. Responds with the stored state so the client does
// not have to guess the timestamp it was given.
func markPaidHandler(rec *receipts.Repository) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if rec == nil {
			log.Error().Msg("Receipts database unavailable, cannot record paid state")
			http.Error(w, "receipts database unavailable", http.StatusServiceUnavailable)
			return
		}

		provider, period, err := validate(w, r)
		if err != nil {
			log.Err(err).Msg("Error validating mark-paid request")
			return
		}
		account := accountParam(r)

		var body struct {
			Paid bool `json:"paid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		at, err := rec.MarkPaid(r.Context(), provider, account, period, body.Paid)
		if err != nil {
			log.Err(err).
				Str("provider", provider).
				Str("account", account).
				Str("period", period).
				Msg("Error persisting paid state")
			http.Error(w, "could not persist paid state", http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"provider": provider,
			"account":  account,
			"period":   normalizePeriod(period),
			"paid":     body.Paid,
			"paid_at":  at,
		}); err != nil {
			log.Err(err).Msg("Error encoding mark-paid response")
		}
	}
}
