package storage

import (
	"context"
	"os"
	"path/filepath"
)

var _ Interface = &Local{}

// Local stores receipts as plain files under a root directory (download_path).
type Local struct {
	root string
}

func NewLocal(root string) *Local {
	return &Local{root: root}
}

// Save writes the receipt to root/key, creating intermediate folders as
// needed so a new month never has to be prepared in advance.
func (l *Local) Save(_ context.Context, key string, data []byte) error {
	target := filepath.Join(l.root, filepath.FromSlash(key))

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	return os.WriteFile(target, data, 0o644)
}

// Load reads root/key back off disk. A missing file surfaces as an os error,
// which the caller turns into a 404.
func (l *Local) Load(_ context.Context, key string) ([]byte, error) {
	return os.ReadFile(filepath.Join(l.root, filepath.FromSlash(key)))
}
