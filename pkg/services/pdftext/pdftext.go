// Package pdftext extracts text from invoice PDFs in a layout-aware way.
//
// The raw PDF content stream lists glyphs in draw order, which interleaves
// right-aligned columns (totals, amounts) out of reading order. Rows regroups
// glyphs by their Y coordinate and orders each row left-to-right, recovering the
// "LABEL: value" adjacency that a naive text dump loses. The amount and period
// helpers then parse the Serbian-locale values invoices carry.
package pdftext

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ledongthuc/pdf"
)

// numberRe matches a numeric token with optional grouping and decimal
// separators, e.g. "1,912.00", "1.912,00" or "400.00".
var numberRe = regexp.MustCompile(`\d[\d.,]*\d|\d`)

// yearRe matches a four-digit year.
var yearRe = regexp.MustCompile(`\b(\d{4})\b`)

// serbianMonths maps a lower-cased Serbian month name (Latin and Cyrillic) onto
// its month number, so "Račun period: Maj 2026" resolves to month 5.
var serbianMonths = map[string]int{
	// Latin:
	"januar":    1,
	"februar":   2,
	"mart":      3,
	"april":     4,
	"maj":       5,
	"jun":       6,
	"jul":       7,
	"avgust":    8,
	"septembar": 9,
	"oktobar":   10,
	"novembar":  11,
	"decembar":  12,
	// Cyrillic:
	"јануар":    1,
	"фебруар":   2,
	"март":      3,
	"април":     4,
	"мај":       5,
	"јун":       6,
	"јул":       7,
	"август":    8,
	"септембар": 9,
	"октобар":   10,
	"новембар":  11,
	"децембар":  12,
}

// Rows reconstructs a PDF's text into visual rows: group glyphs by their Y
// coordinate, then order each row left-to-right by X.
//
// It is defensive about malformed input — the underlying reader can panic on a
// broken content stream, which is recovered into an error so a bad attachment
// never takes down the caller.
func Rows(data []byte) (rows []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdftext: panic while parsing PDF: %v", r)
		}
	}()

	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}

	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}

		rows = append(rows, pageRows(p.Content().Text)...)
	}

	return rows, nil
}

// pageRows groups a page's text fragments into visual rows.
func pageRows(texts []pdf.Text) []string {
	// Round Y to the nearest point so fragments on the same baseline share a key.
	buckets := map[int][]pdf.Text{}
	order := make([]int, 0)
	for _, t := range texts {
		key := int(math.Round(t.Y))
		if _, ok := buckets[key]; !ok {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], t)
	}

	// Top of the page (higher Y) first.
	sort.Sort(sort.Reverse(sort.IntSlice(order)))

	rows := make([]string, 0, len(order))
	for _, y := range order {
		line := buckets[y]
		sort.SliceStable(line, func(i, j int) bool { return line[i].X < line[j].X })

		var sb strings.Builder
		for _, t := range line {
			sb.WriteString(t.S)
		}

		if s := strings.TrimSpace(sb.String()); s != "" {
			rows = append(rows, s)
		}
	}

	return rows
}

// LastAmount parses the rightmost numeric token on a row, which for a
// "LABEL: value" row is the value. It returns false when the row has no number.
func LastAmount(row string) (float64, bool) {
	matches := numberRe.FindAllString(row, -1)
	if len(matches) == 0 {
		return 0, false
	}

	return ParseAmount(matches[len(matches)-1])
}

// ParseAmount converts a formatted amount into a float, tolerating both the
// Serbian ("1.912,00") and Anglo ("1,912.00") conventions: whichever of "." or
// "," appears last is treated as the decimal separator and the other as the
// thousands separator.
func ParseAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}

	lastDot := strings.LastIndex(s, ".")
	lastComma := strings.LastIndex(s, ",")

	switch {
	case lastDot >= 0 && lastComma >= 0:
		if lastComma > lastDot {
			// Serbian: "." groups thousands, "," is decimal.
			s = strings.ReplaceAll(s, ".", "")
			s = strings.ReplaceAll(s, ",", ".")
		} else {
			// Anglo: "," groups thousands, "." is decimal.
			s = strings.ReplaceAll(s, ",", "")
		}
	case lastComma >= 0:
		// Only commas present: treat as the decimal separator.
		s = strings.ReplaceAll(s, ",", ".")
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}

	return f, true
}

// ParsePeriod extracts a billing month and year from a "Račun period: Maj 2026"
// style row, matching a Serbian month name (Latin or Cyrillic) and a four-digit
// year.
func ParsePeriod(row string) (month, year int, ok bool) {
	lower := strings.ToLower(row)

	for name, m := range serbianMonths {
		if !containsWord(lower, name) {
			continue
		}

		y := yearRe.FindString(row)
		if y == "" {
			continue
		}

		parsed, err := strconv.Atoi(y)
		if err != nil {
			continue
		}

		return m, parsed, true
	}

	return 0, 0, false
}

// containsWord reports whether name appears in s bounded by non-letters, so the
// short month "maj" does not match inside a longer word.
func containsWord(s, name string) bool {
	i := strings.Index(s, name)
	for i >= 0 {
		before := i == 0 || !isLetter(rune(s[i-1]))
		afterIdx := i + len(name)
		after := afterIdx >= len(s) || !isLetter(rune(s[afterIdx]))
		if before && after {
			return true
		}

		next := strings.Index(s[i+1:], name)
		if next < 0 {
			break
		}
		i += 1 + next
	}

	return false
}

// isLetter reports whether b is an ASCII letter. Serbian month names are matched
// lower-cased; multibyte Cyrillic bytes are treated as non-letters here, which is
// safe because the month tokens are delimited by ASCII spaces and punctuation in
// the PDF.
func isLetter(b rune) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
