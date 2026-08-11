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
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CerealKiller97/preuzmi.me/database"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/storage"
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
	// account, period). On conflict it refreshes only the file facts, leaving
	// price and status to their dedicated setters.
	upsertReceiptQuery = `
INSERT INTO receipts (provider, account, period, storage_key, size_bytes, price, status, downloaded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(provider, account, period) DO UPDATE SET
	storage_key   = excluded.storage_key,
	size_bytes    = excluded.size_bytes,
	downloaded_at = excluded.downloaded_at;`

	// latestReceiptQuery returns the most recently downloaded receipt for a
	// (provider, account).
	latestReceiptQuery = `
SELECT id, provider, account, period, storage_key, size_bytes, price, status, downloaded_at, paid_at, confirmed_at, due_at, due_reminded_at
FROM receipts
WHERE provider = ? AND account = ?
ORDER BY downloaded_at DESC, id DESC
LIMIT 1;`

	// setPriceQuery updates the amount owed for an already-recorded receipt.
	setPriceQuery = `UPDATE receipts SET price = ? WHERE provider = ? AND account = ? AND period = ?;`

	// setDueAtQuery stores the payment deadline. A changed due_at clears
	// due_reminded_at so a revised deadline can notify again.
	setDueAtQuery = `
UPDATE receipts
SET due_at = ?,
    due_reminded_at = CASE WHEN due_at = ? THEN due_reminded_at ELSE 0 END
WHERE provider = ? AND account = ? AND period = ?;`

	// markDueRemindedQuery stamps the receipts that were just included in a
	// due-soon notification.
	markDueRemindedQuery = `UPDATE receipts SET due_reminded_at = ? WHERE provider = ? AND account = ? AND period = ?;`

	// selectStatusQuery reads the current status for a (provider, account, period).
	selectStatusQuery = `SELECT status FROM receipts WHERE provider = ? AND account = ? AND period = ?;`

	// selectDownloadedAtQuery reads the last download time for a (provider,
	// account, period); 0 (or no row) means it has never actually been
	// downloaded, so the next Record is a first download.
	selectDownloadedAtQuery = `SELECT downloaded_at FROM receipts WHERE provider = ? AND account = ? AND period = ?;`

	// selectNotifiedDownloadQuery / selectNotifiedConfirmedQuery read whether a
	// download or paid-confirmation notification has already fired for a
	// (provider, account, period). A non-zero value means the user was already
	// told once. The columns are nullable (NULL on rows predating them), so
	// COALESCE folds NULL to 0 — "not yet notified" — keeping the scan an int64.
	selectNotifiedDownloadQuery  = `SELECT COALESCE(notified_download_at, 0) FROM receipts WHERE provider = ? AND account = ? AND period = ?;`
	selectNotifiedConfirmedQuery = `SELECT COALESCE(notified_confirmed_at, 0) FROM receipts WHERE provider = ? AND account = ? AND period = ?;`

	// setNotifiedDownloadQuery / setNotifiedConfirmedQuery stamp the "already
	// notified" marker so a later re-run stays silent about the same receipt.
	setNotifiedDownloadQuery  = `UPDATE receipts SET notified_download_at = ? WHERE provider = ? AND account = ? AND period = ?;`
	setNotifiedConfirmedQuery = `UPDATE receipts SET notified_confirmed_at = ? WHERE provider = ? AND account = ? AND period = ?;`

	// selectConfirmedPaidQuery reads the fields that decide whether a refresh can
	// skip a provider account: the download time, provider status, and confirmed_at.
	selectConfirmedPaidQuery = `SELECT downloaded_at, status, confirmed_at FROM receipts WHERE provider = ? AND account = ? AND period = ?;`

	// setStatusPaidQuery marks a receipt paid by the provider, stamping
	// confirmed_at only on the first confirmation so the timestamp is stable.
	setStatusPaidQuery = `
UPDATE receipts
SET status = ?, confirmed_at = CASE WHEN confirmed_at = 0 THEN ? ELSE confirmed_at END
WHERE provider = ? AND account = ? AND period = ?;`

	// setStatusUnpaidQuery clears both the provider status and its confirmation.
	setStatusUnpaidQuery = `UPDATE receipts SET status = ?, confirmed_at = 0 WHERE provider = ? AND account = ? AND period = ?;`

	// selectPriceQuery reads the recorded price for a (provider, account, period).
	selectPriceQuery = `SELECT price FROM receipts WHERE provider = ? AND account = ? AND period = ?;`

	// markPaidQuery stamps paid_at (the user-marked payment), creating a stub row
	// when the receipt was never recorded by a download.
	markPaidQuery = `
INSERT INTO receipts (provider, account, period, storage_key, size_bytes, price, status, downloaded_at, paid_at)
VALUES (?, ?, ?, ?, 0, 0, ?, 0, ?)
ON CONFLICT(provider, account, period) DO UPDATE SET
	paid_at = excluded.paid_at;`

	// selectIPSQRQuery reads the cached IPS QR payload and whether extraction has
	// already been attempted for a (provider, account, period).
	selectIPSQRQuery = `SELECT ips_qr, ips_checked FROM receipts WHERE provider = ? AND account = ? AND period = ?;`

	// setIPSQRQuery stores the decoded IPS QR payload (possibly empty) and marks
	// extraction as done, so a bill with no QR is not re-parsed on every render.
	setIPSQRQuery = `UPDATE receipts SET ips_qr = ?, ips_checked = 1 WHERE provider = ? AND account = ? AND period = ?;`

	// listReceiptsQuery returns every recorded receipt, newest download first.
	listReceiptsQuery = `
SELECT id, provider, account, period, storage_key, size_bytes, price, status, downloaded_at, paid_at, confirmed_at, due_at, due_reminded_at
FROM receipts
ORDER BY downloaded_at DESC, id DESC;`

	// Migration statements, run once at startup by migrate(). Kept here with the
	// rest so every SQL string the package issues lives in one place.

	// backfillConfirmedAtQuery gives already-paid receipts a confirmation time
	// (their download time) so the column is not blank for history predating it.
	backfillConfirmedAtQuery = `UPDATE receipts SET confirmed_at = downloaded_at WHERE status = ? AND confirmed_at = 0 AND downloaded_at > 0;`

	// backfillNotifiedDownloadQuery / backfillNotifiedConfirmedQuery seed the
	// notification markers for receipts that predate the columns, so an upgrade
	// does not re-announce every download and payment already on record. COALESCE
	// matches whether the marker is still NULL (freshly ALTERed in) or an explicit 0.
	backfillNotifiedDownloadQuery  = `UPDATE receipts SET notified_download_at = downloaded_at WHERE COALESCE(notified_download_at, 0) = 0 AND downloaded_at > 0;`
	backfillNotifiedConfirmedQuery = `UPDATE receipts SET notified_confirmed_at = confirmed_at WHERE COALESCE(notified_confirmed_at, 0) = 0 AND confirmed_at > 0;`

	// selectPaymentsQuery / dropPaymentsTableQuery fold a former standalone
	// payments table back into receipts.paid_at, then drop it.
	selectPaymentsQuery    = `SELECT key, paid_at FROM payments;`
	dropPaymentsTableQuery = `DROP TABLE payments;`

	// setImportedPaidQuery writes a paid_at imported from a former store onto its
	// receipt row (a no-op when the row does not exist yet). Legacy stores predate
	// multi-account, so the imported row is always the solo account ('').
	setImportedPaidQuery = `UPDATE receipts SET paid_at = ? WHERE provider = ? AND account = '' AND period = ?;`

	// tableExistsQuery reports whether a table of the given name is present.
	tableExistsQuery = `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?;`

	// selectDashPeriodsQuery lists rows whose period is still in the legacy dash
	// form; updatePeriodByIDQuery rewrites one to the canonical slash form.
	selectDashPeriodsQuery = `SELECT id, period FROM receipts WHERE period LIKE '%-%';`
	updatePeriodByIDQuery  = `UPDATE OR IGNORE receipts SET period = ? WHERE id = ?;`
)

type Receipt struct {
	Provider string `json:"provider"`
	// Account is the provider account this receipt belongs to: "" for the solo
	// account, or a config account id for a named family-member login.
	Account       string  `json:"account,omitempty"`
	Period        string  `json:"period"`
	StorageKey    string  `json:"storage_key"`
	Status        string  `json:"status"`
	Price         float64 `json:"price"`
	SizeBytes     int64   `json:"size_bytes"`
	DownloadedAt  int64   `json:"downloaded_at"`
	PaidAt        int64   `json:"paid_at"`
	ConfirmedAt   int64   `json:"confirmed_at"`
	DueAt         int64   `json:"due_at"`
	DueRemindedAt int64   `json:"due_reminded_at,omitempty"`
	ID            int64   `json:"id"`
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

	// Rebuild the identity constraint on databases created before the account
	// column: a table-level UNIQUE cannot be altered in, so a solo database still
	// carries UNIQUE(provider, period), which would block a second account for
	// the same provider and period once the user adds one.
	if err := migrateReceiptsUnique(db); err != nil {
		return fmt.Errorf("receipts: rebuilding unique constraint: %w", err)
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
	if _, err := db.Exec(backfillConfirmedAtQuery, StatusPaid); err != nil {
		return fmt.Errorf("receipts: backfilling confirmed_at: %w", err)
	}

	// Backfill the notification markers for receipts that predate them, so
	// upgrading does not re-announce every download and payment already on record.
	// The columns are freshly ALTERed in as NULL on an existing database; a
	// receipt with a real download time was effectively already announced, as was
	// one already confirmed, so use those timestamps as the proxy. COALESCE keeps
	// the match working whether the marker is still NULL or an explicit 0.
	if _, err := db.Exec(backfillNotifiedDownloadQuery); err != nil {
		return fmt.Errorf("receipts: backfilling notified_download_at: %w", err)
	}
	if _, err := db.Exec(backfillNotifiedConfirmedQuery); err != nil {
		return fmt.Errorf("receipts: backfilling notified_confirmed_at: %w", err)
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

	rows, err := db.Query(selectPaymentsQuery)
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

	_, err = db.Exec(dropPaymentsTableQuery)

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

	// Legacy stores predate multi-account: everything imports as the solo
	// account ('').
	res, err := db.Exec(setImportedPaidQuery, paidAt, provider, period)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}

	storageKey := storage.ReceiptKey(dashPeriod(period), provider, "")
	_, err = db.Exec(markPaidQuery, provider, "", period, storageKey, StatusUnpaid, paidAt)

	return err
}

// migrateReceiptsUnique rebuilds the receipts table when it still carries the
// pre-multi-account UNIQUE(provider, period) constraint, replacing it with
// UNIQUE(provider, account, period). SQLite cannot drop a table-level constraint
// in place, so the table is recreated from the current schema and its rows
// copied across. It runs after database.Apply, so the account column already
// exists; existing rows keep account '' and are byte-identical afterwards.
//
// The rebuild is skipped whenever the stored table definition no longer contains
// the old constraint, so it is a no-op on fresh databases and on every startup
// after the one that migrates.
func migrateReceiptsUnique(db *sql.DB) error {
	var createSQL string
	switch err := db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='receipts'`,
	).Scan(&createSQL); {
	case err == sql.ErrNoRows:
		return nil
	case err != nil:
		return err
	}

	// Compare against the collapsed, lowercased DDL so whitespace does not matter.
	// The old constraint is UNIQUE(provider,period); the new one carries account,
	// so its absence means the table is already on the new shape.
	collapsed := strings.ToLower(strings.Join(strings.Fields(createSQL), ""))
	if !strings.Contains(collapsed, "unique(provider,period)") {
		return nil
	}

	cols, err := columnNames(db, "receipts")
	if err != nil {
		return err
	}
	list := strings.Join(cols, ", ")

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rolled back only if Commit did not run

	// Move the old table aside, recreate receipts fresh from the schema (which now
	// carries UNIQUE(provider, account, period)), copy the rows over by explicit
	// column list, and drop the old table. The schema's other CREATE TABLE IF NOT
	// EXISTS statements are harmless no-ops here.
	stmts := []string{
		`ALTER TABLE receipts RENAME TO receipts_old`,
		database.Schema,
		fmt.Sprintf("INSERT INTO receipts (%s) SELECT %s FROM receipts_old", list, list),
		`DROP TABLE receipts_old`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// columnNames returns the column names of a table in schema order.
func columnNames(db *sql.DB, table string) ([]string, error) {
	// table comes from our own code, not user input.
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck // read-only rows

	var cols []string
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}

	return cols, rows.Err()
}

// tableExists reports whether a table of the given name is present.
func tableExists(db *sql.DB, name string) (bool, error) {
	var found string
	err := db.QueryRow(tableExistsQuery, name).Scan(&found)
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
	rows, err := db.Query(selectDashPeriodsQuery)
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
		if _, err := db.Exec(updatePeriodByIDQuery, u.period, u.id); err != nil {
			return err
		}
	}

	return nil
}

// Record upserts a downloaded receipt keyed by (provider, account, period): a
// re-download of the same period updates the existing row rather than adding a
// duplicate.
func (s *Repository) Record(ctx context.Context, r Receipt) error {
	if r.DownloadedAt == 0 {
		r.DownloadedAt = time.Now().Unix()
	}

	// The period column is stored in slash form; the storage key stays as the
	// provider wrote it (dash), so paths are unaffected.
	r.Period = slashPeriod(r.Period)

	// Announce a download exactly once, ever. We key off a persistent "already
	// notified" marker rather than "is this the first download": a daily re-run —
	// or any provider that rewrites this receipt's row on every pass — must not
	// re-announce a bill the user has already been told about. Read the marker
	// before the upsert; 0 means it has never been announced.
	var notifiedDownloadAt int64
	_ = s.db.QueryRowContext(ctx, selectNotifiedDownloadQuery, r.Provider, r.Account, r.Period).Scan(&notifiedDownloadAt)
	alreadyNotified := notifiedDownloadAt != 0

	// On a re-download we refresh the file facts but deliberately leave price,
	// status and paid_at untouched — those are set by SetPrice / SetStatus /
	// MarkPaid and must not be reset by the storage-layer recorder.
	_, err := s.db.ExecContext(
		ctx,
		upsertReceiptQuery,
		r.Provider,
		r.Account,
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

	if !alreadyNotified {
		// Stamp the marker in the same call that queues the announcement, so even a
		// crash right after leaves the receipt marked as told (matching the existing
		// behaviour, where state advances before the notification is sent).
		if _, err := s.db.ExecContext(ctx, setNotifiedDownloadQuery, time.Now().Unix(), r.Provider, r.Account, r.Period); err != nil {
			return err
		}

		s.mu.Lock()
		s.newlyDownloaded = append(s.newlyDownloaded, Receipt{Provider: r.Provider, Account: r.Account, Period: r.Period})
		s.mu.Unlock()
	}

	return nil
}

// HasDownloaded reports whether the receipt for (provider, period) is already on
// record — a row exists with a real download time. A paid-only stub, which
// carries downloaded_at 0, does not count, so its PDF is still fetched.
func (s *Repository) HasDownloaded(ctx context.Context, provider, account, period string) bool {
	period = slashPeriod(period)

	var downloadedAt int64
	err := s.db.QueryRowContext(ctx, selectDownloadedAtQuery, provider, account, period).Scan(&downloadedAt)

	return err == nil && downloadedAt > 0
}

// IsConfirmedPaid reports whether the receipt for (provider, period) is
// downloaded and the provider has confirmed it paid: a real download time,
// status "plaćeno", and confirmed_at set. Only then is there nothing left for a
// refresh to fetch or learn — a not-yet-downloaded or still-unpaid bill is worth
// re-checking so its PDF is fetched and a paid-confirmation can fire.
//
// Unlike a fully-settled check this does NOT require the user to have marked the
// receipt paid in the app (paid_at): once the provider itself confirms payment
// there is no reason to keep logging in, whether or not the user clicked "paid".
func (s *Repository) IsConfirmedPaid(ctx context.Context, provider, account, period string) bool {
	period = slashPeriod(period)

	var downloadedAt, confirmedAt int64
	var status string
	err := s.db.QueryRowContext(ctx, selectConfirmedPaidQuery, provider, account, period).Scan(&downloadedAt, &status, &confirmedAt)
	if err != nil {
		return false
	}

	return downloadedAt > 0 && status == StatusPaid && confirmedAt != 0
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

// Latest returns the most recently downloaded receipt for a (provider, account).
func (s *Repository) Latest(ctx context.Context, provider, account string) (Receipt, bool) {
	var r Receipt
	err := s.db.QueryRowContext(ctx, latestReceiptQuery, provider, account).Scan(
		&r.ID,
		&r.Provider,
		&r.Account,
		&r.Period,
		&r.StorageKey,
		&r.SizeBytes,
		&r.Price,
		&r.Status,
		&r.DownloadedAt,
		&r.PaidAt,
		&r.ConfirmedAt,
		&r.DueAt,
		&r.DueRemindedAt,
	)
	if err != nil {
		return Receipt{}, false
	}

	return r, true
}

// SetPrice updates the amount owed for an already-recorded receipt.
func (s *Repository) SetPrice(ctx context.Context, provider, account, period string, price float64) error {
	_, err := s.db.ExecContext(
		ctx,
		setPriceQuery,
		price,
		provider,
		account,
		slashPeriod(period),
	)

	return err
}

// SetDueAt stores the payment deadline for an already-recorded receipt. Passing
// 0 clears it. A changed deadline resets due_reminded_at so reminders can fire
// again for the new date.
func (s *Repository) SetDueAt(ctx context.Context, provider, account, period string, dueAt int64) error {
	_, err := s.db.ExecContext(
		ctx,
		setDueAtQuery,
		dueAt,
		dueAt,
		provider,
		account,
		slashPeriod(period),
	)

	return err
}

// MarkDueReminded stamps due_reminded_at on each (provider, period) so a later
// refresh does not re-send the same due-soon notification.
func (s *Repository) MarkDueReminded(ctx context.Context, items []Receipt, at int64) error {
	if at == 0 {
		at = time.Now().Unix()
	}
	for _, it := range items {
		if _, err := s.db.ExecContext(ctx, markDueRemindedQuery, at, it.Provider, it.Account, slashPeriod(it.Period)); err != nil {
			return err
		}
	}

	return nil
}

// ReconcilePrice corrects a receipt's stored price to want when the two differ
// by more than half a cent, and reports whether it changed anything. It lets the
// IPS QR amount — the figure a banking app actually charges — override a price a
// provider's API or PDF parse recorded differently, so a bill that was filed
// with a wrong total self-heals. A missing row is a no-op.
func (s *Repository) ReconcilePrice(ctx context.Context, provider, account, period string, want float64) (bool, error) {
	period = slashPeriod(period)

	var current float64
	switch err := s.db.QueryRowContext(ctx, selectPriceQuery, provider, account, period).Scan(&current); {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, err
	}

	if math.Abs(current-want) < 0.005 {
		return false, nil
	}

	if _, err := s.db.ExecContext(ctx, setPriceQuery, want, provider, account, period); err != nil {
		return false, err
	}

	return true, nil
}

// SetStatus updates the provider-reported paid state ("plaćeno" / "neplaćeno")
// for an already-recorded receipt.
//
// Becoming paid stamps confirmed_at ("verifikovano") on the first confirmation
// and queues the receipt in newlyVerified so a caller can notify about it via
// DrainNewlyVerified; becoming unpaid clears confirmed_at.
func (s *Repository) SetStatus(ctx context.Context, provider, account, period, status string) error {
	period = slashPeriod(period)

	var prev string
	// A missing row (err != nil) means this receipt was never downloaded. That
	// happens when a fresh invoice reports the *previous* month settled before we
	// ever saw the previous month — e.g. the first time an email provider runs, or
	// a gap in history. There is nothing on record to confirm, so mark nothing and
	// queue no notification: a payment is only confirmed for a receipt we hold.
	existed := s.db.QueryRowContext(ctx, selectStatusQuery, provider, account, period).Scan(&prev) == nil

	// Whether we have already announced this payment. Read it before the status
	// write so a provider that rewrites status every run (e.g. Yettel/eUpravnik
	// flip the current bill back to unpaid, clearing confirmed_at, then re-confirm
	// it later) cannot make us re-announce a payment already reported once.
	var notifiedConfirmedAt int64
	_ = s.db.QueryRowContext(ctx, selectNotifiedConfirmedQuery, provider, account, period).Scan(&notifiedConfirmedAt)

	if status == StatusPaid {
		if _, err := s.db.ExecContext(ctx, setStatusPaidQuery, status, time.Now().Unix(), provider, account, period); err != nil {
			return err
		}
	} else {
		if _, err := s.db.ExecContext(ctx, setStatusUnpaidQuery, status, provider, account, period); err != nil {
			return err
		}
	}

	if existed && status == StatusPaid && notifiedConfirmedAt == 0 {
		// Stamp the marker so this payment is never announced again, even if the
		// status later flaps unpaid → paid.
		if _, err := s.db.ExecContext(ctx, setNotifiedConfirmedQuery, time.Now().Unix(), provider, account, period); err != nil {
			return err
		}

		var price float64
		_ = s.db.QueryRowContext(ctx, selectPriceQuery, provider, account, period).Scan(&price)

		s.mu.Lock()
		s.newlyVerified = append(s.newlyVerified, Receipt{
			Provider: provider,
			Account:  account,
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
func (s *Repository) MarkPaid(ctx context.Context, provider, account, period string, paid bool) (int64, error) {
	period = slashPeriod(period)

	var at int64
	if paid {
		at = time.Now().Unix()
	}

	// The storage key is a path, so it uses the dash form; the period column uses
	// the slash form.
	storageKey := storage.ReceiptKey(dashPeriod(period), provider, account)

	if _, err := s.db.ExecContext(ctx, markPaidQuery, provider, account, period, storageKey, StatusUnpaid, at); err != nil {
		return 0, err
	}

	return at, nil
}

// IPSQR returns the cached NBS IPS QR payload for a receipt and whether
// extraction has already been attempted. checked is true once we have tried to
// parse the PDF, even if it carried no QR (payload is then ""), so callers can
// avoid re-parsing a bill that has none.
func (s *Repository) IPSQR(ctx context.Context, provider, account, period string) (payload string, checked bool, err error) {
	period = slashPeriod(period)

	var checkedInt int64
	err = s.db.QueryRowContext(ctx, selectIPSQRQuery, provider, account, period).Scan(&payload, &checkedInt)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	return payload, checkedInt != 0, nil
}

// SetIPSQR caches the decoded IPS QR payload for a receipt (empty when the bill
// has none) and marks extraction as done. It is a no-op when no receipt row
// exists for the (provider, period) yet.
func (s *Repository) SetIPSQR(ctx context.Context, provider, account, period, payload string) error {
	_, err := s.db.ExecContext(ctx, setIPSQRQuery, payload, provider, account, slashPeriod(period))

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
		if err := rows.Scan(&r.ID, &r.Provider, &r.Account, &r.Period, &r.StorageKey, &r.SizeBytes, &r.Price, &r.Status, &r.DownloadedAt, &r.PaidAt, &r.ConfirmedAt, &r.DueAt, &r.DueRemindedAt); err != nil {
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
