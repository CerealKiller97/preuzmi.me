package http

import (
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
	"slices"
)

func Routes(c *container.Container) {
	fs := http.FileServer(http.Dir("./assets"))
	http.Handle("GET /assets/", http.StripPrefix("/assets/", fs))

	http.HandleFunc("GET /", indexHandler())
	http.HandleFunc("GET /dashboard", dashboardHandler(c.Config))
	http.HandleFunc("GET /receipt/{period}/{provider}", receiptHandler())

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

		_, err = getReceipts()
		if err != nil {
			log.Err(err).Msg("Error getting receipts")
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
			//return nil, err
		}

		p := path.Join(wd, "receipts", period, fmt.Sprintf("%s.pdf", provider))

		file, err := os.ReadFile(p)
		if err != nil {
			log.Err(err).Msg("Error reading file")
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
func getReceipts() ([]string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	p := path.Join(wd, "receipts", "04-2025", "mts.pdf")
	return []string{p}, nil
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
