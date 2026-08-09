package receipts

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// A database created with the pre-multi-account UNIQUE(provider, period) must be
// rebuilt on open so two accounts can share a provider and period. This proves
// the legacy row survives the rebuild and a second account then inserts cleanly.
func TestMigrateReceiptsUniqueRebuild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)

	// Build a legacy-shaped table: no account column, old unique.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	_, err = db.Exec(`
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
);
INSERT INTO receipts (provider, period, storage_key, size_bytes, downloaded_at)
VALUES ('eps', '07/2026', '07-2026/eps.pdf', 10, 100);`)
	if err != nil {
		t.Fatalf("seed legacy db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	// Open through New, which runs the migration (schema apply + unique rebuild).
	store, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	// The legacy solo row survived with account ''.
	if !store.HasDownloaded(ctx, "eps", "", "07/2026") {
		t.Fatal("legacy solo receipt did not survive the unique rebuild")
	}

	// A second account for the same provider and period now inserts cleanly —
	// impossible under the old UNIQUE(provider, period).
	err = store.Record(ctx, Receipt{
		Provider:   "eps",
		Account:    "tata",
		Period:     "07/2026",
		StorageKey: "07-2026/eps/tata.pdf",
		SizeBytes:  20,
	})
	if err != nil {
		t.Fatalf("record second account: %v", err)
	}

	if !store.HasDownloaded(ctx, "eps", "tata", "07/2026") {
		t.Fatal("second account receipt not recorded after rebuild")
	}
}

// Two accounts of the same provider and period are independent rows: marking one
// paid or setting its price must not touch the other.
func TestAccountsAreIndependent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()
	for _, acc := range []string{"mama", "tata"} {
		if err := store.Record(ctx, Receipt{
			Provider:   "eps",
			Account:    acc,
			Period:     "07/2026",
			StorageKey: "07-2026/eps/" + acc + ".pdf",
			SizeBytes:  10,
		}); err != nil {
			t.Fatalf("record %s: %v", acc, err)
		}
	}

	if _, err := store.MarkPaid(ctx, "eps", "mama", "07/2026", true); err != nil {
		t.Fatalf("MarkPaid mama: %v", err)
	}
	if err := store.SetPrice(ctx, "eps", "tata", "07/2026", 1234.5); err != nil {
		t.Fatalf("SetPrice tata: %v", err)
	}

	mama, ok := store.Latest(ctx, "eps", "mama")
	if !ok {
		t.Fatal("mama receipt missing")
	}
	tata, ok := store.Latest(ctx, "eps", "tata")
	if !ok {
		t.Fatal("tata receipt missing")
	}

	if mama.PaidAt == 0 {
		t.Fatal("mama should be marked paid")
	}
	if tata.PaidAt != 0 {
		t.Fatal("tata must not be affected by marking mama paid")
	}
	if tata.Price != 1234.5 {
		t.Fatalf("tata price = %v, want 1234.5", tata.Price)
	}
	if mama.Price != 0 {
		t.Fatalf("mama price = %v, want 0 (untouched)", mama.Price)
	}
}
