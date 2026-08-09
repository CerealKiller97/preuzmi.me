package cmd

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/container"
	handlers "github.com/CerealKiller97/preuzmi.me/pkg/http"
	receiptsrepo "github.com/CerealKiller97/preuzmi.me/pkg/repositories/receipts"
	"github.com/CerealKiller97/preuzmi.me/pkg/script"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/ipsqr"
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
	case "view", "show":
		receiptsView(c, os.Args[3:])
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
		"  view <key>\n"+
		"    Show a receipt's details and its payment QR code.\n"+
		"    The display is auto-detected: an inline image on terminals that\n"+
		"    support it (iTerm2, Warp), a PNG opened in the viewer on a desktop,\n"+
		"    or a text QR on a remote/headless shell. Force it with PREUZMI_QR:\n"+
		"      inline · image · open · path · blocks · braille\n"+
		"    For a QR you must scan over SSH / RPi Connect, use PREUZMI_QR=blocks\n"+
		"    (full-block text QR); braille is a smaller fallback if it is too wide.\n"+
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

	// tr transliterates the fixed Serbian labels below to Cyrillic when the
	// configured script is cyrillic. Only static label text (or text whose
	// interpolated data is digits/slashes, where transliteration is a no-op) is
	// passed through it; provider keys, amounts and dates stay verbatim.
	tr := func(s string) string { return script.Apply(cfg.Lang, s) }

	// Show the period back in the MM/YYYY form the user types.
	periodLabel := strings.ReplaceAll(period, "-", "/")

	if len(items) == 0 {
		fmt.Println(tr(fmt.Sprintf("🧾 Nema računa za %s.", periodLabel)))
		return
	}

	fmt.Println(p.c(ansiBold, tr("🧾 Računi · "+periodLabel)))
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
			plain(tr("PROVAJDER")),
			plain(tr("PERIOD")),
			plain(tr("IZNOS")),
			plain(tr("STATUS")),
			plain(tr("VERIFIKOVANO")),
			plain(tr("PLAĆENO")),
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
			label := tr(s)
			return cell{"🟢 " + label, "🟢 " + p.c(ansiGreen, label)}
		case receiptsrepo.StatusUnpaid:
			label := tr(s)
			return cell{"🔴 " + label, "🔴 " + p.c(ansiYellow, label)}
		default:
			label := tr("nepoznato")
			return cell{"⚪ " + label, "⚪ " + p.c(ansiDim, label)}
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
	fmt.Println(p.c(ansiDim, tr(fmt.Sprintf("%d račun(a) · %d označeni kao plaćeni", len(items), paidCount))))
}

// receiptsView shows a single receipt's details and renders its NBS IPS payment
// QR straight in the terminal, so the download → scan → pay loop closes without
// leaving the shell. The receipt facts come from the same source the dashboard
// uses; the QR payload is resolved (cache first, PDF fallback) exactly as the web
// card does, so the two always show the identical, scannable code.
func receiptsView(c *container.Container, args []string) {
	provider, period, err := parseReceiptKey(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", err.Error())
		receiptsUsage()
		os.Exit(1)
	}
	period = handlers.NormalizePeriod(period)

	cfg := c.GetConfig()
	p := newPainter()
	tr := func(s string) string { return script.Apply(cfg.Lang, s) }
	key := fmt.Sprintf("%s/%s", provider, strings.ReplaceAll(period, "-", "/"))

	all, err := handlers.CollectReceipts(cfg, c.GetReceiptsStore())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to collect receipts")
	}

	var rec *handlers.APIReceipt
	for i := range all {
		if all[i].Provider == provider && all[i].Period == period {
			rec = &all[i]
			break
		}
	}
	if rec == nil {
		fmt.Fprintln(os.Stderr, tr(fmt.Sprintf("🧾 Nema računa za %s.", key)))
		os.Exit(1)
	}

	// line prints one "  LABEL  value" row; labels are transliterated, values are
	// printed verbatim.
	line := func(label, value string) {
		fmt.Printf("  %s  %s\n", p.c(ansiDim, tr(label)), value)
	}
	date := func(at int64) string {
		if at <= 0 {
			return "❌ " + p.c(ansiDim, "—")
		}
		return "✅ " + p.c(ansiGreen, time.Unix(at, 0).Format("02.01.2006"))
	}

	fmt.Println(p.c(ansiBold, "🧾 "+p.c(ansiCyan, key)))
	fmt.Println()

	amount := "—"
	if rec.Amount != 0 {
		amount = fmt.Sprintf("%.2f", rec.Amount)
		if rec.Currency != "" {
			amount += " " + rec.Currency
		}
	}
	line("IZNOS", p.c(ansiBold, amount))

	switch rec.Status {
	case receiptsrepo.StatusPaid:
		line("STATUS", "🟢 "+p.c(ansiGreen, tr(rec.Status)))
	case receiptsrepo.StatusUnpaid:
		line("STATUS", "🔴 "+p.c(ansiYellow, tr(rec.Status)))
	default:
		line("STATUS", "⚪ "+p.c(ansiDim, tr("nepoznato")))
	}

	line("VERIFIKOVANO", date(rec.ConfirmedAt))
	line("PLAĆENO", date(rec.PaidAt))
	if rec.DueAt > 0 {
		line("ROK", p.c(ansiYellow, time.Unix(rec.DueAt, 0).Format("02.01.2006")))
	}

	// Resolve and render the payment QR. A bill whose layout embeds no readable QR
	// simply shows a note — the receipt details above are still useful on their own.
	payload, ok := handlers.ResolveIPSPayload(context.Background(), c.GetStorage(), c.GetReceiptsStore(), provider, rec.Account, period)
	if !ok {
		fmt.Println()
		fmt.Println(p.c(ansiDim, tr("Ovaj račun nema QR kod za plaćanje.")))
		return
	}

	// Payment details carried by the QR itself: recipient, account, purpose and
	// reference number — the fields a banking app fills in from the scan.
	fmt.Println()
	if name, hasName := ipsqr.Field(payload, "N"); hasName {
		line("PRIMALAC", name)
	}
	if acc, hasAcc := ipsqr.Field(payload, "R"); hasAcc {
		line("RAČUN", acc)
	}
	if purpose, hasPurpose := ipsqr.Field(payload, "S"); hasPurpose {
		line("SVRHA", purpose)
	}
	if ref, hasRef := ipsqr.Field(payload, "RO"); hasRef {
		line("POZIV NA BROJ", ref)
	}

	fmt.Println()
	emitReceiptQR(p, tr, provider, period, payload)
}

// qrMode is a way of showing the payment QR, chosen by resolveQRMode.
type qrMode int

const (
	qrInline   qrMode = iota // real raster drawn in place (iTerm2/Warp protocol)
	qrFileOpen               // PNG written to disk and opened in a desktop viewer
	qrFilePath               // PNG written to disk; only its path is printed
	qrBlocks                 // full-block text QR (most scan-robust)
	qrBraille                // compact half-block text QR
)

// emitReceiptQR shows a receipt's payment QR, picking the rendering that will
// actually reach the user's screen for their terminal (see resolveQRMode). The
// image modes reuse the exact PNG the web dashboard serves, which scans reliably;
// the text modes are the fallback where no image can be shown.
func emitReceiptQR(p painter, tr func(string) string, provider, period, payload string) {
	switch resolveQRMode(payload) {
	case qrBraille:
		emitTextQR(ipsqr.RenderTerminalCompact, p, tr, provider, period, payload)
	case qrBlocks:
		emitTextQR(ipsqr.RenderTerminal, p, tr, provider, period, payload)
	case qrInline:
		png := renderQRPNG(provider, period, payload)
		fmt.Print(iterm2InlineImage(png))
		fmt.Println()
		fmt.Println(p.c(ansiDim, tr("Skenirajte QR kod u mobilnoj banci da platite.")))
	case qrFileOpen:
		path := writeQRFileOrDie(provider, period, renderQRPNG(provider, period, payload))
		openInViewer(path)
		fmt.Println(tr("QR kod (slika): ") + p.c(ansiCyan, path))
		fmt.Println(p.c(ansiDim, tr("Otvorite sliku i skenirajte je mobilnom bankom.")))
	case qrFilePath:
		path := writeQRFileOrDie(provider, period, renderQRPNG(provider, period, payload))
		fmt.Println(tr("QR kod (slika): ") + path)
	}
}

// resolveQRMode decides how to display the QR. PREUZMI_QR forces a specific mode;
// otherwise ("auto", the default) it runs a capability ladder from the richest
// rendering the environment can show down to a plain text QR:
//
//  1. not a TTY (piped/redirected) → write the PNG, print only its path
//  2. terminal speaks an inline-image protocol (iTerm2, Warp) → draw it in place
//  3. local desktop session with an image viewer → open the PNG in it
//  4. remote or headless shell (SSH, RPi Connect, …) → text QR, which is the only
//     thing that reaches the user's own screen
func resolveQRMode(payload string) qrMode {
	if m, ok := parseQRModeOverride(os.Getenv("PREUZMI_QR")); ok {
		return m
	}

	return autoQRMode(
		stdoutIsTTY(),
		inlineImagesSupported(),
		localDesktopViewer(),
		terminalWidth() >= qrBlockColumns(payload),
	)
}

// parseQRModeOverride reads a mode forced through PREUZMI_QR. ok is false for an
// empty, "auto", or unrecognised value — all of which mean "auto-detect".
func parseQRModeOverride(v string) (qrMode, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "inline":
		return qrInline, true
	case "image", "file", "open":
		return qrFileOpen, true
	case "path":
		return qrFilePath, true
	case "blocks", "block", "ascii", "solid", "ansi", "big":
		return qrBlocks, true
	case "braille", "compact", "small":
		return qrBraille, true
	default:
		return 0, false
	}
}

// autoQRMode is the capability ladder, from the richest rendering the
// environment can show down to a text QR. It is a pure function of the detected
// capabilities so the decision can be tested without a real terminal:
//   - not a TTY (piped/redirected): leave the PNG on disk, print only its path
//   - an inline-image terminal: draw the raster in place
//   - a local desktop session: open the PNG in the image viewer
//   - otherwise (remote/headless): a text QR, the only thing that reaches the
//     user's screen — solid blocks when they fit the window, else braille
func autoQRMode(isTTY, inline, desktop, blockFits bool) qrMode {
	switch {
	case !isTTY:
		return qrFilePath
	case inline:
		return qrInline
	case desktop:
		return qrFileOpen
	case blockFits:
		return qrBlocks
	default:
		return qrBraille
	}
}

// renderQRPNG renders the receipt's QR as the same PNG the dashboard serves.
func renderQRPNG(provider, period, payload string) []byte {
	png, err := ipsqr.RenderPNG(payload, 512)
	if err != nil {
		log.Fatal().Err(err).Str("provider", provider).Str("period", period).Msg("Failed to render QR image")
	}

	return png
}

// writeQRFileOrDie writes the PNG and returns its path, aborting on failure.
func writeQRFileOrDie(provider, period string, png []byte) string {
	path, err := writeQRFile(provider, period, png)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to write QR image")
	}

	return path
}

// emitTextQR renders a text QR with the given renderer. Text QRs are opt-in
// because their scannability depends on the terminal's font and line spacing.
func emitTextQR(render func(string) (string, error), p painter, tr func(string) string, provider, period, payload string) {
	qr, err := render(payload)
	if err != nil {
		log.Fatal().Err(err).Str("provider", provider).Str("period", period).Msg("Failed to render QR")
	}
	fmt.Print(qr)
	fmt.Println(p.c(ansiDim, tr("Skenirajte QR kod u mobilnoj banci da platite.")))
}

// inlineImagesSupported reports whether the terminal speaks the iTerm2 inline-
// image protocol. tmux/screen swallow the escape unless configured for
// passthrough, so those are excluded even under a supported terminal.
func inlineImagesSupported() bool {
	if os.Getenv("TMUX") != "" || strings.HasPrefix(os.Getenv("TERM"), "screen") {
		return false
	}

	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WarpTerminal":
		return true
	default:
		return false
	}
}

// stdoutIsTTY reports whether standard output is a terminal (rather than a pipe
// or file), the same test the colour painter uses.
func stdoutIsTTY() bool {
	fd := os.Stdout.Fd()

	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// isRemoteSession reports whether the shell is reached over the network, where a
// file written locally or a viewer launched locally lands on the wrong machine.
// It recognises SSH by its environment; other remote shells that do not set it
// are still handled by the headless checks in localDesktopViewer.
func isRemoteSession() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CLIENT") != ""
}

// localDesktopViewer reports whether opening the PNG in a viewer would actually
// show it to the user: a local (non-remote) session with a graphical desktop.
func localDesktopViewer() bool {
	if isRemoteSession() {
		return false
	}

	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	case "linux":
		return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	default:
		return false
	}
}

// qrBlockColumns is the column width the half-block QR occupies for payload, used
// to decide whether it fits the terminal before falling back to braille.
func qrBlockColumns(payload string) int {
	n, err := ipsqr.TerminalSize(payload)
	if err != nil {
		return 0
	}

	return n
}

// iterm2InlineImage wraps a PNG in the iTerm2 inline-image escape (OSC 1337),
// which iTerm2 and Warp render as a real raster in the scrollback.
func iterm2InlineImage(png []byte) string {
	b64 := base64.StdEncoding.EncodeToString(png)

	return fmt.Sprintf("\033]1337;File=inline=1;size=%d;preserveAspectRatio=1:%s\a", len(png), b64)
}

// qrFileSlug reduces one part of a receipt key to a filename-safe token, keeping
// only ASCII letters, digits, dash and underscore. Provider and period come from
// user-supplied CLI arguments, so this guarantees the QR image name cannot carry
// a path separator or "…" and escape the temp directory.
func qrFileSlug(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
}

// writeQRFile saves the QR PNG to a stable per-receipt path in the temp dir and
// returns it. Reusing the same name means repeated views overwrite rather than
// litter the directory. The key parts are slugged and the result reduced to a
// bare basename, so the write stays inside the temp directory for any input.
func writeQRFile(provider, period string, png []byte) (string, error) {
	name := filepath.Base(fmt.Sprintf("preuzmi-qr-%s-%s.png", qrFileSlug(provider), qrFileSlug(period)))
	path := filepath.Join(os.TempDir(), name)
	if err := os.WriteFile(path, png, 0o600); err != nil {
		return "", err
	}

	return path, nil
}

// openInViewer opens path in the OS image viewer, best-effort: the path is
// printed regardless, so a headless or unsupported environment still works.
func openInViewer(path string) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{path}
	case "linux":
		name, args = "xdg-open", []string{path}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", path}
	default:
		return
	}

	// The opener name is a fixed per-OS constant and path is a sanitized basename
	// (see writeQRFile) under os.TempDir that this process just wrote — not shell-
	// interpreted, so there is no injection surface here.
	// #nosec G204,G702 -- fixed opener + sanitized temp path we just wrote
	if err := exec.Command(name, args...).Start(); err != nil {
		// Best-effort: the caller prints the path regardless, so a missing opener
		// (headless box, no xdg-open) is a debug note, not a failure.
		log.Debug().Err(err).Str("path", path).Msg("Could not open QR image in the viewer")
	}
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

	// The CLI mark key is <provider>/<period> with no account, so it targets the
	// solo account (""). Multi-account marking is done from the web/mobile UI.
	if _, err := rec.MarkPaid(context.Background(), provider, "", period, paidState); err != nil {
		log.Fatal().Err(err).
			Str("provider", provider).
			Str("period", period).
			Msg("Failed to persist paid state")
	}

	p := newPainter()
	tr := func(s string) string { return script.Apply(c.GetConfig().Lang, s) }
	label := fmt.Sprintf("%s/%s", provider, handlers.NormalizePeriod(period))
	if paidState {
		fmt.Printf("✅ %s %s %s.\n", p.c(ansiCyan, label), tr("označen kao"), p.c(ansiGreen, tr("plaćen")))
	} else {
		fmt.Printf("↩️  %s %s %s.\n", p.c(ansiCyan, label), tr("označen kao"), p.c(ansiYellow, tr("neplaćen")))
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
