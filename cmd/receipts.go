package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	handlers "github.com/CerealKiller97/preuzmi.me/pkg/http"
	receiptsrepo "github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/mattn/go-isatty"
	"github.com/rs/zerolog/log"
)

// ANSI SGR codes for the coloured CLI output.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
)

// painter colours strings when output is a terminal. The zero value (on:false)
// is a plain, colour-free painter, so callers can always call its methods.
type painter struct{ on bool }

// newPainter decides whether to emit colour: never when NO_COLOR is set, always
// when FORCE_COLOR is set (handy for piping into a pager), otherwise only when
// stdout is a real terminal.
func newPainter() painter {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return painter{on: false}
	}
	if _, ok := os.LookupEnv("FORCE_COLOR"); ok {
		return painter{on: true}
	}

	fd := os.Stdout.Fd()

	return painter{on: isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)}
}

// c colours a string with the given SGR code, or returns it unchanged when
// colour is off.
func (p painter) c(code, s string) string {
	if !p.on {
		return s
	}

	return code + s + ansiReset
}

// dispWidth is the number of terminal columns s occupies. ANSI escape sequences
// count as zero, the emojis this CLI prints (🟢 🔴 ⚪ ✅ ❌) count as two, and
// variation selectors / combining marks count as zero. It is deliberately small
// rather than a full Unicode width table: the table only ever renders those few
// emojis plus ASCII and Latin text.
func dispWidth(s string) int {
	w := 0
	inEscape := false
	for _, r := range s {
		switch {
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		case r == 0x1b: // ESC — start of an ANSI SGR sequence
			inEscape = true
		case r == 0xFE0F || (r >= 0x0300 && r <= 0x036F): // variation selector / combining marks
		case r >= 0x1F000 || (r >= 0x2600 && r <= 0x27BF): // emoji / pictographs
			w += 2
		default:
			w++
		}
	}

	return w
}

// Receipts is the `receipts` command: a small CLI over the same receipt state
// the web dashboard shows (the receipts database, meta.json amounts and the
// payments store). It dispatches on the sub-command in os.Args[2].
//
//	receipts list
//	receipts mark:as-paid   <provider>/<period>
//	receipts mark:as-unpaid <provider>/<period>
func Receipts(c *container.Container) {
	sub := ""
	if len(os.Args) > 2 {
		sub = os.Args[2]
	}

	switch sub {
	case "list", "ls":
		receiptsList(c, os.Args[3:])
	case "mark:as-paid":
		receiptsMark(c, os.Args[3:], true)
	case "mark:as-unpaid":
		receiptsMark(c, os.Args[3:], false)
	case "", "help", "-h", "--help":
		receiptsUsage()
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown receipts sub-command %q\n\n", sub)
		receiptsUsage()
		os.Exit(1)
	}
}

// receiptsUsage prints the receipts sub-command help.
func receiptsUsage() {
	name := os.Args[0]
	fmt.Printf("Usage of %s receipts [SUBCOMMAND]\n\n"+
		"Subcommands:\n"+
		"  list [MM/YYYY]\n"+
		"    List receipts and their paid state for a period.\n"+
		"    Defaults to the previous month; pass a period to override, e.g. 07/2026\n"+
		"  mark:as-paid <key>\n"+
		"    Mark the receipt with the given key as paid\n"+
		"  mark:as-unpaid <key>\n"+
		"    Mark the receipt with the given key as not paid\n\n"+
		"A key is the PROVIDER/PERIOD shown by `list`, e.g. mts/10-2025.\n"+
		"You may also pass the two parts as separate arguments: mts 10-2025.\n",
		name,
	)
}

// resolveListPeriod returns the normalized "MM-YYYY" period the list should
// show. With no argument it is the previous calendar month — the bill you would
// normally be settling now. Otherwise the argument is parsed from the MM/YYYY
// form (a dash separator is accepted too).
func resolveListPeriod(args []string) (string, error) {
	if len(args) == 0 {
		now := time.Now()
		y, m := now.Year(), int(now.Month())
		if m--; m == 0 {
			m, y = 12, y-1
		}

		return fmt.Sprintf("%02d-%d", m, y), nil
	}

	raw := strings.ReplaceAll(args[0], "/", "-")
	parts := strings.Split(raw, "-")
	if len(parts) == 2 {
		m, errM := strconv.Atoi(parts[0])
		y, errY := strconv.Atoi(parts[1])
		if errM == nil && errY == nil && m >= 1 && m <= 12 && y >= 1000 && y <= 9999 {
			return fmt.Sprintf("%02d-%d", m, y), nil
		}
	}

	return "", fmt.Errorf("invalid period %q, expected MM/YYYY (e.g. 07/2026)", args[0])
}

// receiptsList prints the recorded receipts for a period and their paid state.
//
// With no argument it shows the previous month (the bill you would normally be
// settling now); an optional MM/YYYY argument selects another period.
//
// Paid state is read from the payments table in the receipts database — the
// single source of truth shared with the web dashboard.
func receiptsList(c *container.Container, args []string) {
	period, err := resolveListPeriod(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", err.Error())
		receiptsUsage()
		os.Exit(1)
	}

	cfg := c.GetConfig()

	// CollectReceipts supplies the same view the dashboard renders: the receipt
	// list, amounts, provider status and paid state (from the payments table).
	all, err := handlers.CollectReceipts(cfg, c.GetReceiptsStore())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to collect receipts")
	}

	// Keep only the requested period; CollectReceipts already normalizes each
	// receipt's period to MM-YYYY, matching resolveListPeriod.
	items := make([]handlers.APIReceipt, 0, len(all))
	for _, it := range all {
		if it.Period == period {
			items = append(items, it)
		}
	}

	p := newPainter()

	// Show the period back in the MM/YYYY form the user types.
	periodLabel := strings.ReplaceAll(period, "-", "/")

	if len(items) == 0 {
		fmt.Printf("🧾 Nema računa za %s.\n", periodLabel)
		return
	}

	fmt.Println(p.c(ansiBold, "🧾 Računi · "+periodLabel))
	fmt.Println()

	// A cell keeps the visible text apart from its coloured form so columns can
	// be padded by what actually shows on screen — ANSI codes are invisible and
	// tabwriter has no way to exclude them, so the layout is done by hand here.
	type cell struct{ text, colored string }
	plain := func(s string) cell {
		return cell{
			text:    s,
			colored: s,
		}
	}

	rows := [][]cell{
		{
			plain("PROVAJDER"),
			plain("PERIOD"),
			plain("IZNOS"),
			plain("STATUS"),
			plain("VERIFIKOVANO"),
			plain("PLAĆENO"),
		},
	}

	// dateCell renders a timestamp column: a green date with a ✅ when set, or a
	// dim ❌ — when not.
	dateCell := func(at int64) cell {
		if at <= 0 {
			return cell{"❌ —", "❌ " + p.c(ansiDim, "—")}
		}
		when := time.Unix(at, 0).Format("02.01.2006")
		return cell{"✅ " + when, "✅ " + p.c(ansiGreen, when)}
	}

	// statusCell renders the provider-reported label (plaćeno / neplaćeno).
	statusCell := func(s string) cell {
		switch s {
		case receiptsrepo.StatusPaid:
			return cell{"🟢 " + s, "🟢 " + p.c(ansiGreen, s)}
		case receiptsrepo.StatusUnpaid:
			return cell{"🔴 " + s, "🔴 " + p.c(ansiYellow, s)}
		default:
			return cell{"⚪ nepoznato", "⚪ " + p.c(ansiDim, "nepoznato")}
		}
	}

	paidCount := 0
	for _, it := range items {
		amount := "-"
		if it.Amount != 0 {
			if it.Currency != "" {
				amount = fmt.Sprintf("%.2f %s", it.Amount, it.Currency)
			} else {
				amount = fmt.Sprintf("%.2f", it.Amount)
			}
		}

		if it.Paid {
			paidCount++
		}

		rows = append(rows, []cell{
			{it.Provider, p.c(ansiCyan, it.Provider)},
			plain(strings.ReplaceAll(it.Period, "-", "/")),
			{amount, p.c(ansiBold, amount)},
			statusCell(it.Status),    // provider-reported label
			dateCell(it.ConfirmedAt), // provider confirmation (verifikovano)
			dateCell(it.PaidAt),      // user marked paid
		})
	}

	// Widen each column to its longest visible cell.
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cl := range row {
			if wCell := dispWidth(cl.text); wCell > widths[i] {
				widths[i] = wCell
			}
		}
	}

	// border draws a horizontal rule with the given corner/junction glyphs, e.g.
	// ┌────┬────┐. Each column reserves its content width plus a one-space gutter
	// on either side.
	border := func(left, mid, right string) string {
		var b strings.Builder
		b.WriteString(left)
		for i, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				b.WriteString(mid)
			}
		}
		b.WriteString(right)

		return p.c(ansiDim, b.String())
	}

	// renderRow draws one "│ a │ b │" line, padding each cell to its column width
	// by what is actually visible (ANSI codes and emoji handled by dispWidth).
	bar := p.c(ansiDim, "│")
	renderRow := func(row []cell) string {
		var b strings.Builder
		b.WriteString(bar)
		for i, cl := range row {
			b.WriteString(" ")
			b.WriteString(cl.colored)
			b.WriteString(strings.Repeat(" ", widths[i]-dispWidth(cl.text)))
			b.WriteString(" ")
			b.WriteString(bar)
		}

		return b.String()
	}

	fmt.Println(border("┌", "┬", "┐"))
	fmt.Println(renderRow(rows[0]))
	fmt.Println(border("├", "┼", "┤"))
	for _, row := range rows[1:] {
		fmt.Println(renderRow(row))
	}
	fmt.Println(border("└", "┴", "┘"))

	fmt.Println()
	fmt.Println(p.c(ansiDim, fmt.Sprintf("%d račun(a) · %d označeni kao plaćeni", len(items), paidCount)))
}

// receiptsMark marks a single receipt paid (or unpaid) by stamping paid_at on
// its receipts row — the same state the web dashboard reads.
func receiptsMark(c *container.Container, args []string, paidState bool) {
	provider, period, err := parseReceiptKey(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", err.Error())
		receiptsUsage()
		os.Exit(1)
	}

	rec := c.GetReceiptsStore()
	if rec == nil {
		log.Fatal().Msg("Receipts database unavailable, cannot record paid state")
	}

	if _, err := rec.MarkPaid(context.Background(), provider, period, paidState); err != nil {
		log.Fatal().Err(err).
			Str("provider", provider).
			Str("period", period).
			Msg("Failed to persist paid state")
	}

	p := newPainter()
	label := fmt.Sprintf("%s/%s", provider, handlers.NormalizePeriod(period))
	if paidState {
		fmt.Printf("✅ Marked %s as %s.\n", p.c(ansiCyan, label), p.c(ansiGreen, "paid"))
	} else {
		fmt.Printf("↩️  Marked %s as %s.\n", p.c(ansiCyan, label), p.c(ansiYellow, "unpaid"))
	}
}

// parseReceiptKey accepts a receipt key as a single "provider/period" argument
// or as two separate "provider period" arguments.
func parseReceiptKey(args []string) (provider, period string, err error) {
	switch {
	case len(args) >= 2:
		return args[0], args[1], nil
	case len(args) == 1:
		if provider, period, ok := splitKey(args[0]); ok {
			return provider, period, nil
		}
		return "", "", fmt.Errorf("invalid key %q, expected PROVIDER/PERIOD (e.g. mts/10-2025)", args[0])
	default:
		return "", "", errors.New("a receipt key is required, e.g. mts/10-2025")
	}
}

// splitKey pulls provider and period out of a single key string. It understands
// the CLI form "provider/period" and the internal payments form "period|provider".
func splitKey(s string) (provider, period string, ok bool) {
	if period, provider, found := strings.Cut(s, "|"); found {
		return provider, period, true
	}
	if provider, period, found := strings.Cut(s, "/"); found {
		return provider, period, true
	}

	return "", "", false
}
