package utils_test

import (
	"os"
	"testing"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/utils"

	"github.com/stretchr/testify/require"
)

func TestSuccess(t *testing.T) {
	// Arrange
	assert := require.New(t)

	path := t.TempDir()
	t.Logf("path: %s", path)
	// Act
	err := utils.MonthlyFolder(path)

	// Assert
	assert.NoError(err)

	time := time.Now()
	month := int(time.Month())

	dirs, err := os.ReadDir(path)
	assert.Len(dirs, 12-month+1) // +1 inclusion of current month
	assert.NoError(err)
}

func TestEnsureFolderNonAbsolutePath(t *testing.T) {
	// Arrange
	assert := require.New(t)
	path := t.Name()

	// Act
	err := utils.EnsureFolderExists(path)

	// Assert
	assert.Error(err)
	assert.ErrorIs(err, utils.ErrAbsolutePath)
}

func TestEnsureFolderSuccess(t *testing.T) {
	// Arrange
	assert := require.New(t)
	path := t.TempDir() + "/testFolder"

	// Act
	err := utils.EnsureFolderExists(path)

	// Assert
	assert.NoError(err)
}
