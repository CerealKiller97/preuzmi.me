package utils

import (
	"errors"
	"github.com/CerealKiller97/preuzmi.me/pkg/config"
)

var ErrEmptyProviders = errors.New("empty providers")

// GetPairs returns the distinct names of providers that have at least one fully
// configured account. It backs the parts of the UI that only need to know which
// providers exist; the refresh path uses config.ConfiguredAccounts, which
// enumerates individual (provider, account) pairs.
func GetPairs(cfg *config.Config) ([]string, error) {
	seen := make(map[string]struct{}, len(cfg.Providers))
	providers := make([]string, 0, len(cfg.Providers))

	for provider, accounts := range cfg.Providers {
		for _, a := range accounts {
			if !a.Configured() {
				continue
			}
			if _, ok := seen[string(provider)]; !ok {
				seen[string(provider)] = struct{}{}
				providers = append(providers, string(provider))
			}
		}
	}

	if len(providers) == 0 {
		return nil, ErrEmptyProviders
	}

	return providers, nil
}
