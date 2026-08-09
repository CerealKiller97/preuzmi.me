package utils_test

import (
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/stretchr/testify/require"
)

func TestGetPairsReturnsOnlyFullyConfiguredProviders(t *testing.T) {
	// Arrange
	assert := require.New(t)
	cfg := config.Config{
		Providers: map[config.Provider]config.ProviderAccounts{
			"mts": {{Username: "username", Password: "password"}},
			"a1":  {{Username: "", Password: ""}},
		},
	}

	// Act
	pairs, err := utils.GetPairs(&cfg)

	// Assert
	assert.NoError(err)
	assert.Equal([]string{"mts"}, pairs)
}

func TestGetPairsSkipsProvidersMissingOneHalfOfThePair(t *testing.T) {
	// Arrange
	assert := require.New(t)
	cfg := config.Config{
		Providers: map[config.Provider]config.ProviderAccounts{
			"mts": {{Username: "username", Password: ""}},
			"a1":  {{Username: "", Password: "password"}},
		},
	}

	// Act
	pairs, err := utils.GetPairs(&cfg)

	// Assert
	assert.Empty(pairs)
	assert.ErrorIs(err, utils.ErrEmptyProviders)
}

func TestGetPairsErrorsWhenNothingIsConfigured(t *testing.T) {
	// Arrange
	assert := require.New(t)
	cfg := config.Config{
		Providers: map[config.Provider]config.ProviderAccounts{
			"mts": {{Username: "", Password: ""}},
			"a1":  {{Username: "", Password: ""}},
		},
	}

	// Act
	pairs, err := utils.GetPairs(&cfg)

	// Assert
	assert.Empty(pairs)
	assert.ErrorIs(err, utils.ErrEmptyProviders)
}
