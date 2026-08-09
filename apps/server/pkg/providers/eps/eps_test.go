package eps

import "testing"

func TestBillSettled(t *testing.T) {
	bill := financialDocument{Amount: 2364.66, IssueDate: "2026-07-07T00:00:00"}

	cases := []struct {
		name     string
		payments []payment
		want     bool
	}{
		{
			name:     "matching amount, paid after issue",
			payments: []payment{{Amount: 2364.66, PaymentDate: "2026-07-09T00:00:00"}},
			want:     true,
		},
		{
			name:     "payment before issue date does not count",
			payments: []payment{{Amount: 2364.66, PaymentDate: "2026-07-06T00:00:00"}},
			want:     false,
		},
		{
			name:     "different amount does not match",
			payments: []payment{{Amount: 1000.00, PaymentDate: "2026-07-09T00:00:00"}},
			want:     false,
		},
		{
			name:     "refund of the same amount is ignored",
			payments: []payment{{Amount: 2364.66, PaymentDate: "2026-07-09T00:00:00", IsRefund: true}},
			want:     false,
		},
		{
			name: "one matching payment among several settles the bill",
			payments: []payment{
				{Amount: 500.00, PaymentDate: "2026-07-08T00:00:00"},
				{Amount: 2364.66, PaymentDate: "2026-07-09T00:00:00"},
			},
			want: true,
		},
		{
			name:     "no payments",
			payments: nil,
			want:     false,
		},
	}

	for _, c := range cases {
		if got := billSettled(bill, c.payments); got != c.want {
			t.Errorf("%s: billSettled = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBillPeriodUsesBillMonthYear(t *testing.T) {
	if got := billPeriod(financialDocument{Month: 6, Year: 2026}); got != "06-2026" {
		t.Errorf("billPeriod = %q, want 06-2026", got)
	}
	// Missing month/year falls back to a non-empty MM-YYYY heuristic.
	if got := billPeriod(financialDocument{}); len(got) < 6 {
		t.Errorf("billPeriod fallback = %q, want a non-empty MM-YYYY", got)
	}
}

func TestBillSettledBadIssueDate(t *testing.T) {
	bill := financialDocument{Amount: 100, IssueDate: "not-a-date"}
	if billSettled(bill, []payment{{Amount: 100, PaymentDate: "2026-07-09T00:00:00"}}) {
		t.Error("expected false when the bill's issue date cannot be parsed")
	}
}
