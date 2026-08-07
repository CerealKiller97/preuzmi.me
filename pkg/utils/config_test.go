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
		Providers: map[config.Provider]config.Credentials{
			"mts": {
				Username: "username",
				Password: "password",
			},
			"a1": {
				Username: "",
				Password: "",
			},
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
		Providers: map[config.Provider]config.Credentials{
			"mts": {
				Username: "username",
				Password: "",
			},
			"a1": {
				Username: "",
				Password: "password",
			},
		},
	}

	// Act
	pairs, err := utils.GetPairs(&cfg)

	// Assert
	assert.Empty(pairs)
	assert.ErrorIs(err, utils.ErrEmptyProviders)
}

func TestGetPairsReturnsExtraAccountKeys(t *testing.T) {
	// Arrange: a provider with two accounts (a family member's own login).
	assert := require.New(t)
	cfg := config.Config{
		Providers: map[config.Provider]config.Credentials{
			"a1":      {Username: "me", Password: "p"},
			"a1-mama": {Username: "mama", Password: "p"},
		},
	}

	// Act
	pairs, err := utils.GetPairs(&cfg)

	// Assert: both account keys are returned so each is refreshed independently.
	assert.NoError(err)
	assert.ElementsMatch([]string{"a1", "a1-mama"}, pairs)
}

func TestGetPairsErrorsWhenNothingIsConfigured(t *testing.T) {
	// Arrange
	assert := require.New(t)
	cfg := config.Config{
		Providers: map[config.Provider]config.Credentials{
			"mts": {Username: "", Password: ""},
			"a1":  {Username: "", Password: ""},
		},
	}

	// Act
	pairs, err := utils.GetPairs(&cfg)

	// Assert
	assert.Empty(pairs)
	assert.ErrorIs(err, utils.ErrEmptyProviders)
}
