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

func (c *Container) MTSProvider() provider.Interface {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.mtsProvider == nil {
		c.mtsProvider = mts.New(
			c.config.Providers["mts"],
			c.getStorageLocked(),
			c.providerLogger("mts"),
			c.getReceiptsStoreLocked(),
		)
	}

	return c.mtsProvider
}

func (c *Container) A1Provider() provider.Interface {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.a1Provider == nil {
		c.a1Provider = a1.New(
			c.config.Providers["a1"],
			c.providerLogger("a1"),
			c.getStorageLocked(),
			c.getReceiptsStoreLocked(),
		)
	}

	return c.a1Provider
}

func (c *Container) EsanduceProvider() provider.Interface {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.esanduceProvider == nil {
		c.esanduceProvider = esanduce.New(
			c.config.Providers["esanduce"],
			c.providerLogger("esanduce"),
			c.getStorageLocked(),
			c.getReceiptsStoreLocked(),
		)
	}

	return c.esanduceProvider
}

// YettelProvider builds the Yettel provider, which reads the invoice PDF from
// the configured mailbox over IMAP (Yettel emails it as an attachment). A
// mailbox that cannot be constructed yields nil, so the provider is skipped with
// a warning rather than crashing the run.
func (c *Container) YettelProvider() provider.Interface {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.yettelProvider == nil {
		reader, err := c.newMailboxReaderLocked("yettel")
		if err != nil {
			c.Logger.Err(err).Msg("Could not configure Yettel mailbox, provider unavailable")
			return nil
		}

		c.yettelProvider = yettel.New(
			reader,
			c.providerLogger("yettel"),
			c.getStorageLocked(),
			c.getReceiptsStoreLocked(),
		)
	}

	return c.yettelProvider
}

func (c *Container) EPSProvider() provider.Interface {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.epsProvider == nil {
		c.epsProvider = eps.New(
			c.config.Providers["eps"],
			c.providerLogger("eps"),
			c.getStorageLocked(),
			c.getReceiptsStoreLocked(),
		)
	}

	return c.epsProvider
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

// EupravnikProvider builds the eUpravnik provider, which reads the invoice from
// the configured mailbox over IMAP.
//
// A mailbox that cannot be constructed (e.g. an unknown provider or a custom
// provider with no host) yields nil, so the provider is skipped with a warning
// rather than crashing the run — matching how unconfigured providers behave.
func (c *Container) EupravnikProvider() provider.Interface {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.eupravnikProvider == nil {
		reader, err := c.newMailboxReaderLocked("eupravnik")
		if err != nil {
			c.Logger.Err(err).Msg("Could not configure eUpravnik mailbox, provider unavailable")
			return nil
		}

		c.eupravnikProvider = eupravnik.New(
			reader,
			c.providerLogger("eupravnik"),
			c.getStorageLocked(),
			c.getReceiptsStoreLocked(),
		)
	}

	return c.eupravnikProvider
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

// GetProviders resolves configured provider names to their implementations,
// keyed by name so callers can report results per provider.
//
// Names without an implementation yet are skipped with a warning rather than
// failing the whole run, so filling them into config.json ahead of time is
// harmless.
func (c *Container) GetProviders(names []string) map[string]provider.Interface {
	providers := make(map[string]provider.Interface, len(names))

	for _, name := range names {
		switch config.Provider(name) {
		case "mts":
			providers[name] = c.MTSProvider()
		case "a1":
			providers[name] = c.A1Provider()
		case "esanduce":
			providers[name] = c.EsanduceProvider()
		case "eps":
			providers[name] = c.EPSProvider()
		case "yettel":
			// A misconfigured mailbox yields a nil provider; skip it rather than
			// adding a nil that would panic in the refresh runner.
			if p := c.YettelProvider(); p != nil {
				providers[name] = p
			} else {
				c.Logger.Warn().
					Str("provider", name).
					Msg("Yettel is configured but its mailbox could not be initialized, skipping")
			}
		case "eupravnik":
			// A misconfigured mailbox yields a nil provider; skip it rather than
			// adding a nil that would panic in the refresh runner.
			if p := c.EupravnikProvider(); p != nil {
				providers[name] = p
			} else {
				c.Logger.Warn().
					Str("provider", name).
					Msg("eUpravnik is configured but its mailbox could not be initialized, skipping")
			}
		default:
			c.Logger.Warn().
				Str("provider", name).
				Msg("Configured provider has no implementation yet, skipping")
		}
	}

	return providers
}
