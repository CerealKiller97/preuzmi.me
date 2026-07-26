package receipts

import (
	"context"
	"path"
	"strings"

	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
	"github.com/rs/zerolog"
)

// recordingStorage decorates a storage backend so every successful save is also
// indexed in the receipts database. Recording is best-effort: the PDF is what
// matters, so a database error is logged but never fails the download.
type recordingStorage struct {
	inner  storage.Interface
	store  *Repository
	logger zerolog.Logger
}

// NewRecordingStorage wraps inner so that saved receipts are recorded in store.
// A nil store yields a plain pass-through, so callers can wire this in even when
// the database failed to open.
func NewRecordingStorage(inner storage.Interface, store *Repository, logger zerolog.Logger) storage.Interface {
	return &recordingStorage{inner: inner, store: store, logger: logger}
}

func (r *recordingStorage) Save(ctx context.Context, key string, data []byte) error {
	if err := r.inner.Save(ctx, key, data); err != nil {
		return err
	}

	if r.store == nil {
		return nil
	}

	provider, period := parseKey(key)

	rec := Receipt{
		Provider:   provider,
		Period:     period,
		StorageKey: key,
		SizeBytes:  int64(len(data)),
	}

	if err := r.store.Record(ctx, rec); err != nil {
		r.logger.Err(err).Str("key", key).Msg("Failed to record downloaded receipt in database")
	}

	return nil
}

// parseKey splits a storage key like "07-2026/eps.pdf" into its provider and
// period. A key without a slash yields an empty period.
func parseKey(key string) (provider, period string) {
	dir, file := path.Split(key)
	period = strings.Trim(dir, "/")
	provider = strings.TrimSuffix(file, path.Ext(file))

	return provider, period
}
