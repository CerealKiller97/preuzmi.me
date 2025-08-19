package app

import (
	"fmt"
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	handlers "github.com/CerealKiller97/preuzmi.me/pkg/http"
	"net/http"
)

type (
	App struct {
		c *container.Container
		//middleware []middleware.Middleware
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
	socket := fmt.Sprintf(
		"%s:%d",
		a.c.Config.Application.Host,
		a.c.Config.Application.Port,
	)

	a.c.Logger.
		Info().
		Msgf("starting the http server on: http://%s", socket)

	return http.ListenAndServeTLS(
		socket,
		a.c.Config.Application.Certs.Certificate,
		a.c.Config.Application.Certs.PrivateKey,
		nil,
	)
}

func (a *App) Close() error {
	return a.c.Close()
}
