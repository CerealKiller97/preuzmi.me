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
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
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

// NotifyRefreshResults sends the notifications for a finished refresh. It is the
// single path shared by the UI refresh button and the `checks` scheduler, so
// both behave identically:
//
//   - a download message goes out only for receipts recorded for the first time
//     in this run, so a daily re-run never re-announces an already-downloaded
//     bill; and
//   - a paid-confirmation goes out for any receipt the provider just flipped to
//     paid.
//
// Both dedupe against the receipts database. With no database there is nothing
// to dedupe against, so every successful download is announced (as before the
// database existed) and no paid-confirmations fire.
func (c *Container) NotifyRefreshResults(results []refresh.Result) {
	notifier := c.GetNotifier()
	store := c.GetReceiptsStore()

	// Work on a copy: the caller's slice is also the refresh service's persisted
	// state, so enriching it in place would race concurrent readers.
	enriched := append([]refresh.Result(nil), results...)

	if store == nil {
		for i := range enriched {
			enriched[i].New = enriched[i].OK
		}
		notifier.HandleResults(enriched)

		return
	}

	newly := store.DrainNewlyDownloaded()
	isNew := make(map[string]struct{}, len(newly))
	for _, r := range newly {
		isNew[downloadKey(r.Provider, r.Period)] = struct{}{}
	}

	for i := range enriched {
		if !enriched[i].OK {
			continue
		}

		// Name the month and amount from the database when available.
		if rec, ok := store.Latest(c.Ctx, enriched[i].Provider); ok {
			enriched[i].Period = rec.Period
			enriched[i].Price = rec.Price
		}

		if _, ok := isNew[downloadKey(enriched[i].Provider, enriched[i].Period)]; ok {
			enriched[i].New = true
		}
	}

	notifier.HandleResults(enriched)

	verified := store.DrainNewlyVerified()
	if len(verified) == 0 {
		return
	}

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

// downloadKey joins a receipt's provider and period into the map key used to
// match run results against the newly-downloaded set.
func downloadKey(provider, period string) string {
	return provider + "\x00" + period
}
