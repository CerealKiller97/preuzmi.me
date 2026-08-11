package http

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/config"
	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
)

// TestScanReceiptsJoinsStatus verifies that the dashboard scan annotates each
// receipt with the provider-reported status (and price) from the database.
func TestScanReceiptsJoinsStatus(t *testing.T) {
	dir := t.TempDir()

	// A PDF on disk so the scan finds a receipt.
	period := filepath.Join(dir, "05-2026")
	if err := os.MkdirAll(period, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(period, "mts.pdf"), []byte("%PDF-1.4 test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A database row carrying status + price for that receipt.
	store, err := receipts.New(dir)
	if err != nil {
		t.Fatalf("receipts.New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()
	if err := store.Record(ctx, receipts.Receipt{Provider: "mts", Period: "05-2026", StorageKey: "05-2026/mts.pdf", SizeBytes: 13}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.SetStatus(ctx, "mts", "", "05-2026", receipts.StatusPaid); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if err := store.SetPrice(ctx, "mts", "", "05-2026", 1819.46); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	cfg := &config.Config{Storage: config.StorageLocal, DownloadPath: dir}
	items, err := scanReceipts(cfg, store)
	if err != nil {
		t.Fatalf("scanReceipts: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(items))
	}
	if items[0].Status != receipts.StatusPaid {
		t.Fatalf("status = %q, want %q", items[0].Status, receipts.StatusPaid)
	}
	if items[0].Amount != 1819.46 {
		t.Fatalf("amount = %v, want 1819.46 (from db price fallback)", items[0].Amount)
	}
}

// TestCollectReceiptsS3ListsFromDB verifies that with S3 storage the dashboard
// lists receipts from the database, not the (empty) local filesystem — the PDFs
// live in the bucket, so an on-disk walk would find nothing.
func TestCollectReceiptsS3ListsFromDB(t *testing.T) {
	dir := t.TempDir() // no PDFs on disk, mimicking an S3 deployment

	store, err := receipts.New(dir)
	if err != nil {
		t.Fatalf("receipts.New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()
	if err := store.Record(ctx, receipts.Receipt{Provider: "a1", Period: "06-2026", StorageKey: "06-2026/a1.pdf", SizeBytes: 42}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.SetPrice(ctx, "a1", "", "06-2026", 499); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	cfg := &config.Config{}
	cfg.Storage = config.StorageS3
	cfg.DownloadPath = dir

	items, err := collectReceipts(cfg, store)
	if err != nil {
		t.Fatalf("collectReceipts: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 receipt from db, got %d", len(items))
	}
	got := items[0]
	if got.Provider != "a1" || got.Period != "06-2026" {
		t.Fatalf("unexpected receipt: %+v", got)
	}
	if got.URL != "/receipt/06-2026/a1" || got.FileName != "a1.pdf" {
		t.Fatalf("URL/filename = %q / %q", got.URL, got.FileName)
	}
	if got.Amount != 499 {
		t.Fatalf("amount = %v, want 499 (db price)", got.Amount)
	}

	// Sanity: the local walk really would have returned nothing here.
	fsItems, err := scanReceipts(cfg, store)
	if err != nil {
		t.Fatalf("scanReceipts: %v", err)
	}
	if len(fsItems) != 0 {
		t.Fatalf("expected 0 receipts from empty disk, got %d", len(fsItems))
	}
}
