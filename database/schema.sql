-- Desired shape of the SQLite store. database.Apply creates missing tables and
-- ALTERs in any columns listed here that an older database is missing, so adding
-- a column is an edit to this file only (rebuild; no Go migration stub).
--
-- New columns that may appear on non-empty databases must include a DEFAULT so
-- SQLite can ADD COLUMN them. Table-level constraints (UNIQUE below) apply to
-- fresh CREATE TABLE only; they are not retrofitted onto existing tables.
CREATE TABLE IF NOT EXISTS receipts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    provider      TEXT    NOT NULL,
    period        TEXT    NOT NULL,
    storage_key   TEXT    NOT NULL,
    size_bytes    INTEGER NOT NULL,
    price         REAL    NOT NULL DEFAULT 0.00,
    status        TEXT    NOT NULL DEFAULT 'neplaćeno',
    downloaded_at INTEGER NOT NULL,
    -- paid_at: when the user marked the receipt paid in this app (e.g. after
    -- scanning the payment QR). confirmed_at: when the provider confirmed the
    -- payment on their side ("verifikovano"), stamped when status becomes paid.
    -- Both are 0 when not set.
    paid_at       INTEGER NOT NULL DEFAULT 0,
    confirmed_at  INTEGER NOT NULL DEFAULT 0,
    -- ips_qr: the NBS IPS payment QR payload decoded from the bill PDF, so the
    -- dashboard can render a scannable copy on the receipt card. Empty when the
    -- bill carries no readable QR. ips_checked flips to 1 the first time
    -- extraction runs, so a bill with no QR is not re-parsed on every page load.
    ips_qr        TEXT    NOT NULL DEFAULT '',
    ips_checked   INTEGER NOT NULL DEFAULT 0,
    -- notified_download_at / notified_confirmed_at record that a download or a
    -- paid-confirmation notification has already gone out for this receipt, so a
    -- daily re-run never re-announces it. They are set once, when the first
    -- notification fires, and are deliberately NOT cleared when a provider churns
    -- the status column (some providers rewrite status on every run): "already
    -- told the user once" must survive that.
    --
    -- Nullable on purpose: these are added to existing databases with a plain
    -- ALTER TABLE ADD COLUMN (see database.Apply), so every row that predates the
    -- columns starts as NULL. NULL means "not yet notified" — callers COALESCE it
    -- to 0 — and the migration backfill (receipts.migrate) fills in the rows that
    -- were already downloaded/confirmed so upgrading does not re-announce history.
    notified_download_at  INTEGER,
    notified_confirmed_at INTEGER,
    UNIQUE (provider, period)
);
