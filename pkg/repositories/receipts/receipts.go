// Package receipts keeps a SQLite record of every receipt the app has
// downloaded.
//
// The PDF bytes themselves live wherever the storage backend puts them (local
// folder or S3); this table is the queryable index of what was fetched, when,
// and how big it was. The database file sits next to the other state files in
// the download folder (receipts.db, alongside payments.json / refresh.json).
package receipts

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/CerealKiller97/preuzmi.me/database"
	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

const fileName = "receipts.db"

// Paid-state labels stored in the status column, as reported by the provider
// (distinct from paid_at, which reflects the user marking it paid in this app).
const (
	StatusPaid   = "plaćeno"
	StatusUnpaid = "neplaćeno"
)

// SQL statements run against the receipts table. Kept together so the shape of
// every query is visible in one place.
const (
	// upsertReceiptQuery records a downloaded receipt keyed by (provider,
	// period). On conflict it refreshes only the file facts, leaving price,
	// status and paid_at to their dedicated setters.
	upsertReceiptQuery = `
INSERT INTO receipts (provider, period, storage_key, size_bytes, price, status, downloaded_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(provider, period) DO UPDATE SET
	storage_key   = excluded.storage_key,
	size_bytes    = excluded.size_bytes,
	downloaded_at = excluded.downloaded_at;`

	// latestReceiptQuery returns the most recently downloaded receipt for a
	// provider.
	latestReceiptQuery = `
SELECT id, provider, period, storage_key, size_bytes, price, status, downloaded_at, paid_at
FROM receipts
WHERE provider = ?
ORDER BY downloaded_at DESC, id DESC
LIMIT 1;`

	// setPriceQuery updates the amount owed for an already-recorded receipt.
	setPriceQuery = `UPDATE receipts SET price = ? WHERE provider = ? AND period = ?;`

	// selectStatusQuery reads the current status for a (provider, period).
	selectStatusQuery = `SELECT status FROM receipts WHERE provider = ? AND period = ?;`

	// setStatusQuery updates the provider-reported paid state.
	setStatusQuery = `UPDATE receipts SET status = ? WHERE provider = ? AND period = ?;`

	// selectPriceQuery reads the recorded price for a (provider, period).
	selectPriceQuery = `SELECT price FROM receipts WHERE provider = ? AND period = ?;`

	// markPaidQuery stamps paid_at, creating a stub row when the receipt was
	// never recorded by a download.
	markPaidQuery = `
INSERT INTO receipts (provider, period, storage_key, size_bytes, price, downloaded_at, paid_at)
VALUES (?, ?, ?, 0, 0, 0, ?)
ON CONFLICT(provider, period) DO UPDATE SET
	paid_at = excluded.paid_at;`

	// listReceiptsQuery returns every recorded receipt, newest download first.
	listReceiptsQuery = `
SELECT id, provider, period, storage_key, size_bytes, price, status, downloaded_at, paid_at
FROM receipts
ORDER BY downloaded_at DESC, id DESC;`
)

type Receipt struct {
	Provider     string  `json:"provider"`
	Period       string  `json:"period"`
	StorageKey   string  `json:"storage_key"`
	Status       string  `json:"status"`
	Price        float64 `json:"price"`
	SizeBytes    int64   `json:"size_bytes"`
	DownloadedAt int64   `json:"downloaded_at"`
	PaidAt       int64   `json:"paid_at"`
	ID           int64   `json:"id"`
}

// Repository is a SQLite-backed index of downloaded receipts.
type Repository struct {
	db *sql.DB

	// newlyVerified collects receipts whose provider status flipped from unpaid
	// to paid, so a caller can drain and notify about them after a refresh.
	mu            sync.Mutex
	newlyVerified []Receipt
}

// New opens (creating if needed) the receipts database in dir and ensures the
// schema exists.
func New(dir string) (*Repository, error) {
	// busy_timeout keeps concurrent writers (parallel provider downloads) from
	// failing outright on a locked database; WAL improves read/write overlap.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)",
		filepath.Join(dir, fileName),
	)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := migrate(db); err != nil {
		if err := db.Close(); err != nil {
			return nil, err
		}
		return nil, err
	}

	return &Repository{
		db: db,
	}, nil
}

// migrate applies the schema embedded from database/schema.sql. It is a
// CREATE TABLE IF NOT EXISTS, so running it on every start is a no-op once the
// table exists and never touches existing rows.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(database.Schema); err != nil {
		return fmt.Errorf("receipts: applying schema: %w", err)
	}

	return nil
}

// Record upserts a downloaded receipt keyed by (provider, period): a
// re-download of the same period updates the existing row rather than adding a
// duplicate.
func (s *Repository) Record(ctx context.Context, r Receipt) error {
	if r.DownloadedAt == 0 {
		r.DownloadedAt = time.Now().Unix()
	}

	// On a re-download we refresh the file facts but deliberately leave price,
	// status and paid_at untouched — those are set by SetPrice / SetStatus /
	// MarkPaid and must not be reset by the storage-layer recorder.
	_, err := s.db.ExecContext(
		ctx,
		upsertReceiptQuery,
		r.Provider,
		r.Period,
		r.StorageKey,
		r.SizeBytes,
		r.Price,
		r.Status,
		r.DownloadedAt,
	)

	return err
}

// Latest returns the most recently downloaded receipt for a provider.
func (s *Repository) Latest(ctx context.Context, provider string) (Receipt, bool) {
	var r Receipt
	err := s.db.QueryRowContext(ctx, latestReceiptQuery, provider).Scan(
		&r.ID,
		&r.Provider,
		&r.Period,
		&r.StorageKey,
		&r.SizeBytes,
		&r.Price,
		&r.Status,
		&r.DownloadedAt,
		&r.PaidAt,
	)
	if err != nil {
		return Receipt{}, false
	}

	return r, true
}

// SetPrice updates the amount owed for an already-recorded receipt.
func (s *Repository) SetPrice(ctx context.Context, provider, period string, price float64) error {
	_, err := s.db.ExecContext(
		ctx,
		setPriceQuery,
		price,
		provider,
		period,
	)

	return err
}

// SetStatus updates the provider-reported paid state ("plaćeno" / "neplaćeno")
// for an already-recorded receipt.
//
// When the status becomes paid and it was not paid before — the moment the
// provider confirms payment — the receipt is queued in newlyVerified so a caller
// can notify about it via DrainNewlyVerified.
func (s *Repository) SetStatus(ctx context.Context, provider, period, status string) error {
	var prev string
	// Ignore the error: a missing row just yields an empty previous status.
	_ = s.db.QueryRowContext(ctx, selectStatusQuery, provider, period).Scan(&prev)

	if _, err := s.db.ExecContext(ctx, setStatusQuery, status, provider, period); err != nil {
		return err
	}

	if status == StatusPaid && prev != StatusPaid {
		var price float64
		_ = s.db.QueryRowContext(ctx, selectPriceQuery, provider, period).Scan(&price)

		s.mu.Lock()
		s.newlyVerified = append(s.newlyVerified, Receipt{
			Provider: provider,
			Period:   period,
			Price:    price,
		})
		s.mu.Unlock()
	}

	return nil
}

// DrainNewlyVerified returns and clears the receipts that flipped to paid since
// the last drain.
func (s *Repository) DrainNewlyVerified() []Receipt {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := s.newlyVerified
	s.newlyVerified = nil

	return out
}

// MarkPaid records that a receipt has been paid, stamping paid_at with the
// given unix time (pass 0 to mark it unpaid again).
//
// It upserts so a receipt can be marked paid even if it was never recorded by a
// download (e.g. it predates the database): a stub row is created with the
// conventional storage key and an unknown download time, which a later download
// fills in without disturbing paid_at.
func (s *Repository) MarkPaid(ctx context.Context, provider, period string, paidAt int64) error {
	storageKey := fmt.Sprintf("%s/%s.pdf", period, provider)

	_, err := s.db.ExecContext(
		ctx,
		markPaidQuery,
		provider,
		period,
		storageKey,
		paidAt,
	)

	return err
}

// List returns every recorded receipt, newest download first.
func (s *Repository) List(ctx context.Context) ([]Receipt, error) {
	rows, err := s.db.QueryContext(ctx, listReceiptsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // read-only rows

	var out []Receipt
	for rows.Next() {
		var r Receipt
		if err := rows.Scan(&r.ID, &r.Provider, &r.Period, &r.StorageKey, &r.SizeBytes, &r.Price, &r.Status, &r.DownloadedAt, &r.PaidAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}

	return out, rows.Err()
}

// Close releases the database handle.
func (s *Repository) Close() error {
	return s.db.Close()
}
