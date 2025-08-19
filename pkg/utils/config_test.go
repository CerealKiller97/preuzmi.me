package utils_test

import (
	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/utils"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestName(t *testing.T) {
	// Arrange
	assert := require.New(t)
	cfg := config.Config{
		Application: struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		}{},
		DownloadPath: "",
		LogLevel:     "",
		PrettyPrint:  false,
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
	pairs, err := utils.GetPairs(cfg)
	// Assert

	assert.Equal([]config.Provider{"mts"}, pairs)
	assert.NoError(err)
}

func TestEmpty(t *testing.T) {
	// Arrange
	assert := require.New(t)
	cfg := config.Config{
		Application: struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		}{},
		DownloadPath: "",
		LogLevel:     "",
		PrettyPrint:  false,
		Providers: map[config.Provider]config.Credentials{
			"mts": {
				Username: "",
				Password: "",
			},
			"a1": {
				Username: "",
				Password: "",
			},
		},
	}
	// Act
	pairs, err := utils.GetPairs(cfg)
	// Assert

	assert.Empty(pairs)
	assert.Error(err)
}
