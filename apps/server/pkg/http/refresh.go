package http

import (
	"errors"
	"net/http"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/notify"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
)

// refreshStatusHandler reports whether a refresh is running and when the last
// one finished. The UI polls this while a run is in flight.
func refreshStatusHandler(cfg *config.Config, svc *refresh.Service) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			http.Error(w, "refresh unavailable", http.StatusServiceUnavailable)
			return
		}

		// The `checks` cron writes refresh.json from a separate process, so pull
		// in any newer run before reporting the last-download time.
		svc.SyncFromDisk()

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
	getProviders func([]config.ProviderAccount) map[string]provider.Job,
	skip func(map[string]provider.Job) map[string]provider.Job,
	svc *refresh.Service,
	after func([]refresh.Result),
) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			http.Error(w, "refresh unavailable", http.StatusServiceUnavailable)
			return
		}

		if !cfg.RefreshAllowed() {
			writeJSONStatus(w, http.StatusForbidden,
				svc.State().WithWindow(cfg.CheckUntil, false))

			return
		}

		accounts := cfg.ConfiguredAccounts()
		if len(accounts) == 0 {
			log.Err(utils.ErrEmptyProviders).Msg("Refresh requested with no providers configured")
			http.Error(w, "no providers configured", http.StatusBadRequest)

			return
		}

		providers := getProviders(accounts)
		if len(providers) == 0 {
			http.Error(w, "no providers with an implementation are configured", http.StatusBadRequest)
			return
		}

		// Skip providers whose receipt for the current period is fully settled
		// (paid_at != 0, status plaćeno, confirmed_at != 0), so clicking refresh
		// does not re-login for bills that are already done. Unpaid or
		// unverified bills still run so status can flip. Every provider bills
		// for the previous month; with none left, return the last run's state
		// without starting anything. A not-running state is how the client tells
		// "already up to date" from a freshly started run.
		providers = skip(providers)
		if len(providers) == 0 {
			log.Info().Msg("Refresh requested but every provider's previous-month receipt is settled")

			writeJSON(w, svc.State().WithWindow(cfg.CheckUntil, true))

			return
		}

		state, err := svc.Start(providers, after)
		state = state.WithWindow(cfg.CheckUntil, true)
		if err != nil {
			// Already running is not a failure, the client just gets the
			// in-flight state back.
			if errors.Is(err, refresh.ErrAlreadyRunning) {
				writeJSONStatus(w, http.StatusConflict, state)

				return
			}

			log.Err(err).Msg("Error starting refresh")
			http.Error(w, "could not start refresh", http.StatusInternalServerError)

			return
		}

		log.Info().Interface("accounts", accounts).Msg("Refresh started from the UI")

		writeJSONStatus(w, http.StatusAccepted, state)
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
