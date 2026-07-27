package pdftext

import "testing"

func TestParseAmount(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"1,912.00", 1912.00, true},          // Anglo: comma thousands, dot decimal
		{"1.912,00", 1912.00, true},          // Serbian: dot thousands, comma decimal
		{"400.00", 400.00, true},             // plain dot decimal
		{"0.00", 0, true},                    // zero
		{"12.345.678,90", 12345678.90, true}, // multiple thousands groups
		{"1234", 1234, true},                 // no separators
		{"0,00", 0, true},                    // comma decimal
		{"", 0, false},                       // empty
		{"n/a", 0, false},                    // not a number
	}

	for _, c := range cases {
		got, ok := ParseAmount(c.in)
		if ok != c.ok {
			t.Errorf("ParseAmount(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("ParseAmount(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestLastAmountTakesRightmostNumber(t *testing.T) {
	// A "LABEL:value" row can contain other numbers; the value is rightmost.
	got, ok := LastAmount("UKUPNO:1,912.00")
	if !ok || got != 1912.00 {
		t.Errorf("LastAmount = %v (ok=%v), want 1912.00", got, ok)
	}
	if _, ok := LastAmount("no numbers here"); ok {
		t.Error("expected ok=false for a row with no number")
	}
}

func TestParsePeriod(t *testing.T) {
	cases := []struct {
		row       string
		wantMonth int
		wantYear  int
		ok        bool
	}{
		{"Račun period: Maj 2026", 5, 2026, true},
		{"Period:Maj 2026", 5, 2026, true},
		{"Period: Januar 2027", 1, 2027, true},
		{"Period: Decembar 2025", 12, 2025, true},
		{"Period: Мај 2026", 5, 2026, true}, // Cyrillic
		{"Period: Maj", 0, 0, false},        // no year
		{"Beograd 11000", 0, 0, false},      // no month
	}

	for _, c := range cases {
		m, y, ok := ParsePeriod(c.row)
		if ok != c.ok {
			t.Errorf("ParsePeriod(%q) ok = %v, want %v", c.row, ok, c.ok)
			continue
		}
		if ok && (m != c.wantMonth || y != c.wantYear) {
			t.Errorf("ParsePeriod(%q) = %d/%d, want %d/%d", c.row, m, y, c.wantMonth, c.wantYear)
		}
	}
}
