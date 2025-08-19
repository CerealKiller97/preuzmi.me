package a1

import (
	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/rs/zerolog"
	"net/http"
	"time"
)

type (
	Service struct {
		config       config.Credentials
		logger       zerolog.Logger
		downloadPath string
		http         *http.Client
	}
)

var _ provider.Interface = &Service{}

func New(config config.Credentials, logger zerolog.Logger, downloadPath string) *Service {
	return &Service{
		config:       config,
		logger:       logger,
		downloadPath: downloadPath,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s Service) DownloadReceipt() error {
	return s.login()
}

func (s Service) login() error {

	return nil
}
