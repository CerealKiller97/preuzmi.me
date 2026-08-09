package ipsqr

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"github.com/makiuchi-d/gozxing/qrcode/decoder"
	"github.com/makiuchi-d/gozxing/qrcode/encoder"
)

// DefaultSize is the pixel width/height used when rendering a QR without an
// explicit size — large enough to scan comfortably off a screen.
const DefaultSize = 320

// RenderPNG regenerates a crisp black-on-white QR PNG from an IPS payload.
//
// It re-encodes rather than re-serving the pixels lifted from the PDF: the
// embedded QR can be low-resolution, oddly cropped, or in an inconvenient
// polarity, whereas a re-encode from the exact decoded string is always sharp
// and a predictable size. size is the image side in pixels (a sensible minimum
// is enforced so the modules never collapse).
func RenderPNG(payload string, size int) ([]byte, error) {
	if size < 128 {
		size = DefaultSize
	}

	hints := map[gozxing.EncodeHintType]interface{}{
		// IPS payloads carry Serbian text (recipient name, purpose); UTF-8 byte
		// mode is what the NBS spec mandates and what banking apps expect.
		gozxing.EncodeHintType_CHARACTER_SET: "UTF-8",
		// A quiet zone is part of the QR spec; without it many scanners refuse.
		gozxing.EncodeHintType_MARGIN: 2,
	}

	matrix, err := qrcode.NewQRCodeWriter().Encode(payload, gozxing.BarcodeFormat_QR_CODE, size, size, hints)
	if err != nil {
		return nil, fmt.Errorf("ipsqr: encode QR: %w", err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, bitMatrixImage{matrix}); err != nil {
		return nil, fmt.Errorf("ipsqr: encode PNG: %w", err)
	}

	return buf.Bytes(), nil
}

// Half-block ANSI cell: '▀' painted with an explicit foreground (the upper half)
// and background (the lower half) colour. Pinning both colours keeps the QR's
// polarity correct — dark modules black, light modules and the quiet zone white —
// regardless of the terminal's own theme, so it scans off a dark background too.
const (
	ansiFGWhite = "97"
	ansiFGBlack = "30"
	ansiBGWhite = "107"
	ansiBGBlack = "40"
	ansiReset   = "\033[0m"
)

// quietZone is the light border kept around the symbol. The spec asks for 4
// modules; 2 is enough to scan off a screen and saves cells on every side.
const quietZone = 2

// qrGrid encodes payload to the raw QR module grid and returns the symbol's
// module count together with a dark(x, y) predicate. Coordinates are measured
// from the top-left of the quiet zone, so the grid is (size × size) modules with
// the quiet border already folded in; anything outside the symbol reads light.
//
// Level L keeps the symbol as small as the payload allows — the fewest modules,
// so the fewest terminal cells — while staying comfortably scannable; it also
// matches the level RenderPNG uses.
func qrGrid(payload string) (size int, dark func(x, y int) bool, err error) {
	code, err := encoder.Encoder_encode(payload, decoder.ErrorCorrectionLevel_L, map[gozxing.EncodeHintType]interface{}{
		// IPS payloads carry Serbian text; UTF-8 is what the NBS spec mandates.
		gozxing.EncodeHintType_CHARACTER_SET: "UTF-8",
	})
	if err != nil {
		return 0, nil, fmt.Errorf("ipsqr: encode QR: %w", err)
	}

	m := code.GetMatrix()
	if m == nil {
		return 0, nil, errors.New("ipsqr: encoder returned no matrix")
	}

	w, h := m.GetWidth(), m.GetHeight()
	dark = func(x, y int) bool {
		x, y = x-quietZone, y-quietZone
		if x < 0 || y < 0 || x >= w || y >= h {
			return false
		}

		return m.Get(x, y) == 1
	}

	return w + 2*quietZone, dark, nil
}

// TerminalSize returns the side length, in character columns, that RenderTerminal
// emits for payload — the QR's module count plus its quiet zone. Callers use it
// to check whether the block QR fits the terminal before choosing a rendering.
func TerminalSize(payload string) (int, error) {
	size, _, err := qrGrid(payload)

	return size, err
}

// RenderTerminal draws the QR with '▀' half-block characters: one column per
// module, two module rows per line, so the code stays square and every module is
// a solid block. Colours are pinned — dark modules black, light modules and the
// quiet zone white — so it reads correctly on any terminal theme. This is the
// most robust rendering; RenderTerminalCompact is far smaller when the block
// version overflows the screen.
func RenderTerminal(payload string) (string, error) {
	total, dark, err := qrGrid(payload)
	if err != nil {
		return "", err
	}

	// colour returns the SGR for a half-block cell whose top/bottom halves take the
	// given darkness. The glyph's foreground paints the top half, background the
	// bottom.
	colour := func(top, bottom bool) string {
		fg, bg := ansiFGWhite, ansiBGWhite
		if top {
			fg = ansiFGBlack
		}
		if bottom {
			bg = ansiBGBlack
		}

		return "\033[" + fg + ";" + bg + "m"
	}

	var b strings.Builder
	for y := 0; y < total; y += 2 {
		for x := 0; x < total; x++ {
			b.WriteString(colour(dark(x, y), dark(x, y+1)))
			b.WriteRune('▀')
		}
		b.WriteString(ansiReset)
		b.WriteByte('\n')
	}

	return b.String(), nil
}

// brailleDots maps a (row, col) position inside a braille cell's 2-wide × 4-tall
// grid to its Unicode dot bit. A braille glyph is U+2800 plus the OR of the bits
// for every raised dot, which lets one character carry eight modules.
var brailleDots = [4][2]rune{
	{0x01, 0x08}, // dots 1, 4
	{0x02, 0x10}, // dots 2, 5
	{0x04, 0x20}, // dots 3, 6
	{0x40, 0x80}, // dots 7, 8
}

// RenderTerminalCompact draws the QR with Unicode braille characters, packing a
// 2×4 block of modules into every cell — roughly four times fewer terminal cells
// than the half-block RenderTerminal, so the whole code fits on screen and small
// enough for a phone camera to frame. A braille cell's 2:4 shape keeps the
// modules square. Dark modules are raised dots painted black on a white
// background so the polarity is correct on any terminal theme.
//
// The trade-off is that a "dark" region is dots with hair-line gaps rather than a
// solid fill; most scanners cope, but RenderTerminal is the fallback when one
// does not.
func RenderTerminalCompact(payload string) (string, error) {
	total, dark, err := qrGrid(payload)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for y := 0; y < total; y += 4 {
		b.WriteString("\033[" + ansiFGBlack + ";" + ansiBGWhite + "m")
		for x := 0; x < total; x += 2 {
			cell := rune(0x2800)
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					if dark(x+dx, y+dy) {
						cell |= brailleDots[dy][dx]
					}
				}
			}
			b.WriteRune(cell)
		}
		b.WriteString(ansiReset)
		b.WriteByte('\n')
	}

	return b.String(), nil
}

// bitMatrixImage adapts a gozxing BitMatrix to image.Image so the standard PNG
// encoder can write it: a set bit is black, an unset bit white.
type bitMatrixImage struct {
	m *gozxing.BitMatrix
}

func (b bitMatrixImage) ColorModel() color.Model { return color.GrayModel }

func (b bitMatrixImage) Bounds() image.Rectangle {
	return image.Rect(0, 0, b.m.GetWidth(), b.m.GetHeight())
}

func (b bitMatrixImage) At(x, y int) color.Color {
	if b.m.Get(x, y) {
		return color.Gray{Y: 0}
	}

	return color.Gray{Y: 255}
}
