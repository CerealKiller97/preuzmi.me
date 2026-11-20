package pdftext

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseAmount(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   float64
		wantOK bool
	}{
		{"anglo comma thousands dot decimal", "1,912.00", 1912.00, true},
		{"serbian dot thousands comma decimal", "1.912,00", 1912.00, true},
		{"plain dot decimal", "400.00", 400.00, true},
		{"zero", "0.00", 0, true},
		{"multiple thousands groups", "12.345.678,90", 12345678.90, true},
		{"no separators", "1234", 1234, true},
		{"comma decimal", "0,00", 0, true},
		{"empty", "", 0, false},
		{"not a number", "n/a", 0, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Arrange
			assert := require.New(t)

			// Act
			got, ok := ParseAmount(c.in)

			// Assert
			assert.Equal(c.wantOK, ok)
			if c.wantOK {
				assert.InDelta(c.want, got, 0.001)
			}
		})
	}
}

func TestLastAmountTakesRightmostNumber(t *testing.T) {
	// Arrange
	assert := require.New(t)
	// A "LABEL:value" row can contain other numbers; the value is rightmost.
	row := "UKUPNO:1,912.00"

	// Act
	got, ok := LastAmount(row)

	// Assert
	assert.True(ok)
	assert.InDelta(1912.00, got, 0.001)
}

func TestLastAmountWithoutNumber(t *testing.T) {
	// Arrange
	assert := require.New(t)
	row := "no numbers here"

	// Act
	_, ok := LastAmount(row)

	// Assert
	assert.False(ok)
}

func TestParseDate(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   time.Time
		wantOK bool
	}{
		{"trailing period", "15.07.2026.", time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local), true},
		{"latin label", "Datum valute:23.06.2026", time.Date(2026, 6, 23, 0, 0, 0, 0, time.Local), true},
		{"cyrillic label", "Датум доспећа31.07.2026. године", time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local), true},
		{"invalid day", "31.02.2026", time.Time{}, false},
		{"no date", "no date here", time.Time{}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Arrange
			assert := require.New(t)

			// Act
			got, ok := ParseDate(c.in)

			// Assert
			assert.Equal(c.wantOK, ok)
			if c.wantOK {
				assert.True(c.want.Equal(got), "got %v, want %v", got, c.want)
			}
		})
	}
}

func TestFindDueDate(t *testing.T) {
	cases := []struct {
		name   string
		rows   []string
		want   time.Time
		wantOK bool
	}{
		{
			name:   "same row latin valute",
			rows:   []string{"Datum valute:23.06.2026", "other"},
			want:   time.Date(2026, 6, 23, 0, 0, 0, 0, time.Local),
			wantOK: true,
		},
		{
			name:   "same row cyrillic dospeća",
			rows:   []string{"Датум доспећа31.07.2026. године"},
			want:   time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local),
			wantOK: true,
		},
		{
			name:   "next-row rok za plaćanje",
			rows:   []string{"Rok za plaćanje:", "15.07.2026.", "Adresa"},
			want:   time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local),
			wantOK: true,
		},
		{
			name:   "cyrillic rok za plaćanje",
			rows:   []string{"Рок за плаћање: 28.07.2026."},
			want:   time.Date(2026, 7, 28, 0, 0, 0, 0, time.Local),
			wantOK: true,
		},
		{
			name:   "rejects prigovor deadline",
			rows:   []string{"Rok za prigovor: 14.08.2026. god."},
			wantOK: false,
		},
		{
			name:   "no due label",
			rows:   []string{"Datum izdavanja: 01.07.2026.", "Beograd"},
			wantOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Arrange
			assert := require.New(t)

			// Act
			got, ok := FindDueDate(c.rows)

			// Assert
			assert.Equal(c.wantOK, ok)
			if c.wantOK {
				assert.True(c.want.Equal(got), "got %v, want %v", got, c.want)
			}
		})
	}
}

func TestDueDateFromTestdataPDFs(t *testing.T) {
	cases := []struct {
		name string
		file string
		want time.Time
	}{
		{"mts", "../../services/ipsqr/testdata/mts.pdf", time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local)},
		{"esanduce", "../../services/ipsqr/testdata/esanduce.pdf", time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local)},
		{"eupravnik", "../../services/ipsqr/testdata/eupravnik.pdf", time.Date(2026, 6, 23, 0, 0, 0, 0, time.Local)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Arrange
			assert := require.New(t)
			data, err := os.ReadFile(c.file)
			assert.NoError(err)

			// Act
			got, ok := DueDate(data)

			// Assert
			assert.True(ok)
			assert.True(c.want.Equal(got), "got %v, want %v", got, c.want)
		})
	}
}

func TestParsePeriod(t *testing.T) {
	cases := []struct {
		name      string
		row       string
		wantMonth int
		wantYear  int
		wantOK    bool
	}{
		{"label with space", "Račun period: Maj 2026", 5, 2026, true},
		{"no space after colon", "Period:Maj 2026", 5, 2026, true},
		{"january", "Period: Januar 2027", 1, 2027, true},
		{"december", "Period: Decembar 2025", 12, 2025, true},
		{"cyrillic", "Period: Мај 2026", 5, 2026, true},
		{"month glued to year", "Period:oktobar2026", 10, 2026, true},
		{"payment slip month glued to year", "Račun period: oktobar2026", 10, 2026, true},
		{"no year", "Period: Maj", 0, 0, false},
		{"no month", "Beograd 11000", 0, 0, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Arrange
			assert := require.New(t)

			// Act
			month, year, ok := ParsePeriod(c.row)

			// Assert
			assert.Equal(c.wantOK, ok)
			assert.Equal(c.wantMonth, month)
			assert.Equal(c.wantYear, year)
		})
	}
}
