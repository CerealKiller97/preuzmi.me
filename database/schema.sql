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
    UNIQUE (provider, period)
);
