package main

import (
	"context"
	_ "embed"
	"os"

	"github.com/CerealKiller97/preuzmi.me/cmd"
	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
)

//go:embed version
var version string

func main() {
	cfg, err := config.New()
	if err != nil {
		log.Err(err).Msg("Error while loading config")
		os.Exit(1)
	}

	utils.ConfigureDefaultLogger("info", cfg.PrettyPrint)

	// The download folder is needed even with S3 storage: payments.json and
	// meta.json live there, and the dashboard reads receipts from it.
	if err = utils.EnsureFolderExists(cfg.DownloadPath); err != nil {
		log.Err(err).Msg("Error while creating folder")
		os.Exit(1)
	}

	// Monthly subfolders only matter when receipts are written locally.
	if cfg.Storage == config.StorageLocal {
		if err = utils.MonthlyFolder(cfg.DownloadPath); err != nil {
			log.Err(err).Msg("Error while creating monthly folder")
			os.Exit(1)
		}
	}

	// mtsService := mts.New(
	//	cfg.Providers.MTS,
	//	cfg.DownloadPath,
	//	log.With().Str("provider", "mts").Logger(),
	// )
	//
	// if err = mtsService.DownloadReceipt(); err != nil {
	//	log.Err(err).
	//		Str("provider", "mts").
	//		Msg("Error while downloading receipt")
	// }

	c := container.New(context.Background(), version, &cfg)

	cmd.Run(c)
}
