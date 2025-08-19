package config

import (
	"encoding/json"
	"io"
	"os"
	"path"
)

type (
	Provider    string
	Credentials struct {
		Username string `json:"identifier"`
		Password string `json:"password"`
	}

	Providers struct {
		MTS      Credentials `json:"mts"`
		A1       Credentials `json:"a1"`
		Yettel   Credentials `json:"yettel"`
		EPS      Credentials `json:"eps"`
		Esanduce Credentials `json:"esanduce"`
	}

	Config struct {
		Application struct {
			Host  string `json:"host"`
			Port  int    `json:"port"`
			Certs struct {
				Certificate string `json:"cert"`
				PrivateKey  string `json:"key"`
			} `json:"certs"`
		} `json:"application"`
		DownloadPath string                   `json:"download_path"`
		LogLevel     string                   `json:"log_level"`
		PrettyPrint  bool                     `json:"pretty_print"`
		Providers    map[Provider]Credentials `json:"providers"`
	}
)

func New() (Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}

	f, err := os.OpenFile(path.Join(cwd, "config.json"), os.O_RDONLY, 0o600)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{}

	data, err := io.ReadAll(f)
	if err != nil {
		return Config{}, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
