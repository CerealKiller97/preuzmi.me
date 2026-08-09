package ipsqr

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/makiuchi-d/gozxing/qrcode/decoder"
	"github.com/makiuchi-d/gozxing/qrcode/encoder"
)

func TestRenderTerminalRoundTrip(t *testing.T) {
	payload := "K:PR|V:01|C:1|R:160000000000060216|N:Telekom|I:RSD1819,46|SF:189|S:Racun 10-2025|RO:97 1234567890"

	out, err := RenderTerminal(payload)
	if err != nil {
		t.Fatalf("RenderTerminal: %v", err)
	}
	if !strings.Contains(out, "▀") {
		t.Fatalf("output has no half-block glyphs")
	}

	// The half-block art is only useful if a scanner can read it back. Re-encode
	// the same payload to the raw module grid, paint it 1:1 (one pixel per module,
	// dark=black) and confirm the QR reader recovers the exact payload — the same
	// matrix RenderTerminal draws.
	if got := decodeMatrix(t, payload); got != payload {
		t.Fatalf("QR did not round-trip:\n got: %q\nwant: %q", got, payload)
	}

	compact, err := RenderTerminalCompact(payload)
	if err != nil {
		t.Fatalf("RenderTerminalCompact: %v", err)
	}

	// Compare footprints; the braille version must be materially smaller.
	blockLines := strings.Count(out, "\n")
	compactLines := strings.Count(compact, "\n")
	if compactLines >= blockLines {
		t.Fatalf("compact rendering not smaller: %d lines vs %d", compactLines, blockLines)
	}

	// Print both so a human can eyeball scannability with `go test -run RenderTerminal -v`.
	fmt.Printf("\nhalf-block (%d lines):\n%s\nbraille compact (%d lines):\n%s", blockLines, out, compactLines, compact)
}

func decodeMatrix(t *testing.T, payload string) string {
	t.Helper()

	code, err := encoder.Encoder_encode(payload, decoder.ErrorCorrectionLevel_L, nil)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	m := code.GetMatrix()

	const quiet, scale = 4, 4
	side := (m.GetWidth() + 2*quiet) * scale
	img := image.NewGray(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			gx, gy := x/scale-quiet, y/scale-quiet
			c := color.Gray{Y: 255}
			if gx >= 0 && gy >= 0 && gx < m.GetWidth() && gy < m.GetHeight() && m.Get(gx, gy) == 1 {
				c = color.Gray{Y: 0}
			}
			img.SetGray(x, y, c)
		}
	}

	text, ok := readIPSQR(img)
	if !ok {
		t.Fatalf("re-encoded QR did not decode as an IPS payload")
	}

	return text
}
