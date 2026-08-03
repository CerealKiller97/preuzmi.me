// Package receipts keeps a SQLite record of every receipt the app has
// downloaded.
//
// The PDF bytes themselves live wherever the storage backend puts them (local
// folder or S3); the receipts table is the queryable index of what was fetched,
// when, and how big it was, and the payments table is the single source of truth
// for which receipts the user has marked paid. The database file sits next to
// the other state files in the download folder (receipts.db, refresh.json).
package receipts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CerealKiller97/preuzmi.me/database"
	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"
)

const fileName = "receipts.db"

// The period column is stored as "MM/YYYY" (e.g. "06/2026"). Storage keys and
// URLs, however, embed the period as a path segment where a slash would be read
// as a separator, so those keep the "MM-YYYY" form (built by the providers).
// slashPeriod converts to the column form, and every repository method
// normalizes its period argument with it so a caller may pass either form.

// periodParts parses "MM-YYYY", "0MM-YYYY" or "MM/YYYY" into month and year.
func periodParts(p string) (month, year int, ok bool) {
	parts := strings.Split(strings.ReplaceAll(p, "/", "-"), "-")
	if len(parts) != 2 {
		return 0, 0, false
	}

	m, errM := strconv.Atoi(parts[0])
	y, errY := strconv.Atoi(parts[1])
	if errM != nil || errY != nil {
		return 0, 0, false
	}

	return m, y, true
}

// slashPeriod returns the canonical "MM/YYYY" form kept in the period column.
// Unparseable input is returned unchanged so nothing is silently dropped.
func slashPeriod(p string) string {
	m, y, ok := periodParts(p)
	if !ok {
		return p
	}

	return fmt.Sprintf("%02d/%d", m, y)
}

// dashPeriod returns the "MM-YYYY" form used inside storage keys, where a slash
// would be read as a path separator.
func dashPeriod(p string) string {
	m, y, ok := periodParts(p)
	if !ok {
		return p
	}

	return fmt.Sprintf("%02d-%d", m, y)
}

// Paid-state labels stored in the status column, as reported by the provider.
// The status flip to StatusPaid is what stamps confirmed_at; paid_at is separate
// and reflects the user marking the receipt paid in this app.
const (
	StatusPaid   = "plaćeno"
	StatusUnpaid = "neplaćeno"
)

// SQL statements run against the database. Kept together so the shape of every
// query is visible in one place.
const (
	// upsertReceiptQuery records a downloaded receipt keyed by (provider,
	// period). On conflict it refreshes only the file facts, leaving price and
	// status to their dedicated setters.
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
SELECT id, provider, period, storage_key, size_bytes, price, status, downloaded_at, paid_at, confirmed_at
FROM receipts
WHERE provider = ?
ORDER BY downloaded_at DESC, id DESC
LIMIT 1;`

	// setPriceQuery updates the amount owed for an already-recorded receipt.
	setPriceQuery = `UPDATE receipts SET price = ? WHERE provider = ? AND period = ?;`

	// selectStatusQuery reads the current status for a (provider, period).
	selectStatusQuery = `SELECT status FROM receipts WHERE provider = ? AND period = ?;`

	// selectDownloadedAtQuery reads the last download time for a (provider,
	// period); 0 (or no row) means it has never actually been downloaded, so the
	// next Record is a first download.
	selectDownloadedAtQuery = `SELECT downloaded_at FROM receipts WHERE provider = ? AND period = ?;`

	// selectSettledQuery reads the fields that decide whether a refresh can skip
	// a provider: user paid_at, provider status, and confirmed_at.
	selectSettledQuery = `SELECT paid_at, status, confirmed_at FROM receipts WHERE provider = ? AND period = ?;`

	// setStatusPaidQuery marks a receipt paid by the provider, stamping
	// confirmed_at only on the first confirmation so the timestamp is stable.
	setStatusPaidQuery = `
UPDATE receipts
SET status = ?, confirmed_at = CASE WHEN confirmed_at = 0 THEN ? ELSE confirmed_at END
WHERE provider = ? AND period = ?;`

	// setStatusUnpaidQuery clears both the provider status and its confirmation.
	setStatusUnpaidQuery = `UPDATE receipts SET status = ?, confirmed_at = 0 WHERE provider = ? AND period = ?;`

	// selectPriceQuery reads the recorded price for a (provider, period).
	selectPriceQuery = `SELECT price FROM receipts WHERE provider = ? AND period = ?;`

	// markPaidQuery stamps paid_at (the user-marked payment), creating a stub row
	// when the receipt was never recorded by a download.
	markPaidQuery = `
INSERT INTO receipts (provider, period, storage_key, size_bytes, price, status, downloaded_at, paid_at)
VALUES (?, ?, ?, 0, 0, ?, 0, ?)
ON CONFLICT(provider, period) DO UPDATE SET
	paid_at = excluded.paid_at;`

	// listReceiptsQuery returns every recorded receipt, newest download first.
	listReceiptsQuery = `
SELECT id, provider, period, storage_key, size_bytes, price, status, downloaded_at, paid_at, confirmed_at
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
	ConfirmedAt  int64   `json:"confirmed_at"`
	ID           int64   `json:"id"`
}

// Repository is a SQLite-backed index of downloaded receipts.
type Repository struct {
	db *sql.DB

	// newlyVerified collects receipts whose provider status flipped from unpaid
	// to paid; newlyDownloaded collects receipts downloaded for the first time.
	// A caller drains each after a refresh to notify about them once — so a daily
	// re-run neither re-announces a download nor re-announces a payment.
	mu              sync.Mutex
	newlyVerified   []Receipt
	newlyDownloaded []Receipt
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

	if err := migrate(db, dir); err != nil {
		if err := db.Close(); err != nil {
			return nil, err
		}
		return nil, err
	}

	return &Repository{
		db: db,
	}, nil
}

// migrate applies database/schema.sql (creating tables and adding any columns
// that appear there but are missing from an older database) and then runs
// receipts-specific data migrations. Schema shape changes belong in
// database/schema.sql only; this function keeps the data rewrites.
func migrate(db *sql.DB, dir string) error {
	if err := database.Apply(db); err != nil {
		return fmt.Errorf("receipts: applying schema: %w", err)
	}

	if err := migratePeriodsToSlash(db); err != nil {
		return fmt.Errorf("receipts: migrating periods: %w", err)
	}

	// Fold any earlier separate paid-state stores back into the receipts row.
	if err := migratePaymentsTableIntoReceipts(db); err != nil {
		return fmt.Errorf("receipts: merging payments table: %w", err)
	}

	if err := migratePaymentsJSONIntoReceipts(db, dir); err != nil {
		return fmt.Errorf("receipts: merging payments.json: %w", err)
	}

	// Give already-confirmed receipts a confirmation time so the column is not
	// blank for history predating it. downloaded_at is the closest known proxy.
	if _, err := db.Exec(
		`UPDATE receipts SET confirmed_at = downloaded_at WHERE status = ? AND confirmed_at = 0 AND downloaded_at > 0`,
		StatusPaid,
	); err != nil {
		return fmt.Errorf("receipts: backfilling confirmed_at: %w", err)
	}

	return nil
}

// migratePaymentsTableIntoReceipts folds a former standalone payments table
// (key -> paid_at) back into the receipts.paid_at column, then drops it. It is a
// no-op when the table is absent, so it is safe on every startup.
func migratePaymentsTableIntoReceipts(db *sql.DB) error {
	has, err := tableExists(db, "payments")
	if err != nil {
		return err
	}
	if !has {
		return nil
	}

	rows, err := db.Query(`SELECT key, paid_at FROM payments`)
	if err != nil {
		return err
	}

	type entry struct {
		key    string
		paidAt int64
	}

	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.key, &e.paidAt); err != nil {
			_ = rows.Close()
			return err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, e := range entries {
		if err := applyImportedPaid(db, e.key, e.paidAt); err != nil {
			return err
		}
	}

	_, err = db.Exec(`DROP TABLE payments`)

	return err
}

// migratePaymentsJSONIntoReceipts imports a legacy receipts/payments.json file
// (key -> paid_at) into receipts.paid_at, then renames it aside so the import
// runs only once. A missing file is the normal case, not an error.
func migratePaymentsJSONIntoReceipts(db *sql.DB, dir string) error {
	path := filepath.Join(dir, "payments.json")

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if trimmed := strings.TrimSpace(string(b)); trimmed != "" {
		var paid map[string]int64
		if err := json.Unmarshal(b, &paid); err != nil {
			return err
		}
		for key, at := range paid {
			if err := applyImportedPaid(db, key, at); err != nil {
				return err
			}
		}
	}

	return os.Rename(path, path+".migrated")
}

// applyImportedPaid writes a paid_at imported from a former store onto its
// receipt row, keyed by the app's lookup key ("period|provider", dash period).
// When no such receipt exists, it recreates the paid-only stub row so a paid
// mark for a never-downloaded receipt is not lost.
func applyImportedPaid(db *sql.DB, key string, paidAt int64) error {
	dashPart, providerPart, ok := strings.Cut(key, "|")
	if !ok {
		return nil // unrecognizable key; skip rather than fail the migration
	}

	provider := providerPart
	period := slashPeriod(dashPart)

	res, err := db.Exec(`UPDATE receipts SET paid_at = ? WHERE provider = ? AND period = ?`, paidAt, provider, period)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}

	storageKey := fmt.Sprintf("%s/%s.pdf", dashPeriod(period), provider)
	_, err = db.Exec(markPaidQuery, provider, period, storageKey, StatusUnpaid, paidAt)

	return err
}

// tableExists reports whether a table of the given name is present.
func tableExists(db *sql.DB, name string) (bool, error) {
	var found string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// migratePeriodsToSlash rewrites any legacy dash-form period ("06-2026") in the
// period column to the slash form ("06/2026"). It is idempotent — slash-form
// rows contain no dash and are skipped — so it can run on every startup. The
// UPDATE ignores the rare case where a slash row already exists for the same
// (provider, period), leaving the legacy row untouched rather than failing the
// whole migration on a unique-constraint violation.
func migratePeriodsToSlash(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, period FROM receipts WHERE period LIKE '%-%'`)
	if err != nil {
		return err
	}

	type update struct {
		period string
		id     int64
	}

	var updates []update
	for rows.Next() {
		var (
			id     int64
			period string
		)
		if err := rows.Scan(&id, &period); err != nil {
			_ = rows.Close()
			return err
		}
		updates = append(updates, update{period: slashPeriod(period), id: id})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, u := range updates {
		if _, err := db.Exec(`UPDATE OR IGNORE receipts SET period = ? WHERE id = ?`, u.period, u.id); err != nil {
			return err
		}
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

	// The period column is stored in slash form; the storage key stays as the
	// provider wrote it (dash), so paths are unaffected.
	r.Period = slashPeriod(r.Period)

	// A first download is one with no prior download time (no row, or a paid-only
	// stub with downloaded_at 0). Read it before the upsert so DrainNewlyDownloaded
	// can announce it once, and a daily re-download stays silent.
	var prevDownloadedAt int64
	_ = s.db.QueryRowContext(ctx, selectDownloadedAtQuery, r.Provider, r.Period).Scan(&prevDownloadedAt)
	firstDownload := prevDownloadedAt == 0

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
	if err != nil {
		return err
	}

	if firstDownload {
		s.mu.Lock()
		s.newlyDownloaded = append(s.newlyDownloaded, Receipt{Provider: r.Provider, Period: r.Period})
		s.mu.Unlock()
	}

	return nil
}

// HasDownloaded reports whether the receipt for (provider, period) is already on
// record — a row exists with a real download time. A paid-only stub, which
// carries downloaded_at 0, does not count, so its PDF is still fetched.
func (s *Repository) HasDownloaded(ctx context.Context, provider, period string) bool {
	period = slashPeriod(period)

	var downloadedAt int64
	err := s.db.QueryRowContext(ctx, selectDownloadedAtQuery, provider, period).Scan(&downloadedAt)

	return err == nil && downloadedAt > 0
}

// IsSettled reports whether the receipt for (provider, period) is fully done:
// the user has stamped paid_at, the provider reports status "plaćeno", and
// confirmed_at is set. Only then is there nothing left for a refresh to learn or
// fetch — an unpaid or unverified bill is still worth re-checking so status can
// flip and paid-confirmation can fire.
func (s *Repository) IsSettled(ctx context.Context, provider, period string) bool {
	period = slashPeriod(period)

	var paidAt, confirmedAt int64
	var status string
	err := s.db.QueryRowContext(ctx, selectSettledQuery, provider, period).Scan(&paidAt, &status, &confirmedAt)
	if err != nil {
		return false
	}

	return paidAt != 0 && status == StatusPaid && confirmedAt != 0
}

// DrainNewlyDownloaded returns and clears the receipts downloaded for the first
// time since the last drain.
func (s *Repository) DrainNewlyDownloaded() []Receipt {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := s.newlyDownloaded
	s.newlyDownloaded = nil

	return out
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
		&r.ConfirmedAt,
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
		slashPeriod(period),
	)

	return err
}

// SetStatus updates the provider-reported paid state ("plaćeno" / "neplaćeno")
// for an already-recorded receipt.
//
// Becoming paid stamps confirmed_at ("verifikovano") on the first confirmation
// and queues the receipt in newlyVerified so a caller can notify about it via
// DrainNewlyVerified; becoming unpaid clears confirmed_at.
func (s *Repository) SetStatus(ctx context.Context, provider, period, status string) error {
	period = slashPeriod(period)

	var prev string
	// A missing row (err != nil) means this receipt was never downloaded. That
	// happens when a fresh invoice reports the *previous* month settled before we
	// ever saw the previous month — e.g. the first time an email provider runs, or
	// a gap in history. There is nothing on record to confirm, so mark nothing and
	// queue no notification: a payment is only confirmed for a receipt we hold.
	existed := s.db.QueryRowContext(ctx, selectStatusQuery, provider, period).Scan(&prev) == nil

	if status == StatusPaid {
		if _, err := s.db.ExecContext(ctx, setStatusPaidQuery, status, time.Now().Unix(), provider, period); err != nil {
			return err
		}
	} else {
		if _, err := s.db.ExecContext(ctx, setStatusUnpaidQuery, status, provider, period); err != nil {
			return err
		}
	}

	if existed && status == StatusPaid && prev != StatusPaid {
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

// MarkPaid records the user marking a receipt paid (or unpaid) by stamping
// paid_at, and returns the stored timestamp (0 when unmarking). It upserts so a
// receipt can be marked paid even if it was never recorded by a download: a stub
// row is created with the conventional storage key, which a later download fills
// in without disturbing paid_at.
func (s *Repository) MarkPaid(ctx context.Context, provider, period string, paid bool) (int64, error) {
	period = slashPeriod(period)

	var at int64
	if paid {
		at = time.Now().Unix()
	}

	// The storage key is a path, so it uses the dash form; the period column uses
	// the slash form.
	storageKey := fmt.Sprintf("%s/%s.pdf", dashPeriod(period), provider)

	if _, err := s.db.ExecContext(ctx, markPaidQuery, provider, period, storageKey, StatusUnpaid, at); err != nil {
		return 0, err
	}

	return at, nil
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
		if err := rows.Scan(&r.ID, &r.Provider, &r.Period, &r.StorageKey, &r.SizeBytes, &r.Price, &r.Status, &r.DownloadedAt, &r.PaidAt, &r.ConfirmedAt); err != nil {
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
