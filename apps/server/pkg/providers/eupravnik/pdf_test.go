package eupravnik

import "testing"

func TestInvoiceFromRows(t *testing.T) {
	// Rows as reconstructed from the real invoice layout.
	rows := []string{
		"Period:Maj 2026",
		"RSD1912.00",
		"DUG:0.00",
		"UKUPNO:1,912.00",
	}

	in, err := invoiceFromRows(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !in.hasTotal || in.total != 1912.00 {
		t.Errorf("total = %v (has=%v), want 1912.00", in.total, in.hasTotal)
	}
	if !in.hasDebt || in.debt != 0 {
		t.Errorf("debt = %v (has=%v), want 0", in.debt, in.hasDebt)
	}
	if !in.paid() {
		t.Error("paid() = false, want true for DUG 0.00")
	}
	if !in.hasPeriod || in.periodString() != "05-2026" {
		t.Errorf("period = %s (has=%v), want 05-2026", in.periodString(), in.hasPeriod)
	}
	if in.previousPeriodString() != "04-2026" {
		t.Errorf("previous = %s, want 04-2026", in.previousPeriodString())
	}
}

func TestPreviousPeriodStringRollsYear(t *testing.T) {
	in := invoice{periodMonth: 1, periodYear: 2026, hasPeriod: true}
	if got := in.previousPeriodString(); got != "12-2025" {
		t.Errorf("previousPeriodString = %s, want 12-2025", got)
	}
}

func TestInvoiceFromRowsUnpaidWhenDebtPositive(t *testing.T) {
	in, err := invoiceFromRows([]string{"UKUPNO:1.912,00", "DUG:1.912,00"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.total != 1912.00 {
		t.Errorf("total = %v, want 1912.00", in.total)
	}
	if in.paid() {
		t.Error("paid() = true, want false when DUG is positive")
	}
}

func TestInvoiceFromRowsErrorsWhenLabelsMissing(t *testing.T) {
	if _, err := invoiceFromRows([]string{"Period:Maj 2026", "Beograd"}); err == nil {
		t.Error("expected an error when neither UKUPNO nor DUG is present")
	}
}

func TestInvoicePaidRequiresDebtLine(t *testing.T) {
	// Without a parsed debt line, paid() must be false so status stays untouched.
	in := invoice{hasTotal: true, total: 100}
	if in.paid() {
		t.Error("paid() = true without a debt line, want false")
	}
}
