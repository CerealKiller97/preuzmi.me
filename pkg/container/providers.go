package container

import (
	"slices"
	"strings"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/a1"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/esanduce"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/mts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/rs/zerolog"
)

// GetStorage returns the receipt storage backend selected by config.json:
// the local download folder or an S3-compatible bucket.
func (c *Container) GetStorage() storage.Interface {
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
	if c.mtsProvider == nil {
		c.mtsProvider = mts.New(
			c.GetConfig().Providers["mts"],
			c.GetStorage(),
			c.providerLogger("mts"),
		)
	}

	return c.mtsProvider
}

func (c *Container) A1Provider() provider.Interface {
	if c.a1Provider == nil {
		c.a1Provider = a1.New(
			c.GetConfig().Providers["a1"],
			c.providerLogger("a1"),
			c.GetStorage(),
		)
	}

	return c.a1Provider
}

func (c *Container) EsanduceProvider() provider.Interface {
	if c.esanduceProvider == nil {
		c.esanduceProvider = esanduce.New(
			c.GetConfig().Providers["esanduce"],
			c.providerLogger("esanduce"),
			c.GetStorage(),
		)
	}

	return c.esanduceProvider
}

// implemented lists the providers that actually have a download implementation.
// GetProviders below switches on exactly these names; keep the two in step.
var implemented = []string{"mts", "a1", "esanduce"}

// IsImplemented reports whether a provider can actually download anything yet.
func IsImplemented(name string) bool {
	return slices.Contains(implemented, strings.ToLower(name))
}

// GetProviders resolves configured provider names to their implementations,
// keyed by name so callers can report results per provider.
//
// Names without an implementation yet (yettel, eps) are skipped with a warning
// rather than failing the whole run, so filling them into config.json ahead of
// time is harmless.
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
		default:
			c.Logger.Warn().
				Str("provider", name).
				Msg("Configured provider has no implementation yet, skipping")
		}
	}

	return providers
}
