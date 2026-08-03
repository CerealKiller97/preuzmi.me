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
    UNIQUE (provider, period)
);
