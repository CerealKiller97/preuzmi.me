package eupravnik

import (
	"fmt"
	"strings"

	"github.com/CerealKiller97/preuzmi.me/pkg/services/pdftext"
)

// eUpravnik invoices carry the payable total on the "UKUPNO:" line and the
// outstanding balance on the "DUG:" line. A zero outstanding balance means the
// account is settled, which is what we record as paid.
const (
	labelTotal  = "UKUPNO"
	labelDebt   = "DUG"
	labelPeriod = "PERIOD"
)

// amountEpsilon absorbs float rounding when testing the debt against zero.
const amountEpsilon = 0.005

// invoice holds the fields parsed out of an eUpravnik PDF.
type invoice struct {
	total       float64
	debt        float64
	periodMonth int
	periodYear  int
	hasTotal    bool
	hasDebt     bool
	hasPeriod   bool
}

// paid reports whether the account is settled: a parsed debt at (or below) zero.
func (in invoice) paid() bool {
	return in.hasDebt && in.debt <= amountEpsilon
}

// periodString returns the "MM-YYYY" folder for the invoice's own billing
// period (e.g. "05-2026" for "Maj 2026").
func (in invoice) periodString() string {
	return fmt.Sprintf("%02d-%d", in.periodMonth, in.periodYear)
}

// previousPeriodString returns the "MM-YYYY" folder for the month before the
// invoice's period, rolling the year back across January.
func (in invoice) previousPeriodString() string {
	month, year := in.periodMonth-1, in.periodYear
	if month < 1 {
		month, year = 12, year-1
	}

	return fmt.Sprintf("%02d-%d", month, year)
}

// parseInvoice extracts the payable total and outstanding debt from an eUpravnik
// PDF. Text is reconstructed into visual rows first, because the raw content
// stream interleaves the right-aligned totals column out of order.
func parseInvoice(data []byte) (invoice, error) {
	rows, err := pdftext.Rows(data)
	if err != nil {
		return invoice{}, err
	}

	return invoiceFromRows(rows)
}

// invoiceFromRows pulls the total and debt out of already-reconstructed text
// rows. It is split from parseInvoice so the label/amount logic can be tested
// without a real PDF.
func invoiceFromRows(rows []string) (invoice, error) {
	var in invoice
	for _, row := range rows {
		switch {
		case matchable(row, labelTotal):
			if v, ok := pdftext.LastAmount(row); ok {
				in.total, in.hasTotal = v, true
			}
		case matchable(row, labelDebt):
			if v, ok := pdftext.LastAmount(row); ok {
				in.debt, in.hasDebt = v, true
			}
		case !in.hasPeriod && strings.Contains(strings.ToUpper(row), labelPeriod):
			if m, y, ok := pdftext.ParsePeriod(row); ok {
				in.periodMonth, in.periodYear, in.hasPeriod = m, y, true
			}
		}
	}

	if !in.hasTotal && !in.hasDebt {
		return invoice{}, fmt.Errorf("eupravnik: could not find UKUPNO or DUG in the PDF")
	}

	return in, nil
}

// matchable reports whether row is the labelled row we want. UKUPNO and DUG are
// distinct prefixes, so a simple contains check on the upper-cased row is enough
// while ignoring the customer-name text that shares the DUG row's Y band in some
// layouts.
func matchable(row, label string) bool {
	return strings.Contains(strings.ToUpper(row), label+":")
}
