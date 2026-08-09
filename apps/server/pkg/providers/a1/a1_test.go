package a1

import (
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
)

func TestBillPeriod(t *testing.T) {
	cases := []struct {
		startDate string
		want      string
	}{
		{"2026-06-01T00:00:00+02:00", "06-2026"},
		{"2025-12-01T00:00:00+01:00", "12-2025"},
		{"2026-01-01T00:00:00+01:00", "01-2026"},
	}
	for _, c := range cases {
		got := billPeriod(bill{Fields: billFields{StartDate: c.startDate}})
		if got != c.want {
			t.Errorf("billPeriod(%q) = %q, want %q", c.startDate, got, c.want)
		}
	}

	// An unparseable date falls back to a non-empty MM-YYYY heuristic.
	if got := billPeriod(bill{Fields: billFields{StartDate: "n/a"}}); len(got) < 6 {
		t.Errorf("billPeriod fallback = %q, want a non-empty MM-YYYY", got)
	}
}

func TestBillStatus(t *testing.T) {
	if got := billStatus(bill{Fields: billFields{PaymentStatus: "paid"}}); got != receipts.StatusPaid {
		t.Errorf("billStatus(paid) = %q, want %q", got, receipts.StatusPaid)
	}
	// Case-insensitive.
	if got := billStatus(bill{Fields: billFields{PaymentStatus: "PAID"}}); got != receipts.StatusPaid {
		t.Errorf("billStatus(PAID) = %q, want %q", got, receipts.StatusPaid)
	}
	if got := billStatus(bill{Fields: billFields{PaymentStatus: "unpaid"}}); got != receipts.StatusUnpaid {
		t.Errorf("billStatus(unpaid) = %q, want %q", got, receipts.StatusUnpaid)
	}
	if got := billStatus(bill{Fields: billFields{PaymentStatus: ""}}); got != receipts.StatusUnpaid {
		t.Errorf("billStatus(empty) = %q, want %q", got, receipts.StatusUnpaid)
	}
}
