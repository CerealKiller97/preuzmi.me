package main

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/CerealKiller97/preuzmi.me/cmd"
	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/rs/zerolog/log"
	"os"
)

//go:embed version
var version string

func main() {
	cfg, err := config.New()
	if err != nil {
		log.Err(err).Msg("Error while loading config")
		os.Exit(1)
	}

	utils.ConfigureDefaultLogger("info", true)

	// make utils to check pairs valid
	if err = utils.EnsureFolderExists(cfg.DownloadPath); err != nil {
		log.Err(err).Msg("Error while creating folder")
		os.Exit(1)
	}

	if err = utils.MonthlyFolder(cfg.DownloadPath); err != nil {
		log.Err(err).Msg("Error while creating monthly folder")
		os.Exit(1)
	}

	fmt.Println(version)

	//mtsService := mts.New(
	//	cfg.Providers.MTS,
	//	cfg.DownloadPath,
	//	log.With().Str("provider", "mts").Logger(),
	//)
	//
	//if err = mtsService.DownloadReceipt(); err != nil {
	//	log.Err(err).
	//		Str("provider", "mts").
	//		Msg("Error while downloading receipt")
	//}

	c := container.New(context.Background(), &cfg)

	cmd.Run(c)
}
