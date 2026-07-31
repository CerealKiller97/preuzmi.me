package http

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
	"github.com/rs/zerolog/log"
)

type Handler func(http.ResponseWriter, *http.Request)

func Routes(c *container.Container) {
	// Path-aware cache so changing download_path in settings recreates this
	// without requiring a process restart. Paid state now lives in the receipts
	// database (c.GetReceiptsStore), so it needs no separate store here.
	var (
		storeMu     sync.Mutex
		refreshPath string
		refreshSvc  *refresh.Service
	)

	getRefresh := func() *refresh.Service {
		storeMu.Lock()
		defer storeMu.Unlock()

		dir := c.GetConfig().DownloadPath
		if refreshSvc == nil || refreshPath != dir {
			s, err := refresh.New(dir)
			if err != nil {
				log.Err(err).Msg("Error loading refresh state, last-fetched time is unavailable")
				refreshSvc = nil
			} else {
				refreshSvc = s
			}
			refreshPath = dir
		}

		return refreshSvc
	}

	// Warm the cache once at startup so the first request is not colder.
	_ = getRefresh()

	http.Handle("GET /assets/", http.StripPrefix("/assets/", staticAssets()))

	// Browsers request /favicon.ico from the root no matter what the markup
	// declares, so serve it there as well.
	http.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./assets/favicon.ico")
	})

	http.HandleFunc("GET /", indexHandler())
	http.HandleFunc("GET /dashboard", dashboardHandler(c.GetConfig(), c.GetVersion()))
	http.HandleFunc("GET /pay", payHandler(c.GetConfig(), c.GetVersion()))
	http.HandleFunc("GET /stats", statsHandler(c.GetConfig(), c.GetVersion()))
	http.HandleFunc("GET /settings", settingsHandler(c.GetConfig(), c.GetVersion(), c.GetNotifier()))
	http.HandleFunc("GET /receipt/{period}/{provider}", receiptHandler(c.GetStorage()))
	// The IPS payment QR lifted from the bill PDF: a rendered PNG for the card,
	// and the raw payload as a copy/deep-link fallback for phone-only users.
	http.HandleFunc("GET /receipt/{period}/{provider}/qr.png", receiptQRImageHandler(c.GetStorage(), c.GetReceiptsStore))
	http.HandleFunc("GET /receipt/{period}/{provider}/qr.txt", receiptQRPayloadHandler(c.GetStorage(), c.GetReceiptsStore))
	// API endpoints
	http.HandleFunc("GET /api/providers", providersAPIHandler(c.GetConfig()))
	http.HandleFunc("GET /api/receipts", receiptsAPIHandler(c.GetConfig(), c.GetReceiptsStore))
	http.HandleFunc("PUT /api/receipts/{period}/{provider}/paid", func(w http.ResponseWriter, r *http.Request) {
		markPaidHandler(c.GetReceiptsStore())(w, r)
	})
	http.HandleFunc("GET /api/stats", statsAPIHandler(c.GetConfig(), c.GetReceiptsStore))
	http.HandleFunc("GET /api/expenses/monthly", expensesMonthlyAPIHandler(c.GetConfig(), c.GetReceiptsStore))

	http.HandleFunc("GET /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		refreshStatusHandler(c.GetConfig(), getRefresh())(w, r)
	})
	http.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		startRefreshHandler(c.GetConfig(), c.GetProviders, c.SkipAlreadyDownloaded, getRefresh(), c.NotifyRefreshResults)(w, r)
	})
	http.HandleFunc("POST /api/notifications/test", testNotificationHandler(c.GetNotifier()))
	http.HandleFunc("PUT /api/settings", updateSettingsHandler(c.GetConfig(), c.Reload))
}

// writeJSON encodes v as the response body.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Err(err).Msg("Error encoding response")
	}
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
