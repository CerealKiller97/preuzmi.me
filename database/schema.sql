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
    -- due_at: payment deadline parsed from the PDF (datum dospeća / rok za
    -- plaćanje / datum valute), unix seconds at local midnight. 0 when unknown.
    -- due_reminded_at: when a due-soon notification last covered this receipt,
    -- so the daily check does not re-nag about the same deadline.
    due_at          INTEGER NOT NULL DEFAULT 0,
    due_reminded_at INTEGER NOT NULL DEFAULT 0,
    UNIQUE (provider, period)
);
