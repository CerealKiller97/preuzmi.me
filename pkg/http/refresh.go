package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/notify"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
)

// notifyResults enriches each successful result with its receipt's period and
// price from the database, then hands them to the notifier so the messages can
// name the month and amount. Enrichment is best-effort: a missing database just
// leaves those fields empty.
func notifyResults(receiptsStore func() *receipts.Repository, notifier *notify.Service) func([]refresh.Result) {
	return func(results []refresh.Result) {
		store := receiptsStore()
		if store != nil {
			for i := range results {
				if !results[i].OK {
					continue
				}
				if rec, ok := store.Latest(context.Background(), results[i].Provider); ok {
					results[i].Period = rec.Period
					results[i].Price = rec.Price
				}
			}
		}

		notifier.HandleResults(results)

		// Notify about receipts whose provider confirmed payment during this run.
		if store != nil {
			verified := store.DrainNewlyVerified()
			if len(verified) > 0 {
				items := make([]notify.VerifiedReceipt, 0, len(verified))
				for _, r := range verified {
					items = append(items, notify.VerifiedReceipt{
						Provider: r.Provider,
						Period:   r.Period,
						Price:    r.Price,
					})
				}
				notifier.HandleVerified(items)
			}
		}
	}
}

// refreshStatusHandler reports whether a refresh is running and when the last
// one finished. The UI polls this while a run is in flight.
func refreshStatusHandler(cfg *config.Config, svc *refresh.Service) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			http.Error(w, "refresh unavailable", http.StatusServiceUnavailable)
			return
		}

		writeJSON(w, svc.State().WithWindow(cfg.CheckUntil, cfg.RefreshAllowed()))
	}
}

// startRefreshHandler triggers a download from every configured provider.
//
// This is the same work as the `checks` CLI command. It returns 202 straight
// away rather than blocking, because a full run talks to several external
// providers and can take a while.
func startRefreshHandler(
	cfg *config.Config,
	getProviders func([]string) map[string]provider.Interface,
	receiptsStore func() *receipts.Repository,
	notifier *notify.Service,
	svc *refresh.Service,
) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			http.Error(w, "refresh unavailable", http.StatusServiceUnavailable)
			return
		}

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

		providers := getProviders(pairs)
		if len(providers) == 0 {
			http.Error(w, "no providers with an implementation are configured", http.StatusBadRequest)
			return
		}

		state, err := svc.Start(providers, notifyResults(receiptsStore, notifier))
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
func testNotificationHandler(notifier *notify.Service) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
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
