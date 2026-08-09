package container

import (
	"slices"
	"strings"

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

// providerLogger returns the shared logger tagged with the provider name.
func (c *Container) providerLogger(name string) zerolog.Logger {
	return c.Logger.With().Str("provider", name).Logger()
}

// buildProviderLocked constructs the implementation for one (provider, account)
// with that account's credentials. It assumes c.mu is held. An email-based
// provider whose mailbox cannot be built yields nil so the account is skipped
// with a warning rather than crashing the run — matching how unconfigured
// providers behave.
func (c *Container) buildProviderLocked(name, account string, creds config.Credentials) provider.Interface {
	log := c.providerLogger(name)

	switch config.Provider(name) {
	case "mts":
		return mts.New(creds, account, c.getStorageLocked(), log, c.getReceiptsStoreLocked())
	case "a1":
		return a1.New(creds, account, log, c.getStorageLocked(), c.getReceiptsStoreLocked())
	case "esanduce":
		return esanduce.New(creds, account, log, c.getStorageLocked(), c.getReceiptsStoreLocked())
	case "eps":
		return eps.New(creds, account, log, c.getStorageLocked(), c.getReceiptsStoreLocked())
	case "yettel":
		reader, err := c.newMailboxReaderLocked(name, creds)
		if err != nil {
			c.Logger.Err(err).Str("account", account).Msg("Could not configure Yettel mailbox, account unavailable")
			return nil
		}
		return yettel.New(reader, account, log, c.getStorageLocked(), c.getReceiptsStoreLocked())
	case "eupravnik":
		reader, err := c.newMailboxReaderLocked(name, creds)
		if err != nil {
			c.Logger.Err(err).Str("account", account).Msg("Could not configure eUpravnik mailbox, account unavailable")
			return nil
		}
		return eupravnik.New(reader, account, log, c.getStorageLocked(), c.getReceiptsStoreLocked())
	default:
		return nil
	}
}

// newMailboxReaderLocked builds an IMAP reader for an email-based provider. The
// mailbox login and app password are the provider's own credentials under
// config.Providers[key]; config.Email only selects the IMAP server. Assumes
// c.mu is held.
func (c *Container) newMailboxReaderLocked(key string, creds config.Credentials) (mailbox.Reader, error) {
	// A provider account may pin its own folder/Gmail label; otherwise fall back
	// to the shared email folder (and mailbox.Config defaults that to INBOX).
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

// implemented lists the providers that actually have a download implementation.
// GetProviders below switches on exactly these names; keep the two in step.
var implemented = []string{"mts", "a1", "esanduce", "eps", "yettel", "eupravnik"}

// IsImplemented reports whether a provider can actually download anything yet.
func IsImplemented(name string) bool {
	return slices.Contains(implemented, strings.ToLower(name))
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
func (c *Container) SkipAlreadyDownloaded(jobs map[string]provider.Job) map[string]provider.Job {
	store := c.GetReceiptsStore()
	if store == nil {
		return jobs
	}

	period := utils.PreviousMonthFolder()

	pending := make(map[string]provider.Job, len(jobs))
	for key, job := range jobs {
		if store.IsConfirmedPaid(c.Ctx, job.Provider, job.Account, period) {
			c.Logger.Info().
				Str("provider", job.Provider).
				Str("account", job.Account).
				Str("period", period).
				Msg("Skipping download: receipt downloaded and confirmed paid (status plaćeno + confirmed_at)")

			continue
		}

		pending[key] = job
	}

	return pending
}

// GetProviders resolves configured accounts to their implementations, keyed by
// provider.JobKey so callers can report results per (provider, account). Each
// job carries its provider, account id and display label.
//
// Accounts whose provider has no implementation yet, or whose email mailbox
// could not be initialized, are skipped with a warning rather than failing the
// whole run — so filling a not-yet-supported provider into config.json ahead of
// time is harmless. Instances are cached per account across calls.
func (c *Container) GetProviders(accounts []config.ProviderAccount) map[string]provider.Job {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.providers == nil {
		c.providers = make(map[string]provider.Interface)
	}

	jobs := make(map[string]provider.Job, len(accounts))
	for _, pa := range accounts {
		if !IsImplemented(pa.Provider) {
			c.Logger.Warn().
				Str("provider", pa.Provider).
				Msg("Configured provider has no implementation yet, skipping")

			continue
		}

		acc, _ := c.config.AccountByID(pa.Provider, pa.Account)
		key := provider.JobKey(pa.Provider, pa.Account)

		impl, ok := c.providers[key]
		if !ok {
			impl = c.buildProviderLocked(pa.Provider, pa.Account, acc.Credentials())
			// Only cache a real instance: a nil (e.g. a mailbox that failed to
			// build) is left uncached so a later config fix can retry.
			if impl != nil {
				c.providers[key] = impl
			}
		}

		if impl == nil {
			c.Logger.Warn().
				Str("provider", pa.Provider).
				Str("account", pa.Account).
				Msg("Provider is configured but could not be initialized, skipping")

			continue
		}

		jobs[key] = provider.Job{
			Impl:     impl,
			Provider: pa.Provider,
			Account:  pa.Account,
			Label:    acc.Label,
		}
	}

	return jobs
}
