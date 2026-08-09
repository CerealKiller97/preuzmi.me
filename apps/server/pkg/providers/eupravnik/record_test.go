package eupravnik

import (
	"context"
	"strings"
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/rs/zerolog"
)

// seedReceipt inserts a bare receipt row so SetPrice/SetStatus (which UPDATE in
// place) have something to update.
func seedReceipt(t *testing.T, store *receipts.Repository, period string) {
	t.Helper()
	err := store.Record(context.Background(), receipts.Receipt{
		Provider:     fileName,
		Period:       period,
		StorageKey:   period + "/" + fileName + ".pdf",
		DownloadedAt: 1,
	})
	if err != nil {
		t.Fatalf("seed %s: %v", period, err)
	}
}

func statusOf(t *testing.T, store *receipts.Repository, period string) string {
	t.Helper()
	list, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// The period column is stored slash-form ("05/2026"); the test seeds and
	// queries dash-form, so compare without regard to the separator.
	want := strings.ReplaceAll(period, "-", "/")
	for _, r := range list {
		if strings.ReplaceAll(r.Period, "-", "/") == want {
			return r.Status
		}
	}
	t.Fatalf("no receipt for period %s", period)
	return ""
}

func TestRecordInvoiceCurrentUnpaidPreviousPaid(t *testing.T) {
	store, err := receipts.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck

	seedReceipt(t, store, "05-2026") // current (Maj)
	seedReceipt(t, store, "04-2026") // previous (April)

	s := New(nil, "", zerolog.Nop(), nil, store)

	// DUG == 0 → current stays unpaid, previous flips to paid.
	in := invoice{
		total: 1912.00, hasTotal: true,
		debt: 0, hasDebt: true,
		periodMonth: 5, periodYear: 2026, hasPeriod: true,
	}
	s.recordInvoice(context.Background(), in, "05-2026", nil)

	if got := statusOf(t, store, "05-2026"); got != receipts.StatusUnpaid {
		t.Errorf("current status = %q, want %q", got, receipts.StatusUnpaid)
	}
	if got := statusOf(t, store, "04-2026"); got != receipts.StatusPaid {
		t.Errorf("previous status = %q, want %q", got, receipts.StatusPaid)
	}
}

func TestRecordInvoiceWithDebtLeavesPreviousUnpaid(t *testing.T) {
	store, err := receipts.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close() //nolint:errcheck

	seedReceipt(t, store, "05-2026")
	seedReceipt(t, store, "04-2026")
	// The previous receipt was recorded unpaid when it was the current one.
	if err := store.SetStatus(context.Background(), fileName, "", "04-2026", receipts.StatusUnpaid); err != nil {
		t.Fatal(err)
	}

	s := New(nil, "", zerolog.Nop(), nil, store)

	// DUG > 0 → previous is not settled; current still recorded as unpaid.
	in := invoice{
		total: 1912.00, hasTotal: true,
		debt: 1912.00, hasDebt: true,
		periodMonth: 5, periodYear: 2026, hasPeriod: true,
	}
	s.recordInvoice(context.Background(), in, "05-2026", nil)

	if got := statusOf(t, store, "05-2026"); got != receipts.StatusUnpaid {
		t.Errorf("current status = %q, want %q", got, receipts.StatusUnpaid)
	}
	if got := statusOf(t, store, "04-2026"); got != receipts.StatusUnpaid {
		t.Errorf("previous status = %q, want unpaid (untouched)", got)
	}
}
