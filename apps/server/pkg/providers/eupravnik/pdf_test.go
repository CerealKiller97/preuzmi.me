package eupravnik

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvoiceFromRows(t *testing.T) {
	// Arrange
	assert := require.New(t)
	// Rows as reconstructed from the real invoice layout.
	rows := []string{
		"Period:Maj 2026",
		"RSD1912.00",
		"DUG:0.00",
		"UKUPNO:1,912.00",
	}

	// Act
	in, err := invoiceFromRows(rows)

	// Assert
	assert.NoError(err)
	assert.True(in.hasTotal)
	assert.InDelta(1912.00, in.total, amountEpsilon)
	assert.True(in.hasDebt)
	assert.InDelta(0, in.debt, amountEpsilon)
	assert.True(in.paid(), "DUG 0.00 means the account is settled")
	assert.True(in.hasPeriod)
	assert.Equal("05-2026", in.periodString())
	assert.Equal("04-2026", in.previousPeriodString())
}

func TestPreviousPeriodStringRollsYear(t *testing.T) {
	// Arrange
	assert := require.New(t)
	in := invoice{periodMonth: 1, periodYear: 2026, hasPeriod: true}

	// Act
	got := in.previousPeriodString()

	// Assert
	assert.Equal("12-2025", got)
}

func TestInvoiceFromRowsUnpaidWhenDebtPositive(t *testing.T) {
	// Arrange
	assert := require.New(t)
	rows := []string{"UKUPNO:1.912,00", "DUG:1.912,00"}

	// Act
	in, err := invoiceFromRows(rows)

	// Assert
	assert.NoError(err)
	assert.InDelta(1912.00, in.total, amountEpsilon)
	assert.False(in.paid(), "a positive DUG means the account is not settled")
}

func TestInvoiceFromRowsErrorsWhenLabelsMissing(t *testing.T) {
	// Arrange
	assert := require.New(t)
	rows := []string{"Period:Maj 2026", "Beograd"}

	// Act
	_, err := invoiceFromRows(rows)

	// Assert
	assert.Error(err, "neither UKUPNO nor DUG is present")
}

func TestInvoicePaidRequiresDebtLine(t *testing.T) {
	// Arrange
	assert := require.New(t)
	// Without a parsed debt line, paid() must be false so status stays untouched.
	in := invoice{hasTotal: true, total: 100}

	// Act
	paid := in.paid()

	// Assert
	assert.False(paid)
}

// TestInvoiceFromRowsGluedPeriod covers invoices whose text layer has no space
// between the month and the year ("Period:oktobar2026"). The October bill is
// issued in November and must still be filed under 10-2026.
func TestInvoiceFromRowsGluedPeriod(t *testing.T) {
	// Arrange
	assert := require.New(t)
	rows := []string{
		"Datum izdavanja:05.11.2026",
		"Datum prometa:31.10.2026",
		"Period:oktobar2026",
		"DUG:0.00",
		"UKUPNO:1,912.00",
		"Račun period: oktobar2026",
	}

	// Act
	in, err := invoiceFromRows(rows)

	// Assert
	assert.NoError(err)
	assert.True(in.hasPeriod)
	assert.Equal("10-2026", in.periodString())
	assert.Equal("09-2026", in.previousPeriodString())
}
