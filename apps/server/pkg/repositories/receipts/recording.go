package receipts

import (
	"context"
	"path"
	"strings"

	"github.com/CerealKiller97/preuzmi.me/pkg/services/pdftext"
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
func NewRecordingStorage(
	inner storage.Interface,
	store *Repository,
	logger zerolog.Logger,
) storage.Interface {
	return &recordingStorage{
		inner:  inner,
		store:  store,
		logger: logger,
	}
}

func (r *recordingStorage) Save(ctx context.Context, key string, data []byte) error {
	if err := r.inner.Save(ctx, key, data); err != nil {
		return err
	}

	// A nil store is the documented pass-through mode: the database failed to
	// open, so the PDF is saved but left unindexed. Recording is best-effort and
	// must never crash a download.
	if r.store == nil {
		return nil
	}

	provider, account, period := parseKey(key)

	rec := Receipt{
		Provider:   provider,
		Account:    account,
		Period:     period,
		StorageKey: key,
		SizeBytes:  int64(len(data)),
	}

	if err := r.store.Record(ctx, rec); err != nil {
		r.logger.Err(err).Str("key", key).Msg("Failed to record downloaded receipt in database")
		return nil
	}

	// Pull the payment deadline out of the PDF once the row exists. Best-effort:
	// a missing label or a broken content stream just leaves due_at unset.
	if due, ok := pdftext.DueDate(data); ok {
		if err := r.store.SetDueAt(ctx, provider, account, period, due.Unix()); err != nil {
			r.logger.Err(err).Str("key", key).Msg("Failed to record receipt due date")
		}
	}

	return nil
}

// Load reads straight through to the wrapped backend: recording only concerns
// writes, so there is nothing to index on a read.
func (r *recordingStorage) Load(ctx context.Context, key string) ([]byte, error) {
	return r.inner.Load(ctx, key)
}

// parseKey splits a storage key into its provider, account and period. It
// mirrors storage.ReceiptKey: a solo key "07-2026/eps.pdf" yields provider
// "eps" and an empty account, while a per-account key "07-2026/eps/mama.pdf"
// yields provider "eps" and account "mama". A key without a slash yields an
// empty period.
func parseKey(key string) (provider, account, period string) {
	key = strings.TrimSuffix(key, path.Ext(key))

	parts := strings.Split(strings.Trim(key, "/"), "/")
	switch len(parts) {
	case 2:
		// period/provider
		return parts[1], "", parts[0]
	case 3:
		// period/provider/account
		return parts[1], parts[2], parts[0]
	default:
		// Unrecognized shape: keep the historical best-effort behaviour of using
		// the last segment as the provider and the leading directory as the period.
		dir, file := path.Split(key)
		return file, "", strings.Trim(dir, "/")
	}
}
