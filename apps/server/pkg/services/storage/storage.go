// Package storage abstracts where downloaded receipts end up.
//
// Providers produce a receipt as bytes plus a key like "07-2026/mts.pdf" and
// hand both to a Storage; whether that lands in a folder on disk or in an
// S3-compatible bucket is decided once, by configuration, at startup.
package storage

import (
	"context"
	"fmt"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
)

// Interface persists and retrieves a single receipt under the given key.
//
// The key uses forward slashes regardless of storage backend, e.g.
// "07-2026/mts.pdf": the local backend maps it onto the filesystem, the S3
// backend uses it verbatim as the object name.
type Interface interface {
	Save(ctx context.Context, key string, data []byte) error
	// Load returns the bytes stored under key. A missing key is an error, so
	// callers can surface it as a 404.
	Load(ctx context.Context, key string) ([]byte, error)
}

// ReceiptKey builds the storage key for a receipt PDF. A solo account (empty
// account ID) keeps the historical flat layout "07-2026/eps.pdf"; a named
// account files under a per-account subfolder "07-2026/eps/<account>.pdf", so
// several family members' bills for the same provider and period never collide.
func ReceiptKey(period, provider, account string) string {
	if account == "" {
		return fmt.Sprintf("%s/%s.pdf", period, provider)
	}

	return fmt.Sprintf("%s/%s/%s.pdf", period, provider, account)
}

// New builds the storage backend named by cfg.Storage.
func New(cfg *config.Config) (Interface, error) {
	switch cfg.Storage {
	case config.StorageLocal:
		return NewLocal(cfg.DownloadPath), nil
	case config.StorageS3:
		return NewS3(cfg.S3)
	default:
		return nil, fmt.Errorf("unknown storage backend %q", cfg.Storage)
	}
}
