package http

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/notify"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/payments"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
)

func Routes(c *container.Container) {
	// The configured download path is the single source of truth: it is where
	// the providers write and therefore where everything else must read from.
	dir := c.GetConfig().DownloadPath

	// A nil store is tolerated: receipts then simply render as unpaid, and the
	// mark-paid endpoint reports the failure rather than silently resetting a
	// file that may just be corrupt.
	paidStore, err := paymentsStore(dir)
	if err != nil {
		log.Err(err).Msg("Error loading payments store, paid state is unavailable")
	}

	http.Handle("GET /assets/", http.StripPrefix("/assets/", staticAssets()))

	// Browsers request /favicon.ico from the root no matter what the markup
	// declares, so serve it there as well.
	http.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./assets/favicon.ico")
	})

	http.HandleFunc("GET /", indexHandler())
	http.HandleFunc("GET /dashboard", dashboardHandler(
		c.GetConfig(),
		c.GetVersion(),
	))
	http.HandleFunc("GET /stats", statsHandler(
		c.GetConfig(),
		c.GetVersion(),
	))
	http.HandleFunc("GET /settings", settingsHandler(c))
	http.HandleFunc("GET /receipt/{period}/{provider}", receiptHandler(dir))
	// API endpoints
	http.HandleFunc("GET /api/providers", providersAPIHandler(c.GetConfig()))
	http.HandleFunc("GET /api/receipts", receiptsAPIHandler(dir, paidStore))
	http.HandleFunc("PUT /api/receipts/{period}/{provider}/paid", markPaidHandler(paidStore))
	http.HandleFunc("GET /api/stats", statsAPIHandler(dir))
	http.HandleFunc("GET /api/expenses/monthly", expensesMonthlyAPIHandler(dir))

	refreshSvc, err := refresh.New(dir)
	if err != nil {
		log.Err(err).Msg("Error loading refresh state, last-fetched time is unavailable")
	}
	http.HandleFunc("GET /api/refresh", refreshStatusHandler(c, refreshSvc))
	http.HandleFunc("POST /api/refresh", startRefreshHandler(c, refreshSvc))
	http.HandleFunc("POST /api/notifications/test", testNotificationHandler(c))
}

// refreshStatusHandler reports whether a refresh is running and when the last
// one finished. The UI polls this while a run is in flight.
func refreshStatusHandler(c *container.Container, svc *refresh.Service) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			http.Error(w, "refresh unavailable", http.StatusServiceUnavailable)
			return
		}

		cfg := c.GetConfig()
		writeJSON(w, svc.State().WithWindow(cfg.CheckUntil, cfg.RefreshAllowed()))
	}
}

// startRefreshHandler triggers a download from every configured provider.
//
// This is the same work as the `checks` CLI command. It returns 202 straight
// away rather than blocking, because a full run talks to several external
// providers and can take a while.
func startRefreshHandler(c *container.Container, svc *refresh.Service) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			http.Error(w, "refresh unavailable", http.StatusServiceUnavailable)
			return
		}

		cfg := c.GetConfig()
		if !cfg.RefreshAllowed() {
			w.WriteHeader(http.StatusForbidden)
			writeJSON(w, svc.State().WithWindow(cfg.CheckUntil, false))

			return
		}

		pairs, err := utils.GetPairs(cfg)
		if err != nil {
			log.Err(err).Msg("Refresh requested with no providers configured")
			http.Error(w, "no providers configured", http.StatusBadRequest)

			return
		}

		providers := c.GetProviders(pairs)
		if len(providers) == 0 {
			http.Error(w, "no providers with an implementation are configured", http.StatusBadRequest)
			return
		}

		state, err := svc.Start(providers, c.GetNotifier().HandleResults)
		state = state.WithWindow(cfg.CheckUntil, true)
		if err != nil {
			// Already running is not a failure, the client just gets the
			// in-flight state back.
			if errors.Is(err, refresh.ErrAlreadyRunning) {
				w.WriteHeader(http.StatusConflict)
				writeJSON(w, state)

				return
			}

			log.Err(err).Msg("Error starting refresh")
			http.Error(w, "could not start refresh", http.StatusInternalServerError)

			return
		}

		log.Info().Interface("providers", pairs).Msg("Refresh started from the UI")

		w.WriteHeader(http.StatusAccepted)
		writeJSON(w, state)
	}
}

// testNotificationHandler sends a probe message through the configured
// driver so the user can verify bot/SMTP credentials without a full refresh.
func testNotificationHandler(c *container.Container) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		notifier := c.GetNotifier()
		if err := notifier.Test(); err != nil {
			if errors.Is(err, notify.ErrNotConfigured) {
				http.Error(w, "notification driver is not configured", http.StatusBadRequest)
				return
			}

			log.Err(err).Msg("Test notification failed")
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		writeJSON(w, map[string]any{"ok": true})
	}
}

// writeJSON encodes v as the response body.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Err(err).Msg("Error encoding response")
	}
}

// paymentsStore opens the paid-receipts store inside the receipts directory.
func paymentsStore(dir string) (*payments.Store, error) {
	return payments.New(dir)
}

// staticAssets serves ./assets, forcing the browser to revalidate every time.
//
// http.FileServer only sets Last-Modified, and with no Cache-Control browsers
// fall back to heuristic freshness (roughly 10% of the file's age), which means
// a rebuilt stylesheet can keep serving from cache for hours. "no-cache" does
// not disable caching, it just requires a revalidation, so unchanged files
// still come back as a cheap 304.
func staticAssets() http.Handler {
	fs := http.FileServer(http.Dir("./assets"))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		fs.ServeHTTP(w, r)
	})
}

type PageData struct {
	URL   string
	Pairs []string
	// Active marks the current nav item so the shared header can highlight it.
	Active string
	// Title and Description feed both the <title> tag and the Open Graph cards.
	Title       string
	Description string
	// BaseURL is the absolute scheme://host the page was served from. Open
	// Graph requires absolute URLs, and hardcoding one would break the moment
	// the app is reached on a different host or port.
	BaseURL string
	Version string
}

// baseURL reconstructs the absolute origin of the current request, honouring
// the headers a reverse proxy would set.
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded != "" {
		scheme = forwarded
	}

	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = forwarded
	}

	return scheme + "://" + host
}

type ProviderTotal struct {
	Provider string  `json:"provider"`
	Amount   float64 `json:"amount"`
}

// ProviderMonthly carries one provider's amounts for every month of the year,
// so the UI can draw a separate chart line and table column per provider.
type ProviderMonthly struct {
	Provider string    `json:"provider"`
	Monthly  []float64 `json:"monthly"`
}

type StatsResponse struct {
	Year              int               `json:"year"`
	Monthly           []float64         `json:"monthly"`
	MonthlyCounts     []int             `json:"monthly_counts"`
	ByProvider        []ProviderTotal   `json:"by_provider"`
	MonthlyByProvider []ProviderMonthly `json:"monthly_by_provider"`
	Total             float64           `json:"total"`
	Average           float64           `json:"average"`
	Currency          string            `json:"currency"`
}

type Handler func(http.ResponseWriter, *http.Request)

func indexHandler() Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/dashboard")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}
}

func dashboardHandler(cfg *config.Config, version string) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		template, err := template.ParseFiles(
			"./templates/index.html",
			"./templates/partials.html",
		)
		if err != nil {
			log.Err(err).Msg("Error parsing template")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		pairs, err := utils.GetPairs(cfg)
		if err != nil {
			// An unconfigured install still renders; the list is just empty.
			log.Err(err).Msg("Error getting pairs")
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		viewModel := PageData{
			URL:         "/dashboard",
			Pairs:       pairs,
			Active:      "receipts",
			Title:       "Preuzmi.me — Računi",
			Description: "Automatsko preuzimanje računa za internet, telefon i struju na jednom mestu.",
			BaseURL:     baseURL(r),
			Version:     version,
		}

		if err := template.Execute(w, viewModel); err != nil {
			log.Err(err).Msg("Error executing template")
			return
		}
	}
}

func validate(w http.ResponseWriter, req *http.Request) (string, string, error) {
	provider := req.PathValue("provider")
	if provider == "" {
		w.WriteHeader(http.StatusBadRequest)
		return "", "", errors.New("provider is empty")
	}

	if !slices.Contains([]string{"mts", "a1", "esanduce", "yettel", "eps"}, provider) {
		w.WriteHeader(http.StatusBadRequest)
		return "", "", errors.New("provider is invalid")
	}

	period := req.PathValue("period")

	if period == "" {
		w.WriteHeader(http.StatusBadRequest)
		return "", "", errors.New("period is empty")
	}

	return provider, period, nil
}

func receiptHandler(dir string) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		provider, period, err := validate(w, r)
		if err != nil {
			log.Err(err).Msg("Error validating request")
			return
		}

		p := path.Join(dir, period, fmt.Sprintf("%s.pdf", provider))

		file, err := os.ReadFile(p)
		if err != nil {
			log.Err(err).Msg("Error reading file")
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/pdf")
		_, err = w.Write(file)
		if err != nil {
			log.Err(err).Msg("Error writing file")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// API models

type APIReceipt struct {
	Provider string `json:"provider"`
	Period   string `json:"period"`
	URL      string `json:"url"`
	FileName string `json:"filename"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
	// Amount and Currency come from receipts/meta.json and are zero-valued
	// when a receipt has no matching metadata entry.
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	// Paid and PaidAt come from receipts/payments.json.
	Paid   bool  `json:"paid"`
	PaidAt int64 `json:"paid_at"`
}

// scanReceipts walks the ./receipts directory and returns all available
// receipts. A nil paid store simply leaves every receipt marked unpaid.
func scanReceipts(dir string, paid *payments.Store) ([]APIReceipt, error) {
	root := dir

	// Index the metadata so each receipt can be annotated with its amount.
	meta, err := loadReceiptMeta(dir)
	if err != nil {
		return nil, err
	}
	amounts := make(map[string]ReceiptMeta, len(meta))
	for _, m := range meta {
		amounts[metaKey(m.Period, m.Provider)] = m
	}

	entries := make([]APIReceipt, 0, 16)
	// Walk directories like receipts/{period}/*.pdf
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
		// Extract period and provider
		dir := filepath.Base(filepath.Dir(p))
		provider := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		// Build URL
		url := fmt.Sprintf("/receipt/%s/%s", dir, provider)
		// Stat for size/mod time
		fi, statErr := os.Stat(p)
		if statErr != nil {
			return statErr
		}

		receipt := APIReceipt{
			Provider: provider,
			Period:   normalizePeriod(dir),
			URL:      url,
			FileName: filepath.Base(p),
			Size:     fi.Size(),
			Modified: fi.ModTime().Unix(),
		}

		key := metaKey(dir, provider)

		if m, ok := amounts[key]; ok {
			receipt.Amount = m.Amount
			receipt.Currency = m.Currency
		}

		if paid != nil {
			if at, ok := paid.PaidAt(key); ok {
				receipt.Paid = true
				receipt.PaidAt = at
			}
		}

		entries = append(entries, receipt)

		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// statsHandler renders the statistics page
type StatsPageData = PageData

func statsHandler(cfg *config.Config, version string) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles(
			"./templates/stats.html",
			"./templates/partials.html",
		)
		if err != nil {
			log.Err(err).Msg("Error parsing stats template")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		pairs, err := utils.GetPairs(cfg)
		if err != nil {
			// not fatal for page; keep empty list
			log.Err(err).Msg("Error getting pairs for stats")
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		viewModel := StatsPageData{
			URL:         "/stats",
			Pairs:       pairs,
			Active:      "stats",
			Title:       "Preuzmi.me — Statistika",
			Description: "Pregled troškova po mesecima i provajderima za izabranu godinu.",
			BaseURL:     baseURL(r),
			Version:     version,
		}
		if err := tmpl.Execute(w, viewModel); err != nil {
			log.Err(err).Msg("Error executing stats template")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

// parsePeriod parses a period like "04-2025" or "010-2025" into (month, year). Returns (0,0) on error.
func parsePeriod(p string) (int, int) {
	parts := strings.Split(p, "-")
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

type ReceiptMeta struct {
	Provider string  `json:"provider"`
	Period   string  `json:"period"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
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
func receiptsAPIHandler(dir string, paid *payments.Store) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := scanReceipts(dir, paid)
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
func markPaidHandler(paid *payments.Store) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if paid == nil {
			log.Error().Msg("Payments store unavailable, cannot record paid state")
			http.Error(w, "payments store unavailable", http.StatusServiceUnavailable)
			return
		}

		provider, period, err := validate(w, r)
		if err != nil {
			log.Err(err).Msg("Error validating mark-paid request")
			return
		}

		var body struct {
			Paid bool `json:"paid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		at, err := paid.Set(metaKey(period, provider), body.Paid)
		if err != nil {
			log.Err(err).
				Str("provider", provider).
				Str("period", period).
				Msg("Error persisting paid state")
			http.Error(w, "could not persist paid state", http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"provider": provider,
			"period":   normalizePeriod(period),
			"paid":     body.Paid,
			"paid_at":  at,
		}); err != nil {
			log.Err(err).Msg("Error encoding mark-paid response")
		}
	}
}

// statsAPIHandler aggregates monthly and annual totals; query: year (int), provider (comma-separated)
func statsAPIHandler(dir string) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := scanReceipts(dir, nil)
		if err != nil {
			log.Err(err).Msg("Error scanning receipts for stats")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		meta, err := loadReceiptMeta(dir)
		if err != nil {
			log.Err(err).Msg("Error loading receipt metadata")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Provider filter
		allowed := map[string]struct{}{}
		providersParam := strings.TrimSpace(r.URL.Query().Get("provider"))
		if providersParam != "" {
			for _, p := range strings.Split(providersParam, ",") {
				p = strings.TrimSpace(strings.ToLower(p))
				if p != "" {
					allowed[p] = struct{}{}
				}
			}
		}
		// Year
		year := 0
		if yStr := strings.TrimSpace(r.URL.Query().Get("year")); yStr != "" {
			if y, err := strconv.Atoi(yStr); err == nil {
				year = y
			}
		}
		if year == 0 {
			year = time.Now().Year()
		}
		// Aggregate
		monthly := make([]float64, 12)
		counts := make([]int, 12)
		currency := ""
		for _, it := range items {
			m, y := parsePeriod(it.Period)
			if m < 1 || m > 12 || y != year {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[strings.ToLower(it.Provider)]; !ok {
					continue
				}
			}
			idx := m - 1
			counts[idx]++
		}
		// Aggregate amounts from metadata independently (dummy/demo friendly)
		perProvider := map[string]float64{}
		perProviderMonthly := map[string]*[12]float64{}
		for _, metaRec := range meta {
			m, y := parsePeriod(metaRec.Period)
			if m < 1 || m > 12 || y != year {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[strings.ToLower(metaRec.Provider)]; !ok {
					continue
				}
			}
			idx := m - 1
			name := strings.ToLower(metaRec.Provider)
			monthly[idx] += metaRec.Amount
			perProvider[name] += metaRec.Amount

			months, ok := perProviderMonthly[name]
			if !ok {
				months = &[12]float64{}
				perProviderMonthly[name] = months
			}
			months[idx] += metaRec.Amount

			if currency == "" && metaRec.Currency != "" {
				currency = metaRec.Currency
			}
		}

		total := 0.0
		for _, v := range monthly {
			total += v
		}

		// Largest spender first, so the UI can render the breakdown as-is.
		byProvider := make([]ProviderTotal, 0, len(perProvider))
		for name, amount := range perProvider {
			byProvider = append(byProvider, ProviderTotal{Provider: name, Amount: amount})
		}
		slices.SortFunc(byProvider, func(a, b ProviderTotal) int {
			if a.Amount != b.Amount {
				return cmp.Compare(b.Amount, a.Amount)
			}

			return strings.Compare(a.Provider, b.Provider)
		})

		// Same order as the totals, so chart lines and table columns line up
		// with the breakdown list.
		monthlyByProvider := make([]ProviderMonthly, 0, len(byProvider))
		for _, pt := range byProvider {
			monthlyByProvider = append(monthlyByProvider, ProviderMonthly{
				Provider: pt.Provider,
				Monthly:  perProviderMonthly[pt.Provider][:],
			})
		}

		// Model
		resp := StatsResponse{
			Year:              year,
			Monthly:           monthly,
			MonthlyCounts:     counts,
			ByProvider:        byProvider,
			MonthlyByProvider: monthlyByProvider,
			Total:             total,
			Average:           total / 12.0,
			Currency:          currency,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Err(err).Msg("Error encoding stats")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// expensesMonthlyAPIHandler returns monthly expenses aggregated from metadata only
func expensesMonthlyAPIHandler(dir string) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		meta, err := loadReceiptMeta(dir)
		if err != nil {
			log.Err(err).Msg("Error loading receipt metadata for expenses")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Provider filter
		allowed := map[string]struct{}{}
		if providersParam := strings.TrimSpace(r.URL.Query().Get("provider")); providersParam != "" {
			for _, p := range strings.Split(providersParam, ",") {
				p = strings.TrimSpace(strings.ToLower(p))
				if p != "" {
					allowed[p] = struct{}{}
				}
			}
		}

		// Year param (default to current)
		year := 0
		if yStr := strings.TrimSpace(r.URL.Query().Get("year")); yStr != "" {
			if y, err := strconv.Atoi(yStr); err == nil {
				year = y
			}
		}
		if year == 0 {
			year = time.Now().Year()
		}

		monthly := make([]float64, 12)
		currency := ""

		for _, m := range meta {
			mm, yy := parsePeriod(m.Period)
			if mm < 1 || mm > 12 || yy != year {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[strings.ToLower(m.Provider)]; !ok {
					continue
				}
			}
			monthly[mm-1] += m.Amount
			if currency == "" && m.Currency != "" {
				currency = m.Currency
			}
		}

		names := []string{"Jan", "Feb", "Mar", "Apr", "Maj", "Jun", "Jul", "Avg", "Sep", "Okt", "Nov", "Dec"}
		months := make([]map[string]any, 12)
		total := 0.0
		for i := 0; i < 12; i++ {
			amt := monthly[i]
			total += amt
			months[i] = map[string]any{
				"index":  i + 1,
				"name":   names[i],
				"amount": amt,
			}
		}
		avg := total / 12.0

		resp := map[string]any{
			"year":     year,
			"currency": currency,
			"months":   months,
			"total":    total,
			"average":  avg,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Err(err).Msg("Error encoding monthly expenses")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}
