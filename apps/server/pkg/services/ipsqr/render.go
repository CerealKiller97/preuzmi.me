package ipsqr

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	skipqr "github.com/skip2/go-qrcode"
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

// RenderTerminal draws the QR as a full-block text image via skip2/go-qrcode:
// two characters (`██` / spaces) per module, one text line per module row. Every
// module is a whole character cell, so — unlike a half-block or braille rendering
// — no glyph is split across a cell boundary and the terminal's line spacing
// cannot open the gaps between modules that defeat a scanner. This is the
// rendering to reach for on an unknown or remote terminal (SSH, RPi Connect).
//
// Colours follow the terminal's own palette; the default reads correctly on the
// usual dark background. The cost of the full cells is width — twice the module
// count — so shrink the terminal font if it wraps, because a wrapped QR will not
// scan.
func RenderTerminal(payload string) (string, error) {
	code, err := skipqr.New(payload, skipqr.Low)
	if err != nil {
		return "", fmt.Errorf("ipsqr: encode QR: %w", err)
	}

	return code.ToString(false), nil
}

// RenderTerminalCompact draws the QR at half the width and height of
// RenderTerminal using '▀'/'▄' half-block characters (two module rows per line),
// for when the full-block version is too wide for the window. Splitting each cell
// makes it more sensitive to line spacing, so RenderTerminal is the more reliable
// choice whenever it fits.
func RenderTerminalCompact(payload string) (string, error) {
	code, err := skipqr.New(payload, skipqr.Low)
	if err != nil {
		return "", fmt.Errorf("ipsqr: encode QR: %w", err)
	}

	return code.ToSmallString(false), nil
}

// TerminalSize returns the column width RenderTerminal emits for payload — two
// columns per module across the symbol and its quiet zone — so callers can tell
// whether the full-block QR fits the terminal before falling back to the compact
// half-block rendering.
func TerminalSize(payload string) (int, error) {
	code, err := skipqr.New(payload, skipqr.Low)
	if err != nil {
		return 0, fmt.Errorf("ipsqr: encode QR: %w", err)
	}

	return 2 * len(code.Bitmap()), nil
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
