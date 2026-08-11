package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A solo provider entry (a bare credential object) must parse into a single
// account with an empty ID, and marshal back to the very same object shape so an
// existing config.json the settings screen re-saves stays byte-identical.
func TestProviderAccountsSoloRoundTrips(t *testing.T) {
	const in = `{"identifier":"user","password":"pass"}`

	var pa ProviderAccounts
	require.NoError(t, json.Unmarshal([]byte(in), &pa))
	require.Len(t, pa, 1)
	assert.Equal(t, "", pa[0].ID)
	assert.Equal(t, "user", pa[0].Username)

	out, err := json.Marshal(pa)
	require.NoError(t, err)
	assert.JSONEq(t, in, string(out))
}

// An array of accounts must parse into a slice and marshal back to an array,
// preserving each account's id and label.
func TestProviderAccountsMultiRoundTrips(t *testing.T) {
	const in = `[{"id":"mama","label":"Mama","identifier":"m","password":"p1"},` +
		`{"id":"tata","label":"Tata","identifier":"t","password":"p2"}]`

	var pa ProviderAccounts
	require.NoError(t, json.Unmarshal([]byte(in), &pa))
	require.Len(t, pa, 2)
	assert.Equal(t, "mama", pa[0].ID)
	assert.Equal(t, "Tata", pa[1].Label)

	out, err := json.Marshal(pa)
	require.NoError(t, err)
	assert.JSONEq(t, in, string(out))
}

func TestValidateProviders(t *testing.T) {
	base := func(accounts ProviderAccounts) Config {
		return Config{
			Storage:      StorageLocal,
			DownloadPath: "/tmp",
			Providers:    map[Provider]ProviderAccounts{"eps": accounts},
		}
	}

	t.Run("solo without id is allowed", func(t *testing.T) {
		cfg := base(ProviderAccounts{{Username: "u", Password: "p"}})
		require.NoError(t, cfg.Validate())
	})

	t.Run("multi requires ids", func(t *testing.T) {
		cfg := base(ProviderAccounts{
			{Username: "u1", Password: "p1"},
			{Username: "u2", Password: "p2"},
		})
		require.Error(t, cfg.Validate())
	})

	t.Run("duplicate ids rejected", func(t *testing.T) {
		cfg := base(ProviderAccounts{
			{ID: "mama", Username: "u1", Password: "p1"},
			{ID: "mama", Username: "u2", Password: "p2"},
		})
		require.Error(t, cfg.Validate())
	})

	t.Run("invalid slug rejected", func(t *testing.T) {
		cfg := base(ProviderAccounts{
			{ID: "Mama Bear", Username: "u1", Password: "p1"},
			{ID: "tata", Username: "u2", Password: "p2"},
		})
		require.Error(t, cfg.Validate())
	})

	t.Run("valid multi passes", func(t *testing.T) {
		cfg := base(ProviderAccounts{
			{ID: "mama", Label: "Mama", Username: "u1", Password: "p1"},
			{ID: "tata", Label: "Tata", Username: "u2", Password: "p2"},
		})
		require.NoError(t, cfg.Validate())
	})
}

// ConfiguredAccounts enumerates one pair per fully configured account, skipping
// blanks, so the refresh path fans out over exactly the accounts that can run.
func TestConfiguredAccounts(t *testing.T) {
	cfg := Config{
		Providers: map[Provider]ProviderAccounts{
			"eps": {
				{ID: "mama", Username: "u1", Password: "p1"},
				{ID: "tata", Username: "u2", Password: ""}, // half-configured: skipped
			},
			"mts": {{Username: "u", Password: "p"}}, // solo
		},
	}

	got := cfg.ConfiguredAccounts()
	require.Len(t, got, 2)
	// Sorted by provider then account.
	assert.Equal(t, ProviderAccount{Provider: "eps", Account: "mama"}, got[0])
	assert.Equal(t, ProviderAccount{Provider: "mts", Account: ""}, got[1])
}

// MergeSecrets must restore each account's password by matching account id, so a
// multi-account form submit that omits secrets keeps every account's password.
func TestMergeSecretsMultiAccount(t *testing.T) {
	prev := Config{
		Providers: map[Provider]ProviderAccounts{
			"eps": {
				{ID: "mama", Username: "u1", Password: "secret1"},
				{ID: "tata", Username: "u2", Password: "secret2"},
			},
		},
	}
	next := Config{
		Providers: map[Provider]ProviderAccounts{
			"eps": {
				// Reordered, secrets blanked, one label changed.
				{ID: "tata", Username: "u2", Password: ""},
				{ID: "mama", Username: "u1", Password: SecretPlaceholder},
			},
		},
	}

	next.MergeSecrets(prev)

	assert.Equal(t, "secret2", next.Providers["eps"][0].Password)
	assert.Equal(t, "secret1", next.Providers["eps"][1].Password)
}
