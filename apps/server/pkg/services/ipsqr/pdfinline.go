package ipsqr

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"encoding/hex"
	"image"
	"image/jpeg"
	"io"
	"regexp"

	"github.com/ledongthuc/pdf"
)

// This file handles QRs embedded as *inline images* — the BI/ID/EI operators in
// a content stream, where the image bytes live inline rather than as a separate
// XObject. EPS bills draw the IPS QR this way (a 1-bit ASCII85+Flate image), so
// neither carveImages (XObject rasters) nor the vector rasteriser finds it.
//
// It also carries sanitizePDF, because the same EPS bills append junk after the
// final %%EOF, which makes the ledongthuc reader refuse the file outright.

// eofMarker ends a well-formed PDF; anything after the last one is trailing junk.
var eofMarker = []byte("%%EOF")

// sanitizePDF trims trailing bytes after the final %%EOF. Some providers (EPS)
// emit a few hundred bytes of garbage past the end, which makes the PDF reader
// reject the whole file; cutting to the last %%EOF recovers it without touching
// any real content (incremental-update PDFs keep every %%EOF up to the last).
func sanitizePDF(data []byte) []byte {
	if idx := bytes.LastIndex(data, eofMarker); idx >= 0 {
		return data[:idx+len(eofMarker)]
	}
	return data
}

// openPDF sanitises then opens a PDF for the ledongthuc reader.
func openPDF(data []byte) (*pdf.Reader, bool) {
	clean := sanitizePDF(data)
	r, err := pdf.NewReader(bytes.NewReader(clean), int64(len(clean)))
	if err != nil {
		return nil, false
	}
	return r, true
}

// contentStreams returns every page and Form-XObject content stream in the PDF,
// decoded. Inline images are scanned out of these.
func contentStreams(data []byte) (streams [][]byte) {
	defer func() { _ = recover() }() // a broken PDF must never panic the caller

	r, ok := openPDF(data)
	if !ok {
		return nil
	}

	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		if c := pageContent(p); len(c) > 0 {
			streams = append(streams, c)
		}
		collectFormContents(p.Resources(), &streams, 0)
	}

	return streams
}

// collectFormContents appends the content stream of every Form XObject reachable
// from resources, recursively (bounded), so an inline image nested in a form is
// still found.
func collectFormContents(resources pdf.Value, out *[][]byte, depth int) {
	if depth > 8 || resources.IsNull() {
		return
	}

	xobjects := resources.Key("XObject")
	for _, name := range xobjects.Keys() {
		form := xobjects.Key(name)
		if form.Key("Subtype").Name() != "Form" {
			continue
		}
		if c := streamBytes(form); len(c) > 0 {
			*out = append(*out, c)
		}
		collectFormContents(form.Key("Resources"), out, depth+1)
	}
}

var (
	reInlineW   = regexp.MustCompile(`/W(?:idth)?\s+(\d+)`)
	reInlineH   = regexp.MustCompile(`/H(?:eight)?\s+(\d+)`)
	reInlineBPC = regexp.MustCompile(`/(?:BPC|BitsPerComponent)\s+(\d+)`)
	reInlineIM  = regexp.MustCompile(`/(?:IM|ImageMask)\s+true`)
	reInlineF   = regexp.MustCompile(`/(?:F|Filter)\s*(\[[^\]]*\]|/\w+)`)
	reFilterTok = regexp.MustCompile(`/(A85|ASCII85Decode|AHx|ASCIIHexDecode|Fl|FlateDecode|RL|RunLengthDecode|CCF|CCITTFaxDecode|DCT|DCTDecode|LZW|LZWDecode)`)
)

// carveInlineImages finds BI/ID/EI inline images in a content stream and returns
// the ones it can decode as bitmaps.
func carveInlineImages(content []byte) []image.Image {
	var out []image.Image

	i := 0
	for i < len(content) {
		bi := findOperator(content, "BI", i)
		if bi < 0 {
			break
		}
		id := findOperator(content, "ID", bi+2)
		if id < 0 {
			break
		}

		dict := content[bi+2 : id]
		// The image data starts one whitespace byte after "ID".
		dataStart := id + 2
		if dataStart < len(content) && isWS(content[dataStart]) {
			dataStart++
		}

		end, next := inlineDataEnd(content, dataStart, dict)
		if end < dataStart {
			break
		}

		if img := decodeInlineImage(dict, content[dataStart:end]); img != nil {
			out = append(out, img)
		}
		i = next
	}

	return out
}

// findOperator returns the index of a standalone operator token (bounded by
// whitespace/delimiters), so "BI"/"ID" are not matched inside names or numbers.
func findOperator(b []byte, op string, from int) int {
	for i := from; i+len(op) <= len(b); i++ {
		if string(b[i:i+len(op)]) != op {
			continue
		}
		beforeOK := i == 0 || isWS(b[i-1]) || isDelim(b[i-1])
		after := i + len(op)
		afterOK := after >= len(b) || isWS(b[after]) || isDelim(b[after])
		if beforeOK && afterOK {
			return i
		}
	}
	return -1
}

// inlineDataEnd locates the end of an inline image's data and the index just past
// its EI operator. For ASCII filters the encoded data has a definite terminator
// (~> or >); otherwise it falls back to the first whitespace-bounded EI.
func inlineDataEnd(content []byte, start int, dict []byte) (end, next int) {
	filters := inlineFilters(dict)
	first := ""
	if len(filters) > 0 {
		first = filters[0]
	}

	switch first {
	case "A85", "ASCII85Decode":
		if p := bytes.Index(content[start:], []byte("~>")); p >= 0 {
			e := start + p + 2
			return e, skipToAfterEI(content, e)
		}
	case "AHx", "ASCIIHexDecode":
		if p := bytes.IndexByte(content[start:], '>'); p >= 0 {
			e := start + p + 1
			return e, skipToAfterEI(content, e)
		}
	}

	// Fallback: first EI that is whitespace/delimiter bounded.
	if ei := findOperator(content, "EI", start); ei >= 0 {
		e := ei
		for e > start && isWS(content[e-1]) {
			e--
		}
		return e, ei + 2
	}

	return len(content), len(content)
}

func skipToAfterEI(content []byte, from int) int {
	if ei := findOperator(content, "EI", from); ei >= 0 {
		return ei + 2
	}
	return len(content)
}

// inlineFilters returns the inline image's filter chain in decode order, using
// canonical short names.
func inlineFilters(dict []byte) []string {
	m := reInlineF.FindSubmatch(dict)
	if m == nil {
		return nil
	}
	var filters []string
	for _, tok := range reFilterTok.FindAllSubmatch(m[1], -1) {
		filters = append(filters, string(tok[1]))
	}
	return filters
}

// decodeInlineImage decodes one inline image's dict + data into a bitmap, or nil
// when its format is unsupported.
func decodeInlineImage(dict, data []byte) image.Image {
	w := atoiSub(reInlineW.FindSubmatch(dict))
	h := atoiSub(reInlineH.FindSubmatch(dict))
	if w == 0 || h == 0 {
		return nil
	}

	bpc := atoiSub(reInlineBPC.FindSubmatch(dict))
	mask := reInlineIM.Match(dict)
	if mask {
		bpc = 1
	}
	if bpc == 0 {
		bpc = 8
	}

	// Apply the filter chain in order; the trailing filter is the image codec.
	samples := data
	filters := inlineFilters(dict)
	for idx, f := range filters {
		last := idx == len(filters)-1

		switch f {
		case "A85", "ASCII85Decode":
			samples = decodeA85(samples)
		case "AHx", "ASCIIHexDecode":
			samples = decodeAHx(samples)
		case "Fl", "FlateDecode":
			samples = inflate(samples)
		case "RL", "RunLengthDecode":
			samples = runLengthDecode(samples)
		case "CCF", "CCITTFaxDecode":
			if last {
				return decodeCCITT(dict, samples, w, h)
			}
			return nil
		case "DCT", "DCTDecode":
			if last {
				img, err := decodeJPEG(samples)
				if err != nil {
					return nil
				}
				return img
			}
			return nil
		default:
			return nil
		}
		if samples == nil {
			return nil
		}
	}

	switch bpc {
	case 1, 8:
		img, err := decodeGrayReader(bytes.NewReader(samples), w, h, bpc, mask)
		if err != nil {
			return nil
		}
		return img
	}

	return nil
}

func decodeJPEG(samples []byte) (image.Image, error) {
	return jpeg.Decode(bytes.NewReader(samples))
}

// --- filter primitives ---------------------------------------------------

func decodeA85(b []byte) []byte {
	if p := bytes.Index(b, []byte("~>")); p >= 0 {
		b = b[:p]
	}
	out, err := io.ReadAll(ascii85.NewDecoder(bytes.NewReader(b)))
	if err != nil {
		return out // partial is still worth a decode attempt
	}
	return out
}

func decodeAHx(b []byte) []byte {
	// Strip whitespace and the '>' terminator, then hex-decode.
	var clean []byte
	for _, c := range b {
		if c == '>' {
			break
		}
		if !isWS(c) {
			clean = append(clean, c)
		}
	}
	if len(clean)%2 == 1 {
		clean = append(clean, '0')
	}
	out := make([]byte, hex.DecodedLen(len(clean)))
	n, err := hex.Decode(out, clean)
	if err != nil {
		return out[:n]
	}
	return out[:n]
}

func inflate(b []byte) []byte {
	zr, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	defer zr.Close() //nolint:errcheck
	out, err := io.ReadAll(zr)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil
	}
	return out
}

// runLengthDecode reverses PDF RunLengthDecode (a PackBits variant): a length
// byte L in 0-127 means "copy the next L+1 bytes", 129-255 means "repeat the next
// byte 257-L times", and 128 is end-of-data.
func runLengthDecode(b []byte) []byte {
	var out []byte
	i := 0
	for i < len(b) {
		l := b[i]
		i++
		switch {
		case l == 128:
			return out
		case l < 128:
			n := int(l) + 1
			if i+n > len(b) {
				n = len(b) - i
			}
			out = append(out, b[i:i+n]...)
			i += n
		default:
			n := 257 - int(l)
			if i >= len(b) {
				return out
			}
			v := b[i]
			i++
			for k := 0; k < n; k++ {
				out = append(out, v)
			}
		}
	}
	return out
}
