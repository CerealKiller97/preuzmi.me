package receipts

import (
	"context"
	"testing"
)

// TestNotifyOncePersistsAcrossRuns simulates repeated `checks` cron runs against
// the same on-disk database and asserts a receipt is announced (download) and
// confirmed (payment) at most once — even when a provider rewrites the status on
// every run the way Yettel/eUpravnik do.
func TestNotifyOncePersistsAcrossRuns(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Run 1: first download + first confirmation. Both should be announced.
	s1, err := New(dir)
	if err != nil {
		t.Fatalf("New run1: %v", err)
	}
	if err := s1.Record(ctx, Receipt{Provider: "yettel", Period: "06-2026", StorageKey: "06-2026/yettel.pdf"}); err != nil {
		t.Fatalf("Record run1: %v", err)
	}
	if err := s1.SetStatus(ctx, "yettel", "", "06-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus run1: %v", err)
	}
	if n := len(s1.DrainNewlyDownloaded()); n != 1 {
		t.Fatalf("run1 download: want 1 announcement, got %d", n)
	}
	if v := len(s1.DrainNewlyVerified()); v != 1 {
		t.Fatalf("run1 confirm: want 1 announcement, got %d", v)
	}
	s1.Close() //nolint:errcheck

	// Run 2: fresh process, same DB. The provider re-downloads and — as Yettel
	// does — flips the current bill back to unpaid (clearing confirmed_at) before
	// re-confirming it. Neither must be re-announced.
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New run2: %v", err)
	}
	defer s2.Close() //nolint:errcheck
	if err := s2.Record(ctx, Receipt{Provider: "yettel", Period: "06-2026", StorageKey: "06-2026/yettel.pdf"}); err != nil {
		t.Fatalf("Record run2: %v", err)
	}
	if err := s2.SetStatus(ctx, "yettel", "", "06-2026", StatusUnpaid); err != nil {
		t.Fatalf("SetStatus unpaid run2: %v", err)
	}
	if err := s2.SetStatus(ctx, "yettel", "", "06-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus paid run2: %v", err)
	}
	if n := len(s2.DrainNewlyDownloaded()); n != 0 {
		t.Errorf("run2 download re-announced: got %d, want 0", n)
	}
	if v := len(s2.DrainNewlyVerified()); v != 0 {
		t.Errorf("run2 confirm re-announced: got %d, want 0", v)
	}
}

// TestUpgradeDoesNotReannounceHistory guards the migration backfill: a database
// that already holds a downloaded, confirmed receipt but with the notification
// markers still at 0 (exactly the state right after the columns are ALTERed in)
// must not re-announce that history on the next run.
func TestUpgradeDoesNotReannounceHistory(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s1, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s1.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := s1.SetStatus(ctx, "eps", "", "06-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	s1.DrainNewlyDownloaded()
	s1.DrainNewlyVerified()

	// Force the pre-upgrade state: markers NULL, but downloaded_at/confirmed_at
	// intact — exactly what an older database looks like the instant the nullable
	// columns are ALTERed in.
	if _, err := s1.db.ExecContext(ctx, `UPDATE receipts SET notified_download_at = NULL, notified_confirmed_at = NULL`); err != nil {
		t.Fatalf("clear markers: %v", err)
	}
	s1.Close() //nolint:errcheck

	// Reopen — migrate() runs and must backfill the markers from the timestamps.
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New reopen: %v", err)
	}
	defer s2.Close() //nolint:errcheck
	if err := s2.Record(ctx, Receipt{Provider: "eps", Period: "06-2026", StorageKey: "06-2026/eps.pdf"}); err != nil {
		t.Fatalf("Record after upgrade: %v", err)
	}
	if err := s2.SetStatus(ctx, "eps", "", "06-2026", StatusPaid); err != nil {
		t.Fatalf("SetStatus after upgrade: %v", err)
	}
	if n := len(s2.DrainNewlyDownloaded()); n != 0 {
		t.Errorf("upgrade re-announced download: got %d, want 0", n)
	}
	if v := len(s2.DrainNewlyVerified()); v != 0 {
		t.Errorf("upgrade re-announced confirmation: got %d, want 0", v)
	}
}
