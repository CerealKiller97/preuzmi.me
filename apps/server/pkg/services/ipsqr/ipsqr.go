// Package ipsqr lifts the NBS IPS payment QR out of a Serbian utility-bill PDF.
//
// Serbian invoices embed an "NBS IPS QR" — a QR code carrying the structured
// payment order (recipient account, amount, payment code, reference number) that
// a banking app scans to pre-fill a transfer. This package finds that QR among
// the PDF's embedded rasters, inline images, or vector fills, decodes it, and
// returns the exact payload string so the dashboard can render a crisp,
// scannable copy on the receipt card — turning the download → scan → pay loop
// into a single screen.
//
// It works off the *embedded* QR rather than reconstructing one from parsed
// text: the payload moves money, so the account and reference number must be the
// ones the provider actually printed, byte for byte.
package ipsqr

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// Extract scans a bill PDF for an NBS IPS QR and returns its payload, or
// ok=false when none is recognisable (a normal outcome for layouts we cannot
// yet read — the card just shows no QR). It never returns an error for a
// malformed PDF: a bill we cannot parse simply has no QR to show.
//
// Search order:
//  1. Raster image XObjects (mts CCITT, Infostan Flate)
//  2. Inline BI/ID/EI images in content streams (EPS ASCII85+Flate)
//  3. Vector fill paths rasterised per page (eUpravnik)
func Extract(pdf []byte) (payload string, ok bool) {
	pdf = sanitizePDF(pdf)

	// First, the common case: the QR is embedded as a raster image XObject.
	for _, cand := range carveImages(pdf) {
		if s, found := readIPSQR(cand.img); found {
			return s, true
		}
	}

	// EPS (and similar) embed the QR as an inline image in the content stream
	// rather than as a separate XObject — carve those next.
	for _, stream := range contentStreams(pdf) {
		for _, img := range carveInlineImages(stream) {
			if s, found := readIPSQR(img); found {
				return s, true
			}
		}
	}

	// Fallback: some providers (eUpravnik) draw the QR as vector fill paths with
	// no raster image at all. Rasterise each page's fills and scan those.
	for _, img := range renderVectorPages(pdf) {
		if s, found := readIPSQR(img); found {
			return s, true
		}
	}

	return "", false
}

// AmountFromPDF extracts the IPS QR from a bill PDF and returns the payable
// amount it carries. It is the one-call path for providers that want the QR
// amount at download time — the figure a banking app actually charges — without
// handling the payload themselves. ok is false when the bill has no readable QR
// or the QR carries no parseable amount.
func AmountFromPDF(pdf []byte) (float64, bool) {
	payload, ok := Extract(pdf)
	if !ok {
		return 0, false
	}

	return Amount(payload)
}

// readIPSQR attempts to read a single image as a QR code and validates that the
// decoded text is an NBS IPS payload. It retries on an inverted copy of the
// image, since 1-bit bill rasters come in either polarity.
func readIPSQR(img image.Image) (string, bool) {
	for _, candidate := range []image.Image{img, invert(img)} {
		text, err := decodeQR(candidate)
		if err != nil {
			continue
		}
		if IsIPSPayload(text) {
			return normalizePayload(text), true
		}
	}

	return "", false
}

// decodeQR runs the ZXing QR reader over an image with the "try harder" hint,
// which is worth the cost here: bill QRs are small and sometimes rescaled.
func decodeQR(img image.Image) (string, error) {
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}

	hints := map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER: true,
	}

	res, err := qrcode.NewQRCodeReader().Decode(bmp, hints)
	if err != nil {
		return "", err
	}

	return res.GetText(), nil
}

// IsIPSPayload reports whether text is an NBS IPS QR payload. The IPS spec keys
// the payload with an identification tag ("K:PR") and a mandatory recipient
// account field ("R:"); requiring both rejects unrelated QRs (a provider app
// link, a tracking barcode) that happen to share the image list.
func IsIPSPayload(text string) bool {
	t := strings.ToUpper(strings.TrimSpace(text))

	return strings.HasPrefix(t, "K:PR") && strings.Contains(t, "|R:")
}

// Amount parses the payable amount from an IPS payload's `I:` field and reports
// whether one was found. Per the NBS IPS spec the field is a 3-letter ISO-4217
// currency code followed by the amount with a comma decimal separator and no
// thousands grouping, e.g. "I:RSD4376,94" -> 4376.94.
//
// This is the figure a banking app charges when the QR is scanned, so it is the
// authoritative price for the bill — preferred over a provider's API- or
// PDF-parsed total when the two disagree.
func Amount(payload string) (float64, bool) {
	for _, part := range strings.Split(payload, "|") {
		key, val, ok := strings.Cut(part, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "I") {
			continue
		}

		// Drop the leading ISO-4217 currency code (letters), then normalise the
		// Serbian-style number: strip any thousands dots and turn the decimal
		// comma into a point.
		num := strings.TrimLeft(strings.TrimSpace(val), "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
		num = strings.ReplaceAll(num, ".", "")
		num = strings.Replace(num, ",", ".", 1)

		amount, err := strconv.ParseFloat(num, 64)
		if err != nil || amount <= 0 {
			return 0, false
		}

		return amount, true
	}

	return 0, false
}

// Field returns the value of a single pipe-delimited IPS field by its key,
// matched case-insensitively — e.g. "R" (recipient account), "N" (recipient
// name), "S" (payment purpose), "RO" (recipient reference). ok is false when the
// field is absent. It is the read-only counterpart to Amount for the fields a
// CLI or card wants to show alongside the QR.
func Field(payload, key string) (string, bool) {
	for _, part := range strings.Split(payload, "|") {
		k, v, found := strings.Cut(part, ":")
		if found && strings.EqualFold(strings.TrimSpace(k), key) {
			return strings.TrimSpace(v), true
		}
	}

	return "", false
}

// invert returns a value-inverted grayscale copy of an image so a QR printed as
// white-on-black scans as well as black-on-white.
func invert(src image.Image) image.Image {
	b := src.Bounds()
	dst := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := src.At(x, y).RGBA()
			lum := uint8(((r*299 + g*587 + bl*114) / 1000) >> 8)
			dst.SetGray(x, y, color.Gray{Y: 255 - lum})
		}
	}

	return dst
}

// normalizePayload trims surrounding whitespace the reader can pick up without
// touching the payload's internal structure.
func normalizePayload(s string) string {
	return strings.TrimSpace(s)
}
