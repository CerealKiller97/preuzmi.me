package esanduce

import (
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/services/receipts"
)

func TestPeriodFromGGMM(t *testing.T) {
	cases := map[int]string{
		2606: "06-2026",
		2512: "12-2025",
		2601: "01-2026",
	}
	for ggmm, want := range cases {
		if got := periodFromGGMM(ggmm); got != want {
			t.Errorf("periodFromGGMM(%d) = %q, want %q", ggmm, got, want)
		}
	}

	// An out-of-range month falls back to a non-empty MM-YYYY heuristic.
	if got := periodFromGGMM(2699); len(got) < 6 {
		t.Errorf("periodFromGGMM fallback = %q, want a non-empty MM-YYYY", got)
	}
}

func TestBillStatus(t *testing.T) {
	if got := billStatus(Bill{StatusDuga: "плаћен"}); got != receipts.StatusPaid {
		t.Errorf("billStatus(плаћен) = %q, want %q", got, receipts.StatusPaid)
	}
	// Surrounding whitespace is tolerated.
	if got := billStatus(Bill{StatusDuga: "  плаћен  "}); got != receipts.StatusPaid {
		t.Errorf("billStatus(padded) = %q, want %q", got, receipts.StatusPaid)
	}
	// "неплаћен" contains "плаћен" but must not count as paid.
	if got := billStatus(Bill{StatusDuga: "неплаћен"}); got != receipts.StatusUnpaid {
		t.Errorf("billStatus(неплаћен) = %q, want %q", got, receipts.StatusUnpaid)
	}
	if got := billStatus(Bill{StatusDuga: ""}); got != receipts.StatusUnpaid {
		t.Errorf("billStatus(empty) = %q, want %q", got, receipts.StatusUnpaid)
	}
}
