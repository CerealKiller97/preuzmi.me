package receipts

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestRecordAndList(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf", SizeBytes: 100}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(ctx, Receipt{Provider: "mts", Period: "07-2026", StorageKey: "07-2026/mts.pdf", SizeBytes: 200}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 receipts, got %d", len(got))
	}

	// Re-recording the same (provider, period) must upsert, not duplicate.
	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf", SizeBytes: 999}); err != nil {
		t.Fatalf("Record upsert: %v", err)
	}

	got, err = store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 receipts after upsert, got %d", len(got))
	}

	var epsSize int64
	for _, r := range got {
		if r.Provider == "eps" {
			epsSize = r.SizeBytes
		}
	}
	if epsSize != 999 {
		t.Fatalf("expected upserted size 999, got %d", epsSize)
	}
}

func TestDrainNewlyDownloaded(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	// First download of each (provider, period) queues it as newly downloaded.
	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(ctx, Receipt{Provider: "mts", Period: "06-2026", StorageKey: "06-2026/mts.pdf"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	newly := store.DrainNewlyDownloaded()
	if len(newly) != 2 {
		t.Fatalf("expected 2 newly downloaded, got %d", len(newly))
	}

	// Draining clears the queue.
	if again := store.DrainNewlyDownloaded(); len(again) != 0 {
		t.Fatalf("expected drain to clear the queue, got %d", len(again))
	}

	// Re-downloading an already-recorded receipt must NOT re-queue it — this is
	// what keeps a daily cron from re-notifying about the same bill.
	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf"}); err != nil {
		t.Fatalf("Record re-download: %v", err)
	}
	if again := store.DrainNewlyDownloaded(); len(again) != 0 {
		t.Fatalf("re-download must not queue as new, got %d", len(again))
	}
}

func TestPriceAndPaid(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf", SizeBytes: 100}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if err := store.SetPrice(ctx, "eps", "06-2026", 4212.55); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	if _, err := store.MarkPaid(ctx, "eps", "06-2026", true); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}

	// A subsequent re-download must not wipe the price or the paid_at stamp.
	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf", SizeBytes: 250}); err != nil {
		t.Fatalf("Record re-download: %v", err)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 receipt, got %d", len(got))
	}
	r := got[0]
	if r.Price != 4212.55 {
		t.Fatalf("price = %v, want 4212.55 (re-download must not clobber)", r.Price)
	}
	if r.SizeBytes != 250 {
		t.Fatalf("size = %d, want 250 (re-download should refresh)", r.SizeBytes)
	}
	if r.PaidAt == 0 {
		t.Fatalf("paid_at lost after re-download")
	}
}

func TestSetStatus(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	if err := store.Record(ctx, Receipt{Provider: "mts", Period: "05-2026", StorageKey: "05-2026/mts.pdf", SizeBytes: 100}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.SetStatus(ctx, "mts", "05-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	// A re-download must not wipe the status.
	if err := store.Record(ctx, Receipt{Provider: "mts", Period: "05-2026", StorageKey: "05-2026/mts.pdf", SizeBytes: 200}); err != nil {
		t.Fatalf("Record re-download: %v", err)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Status != StatusPaid {
		t.Fatalf("status = %q, want %q (preserved across re-download)", got[0].Status, StatusPaid)
	}
}

func TestNewlyVerifiedTransition(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()
	if err := store.Record(ctx, Receipt{Provider: "mts", Period: "05-2026", StorageKey: "05-2026/mts.pdf"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.SetPrice(ctx, "mts", "05-2026", 1819.46); err != nil {
		t.Fatalf("SetPrice: %v", err)
	}

	// First set to unpaid — not a verification.
	if err := store.SetStatus(ctx, "mts", "05-2026", StatusUnpaid); err != nil {
		t.Fatalf("SetStatus unpaid: %v", err)
	}
	if got := store.DrainNewlyVerified(); len(got) != 0 {
		t.Fatalf("unpaid should not queue a verification, got %d", len(got))
	}

	// Unpaid -> paid is the verification moment.
	if err := store.SetStatus(ctx, "mts", "05-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus paid: %v", err)
	}
	verified := store.DrainNewlyVerified()
	if len(verified) != 1 {
		t.Fatalf("expected 1 newly verified, got %d", len(verified))
	}
	if verified[0].Provider != "mts" || verified[0].Period != "05/2026" || verified[0].Price != 1819.46 {
		t.Fatalf("unexpected verified receipt: %+v", verified[0])
	}
	// Draining clears the queue.
	if got := store.DrainNewlyVerified(); len(got) != 0 {
		t.Fatalf("drain should have cleared the queue, got %d", len(got))
	}

	// Paid -> paid again is not a new verification.
	if err := store.SetStatus(ctx, "mts", "05-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus paid again: %v", err)
	}
	if got := store.DrainNewlyVerified(); len(got) != 0 {
		t.Fatalf("re-confirming paid should not queue a verification, got %d", len(got))
	}
}

// A later invoice can report the previous month settled before that month was
// ever downloaded (e.g. the first run of an email provider). Marking a receipt
// we never held as paid must NOT fire a confirmation.
func TestSetStatusPaidOnMissingReceiptIsNotVerified(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	// No Record for this (provider, period): the row does not exist.
	if err := store.SetStatus(ctx, "yettel", "05-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	if got := store.DrainNewlyVerified(); len(got) != 0 {
		t.Fatalf("confirming a never-downloaded receipt must not queue a verification, got %d", len(got))
	}
}

func TestEmptyToPaidIsVerified(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()
	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	// A receipt whose status was never set (empty) becoming paid counts as a
	// verification — providers that add the status field later must still notify.
	if err := store.SetStatus(ctx, "eps", "06-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if got := store.DrainNewlyVerified(); len(got) != 1 {
		t.Fatalf("empty -> paid should queue a verification, got %d", len(got))
	}
}

func TestMarkPaidCreatesStubWhenUnrecorded(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	// No prior Record: marking paid must still create a stub row carrying paid_at.
	at, err := store.MarkPaid(ctx, "eps", "06-2026", true)
	if err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if at == 0 {
		t.Fatalf("MarkPaid returned zero timestamp")
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 stub receipt, got %d", len(got))
	}
	if got[0].PaidAt != at {
		t.Fatalf("paid_at = %d, want %d", got[0].PaidAt, at)
	}
	if got[0].StorageKey != "06-2026/eps.pdf" {
		t.Fatalf("storage_key = %q, want 06-2026/eps.pdf", got[0].StorageKey)
	}

	// A later download fills in the file facts without clearing paid_at.
	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf", SizeBytes: 512}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	got, _ = store.List(ctx)
	if got[0].PaidAt != at {
		t.Fatalf("paid_at cleared by later Record: got %d", got[0].PaidAt)
	}
	if got[0].SizeBytes != 512 {
		t.Fatalf("size not filled by later Record: got %d", got[0].SizeBytes)
	}

	// Unmarking clears paid_at.
	if _, err := store.MarkPaid(ctx, "eps", "06-2026", false); err != nil {
		t.Fatalf("MarkPaid unmark: %v", err)
	}
	got, _ = store.List(ctx)
	if got[0].PaidAt != 0 {
		t.Fatalf("paid_at = %d, want 0 after unmark", got[0].PaidAt)
	}
}

func TestHasDownloaded(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	// Never recorded: nothing to skip.
	if store.HasDownloaded(ctx, "eps", "06-2026") {
		t.Fatal("HasDownloaded true for an unrecorded receipt")
	}

	// A paid-only stub has no real download time, so it must not count as
	// downloaded — the PDF still needs fetching.
	if _, err := store.MarkPaid(ctx, "eps", "06-2026", true); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if store.HasDownloaded(ctx, "eps", "06-2026") {
		t.Fatal("HasDownloaded true for a paid-only stub (downloaded_at 0)")
	}

	// An actual download counts.
	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf", SizeBytes: 100}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !store.HasDownloaded(ctx, "eps", "06-2026") {
		t.Fatal("HasDownloaded false after a real download")
	}

	// The check is scoped to the exact (provider, period): a different period is
	// still unfetched.
	if store.HasDownloaded(ctx, "eps", "07-2026") {
		t.Fatal("HasDownloaded true for a period that was never downloaded")
	}
}

func TestIsSettled(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	// Never recorded.
	if store.IsSettled(ctx, "eps", "06-2026") {
		t.Fatal("IsSettled true for an unrecorded receipt")
	}

	// Downloaded but unpaid and unmarked — still worth re-checking.
	if err := store.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf", SizeBytes: 100}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if store.IsSettled(ctx, "eps", "06-2026") {
		t.Fatal("IsSettled true for a downloaded unpaid receipt")
	}

	// Provider confirms paid (status + confirmed_at), but paid_at is still 0.
	if err := store.SetStatus(ctx, "eps", "06-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if store.IsSettled(ctx, "eps", "06-2026") {
		t.Fatal("IsSettled true without paid_at")
	}

	// User marks paid too — now settled (paid_at != 0, status plaćeno, confirmed_at != 0).
	if _, err := store.MarkPaid(ctx, "eps", "06-2026", true); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if !store.IsSettled(ctx, "eps", "06-2026") {
		t.Fatal("IsSettled false after paid_at + status paid + confirmed_at")
	}

	// paid_at alone is not enough — status/confirmed_at must also be set.
	if _, err := store.MarkPaid(ctx, "a1", "06-2026", true); err != nil {
		t.Fatalf("MarkPaid stub: %v", err)
	}
	if store.IsSettled(ctx, "a1", "06-2026") {
		t.Fatal("IsSettled true with only paid_at")
	}
	if err := store.SetStatus(ctx, "a1", "06-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus stub: %v", err)
	}
	if !store.IsSettled(ctx, "a1", "06-2026") {
		t.Fatal("IsSettled false after paid_at + status paid + confirmed_at on stub")
	}
}

func TestParseKey(t *testing.T) {
	provider, period := parseKey("07-2026/eps.pdf")
	if provider != "eps" || period != "07-2026" {
		t.Fatalf("parseKey = (%q, %q), want (eps, 07-2026)", provider, period)
	}
}

// TestMigrateAddsIPSQRColumnsFromV110 proves a receipts.db created by v1.1.0
// (no ips_qr / ips_checked) upgrades cleanly on open via database.Apply reading
// schema.sql: existing rows survive and the new columns are usable for the
// payment-QR cache.
func TestMigrateAddsIPSQRColumnsFromV110(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)

	// Build a v1.1.0-shaped database without going through New/migrate.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	const legacySchema = `
CREATE TABLE receipts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    provider      TEXT    NOT NULL,
    period        TEXT    NOT NULL,
    storage_key   TEXT    NOT NULL,
    size_bytes    INTEGER NOT NULL,
    price         REAL    NOT NULL DEFAULT 0.00,
    status        TEXT    NOT NULL DEFAULT 'neplaćeno',
    downloaded_at INTEGER NOT NULL,
    paid_at       INTEGER NOT NULL DEFAULT 0,
    confirmed_at  INTEGER NOT NULL DEFAULT 0,
    UNIQUE (provider, period)
);`
	if _, err := db.Exec(legacySchema); err != nil {
		_ = db.Close()
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO receipts (provider, period, storage_key, size_bytes, price, status, downloaded_at, paid_at, confirmed_at)
VALUES ('eps', '06/2026', '06-2026/eps.pdf', 1234, 2365.0, 'neplaćeno', 1720000000, 0, 0)`); err != nil {
		_ = db.Close()
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	store, err := New(dir)
	if err != nil {
		t.Fatalf("New (upgrade): %v", err)
	}
	defer store.Close() //nolint:errcheck

	ctx := context.Background()

	payload, checked, err := store.IPSQR(ctx, "eps", "06/2026")
	if err != nil {
		t.Fatalf("IPSQR after upgrade: %v", err)
	}
	if payload != "" || checked {
		t.Fatalf("IPSQR = (%q, %v), want empty/unchecked defaults (columns present)", payload, checked)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List len = %d, want 1 (legacy row preserved)", len(got))
	}
	if got[0].Provider != "eps" || got[0].Period != "06/2026" || got[0].Price != 2365.0 {
		t.Fatalf("legacy row mutated: %+v", got[0])
	}

	const wantPayload = "K:PR|V:01|C:1|R:160000000000000000|N:EPS|I:RSD2365,00|RO:97123"
	if err := store.SetIPSQR(ctx, "eps", "06/2026", wantPayload); err != nil {
		t.Fatalf("SetIPSQR: %v", err)
	}
	payload, checked, err = store.IPSQR(ctx, "eps", "06/2026")
	if err != nil {
		t.Fatalf("IPSQR after set: %v", err)
	}
	if !checked || payload != wantPayload {
		t.Fatalf("IPSQR = (%q, %v), want (%q, true)", payload, checked, wantPayload)
	}

	// Re-open: migration must stay idempotent and keep the cached payload.
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	store, err = New(dir)
	if err != nil {
		t.Fatalf("New (re-open): %v", err)
	}
	defer store.Close() //nolint:errcheck

	payload, checked, err = store.IPSQR(ctx, "eps", "06/2026")
	if err != nil {
		t.Fatalf("IPSQR after re-open: %v", err)
	}
	if !checked || payload != wantPayload {
		t.Fatalf("IPSQR after re-open = (%q, %v), want (%q, true)", payload, checked, wantPayload)
	}
}
