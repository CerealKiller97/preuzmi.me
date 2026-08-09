package pdftext

import (
	"os"
	"testing"
	"time"
)

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

func TestParseDate(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
		ok   bool
	}{
		{"15.07.2026.", time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local), true},
		{"Datum valute:23.06.2026", time.Date(2026, 6, 23, 0, 0, 0, 0, time.Local), true},
		{"Датум доспећа31.07.2026. године", time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local), true},
		{"31.02.2026", time.Time{}, false}, // invalid day
		{"no date here", time.Time{}, false},
	}

	for _, c := range cases {
		got, ok := ParseDate(c.in)
		if ok != c.ok {
			t.Errorf("ParseDate(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && !got.Equal(c.want) {
			t.Errorf("ParseDate(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFindDueDate(t *testing.T) {
	cases := []struct {
		name string
		rows []string
		want time.Time
		ok   bool
	}{
		{
			name: "same row latin valute",
			rows: []string{"Datum valute:23.06.2026", "other"},
			want: time.Date(2026, 6, 23, 0, 0, 0, 0, time.Local),
			ok:   true,
		},
		{
			name: "same row cyrillic dospeća",
			rows: []string{"Датум доспећа31.07.2026. године"},
			want: time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local),
			ok:   true,
		},
		{
			name: "next-row rok za plaćanje",
			rows: []string{"Rok za plaćanje:", "15.07.2026.", "Adresa"},
			want: time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local),
			ok:   true,
		},
		{
			name: "cyrillic rok za plaćanje",
			rows: []string{"Рок за плаћање: 28.07.2026."},
			want: time.Date(2026, 7, 28, 0, 0, 0, 0, time.Local),
			ok:   true,
		},
		{
			name: "rejects prigovor deadline",
			rows: []string{"Rok za prigovor: 14.08.2026. god."},
			ok:   false,
		},
		{
			name: "no due label",
			rows: []string{"Datum izdavanja: 01.07.2026.", "Beograd"},
			ok:   false,
		},
	}

	for _, c := range cases {
		got, ok := FindDueDate(c.rows)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if ok && !got.Equal(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDueDateFromTestdataPDFs(t *testing.T) {
	cases := []struct {
		file string
		want time.Time
	}{
		{"../../services/ipsqr/testdata/mts.pdf", time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)},
		{"../../services/ipsqr/testdata/esanduce.pdf", time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local)},
		{"../../services/ipsqr/testdata/eupravnik.pdf", time.Date(2026, 6, 23, 0, 0, 0, 0, time.Local)},
	}

	for _, c := range cases {
		data, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatalf("read %s: %v", c.file, err)
		}
		got, ok := DueDate(data)
		if !ok {
			t.Errorf("%s: DueDate ok=false", c.file)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("%s: DueDate = %v, want %v", c.file, got, c.want)
		}
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
