package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBaseProvider(t *testing.T) {
	// A primary account key is its own base.
	assert.Equal(t, "a1", BaseProvider("a1"))
	assert.Equal(t, "eupravnik", BaseProvider("eupravnik"))
	// An extra account resolves to the base before the first dash.
	assert.Equal(t, "a1", BaseProvider("a1-mama"))
	assert.Equal(t, "mts", BaseProvider("mts-tata"))
	// A label that itself contains a dash still resolves to the base.
	assert.Equal(t, "a1", BaseProvider("a1-baka-mara"))
	// Case and surrounding space are normalized.
	assert.Equal(t, "a1", BaseProvider("  A1-Mama "))
	// An unknown key is returned as-is (lowercased).
	assert.Equal(t, "unknown", BaseProvider("unknown"))
}

func TestCredentialsLabelRoundTrips(t *testing.T) {
	in := Credentials{Username: "u", Password: "p", Label: "Mama"}

	b, err := json.Marshal(in)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"label":"Mama"`)

	var out Credentials
	require.NoError(t, json.Unmarshal(b, &out))
	assert.Equal(t, in, out)

	// An empty label is omitted from the JSON so single-account configs are clean.
	b, err = json.Marshal(Credentials{Username: "u", Password: "p"})
	require.NoError(t, err)
	assert.NotContains(t, string(b), "label")
}

func TestMergeSecretsMultiAccount(t *testing.T) {
	prev := Config{
		Providers: map[Provider]Credentials{
			"a1":      {Username: "me", Password: "p-me"},
			"a1-mama": {Username: "mama", Password: "p-mama"},
			"a1-tata": {Username: "tata", Password: "p-tata"},
		},
	}

	// The incoming form keeps the primary (blank password → merged), edits mama's
	// password, drops tata entirely, and adds a brand-new account with a password.
	next := Config{
		Providers: map[Provider]Credentials{
			"a1":        {Username: "me", Password: ""},
			"a1-mama":   {Username: "mama", Password: "changed"},
			"a1-sestra": {Username: "sestra", Password: "p-sestra"},
		},
	}

	next.MergeSecrets(prev)

	// Primary keeps its stored secret.
	assert.Equal(t, "p-me", next.Providers["a1"].Password)
	// Edited account takes the new password.
	assert.Equal(t, "changed", next.Providers["a1-mama"].Password)
	// New account keeps the password it submitted (prev had none to merge).
	assert.Equal(t, "p-sestra", next.Providers["a1-sestra"].Password)
	// Removed account stays removed — not resurrected with its old secret.
	_, ok := next.Providers["a1-tata"]
	assert.False(t, ok)
}

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
