package cmd

import (
	"sort"

	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
)

// Checks downloads the latest receipt from every configured provider.
//
// The actual work lives in the refresh service, which the UI's refresh button
// calls too, so both entry points behave identically.
func Checks(c *container.Container) {
	cfg := c.GetConfig()

	if !cfg.RefreshAllowed() {
		log.Info().
			Int("check_until", cfg.CheckUntil).
			Msg("Skipping refresh: past check_until day of the month")
		return
	}

	pairs, err := utils.GetPairs(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get configured providers")
	}

	log.Info().Interface("providers", pairs).Msg("Providers configured")

	providers := c.GetProviders(pairs)
	if len(providers) == 0 {
		log.Fatal().Msg("No providers with an implementation are configured")
	}

	// Skip providers whose receipt for the current period is already downloaded
	// and confirmed paid (status plaćeno, confirmed_at != 0). Every provider bills
	// for the previous month; once that month's bill is on disk and the provider
	// has confirmed payment there is nothing left to learn. When none are left,
	// don't run at all — no login, no download, and the last-run time is left
	// untouched.
	providers = c.SkipAlreadyDownloaded(providers)
	if len(providers) == 0 {
		log.Info().Msg("Every provider's previous-month receipt is settled; nothing to download")
		// Still run due-soon reminders: settled providers do not mean every bill
		// on record is paid, and a quiet refresh day should still nag.
		c.NotifyRefreshResults(nil)
		return
	}

	checking := make([]string, 0, len(providers))
	for name := range providers {
		checking = append(checking, name)
	}
	sort.Strings(checking)

	log.Info().Strs("providers", checking).Msg("Checking providers")

	// Record the run into refresh.json, the same file the UI reads for its
	// "last download" time. A scheduled checks pass must advance it too, so the
	// dashboard does not keep showing a stale time as if nothing had run.
	svc, err := refresh.New(cfg.DownloadPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load refresh state")
	}

	failed := 0
	results, err := svc.RunOnce(providers)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to run refresh")
	}
	for _, result := range results {
		event := log.Info()
		if !result.OK {
			event = log.Error()
			failed++
		}

		event.
			Str("provider", result.Provider).
			Int64("duration_ms", result.DurationMS).
			Str("error", result.Error).
			Msg("Receipt download finished")
	}

	c.NotifyRefreshResults(results)

	if failed > 0 {
		log.Warn().Int("failed", failed).Msg("Some providers failed")
	}
}
