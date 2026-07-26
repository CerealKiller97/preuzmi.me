package mts

import (
	"testing"

	"github.com/CerealKiller97/preuzmi.me/pkg/dto"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/receipts"
)

func TestBillStatus(t *testing.T) {
	if got := billStatus(dto.Bill{Status: dto.Paid}); got != receipts.StatusPaid {
		t.Errorf("billStatus(PAID) = %q, want %q", got, receipts.StatusPaid)
	}
	if got := billStatus(dto.Bill{Status: dto.NotPaid}); got != receipts.StatusUnpaid {
		t.Errorf("billStatus(NOT_PAID) = %q, want %q", got, receipts.StatusUnpaid)
	}
	if got := billStatus(dto.Bill{Status: ""}); got != receipts.StatusUnpaid {
		t.Errorf("billStatus(empty) = %q, want %q", got, receipts.StatusUnpaid)
	}
}

func TestBillPrice(t *testing.T) {
	// Prefer the numeric total when the response includes it.
	if got := billPrice(dto.Bill{TotalAmount: 1819.46, TotalAmountFormatted: "1.819,46"}); got != 1819.46 {
		t.Errorf("billPrice numeric = %v, want 1819.46", got)
	}
	// Parse the formatted string when the numeric total is absent (raw API shape).
	if got := billPrice(dto.Bill{TotalAmountFormatted: "1.819,46"}); got != 1819.46 {
		t.Errorf("billPrice formatted = %v, want 1819.46", got)
	}
}

func TestParsePrice(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"1.819,46", 1819.46},
		{"1.234,56 RSD", 1234.56},
		{"2.345,67 din.", 2345.67},
		{"999,00", 999},
		{"12,50", 12.5},
		{"0,00", 0},
		{"", 0},
		{"n/a", 0},
	}

	for _, c := range cases {
		if got := parsePrice(c.in); got != c.want {
			t.Errorf("parsePrice(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestLatestBill(t *testing.T) {
	bills := []dto.Bill{
		{Month: 3, Year: 2026, InvoiceNumber: "mar"},
		{Month: 5, Year: 2026, InvoiceNumber: "may"},
		{Month: 12, Year: 2025, InvoiceNumber: "dec-prev-year"},
		{Month: 4, Year: 2026, InvoiceNumber: "apr"},
	}

	if got := latestBill(bills); got.InvoiceNumber != "may" {
		t.Errorf("latestBill = %q, want may (05-2026)", got.InvoiceNumber)
	}

	// Year takes precedence over month.
	across := []dto.Bill{
		{Month: 11, Year: 2025, InvoiceNumber: "nov-2025"},
		{Month: 1, Year: 2026, InvoiceNumber: "jan-2026"},
	}
	if got := latestBill(across); got.InvoiceNumber != "jan-2026" {
		t.Errorf("latestBill = %q, want jan-2026", got.InvoiceNumber)
	}
}

func TestBillPeriod(t *testing.T) {
	if got := billPeriod(dto.Bill{Month: 5, Year: 2026}); got != "05-2026" {
		t.Errorf("billPeriod = %q, want 05-2026", got)
	}
	if got := billPeriod(dto.Bill{Month: 12, Year: 2025}); got != "12-2025" {
		t.Errorf("billPeriod = %q, want 12-2025", got)
	}
	// Missing month/year falls back to the heuristic (non-empty MM-YYYY).
	if got := billPeriod(dto.Bill{}); len(got) < 6 {
		t.Errorf("billPeriod fallback = %q, want a non-empty MM-YYYY", got)
	}
}
