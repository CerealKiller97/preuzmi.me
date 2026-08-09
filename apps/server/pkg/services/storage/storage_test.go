package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalSaveCreatesMonthFolder(t *testing.T) {
	root := t.TempDir()
	local := NewLocal(root)

	err := local.Save(context.Background(), "07-2026/mts.pdf", []byte("pdf-bytes"))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(root, "07-2026", "mts.pdf"))
	require.NoError(t, err)
	assert.Equal(t, "pdf-bytes", string(data))
}

func TestLocalLoadRoundTrip(t *testing.T) {
	local := NewLocal(t.TempDir())
	ctx := context.Background()

	require.NoError(t, local.Save(ctx, "07-2026/mts.pdf", []byte("pdf-bytes")))

	got, err := local.Load(ctx, "07-2026/mts.pdf")
	require.NoError(t, err)
	assert.Equal(t, "pdf-bytes", string(got))
}

func TestLocalLoadMissingErrors(t *testing.T) {
	local := NewLocal(t.TempDir())

	_, err := local.Load(context.Background(), "07-2026/absent.pdf")
	assert.Error(t, err)
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		in     string
		host   string
		useSSL bool
	}{
		{"", "s3.amazonaws.com", true},
		{"eu-central-1.linodeobjects.com", "eu-central-1.linodeobjects.com", true},
		{"https://eu-central-1.linodeobjects.com", "eu-central-1.linodeobjects.com", true},
		{"http://localhost:9000", "localhost:9000", false},
	}

	for _, tt := range tests {
		host, useSSL := normalizeEndpoint(tt.in)
		assert.Equal(t, tt.host, host, tt.in)
		assert.Equal(t, tt.useSSL, useSSL, tt.in)
	}
}
