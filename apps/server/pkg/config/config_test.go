package config

import (
	"encoding/json"
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
			Host string `json:"host"`
			Port int    `json:"port"`
		}{
			Host: "host",
			Port: 8080,
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

// An older config.json predates due_reminder_days, so the key is simply absent.
// Loading it must apply the default lead time, never a 0-day window.
func TestValidateDefaultsDueReminderDays(t *testing.T) {
	var cfg Config
	require.NoError(t, json.Unmarshal(
		[]byte(`{"storage":"local","download_path":"/tmp"}`), &cfg))
	require.NoError(t, cfg.Validate())
	assert.Equal(t, DefaultDueReminderDays, cfg.Notifications.DueReminderDays)

	// An explicitly configured value is preserved.
	cfg = Config{Storage: StorageLocal, DownloadPath: "/tmp"}
	cfg.Notifications.DueReminderDays = 3
	require.NoError(t, cfg.Validate())
	assert.Equal(t, 3, cfg.Notifications.DueReminderDays)

	// Out-of-range is rejected.
	cfg = Config{Storage: StorageLocal, DownloadPath: "/tmp"}
	cfg.Notifications.DueReminderDays = 61
	require.Error(t, cfg.Validate())
}

func TestValidateLang(t *testing.T) {
	cfg := Config{Storage: StorageLocal, DownloadPath: "/tmp", Lang: "cyrilic"}
	require.NoError(t, cfg.Validate())
	assert.Equal(t, LangCyrillic, cfg.Lang)

	cfg = Config{Storage: StorageLocal, DownloadPath: "/tmp", Lang: "cyrillic"}
	require.NoError(t, cfg.Validate())
	assert.Equal(t, LangCyrillic, cfg.Lang)

	cfg = Config{Storage: StorageLocal, DownloadPath: "/tmp"}
	require.NoError(t, cfg.Validate())
	assert.Equal(t, LangLatin, cfg.Lang)

	cfg = Config{Storage: StorageLocal, DownloadPath: "/tmp", Lang: "german"}
	require.Error(t, cfg.Validate())
}

// The daily cron fires every day; check_until must stop a run past its day.
func TestRefreshAllowedRespectsCheckUntil(t *testing.T) {
	cfg := Config{CheckUntil: 20}

	assert.True(t, cfg.allowedOnDay(1), "1st should run")
	assert.True(t, cfg.allowedOnDay(20), "20th (check_until) should still run")
	assert.False(t, cfg.allowedOnDay(21), "21st must NOT run")
	assert.False(t, cfg.allowedOnDay(31), "31st must NOT run")
}
