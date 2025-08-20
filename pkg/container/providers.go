package container

import (
	"github.com/CerealKiller97/preuzmi.me/pkg/services/mts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/rs/zerolog"
)

func (c *Container) GetLocalStorage() string {
	return ""
}

func (c *Container) GetS3Storage() string {
	return ""
}

func (c *Container) MTSProvider() provider.Interface {
	if c.mtsProvider == nil {
		cfg := c.Config.Providers["mts"]
		c.mtsProvider = mts.New(
			cfg,
			c.Config.DownloadPath,
			zerolog.Logger{}.With().Str("provider", "mts").Logger(),
		)
	}

	return c.mtsProvider
}

func (c *Container) GetProviders(providers []string) []provider.Interface {
	if len(providers) == 0 {
		return []provider.Interface{}
	}

	return []provider.Interface{
		c.MTSProvider(),
	}
}
