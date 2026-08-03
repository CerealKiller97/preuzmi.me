package ipsqr

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
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
