package ipsqr

import (
	"bytes"
	"compress/zlib"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"regexp"
	"strconv"

	"golang.org/x/image/ccitt"
)

// This file carves raster image XObjects out of raw PDF bytes and decodes them
// into image.Image, independent of the text-oriented ledongthuc/pdf reader
// (which auto-applies FlateDecode but panics on DCTDecode/CCITTFaxDecode — the
// very filters the IPS QR is often stored under).
//
// The approach is deliberately byte-level rather than a full PDF parse: the QR
// only needs to be *found and read*, so we scan for image stream objects, pull
// the raw bytes between "stream"/"endstream", and decode by declared filter.
// Anything we cannot decode is skipped, never fatal.

// streamRe matches the header + body of every stream object: the dictionary
// (non-greedy up to the first "stream"), the EOL after the "stream" keyword,
// and the raw payload up to "endstream".
var streamRe = regexp.MustCompile(`(?s)<<(.*?)>>\s*stream\r?\n(.*?)\r?\nendstream`)

var (
	reSubtypeImage = regexp.MustCompile(`/Subtype\s*/Image`)
	reWidth        = regexp.MustCompile(`/Width\s+(\d+)`)
	reHeight       = regexp.MustCompile(`/Height\s+(\d+)`)
	reBPC          = regexp.MustCompile(`/BitsPerComponent\s+(\d+)`)
	reImageMask    = regexp.MustCompile(`/ImageMask\s+true`)
	reColorSpace   = regexp.MustCompile(`/ColorSpace\s*/(DeviceRGB|DeviceGray|DeviceCMYK)`)
	reK            = regexp.MustCompile(`/K\s+(-?\d+)`)
	reColumns      = regexp.MustCompile(`/Columns\s+(\d+)`)
	rePredictor    = regexp.MustCompile(`/Predictor\s+(\d+)`)
	reColors       = regexp.MustCompile(`/Colors\s+(\d+)`)
	reBlackIs1     = regexp.MustCompile(`/BlackIs1\s+true`)
	reDCT          = regexp.MustCompile(`/DCTDecode|/DCT\b`)
	reFlate        = regexp.MustCompile(`/FlateDecode|/Fl\b`)
	reCCITT        = regexp.MustCompile(`/CCITTFaxDecode|/CCF\b`)
)

// pdfImage is a decoded raster candidate lifted from the PDF.
type pdfImage struct {
	img image.Image
}

// carveImages returns every raster image XObject we can decode from the PDF.
//
// Candidates are yielded largest-area-agnostic and in file order; the caller
// QR-decodes each. Images we do not know how to decode (JPXDecode, JBIG2, exotic
// colour spaces) are silently skipped — they are simply never the QR we return.
func carveImages(pdf []byte) []pdfImage {
	var out []pdfImage

	for _, m := range streamRe.FindAllSubmatchIndex(pdf, -1) {
		dict := pdf[m[2]:m[3]]
		if !reSubtypeImage.Match(dict) {
			continue
		}
		raw := pdf[m[4]:m[5]]

		img := decodeImage(dict, raw)
		if img != nil {
			out = append(out, pdfImage{img: img})
		}
	}

	return out
}

// decodeImage decodes one image XObject's raw stream into an image.Image using
// its dictionary. It returns nil for anything unsupported.
func decodeImage(dict, raw []byte) image.Image {
	w := atoiSub(reWidth.FindSubmatch(dict))
	h := atoiSub(reHeight.FindSubmatch(dict))

	switch {
	case reDCT.Match(dict):
		// DCTDecode: the raw stream is a JFIF/JPEG file.
		img, err := jpeg.Decode(bytes.NewReader(raw))
		if err != nil {
			return nil
		}
		return img

	case reCCITT.Match(dict):
		return decodeCCITT(dict, raw, w, h)

	case reFlate.Match(dict):
		return decodeFlateRaster(dict, raw, w, h)
	}

	return nil
}

// decodeCCITT decodes a CCITT Group 3/4 fax image (BitsPerComponent 1). QR codes
// on Serbian bills are frequently stored this way.
func decodeCCITT(dict, raw []byte, w, h int) image.Image {
	if w == 0 || h == 0 {
		return nil
	}

	cols := atoiSub(reColumns.FindSubmatch(dict))
	if cols == 0 {
		cols = w
	}

	k := 0
	if m := reK.FindSubmatch(dict); m != nil {
		k, _ = strconv.Atoi(string(m[1]))
	}

	subFormat := ccitt.Group4
	if k > 0 {
		subFormat = ccitt.Group3
	}

	opts := &ccitt.Options{Invert: reBlackIs1.Match(dict)}

	r := ccitt.NewReader(bytes.NewReader(raw), ccitt.MSB, subFormat, cols, h, opts)
	img, err := decodeGrayReader(r, cols, h, 1, false)
	if err != nil {
		return nil
	}

	return img
}

// decodeFlateRaster inflates a FlateDecode image stream and interprets the raw
// samples as a bitmap. Only the sample layouts QR codes actually use are
// handled: 1-bit masks/gray and 8-bit gray (RGB is handled too, for the odd
// colour QR). A PNG predictor (Predictor >= 10, common on 1-bit bill QRs) is
// undone first, so the underlying samples read correctly.
func decodeFlateRaster(dict, raw []byte, w, h int) image.Image {
	if w == 0 || h == 0 {
		return nil
	}

	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	defer zr.Close() //nolint:errcheck

	samples, err := io.ReadAll(zr)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil
	}

	bpc := atoiSub(reBPC.FindSubmatch(dict))
	if reImageMask.Match(dict) {
		bpc = 1
	}
	if bpc == 0 {
		bpc = 8
	}

	isRGB := reColorSpace.Match(dict) && bytes.Contains(reColorSpace.Find(dict), []byte("DeviceRGB")) && bpc == 8
	colors := 1
	if isRGB {
		colors = 3
	}

	// Undo a PNG predictor if one was applied (each row is prefixed with a
	// filter-type byte the encoder chose per row).
	if pred := atoiSub(rePredictor.FindSubmatch(dict)); pred >= 10 {
		cols := atoiSub(reColumns.FindSubmatch(dict))
		if cols == 0 {
			cols = w
		}
		if c := atoiSub(reColors.FindSubmatch(dict)); c > 0 {
			colors = c
		}
		samples = undoPNGPredictor(samples, cols, colors, bpc)
	}

	r := bytes.NewReader(samples)
	switch {
	case isRGB:
		return decodeRGBReader(r, w, h)
	case bpc == 1:
		img, err := decodeGrayReader(r, w, h, 1, reImageMask.Match(dict))
		if err != nil {
			return nil
		}
		return img
	case bpc == 8:
		img, err := decodeGrayReader(r, w, h, 8, false)
		if err != nil {
			return nil
		}
		return img
	}

	return nil
}

// undoPNGPredictor reverses the per-row PNG filtering that a PDF FlateDecode
// stream applies when DecodeParms declares Predictor >= 10. Each source row is
// bytesPerRow+1 long: a leading filter-type byte (0-4) followed by the filtered
// samples. It returns the reconstructed, tightly packed samples.
func undoPNGPredictor(data []byte, columns, colors, bpc int) []byte {
	bpp := (colors*bpc + 7) / 8 // bytes per pixel, min 1
	if bpp < 1 {
		bpp = 1
	}
	rowLen := (columns*colors*bpc + 7) / 8
	if rowLen == 0 {
		return data
	}

	stride := rowLen + 1
	rows := len(data) / stride
	out := make([]byte, 0, rows*rowLen)
	prev := make([]byte, rowLen)

	for r := 0; r < rows; r++ {
		rowStart := r * stride
		ft := data[rowStart]
		cur := make([]byte, rowLen)
		copy(cur, data[rowStart+1:rowStart+1+rowLen])

		for i := 0; i < rowLen; i++ {
			var a, b, c int // left, up, up-left
			if i >= bpp {
				a = int(cur[i-bpp])
				c = int(prev[i-bpp])
			}
			b = int(prev[i])
			switch ft {
			case 1: // Sub
				cur[i] += byte(a)
			case 2: // Up
				cur[i] += byte(b)
			case 3: // Average
				cur[i] += byte((a + b) / 2)
			case 4: // Paeth
				cur[i] += byte(paeth(a, b, c))
			}
		}

		out = append(out, cur...)
		prev = cur
	}

	return out
}

// paeth is the PNG Paeth predictor.
func paeth(a, b, c int) int {
	p := a + b - c
	pa, pb, pc := abs(p-a), abs(p-b), abs(p-c)
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	default:
		return c
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// decodeGrayReader reads w*h samples of the given bit depth (1 or 8), packed
// MSB-first with each row padded to a byte boundary (the PDF/CCITT convention),
// into a grayscale image. For 1-bit data, bit set = white unless mask inverts.
func decodeGrayReader(r io.Reader, w, h, bpc int, mask bool) (*image.Gray, error) {
	rowBytes := (w*bpc + 7) / 8
	data := make([]byte, rowBytes*h)
	if _, err := io.ReadFull(r, data); err != nil && err != io.ErrUnexpectedEOF {
		return nil, err
	}

	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		row := data[y*rowBytes : (y+1)*rowBytes]
		for x := 0; x < w; x++ {
			var v uint8
			if bpc == 1 {
				bit := (row[x/8] >> (7 - uint(x%8))) & 1
				// In PDF image data a 1-bit sample of 1 is white (max value);
				// an ImageMask paints where the sample is 0, so invert it so the
				// "ink" reads as black either way.
				if mask {
					bit ^= 1
				}
				if bit == 1 {
					v = 255
				}
			} else {
				v = row[x]
			}
			img.SetGray(x, y, color.Gray{Y: v})
		}
	}

	return img, nil
}

// decodeRGBReader reads 8-bit RGB samples (3 bytes/pixel, rows byte-aligned by
// construction) into an image.
func decodeRGBReader(r io.Reader, w, h int) image.Image {
	data := make([]byte, w*h*3)
	if _, err := io.ReadFull(r, data); err != nil && err != io.ErrUnexpectedEOF {
		return nil
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 3
			img.SetNRGBA(x, y, color.NRGBA{R: data[i], G: data[i+1], B: data[i+2], A: 255})
		}
	}

	return img
}

func atoiSub(m [][]byte) int {
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(string(m[1]))
	return n
}
