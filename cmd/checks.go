package cmd

import (
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	provider2 "github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
	"sync"
)

func Checks(c *container.Container) {
	cfg := c.Config
	pairs, err := utils.GetPairs(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get pairs")
	}

	log.Info().Interface("providers", pairs).Msg("Provider configured")

	var wg sync.WaitGroup

	providers := c.GetProviders(pairs)
	if len(providers) == 0 {
		log.Fatal().Msg("No providers found")
	}

	errChan := make(chan error, len(providers))
	for _, provider := range providers {
		wg.Add(1)

		go func(provider provider2.Interface) {
			defer wg.Done()

			if err := provider.DownloadReceipt(); err != nil {
				errChan <- err
			}
		}(provider)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		log.Err(err).Msg("Failed to download receipt")
	}
}
