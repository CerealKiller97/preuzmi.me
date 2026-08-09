package cmd

import (
	"github.com/CerealKiller97/preuzmi.me/pkg/app"
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"os"
	"os/signal"
	"syscall"
)

func Serve(c *container.Container) {
	ctx, cancel := signal.NotifyContext(c.Ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	application := app.
		New(c).
		Middleware().
		Route()

	go func() {
		if err := application.Serve(); err != nil {
			c.Logger.
				Fatal().
				Err(err).
				Msg("startup process failed")
		}
	}()

	<-ctx.Done()

	if err := application.Close(); err != nil {
		c.Logger.
			Warn().
			Err(err).
			Msg("failure while closing application resources")
	}
}
