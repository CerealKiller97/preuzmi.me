package http

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
)

type settingsUpdateResponse struct {
	Message  string   `json:"message,omitempty"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	OK       bool     `json:"ok"`
}

// updateSettingsHandler validates a full config payload, merges unchanged
// secrets, writes config.json, and hot-reloads the running process.
func updateSettingsHandler(cfg *config.Config, reload func(*config.Config)) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		var incoming config.Config
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, settingsUpdateResponse{
				OK:    false,
				Error: "Neispravan JSON u zahtevu.",
			})
			return
		}

		prev := *cfg
		incoming.MergeSecrets(prev)

		// Listen bind cannot hot-reload: keep whatever the process started with,
		// even if the client tries to change host/port/certs.
		incoming.Application = prev.Application

		if err := incoming.Validate(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, settingsUpdateResponse{
				OK:    false,
				Error: config.FriendlyError(err),
			})
			return
		}

		// Normalize download path so relative edits still work.
		if abs, err := filepath.Abs(incoming.DownloadPath); err == nil {
			incoming.DownloadPath = abs
		}

		path, err := config.Path()
		if err != nil {
			log.Err(err).Msg("Could not resolve config path")
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, settingsUpdateResponse{
				OK:    false,
				Error: "Ne mogu da pronađem config.json.",
			})
			return
		}

		if err := config.Save(path, incoming); err != nil {
			log.Err(err).Msg("Could not write config.json")
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, settingsUpdateResponse{
				OK:    false,
				Error: "Čuvanje config.json nije uspelo.",
			})
			return
		}

		if incoming.Storage == config.StorageLocal {
			if err := utils.EnsureFolderExists(incoming.DownloadPath); err != nil {
				log.Err(err).Str("path", incoming.DownloadPath).Msg("Could not ensure download folder")
			} else if err := utils.MonthlyFolder(incoming.DownloadPath); err != nil {
				log.Err(err).Str("path", incoming.DownloadPath).Msg("Could not ensure monthly folders")
			}
		}

		reload(&incoming)

		writeJSON(w, settingsUpdateResponse{
			OK:      true,
			Message: "Izmene su sačuvane i odmah primenjene.",
		})
	}
}
