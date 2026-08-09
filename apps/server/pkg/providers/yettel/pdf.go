package yettel

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/CerealKiller97/preuzmi.me/pkg/services/pdftext"
)

// Labels on the Yettel invoice, matched against the upper-cased row:
//   - the payable total sits on "UKUPNO ZA PLAĆANJE" (ASCII prefix used to dodge
//     the accented character);
//   - "PRETHODNO STANJE" is the balance carried over from before this invoice —
//     zero means the earlier bills are settled, the same signal eUpravnik reads
//     from "DUG".
const (
	labelTotal       = "UKUPNO ZA PLA"
	labelPrevBalance = "PRETHODNO STANJE"
)

// amountEpsilon absorbs float rounding when testing the previous balance against
// zero.
const amountEpsilon = 0.005

// dateRe matches a Serbian DD.MM.YYYY date, e.g. the "Obračunski period:
// 01.06.2026. - 30.06.2026." range.
var dateRe = regexp.MustCompile(`(\d{2})\.(\d{2})\.(\d{4})`)

// invoice holds the fields parsed out of a Yettel PDF.
type invoice struct {
	total          float64
	prevBalance    float64
	periodMonth    int
	periodYear     int
	hasTotal       bool
	hasPrevBalance bool
	hasPeriod      bool
}

// previousSettled reports whether the balance carried into this invoice is zero,
// i.e. the earlier bills are paid.
func (in invoice) previousSettled() bool {
	return in.hasPrevBalance && in.prevBalance <= amountEpsilon
}

// periodString returns the "MM-YYYY" folder for the invoice's own billing period.
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

// parseInvoice extracts the payable total, billing period, and carried-over
// balance from a Yettel PDF. Text is reconstructed into visual rows first (see
// pdftext.Rows) so a label and its right-aligned amount land on the same row.
func parseInvoice(data []byte) (invoice, error) {
	rows, err := pdftext.Rows(data)
	if err != nil {
		return invoice{}, err
	}

	return invoiceFromRows(rows)
}

// invoiceFromRows pulls the fields out of already-reconstructed rows. Split from
// parseInvoice so the label/amount logic is testable without a real PDF.
func invoiceFromRows(rows []string) (invoice, error) {
	var in invoice

	for _, row := range rows {
		up := strings.ToUpper(row)

		if !in.hasPeriod && strings.Contains(up, "PERIOD") {
			if m, y, ok := periodFromRow(row); ok {
				in.periodMonth, in.periodYear, in.hasPeriod = m, y, true
			}
		}
		if !in.hasTotal && strings.Contains(up, labelTotal) {
			if v, ok := pdftext.LastAmount(row); ok {
				in.total, in.hasTotal = v, true
			}
		}
		if !in.hasPrevBalance && strings.Contains(up, labelPrevBalance) {
			if v, ok := pdftext.LastAmount(row); ok {
				in.prevBalance, in.hasPrevBalance = v, true
			}
		}
	}

	if !in.hasTotal && !in.hasPeriod {
		return invoice{}, fmt.Errorf("yettel: could not find a total or period in the PDF")
	}

	return in, nil
}

// periodFromRow resolves the billing month/year from a period row, preferring the
// numeric "Obračunski period: DD.MM.YYYY" date and falling back to a Serbian
// month name (e.g. "JUN 2026").
func periodFromRow(row string) (month, year int, ok bool) {
	if m := dateRe.FindStringSubmatch(row); m != nil {
		mo, _ := strconv.Atoi(m[2])
		yr, _ := strconv.Atoi(m[3])
		if mo >= 1 && mo <= 12 {
			return mo, yr, true
		}
	}

	return pdftext.ParsePeriod(row)
}
