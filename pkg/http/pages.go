package http

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
)

type PageData struct {
	URL         string
	Active      string
	Title       string
	Description string
	BaseURL     string
	Version     string
	Pairs       []string
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

	// Use the container's implemented list as the single source of truth so this
	// route never drifts out of step with the providers that can actually
	// produce a receipt (previously a second hardcoded list here silently 400'd
	// newly added providers like eupravnik).
	if !container.IsImplemented(provider) {
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

func receiptHandler(store storage.Interface) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		provider, period, err := validate(w, r)
		if err != nil {
			log.Err(err).Msg("Error validating request")
			return
		}

		// Read through the configured storage backend so the same handler serves
		// PDFs whether they live on local disk or in an S3 bucket. The key is the
		// same forward-slash form used when the receipt was saved.
		key := fmt.Sprintf("%s/%s.pdf", period, provider)

		file, err := store.Load(r.Context(), key)
		if err != nil {
			log.Err(err).Str("key", key).Msg("Error reading receipt")
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
