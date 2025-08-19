package container

import (
	"context"
	"embed"
	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"io"
)

type Container struct {
	Ctx              context.Context
	cancel           context.CancelFunc
	Config           *config.Config
	Logger           zerolog.Logger
	Assets           embed.FS
	templateFS       embed.FS
	mtsProvider      provider.Interface
	a1Provider       provider.Interface
	esanduceProvider provider.Interface
	yettelProvider   provider.Interface
	epsProvider      provider.Interface
}

var _ io.Closer = &Container{}

func New(ctx context.Context, cfg *config.Config) *Container {
	return &Container{
		Ctx:    ctx,
		Config: cfg,
		Logger: log.
			Logger.
			Level(zerolog.InfoLevel).
			With().
			Str("app", "preuzmi.me").
			Logger(),
	}
}

func (c Container) Close() error {
	// Close resources

	return nil
}
