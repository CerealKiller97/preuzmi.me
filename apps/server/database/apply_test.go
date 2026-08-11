package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestParseSchemaTables(t *testing.T) {
	tables, err := parseSchemaTables(Schema)
	if err != nil {
		t.Fatalf("parseSchemaTables: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	if tables[0].name != "receipts" {
		t.Fatalf("table name = %q, want receipts", tables[0].name)
	}

	want := []string{
		"id", "provider", "account", "period", "storage_key", "size_bytes",
		"price", "status", "downloaded_at", "paid_at", "confirmed_at",
		"due_at", "due_reminded_at",
		"ips_qr", "ips_checked",
		"notified_download_at", "notified_confirmed_at",
	}
	if len(tables[0].columns) != len(want) {
		t.Fatalf("columns = %v, want %v", columnNames(tables[0]), want)
	}
	for i, name := range want {
		if tables[0].columns[i].name != name {
			t.Fatalf("column[%d] = %q, want %q", i, tables[0].columns[i].name, name)
		}
	}

	// Table-level UNIQUE must not be treated as a column.
	for _, col := range tables[0].columns {
		if col.name == "UNIQUE" {
			t.Fatal("UNIQUE constraint parsed as a column")
		}
	}
}

func TestApplyAddsMissingColumns(t *testing.T) {
	db := openTempDB(t)

	// Simulate a pre-paid-state database: core columns only, no paid_at /
	// confirmed_at. Apply must ALTER them in from schema.sql.
	legacy := `
CREATE TABLE receipts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    provider      TEXT    NOT NULL,
    period        TEXT    NOT NULL,
    storage_key   TEXT    NOT NULL,
    size_bytes    INTEGER NOT NULL,
    price         REAL    NOT NULL DEFAULT 0.00,
    status        TEXT    NOT NULL DEFAULT 'neplaćeno',
    downloaded_at INTEGER NOT NULL,
    UNIQUE (provider, period)
);`
	if _, err := db.Exec(legacy); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO receipts (provider, period, storage_key, size_bytes, price, status, downloaded_at)
VALUES ('eps', '06/2026', '06-2026/eps.pdf', 10, 1.5, 'plaćeno', 100)`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	if err := Apply(db); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	cols, err := existingColumns(db, "receipts")
	if err != nil {
		t.Fatalf("existingColumns: %v", err)
	}
	for _, name := range []string{"paid_at", "confirmed_at", "ips_qr", "ips_checked", "notified_download_at", "notified_confirmed_at"} {
		if !cols[name] {
			t.Fatalf("missing column %s after Apply", name)
		}
	}

	var paidAt, confirmedAt int64
	var provider string
	if err := db.QueryRow(`SELECT provider, paid_at, confirmed_at FROM receipts WHERE provider = 'eps'`).
		Scan(&provider, &paidAt, &confirmedAt); err != nil {
		t.Fatalf("select upgraded row: %v", err)
	}
	if provider != "eps" || paidAt != 0 || confirmedAt != 0 {
		t.Fatalf("upgraded row = (%s, %d, %d), want (eps, 0, 0)", provider, paidAt, confirmedAt)
	}

	// Idempotent: a second Apply must be a no-op.
	if err := Apply(db); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
}

func TestApplyCreatesFreshSchema(t *testing.T) {
	db := openTempDB(t)

	if err := Apply(db); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	cols, err := existingColumns(db, "receipts")
	if err != nil {
		t.Fatalf("existingColumns: %v", err)
	}
	for _, name := range []string{"id", "provider", "paid_at", "confirmed_at", "ips_qr", "ips_checked", "notified_download_at", "notified_confirmed_at"} {
		if !cols[name] {
			t.Fatalf("fresh schema missing %s", name)
		}
	}
}

func openTempDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "test.db") + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func columnNames(table schemaTable) []string {
	out := make([]string, len(table.columns))
	for i, c := range table.columns {
		out[i] = c.name
	}

	return out
}
