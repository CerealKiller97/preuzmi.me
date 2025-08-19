package utils

import (
	"errors"
	"github.com/CerealKiller97/preuzmi.me/pkg/config"
)

var ErrEmptyProviders = errors.New("empty providers")

func GetPairs(cfg *config.Config) ([]string, error) {
	providers := make([]string, 0, 5)

	for provider, item := range cfg.Providers {
		if item.Username != "" && item.Password != "" {
			providers = append(providers, string(provider))
		}
	}

	if len(providers) == 0 {
		return nil, ErrEmptyProviders
	}

	return providers, nil
}
