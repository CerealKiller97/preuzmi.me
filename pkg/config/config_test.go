package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeepSecret(t *testing.T) {
	assert.Equal(t, "old", KeepSecret("", "old"))
	assert.Equal(t, "old", KeepSecret(SecretPlaceholder, "old"))
	assert.Equal(t, "new", KeepSecret("new", "old"))
}

func TestMergeSecrets(t *testing.T) {
	prev := Config{
		Application: struct {
			Certs struct {
				Certificate string `json:"cert"`
				PrivateKey  string `json:"key"`
			} `json:"certs"`
			Host string `json:"host"`
			Port int    `json:"port"`
		}{
			Certs: struct {
				Certificate string `json:"cert"`
				PrivateKey  string `json:"key"`
			}{PrivateKey: "priv"},
		},
		S3: S3{AccessKey: "ak", SecretKey: "sk"},
		Notifications: Notifications{
			SMTP:     SMTP{Password: "smtp-pass"},
			Telegram: Telegram{BotToken: "token"},
		},
		Providers: map[Provider]Credentials{
			"mts": {Username: "u", Password: "p"},
		},
	}

	next := Config{
		S3: S3{},
		Notifications: Notifications{
			SMTP:     SMTP{},
			Telegram: Telegram{},
		},
		Providers: map[Provider]Credentials{
			"mts": {Username: "u2", Password: ""},
		},
	}

	next.MergeSecrets(prev)

	assert.Equal(t, "priv", next.Application.Certs.PrivateKey)
	assert.Equal(t, "ak", next.S3.AccessKey)
	assert.Equal(t, "sk", next.S3.SecretKey)
	assert.Equal(t, "smtp-pass", next.Notifications.SMTP.Password)
	assert.Equal(t, "token", next.Notifications.Telegram.BotToken)
	assert.Equal(t, "u2", next.Providers["mts"].Username)
	assert.Equal(t, "p", next.Providers["mts"].Password)
}

func TestValidateRejectsBadCheckUntil(t *testing.T) {
	cfg := Config{Storage: StorageLocal, DownloadPath: "/tmp", CheckUntil: 40}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, FriendlyError(err), "1 i 31")
}
