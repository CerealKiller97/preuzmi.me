package container

import (
	"context"
	"embed"
	"io"
	"sync"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/notify"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Container struct {
	Logger            zerolog.Logger
	esanduceProvider  provider.Interface
	storage           storage.Interface
	receiptsStore     *receipts.Repository
	mtsProvider       provider.Interface
	a1Provider        provider.Interface
	Ctx               context.Context
	yettelProvider    provider.Interface
	epsProvider       provider.Interface
	eupravnikProvider provider.Interface
	config            *config.Config
	Assets            embed.FS
	notifier          *notify.Service
	version           string
	mu                sync.RWMutex
}

var _ io.Closer = &Container{}

func New(ctx context.Context, version string, cfg *config.Config) *Container {
	return &Container{
		Ctx:     ctx,
		config:  cfg,
		version: version,
		Logger: log.
			Logger.
			Level(zerolog.InfoLevel).
			With().
			Str("app", "preuzmi.me").
			Logger(),
	}
}

func (c *Container) GetVersion() string {
	return c.version
}

func (c *Container) GetConfig() *config.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.config
}

// Reload replaces the live configuration and drops cached backends so the next
// download / notification uses the new values without a process restart.
func (c *Container) Reload(cfg *config.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	*c.config = *cfg
	c.storage = nil
	if c.receiptsStore != nil {
		_ = c.receiptsStore.Close()
		c.receiptsStore = nil
	}
	c.notifier = nil
	c.mtsProvider = nil
	c.a1Provider = nil
	c.esanduceProvider = nil
	c.yettelProvider = nil
	c.epsProvider = nil
	c.eupravnikProvider = nil

	level := cfg.LogLevel
	if level == "" {
		level = "info"
	}
	utils.ConfigureDefaultLogger(level, cfg.PrettyPrint)
}

// GetNotifier returns the notification service selected by config.json.
// Modes that are off yield a no-op service, so callers can always call it.
func (c *Container) GetNotifier() *notify.Service {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.notifier == nil {
		n, err := notify.NewFromConfig(c.config, c.Logger.With().Str("component", "notify").Logger())
		if err != nil {
			c.Logger.Fatal().Err(err).Msg("Could not initialize notifications")
		}
		c.notifier = n
	}

	return c.notifier
}

func (c *Container) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.receiptsStore != nil {
		err := c.receiptsStore.Close()
		c.receiptsStore = nil

		return err
	}

	return nil
}
