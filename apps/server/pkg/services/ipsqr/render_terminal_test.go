package ipsqr

import (
	"bytes"
	"fmt"
	"image/png"
	"strings"
	"testing"

	skipqr "github.com/skip2/go-qrcode"
)

func TestRenderTerminalSkip2(t *testing.T) {
	payload := "K:PR|V:01|C:1|R:160000000000060216|N:Telekom|I:RSD1819,46|SF:189|S:Racun 08/2026|RO:97 1234567890"

	full, err := RenderTerminal(payload)
	if err != nil {
		t.Fatalf("RenderTerminal: %v", err)
	}
	if !strings.Contains(full, "█") {
		t.Fatalf("full-block output has no block glyphs")
	}

	compact, err := RenderTerminalCompact(payload)
	if err != nil {
		t.Fatalf("RenderTerminalCompact: %v", err)
	}
	if strings.Count(compact, "\n") >= strings.Count(full, "\n") {
		t.Fatalf("compact rendering not shorter: %d vs %d lines",
			strings.Count(compact, "\n"), strings.Count(full, "\n"))
	}

	if n, err := TerminalSize(payload); err != nil || n <= 0 {
		t.Fatalf("TerminalSize = %d, %v", n, err)
	}

	// skip2 must encode our exact IPS payload scannably: render its own PNG and
	// confirm the QR reader recovers the payload byte-for-byte.
	pngBytes, err := skipqr.Encode(payload, skipqr.Low, 512)
	if err != nil {
		t.Fatalf("skip2 Encode: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	got, ok := readIPSQR(img)
	if !ok || got != payload {
		t.Fatalf("skip2 QR did not round-trip: ok=%v got=%q", ok, got)
	}

	// Print both so a human can eyeball them with `go test -run Skip2 -v`.
	fmt.Printf("\nfull-block:\n%s\ncompact:\n%s", full, compact)
}
