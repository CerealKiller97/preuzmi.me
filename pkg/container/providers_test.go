package container

import (
	"context"
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsImplementedResolvesBaseProvider(t *testing.T) {
	// A primary account and its extra accounts are implemented iff the base is.
	assert.True(t, IsImplemented("a1"))
	assert.True(t, IsImplemented("a1-mama"))
	assert.True(t, IsImplemented("eupravnik-stan"))
	// Unknown providers stay unimplemented, with or without an account suffix.
	assert.False(t, IsImplemented("bogus"))
	assert.False(t, IsImplemented("bogus-mama"))
}

func TestGetProvidersResolvesExtraAccounts(t *testing.T) {
	cfg := &config.Config{
		Storage:      config.StorageLocal,
		DownloadPath: t.TempDir(),
		Providers: map[config.Provider]config.Credentials{
			"a1":      {Username: "me", Password: "p"},
			"a1-mama": {Username: "mama", Password: "p"},
		},
	}

	c := New(context.Background(), "test", cfg)
	t.Cleanup(func() { _ = c.Close() })

	providers := c.GetProviders([]string{"a1", "a1-mama"})

	// Both the primary and the extra account resolve to their own A1 instance,
	// keyed by account key so each files its receipts separately.
	require.Len(t, providers, 2)
	assert.NotNil(t, providers["a1"])
	assert.NotNil(t, providers["a1-mama"])
	assert.NotSame(t, providers["a1"], providers["a1-mama"])
}
