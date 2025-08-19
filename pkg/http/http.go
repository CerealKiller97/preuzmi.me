package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
	"html/template"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func Routes(c *container.Container) {
	fs := http.FileServer(http.Dir("./assets"))
	http.Handle("GET /assets/", http.StripPrefix("/assets/", fs))

	http.HandleFunc("GET /", indexHandler())
	http.HandleFunc("GET /dashboard", dashboardHandler(c.Config))
	http.HandleFunc("GET /stats", statsHandler(c.Config))
	http.HandleFunc("GET /receipt/{period}/{provider}", receiptHandler())
	// API endpoints
	http.HandleFunc("GET /api/providers", providersAPIHandler(c.Config))
	http.HandleFunc("GET /api/receipts", receiptsAPIHandler())
	http.HandleFunc("GET /api/stats", statsAPIHandler())
}

type PageData struct {
	URL     string
	Pairs   []string
	Version string
}

type Handler func(http.ResponseWriter, *http.Request)

func indexHandler() Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/dashboard")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}
}

func dashboardHandler(cfg *config.Config) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		template, err := template.ParseFiles("./templates/index.html")
		if err != nil {
			log.Err(err).Msg("Error parsing template")
		}

		pairs, err := utils.GetPairs(cfg)
		if err != nil {
			log.Err(err).Msg("Error getting pairs")
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		version, err := getVersion()
		if err != nil {
			log.Err(err).Msg("Error getting version")
			return
		}
		viewModel := PageData{
			URL:     "/receipt/04-2025/mts",
			Pairs:   pairs,
			Version: string(version),
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

func receiptHandler() Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		provider, period, err := validate(w, r)
		if err != nil {
			log.Err(err).Msg("Error validating request")
			return
		}

		wd, err := os.Getwd()
		if err != nil {
			log.Err(err).Msg("Error getting working directory")
		}

		p := path.Join(wd, "receipts", period, fmt.Sprintf("%s.pdf", provider))

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
}

// scanReceipts walks the ./receipts directory and returns all available receipts
func scanReceipts() ([]APIReceipt, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root := path.Join(wd, "receipts")

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
		entries = append(entries, APIReceipt{
			Provider: provider,
			Period:   dir,
			URL:      url,
			FileName: filepath.Base(p),
			Size:     fi.Size(),
			Modified: fi.ModTime().Unix(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// statsHandler renders the statistics page
type StatsPageData = PageData

func statsHandler(cfg *config.Config) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("./templates/stats.html")
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
		version, _ := getVersion()
		viewModel := StatsPageData{
			URL:     "/stats",
			Pairs:   pairs,
			Version: string(version),
		}
		if err := tmpl.Execute(w, viewModel); err != nil {
			log.Err(err).Msg("Error executing stats template")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

func getVersion() ([]byte, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	file, err := os.ReadFile(path.Join(wd, "version"))
	if err != nil {
		return nil, err
	}
	return file, nil
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

type ReceiptMeta struct {
	Provider string  `json:"provider"`
	Period   string  `json:"period"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// loadReceiptMeta reads receipts/meta.json if present; otherwise returns empty slice.
func loadReceiptMeta() ([]ReceiptMeta, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	p := path.Join(wd, "receipts", "meta.json")
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
		providers := make([]string, 0, len(cfg.Providers))
		for p := range cfg.Providers {
			providers = append(providers, string(p))
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(providers); err != nil {
			log.Err(err).Msg("Error encoding providers")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}

// receiptsAPIHandler returns the list of receipts; optional query params: provider (comma-separated), period
func receiptsAPIHandler() Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := scanReceipts()
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

// statsAPIHandler aggregates monthly and annual totals; query: year (int), provider (comma-separated)
func statsAPIHandler() Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := scanReceipts()
		if err != nil {
			log.Err(err).Msg("Error scanning receipts for stats")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		meta, err := loadReceiptMeta()
		if err != nil {
			log.Err(err).Msg("Error loading receipt metadata")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Build meta map
		mmap := make(map[string]ReceiptMeta, len(meta))
		for _, m := range meta {
			key := strings.ToLower(fmt.Sprintf("%s|%s", m.Period, m.Provider))
			mmap[key] = m
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
			if metaRec, ok := mmap[strings.ToLower(fmt.Sprintf("%s|%s", it.Period, it.Provider))]; ok {
				monthly[idx] += metaRec.Amount
				if currency == "" && metaRec.Currency != "" {
					currency = metaRec.Currency
				}
			}
		}
		total := 0.0
		for _, v := range monthly {
			total += v
		}
		avg := 0.0
		avg = total / 12.0
		// Model
		resp := map[string]any{
			"year":           year,
			"monthly":        monthly,
			"monthly_counts": counts,
			"total":          total,
			"average":        avg,
			"currency":       currency,
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Err(err).Msg("Error encoding stats")
			w.WriteHeader(http.StatusInternalServerError)
		}
	}
}
