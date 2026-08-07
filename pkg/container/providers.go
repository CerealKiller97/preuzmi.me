package container

import (
	"slices"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/providers/a1"
	"github.com/CerealKiller97/preuzmi.me/pkg/providers/eps"
	"github.com/CerealKiller97/preuzmi.me/pkg/providers/esanduce"
	"github.com/CerealKiller97/preuzmi.me/pkg/providers/eupravnik"
	"github.com/CerealKiller97/preuzmi.me/pkg/providers/mts"
	"github.com/CerealKiller97/preuzmi.me/pkg/providers/yettel"
	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/mailbox"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog"
)

// GetStorage returns the receipt storage backend selected by config.json:
// the local download folder or an S3-compatible bucket.
func (c *Container) GetStorage() storage.Interface {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.storage == nil {
		s, err := storage.New(c.config)
		if err != nil {
			// The config was validated at startup, so this only fires on a
			// programming error (e.g. a new backend without a constructor).
			c.Logger.Fatal().Err(err).Msg("Could not initialize receipt storage")
		}

		c.storage = s
	}

	return c.storage
}

// providerLogger returns the shared logger tagged with the account key.
func (c *Container) providerLogger(name string) zerolog.Logger {
	return c.Logger.With().Str("provider", name).Logger()
}

// getProviderLocked returns the built provider for an account key, memoized in
// c.providers so a given account is built once per config generation. A key with
// no implementation (or an email provider whose mailbox cannot be configured)
// yields nil, which GetProviders skips. Assumes c.mu is held.
func (c *Container) getProviderLocked(key string) provider.Interface {
	if c.providers == nil {
		c.providers = make(map[string]provider.Interface)
	}

	if p, ok := c.providers[key]; ok {
		return p
	}

	p := c.buildProviderLocked(key)
	c.providers[key] = p

	return p
}

// buildProviderLocked constructs the provider for an account key. The base
// provider type (config.BaseProvider) selects the implementation; the full key
// is passed through as the account name so each account files its receipts to
// its own path and receipts row. Assumes c.mu is held.
func (c *Container) buildProviderLocked(key string) provider.Interface {
	creds := c.config.Providers[config.Provider(key)]
	logger := c.providerLogger(key)

	switch config.BaseProvider(key) {
	case "mts":
		return mts.New(key, creds, c.getStorageLocked(), logger, c.getReceiptsStoreLocked())
	case "a1":
		return a1.New(key, creds, logger, c.getStorageLocked(), c.getReceiptsStoreLocked())
	case "esanduce":
		return esanduce.New(key, creds, logger, c.getStorageLocked(), c.getReceiptsStoreLocked())
	case "eps":
		return eps.New(key, creds, logger, c.getStorageLocked(), c.getReceiptsStoreLocked())
	case "yettel", "eupravnik":
		// Email-based providers read the invoice from this account's own mailbox
		// over IMAP. A mailbox that cannot be constructed yields nil, so the
		// provider is skipped with a warning rather than crashing the run.
		reader, err := c.newMailboxReaderLocked(key)
		if err != nil {
			c.Logger.Err(err).Str("provider", key).Msg("Could not configure mailbox, provider unavailable")
			return nil
		}
		if config.BaseProvider(key) == "yettel" {
			return yettel.New(key, reader, logger, c.getStorageLocked(), c.getReceiptsStoreLocked())
		}
		return eupravnik.New(key, reader, logger, c.getStorageLocked(), c.getReceiptsStoreLocked())
	default:
		return nil
	}
}

// newMailboxReaderLocked builds an IMAP reader for an email-based provider. The
// mailbox login and app password are the provider's own credentials under
// config.Providers[key]; config.Email only selects the IMAP server. Assumes
// c.mu is held.
func (c *Container) newMailboxReaderLocked(key string) (mailbox.Reader, error) {
	creds := c.config.Providers[config.Provider(key)]

	// A provider may pin its own folder/Gmail label; otherwise fall back to the
	// shared email folder (and mailbox.Config defaults that to INBOX).
	folder := creds.Mailbox
	if folder == "" {
		folder = c.config.Email.Mailbox
	}

	c.Logger.Info().
		Str("provider", key).
		Str("folder", mailbox.Config{Mailbox: folder}.MailboxOrDefault()).
		Msg("Configured mailbox reader")

	return mailbox.NewIMAP(
		mailbox.Config{
			Provider: c.config.Email.Provider,
			Host:     c.config.Email.Host,
			Port:     c.config.Email.Port,
			Mailbox:  folder,
		},
		creds.Username,
		creds.Password,
	)
}

// getStorageLocked assumes c.mu is already held. The backend is wrapped so that
// every successful download is also indexed in the receipts database.
func (c *Container) getStorageLocked() storage.Interface {
	if c.storage == nil {
		s, err := storage.New(c.config)
		if err != nil {
			c.Logger.Fatal().Err(err).Msg("Could not initialize receipt storage")
		}
		c.storage = s
	}

	return receipts.NewRecordingStorage(
		c.storage,
		c.getReceiptsStoreLocked(),
		c.Logger.With().Str("component", "receipts").Logger(),
	)
}

// getReceiptsStoreLocked lazily opens the receipts database and assumes c.mu is
// already held. A failure to open is logged and returns nil so downloads still
// work — they just go unindexed.
func (c *Container) getReceiptsStoreLocked() *receipts.Repository {
	if c.receiptsStore == nil {
		store, err := receipts.New(c.config.DownloadPath)
		if err != nil {
			c.Logger.Err(err).Msg("Could not open receipts database, downloads will not be indexed")
			return nil
		}

		c.receiptsStore = store
	}

	return c.receiptsStore
}

// GetReceiptsStore returns the receipts database index, or nil if it could not
// be opened.
func (c *Container) GetReceiptsStore() *receipts.Repository {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.getReceiptsStoreLocked()
}

// implemented lists the base providers that actually have a download
// implementation. buildProviderLocked switches on exactly these names; keep the
// two in step.
var implemented = []string{"mts", "a1", "esanduce", "eps", "yettel", "eupravnik"}

// IsImplemented reports whether an account key's provider can actually download
// anything yet. It resolves the base provider first, so an extra account like
// "a1-mama" is implemented exactly when its base ("a1") is.
func IsImplemented(name string) bool {
	return slices.Contains(implemented, config.BaseProvider(name))
}

// SkipAlreadyDownloaded drops any provider whose receipt for the current billing
// period is already downloaded and confirmed paid, returning only the providers
// still worth running.
//
// A provider is skipped once its bill for the period is on disk (a real download
// time), the provider reports status "plaćeno", and confirmed_at is set. Until
// then a refresh still runs: an undownloaded bill must be fetched, and a
// downloaded-but-unpaid bill may flip to paid on a later fetch — that flip is
// what triggers the paid-confirmation notification. Note this does NOT wait for
// the user to mark the receipt paid in the app; once the provider itself
// confirms payment there is nothing left to learn, so we stop logging in.
//
// A refresh otherwise logs back into every provider account and re-downloads
// bills that are already done; consulting the receipts database first turns a
// repeat run (the daily `checks` cron, or an eager click of the refresh button)
// into a no-op for providers that have nothing left to learn.
//
// The period checked is the previous calendar month — the month whose bill a run
// now publishes, and the folder every provider files its latest receipt under.
// The check is conservative: it only ever skips a provider whose bill for that
// exact period is downloaded and confirmed paid, so it can miss an optimization
// but never skips a receipt we still need to fetch or verify.
//
// With no receipts database to consult it returns the providers untouched, so
// downloads still happen — just without the optimization.
func (c *Container) SkipAlreadyDownloaded(providers map[string]provider.Interface) map[string]provider.Interface {
	store := c.GetReceiptsStore()
	if store == nil {
		return providers
	}

	period := utils.PreviousMonthFolder()

	pending := make(map[string]provider.Interface, len(providers))
	for name, p := range providers {
		if store.IsConfirmedPaid(c.Ctx, name, period) {
			c.Logger.Info().
				Str("provider", name).
				Str("period", period).
				Msg("Skipping download: receipt downloaded and confirmed paid (status plaćeno + confirmed_at)")

			continue
		}

		pending[name] = p
	}

	return pending
}

// GetProviders resolves configured account keys to their implementations, keyed
// by account key so callers can report results per account. A key like
// "a1-mama" resolves to the A1 implementation built for that account (see
// buildProviderLocked).
//
// Keys without an implementation yet — or an email account whose mailbox could
// not be initialized — are skipped with a warning rather than failing the whole
// run or adding a nil that would panic the refresh runner, so filling them into
// config.json ahead of time is harmless.
func (c *Container) GetProviders(names []string) map[string]provider.Interface {
	c.mu.Lock()
	defer c.mu.Unlock()

	providers := make(map[string]provider.Interface, len(names))

	for _, name := range names {
		if !IsImplemented(name) {
			c.Logger.Warn().
				Str("provider", name).
				Msg("Configured provider has no implementation yet, skipping")

			continue
		}

		p := c.getProviderLocked(name)
		if p == nil {
			// An email account whose mailbox could not be configured; skip it
			// rather than adding a nil that would panic in the refresh runner.
			c.Logger.Warn().
				Str("provider", name).
				Msg("Provider is configured but could not be initialized, skipping")

			continue
		}

		providers[name] = p
	}

	return providers
}
