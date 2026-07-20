package app

import (
	"fmt"
	"net/http"

	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	handlers "github.com/CerealKiller97/preuzmi.me/pkg/http"
)

type (
	App struct {
		c *container.Container
		// middleware []middleware.Middleware
	}
)

func New(c *container.Container) *App {
	return &App{
		c: c,
	}
}

func (a *App) Middleware() *App {
	return a
}

func (a *App) Route() *App {
	handlers.Routes(a.c)

	return a
}

func (a *App) Serve() error {
	config := a.c.GetConfig()
	socket := fmt.Sprintf(
		"%s:%d",
		config.Application.Host,
		config.Application.Port,
	)

	a.c.Logger.
		Info().
		Str("version", a.c.GetVersion()).
		Msgf("Starting the HTTPS server on: https://%s", socket)

	return http.ListenAndServe(
		socket,
		// config.Application.Certs.Certificate,
		// config.Application.Certs.PrivateKey,
		nil,
	)
}

func (a *App) Close() error {
	return a.c.Close()
}
