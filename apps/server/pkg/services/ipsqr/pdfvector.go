package ipsqr

import (
	"bytes"
	"image"
	"image/color"
	"io"
	"math"
	"strconv"

	"github.com/ledongthuc/pdf"
	"golang.org/x/image/vector"
)

// This file rasterises a PDF page's *vector* fill paths into a bitmap, so a QR
// drawn as ~hundreds of filled square paths (rather than embedded as a raster
// image) can still be located and decoded. Some providers — eUpravnik among them
// — render the IPS QR this way, and carveImages (which only pulls raster image
// XObjects) never sees it.
//
// It is a deliberately partial PDF interpreter: it tracks the graphics-state CTM
// (q/Q/cm), follows Form XObject invocations (Do), flattens Bézier curves, and
// paints only fills — dark fills become black on a white canvas. Text, images,
// strokes and clipping are ignored: the QR's finder patterns survive in the fill
// layer alone, which is all the decoder needs.

// vectorScale renders user-space points at this many device pixels each. A QR is
// only ~25-40 modules wide; a bill page is ~595pt, so 2x keeps modules several
// pixels wide without producing an enormous image.
const vectorScale = 2.0

// maxVectorPixels caps a rendered page so a pathological MediaBox cannot allocate
// an unbounded canvas.
const maxVectorPixels = 4000

// renderVectorPages rasterises each page's fill paths and returns one bitmap per
// page for the QR decoder to scan. A page that cannot be interpreted is skipped.
func renderVectorPages(data []byte) []image.Image {
	var out []image.Image

	defer func() { _ = recover() }() // a broken content stream must never panic the caller

	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}

	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		if img := renderPage(p); img != nil {
			out = append(out, img)
		}
	}

	return out
}

// renderPage rasterises a single page's fill paths.
func renderPage(p pdf.Page) (img image.Image) {
	defer func() {
		if recover() != nil {
			img = nil
		}
	}()

	x0, y0, x1, y1 := mediaBox(p)
	w := int(math.Ceil((x1 - x0) * vectorScale))
	h := int(math.Ceil((y1 - y0) * vectorScale))
	if w <= 0 || h <= 0 || w > maxVectorPixels || h > maxVectorPixels {
		return nil
	}

	// device maps a user-space point (already through the CTM) to a pixel,
	// flipping Y because PDF space is bottom-up and image space is top-down.
	device := func(x, y float64) (float32, float32) {
		return float32((x - x0) * vectorScale), float32((y1 - y) * vectorScale)
	}

	ras := vector.NewRasterizer(w, h)
	st := &interp{
		ctm:      matrix{1, 0, 0, 1, 0, 0},
		fillGray: 0, // PDF default fill colour is black
	}

	content := pageContent(p)
	st.run(content, p.Resources(), ras, device, 0)

	// Rasterise every collected dark subpath in one pass onto a white canvas.
	dst := image.NewGray(image.Rect(0, 0, w, h))
	for i := range dst.Pix {
		dst.Pix[i] = 255
	}
	ras.Draw(dst, dst.Bounds(), image.NewUniform(color.Gray{Y: 0}), image.Point{})

	return dst
}

// matrix is a 2x3 affine transform [a b c d e f] mapping (x,y) ->
// (a*x + c*y + e, b*x + d*y + f).
type matrix struct{ a, b, c, d, e, f float64 }

func (m matrix) mul(n matrix) matrix {
	return matrix{
		a: m.a*n.a + m.b*n.c,
		b: m.a*n.b + m.b*n.d,
		c: m.c*n.a + m.d*n.c,
		d: m.c*n.b + m.d*n.d,
		e: m.e*n.a + m.f*n.c + n.e,
		f: m.e*n.b + m.f*n.d + n.f,
	}
}

func (m matrix) apply(x, y float64) (float64, float64) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

// interp is the running graphics state of the content interpreter.
type interp struct {
	ctm      matrix
	stack    []matrix
	fillGray float64
	// path is the current path as a list of subpaths, each a list of device
	// pixels (post-CTM, pre-Y-flip user space actually — stored in user space and
	// flipped at rasterise time via the device func).
	path [][]point
	cur  []point
	// startX/startY track the current subpath start for h (closepath).
	penX, penY float64
}

type point struct{ x, y float64 }

// run interprets a content stream, painting dark fills into ras. depth guards
// against pathological Form XObject recursion.
func (s *interp) run(content []byte, resources pdf.Value, ras *vector.Rasterizer, device func(x, y float64) (float32, float32), depth int) {
	if depth > 12 {
		return
	}

	toks := tokenize(content)
	var stack []token

	numAt := func(back int) float64 {
		idx := len(stack) - back
		if idx < 0 || idx >= len(stack) {
			return 0
		}
		return stack[idx].num
	}

	for _, tk := range toks {
		if !tk.isOp {
			stack = append(stack, tk)
			continue
		}

		switch tk.op {
		case "q":
			s.stack = append(s.stack, s.ctm)
		case "Q":
			if n := len(s.stack); n > 0 {
				s.ctm = s.stack[n-1]
				s.stack = s.stack[:n-1]
			}
		case "cm":
			m := matrix{numAt(6), numAt(5), numAt(4), numAt(3), numAt(2), numAt(1)}
			s.ctm = m.mul(s.ctm)
		case "m":
			s.flushSub()
			x, y := s.ctm.apply(numAt(2), numAt(1))
			s.penX, s.penY = x, y
			s.cur = []point{{x, y}}
		case "l":
			x, y := s.ctm.apply(numAt(2), numAt(1))
			s.cur = append(s.cur, point{x, y})
			s.penX, s.penY = x, y
		case "c":
			s.bezier(numAt(6), numAt(5), numAt(4), numAt(3), numAt(2), numAt(1))
		case "v":
			// first control point = current point
			s.bezierV(numAt(4), numAt(3), numAt(2), numAt(1))
		case "y":
			s.bezierY(numAt(4), numAt(3), numAt(2), numAt(1))
		case "re":
			s.rect(numAt(4), numAt(3), numAt(2), numAt(1))
		case "h":
			s.closeSub()
		case "g":
			s.fillGray = numAt(1)
		case "rg":
			s.fillGray = lum(numAt(3), numAt(2), numAt(1))
		case "k":
			// CMYK -> approximate gray
			cC, cM, cY, cK := numAt(4), numAt(3), numAt(2), numAt(1)
			s.fillGray = (1 - cC) * (1 - cM) * (1 - cY) * (1 - cK)
		case "sc", "scn":
			switch {
			case len(stack) >= 3:
				s.fillGray = lum(numAt(3), numAt(2), numAt(1))
			case len(stack) >= 1:
				s.fillGray = numAt(1)
			}
		case "f", "F", "f*", "b", "b*", "B", "B*":
			s.fill(ras, device)
			s.reset()
		case "n", "S", "s":
			s.reset()
		case "Do":
			if len(stack) >= 1 {
				s.doXObject(stack[len(stack)-1].name, resources, ras, device, depth)
			}
		}

		stack = stack[:0]
	}
}

// bezier flattens a cubic Bézier (control points in user space) from the current
// pen into line segments.
func (s *interp) bezier(x1, y1, x2, y2, x3, y3 float64) {
	p0 := point{s.penX, s.penY}
	c1x, c1y := s.ctm.apply(x1, y1)
	c2x, c2y := s.ctm.apply(x2, y2)
	ex, ey := s.ctm.apply(x3, y3)
	s.flatten(p0, point{c1x, c1y}, point{c2x, c2y}, point{ex, ey})
}

func (s *interp) bezierV(x2, y2, x3, y3 float64) {
	p0 := point{s.penX, s.penY}
	c2x, c2y := s.ctm.apply(x2, y2)
	ex, ey := s.ctm.apply(x3, y3)
	s.flatten(p0, p0, point{c2x, c2y}, point{ex, ey})
}

func (s *interp) bezierY(x1, y1, x3, y3 float64) {
	p0 := point{s.penX, s.penY}
	c1x, c1y := s.ctm.apply(x1, y1)
	ex, ey := s.ctm.apply(x3, y3)
	s.flatten(p0, point{c1x, c1y}, point{ex, ey}, point{ex, ey})
}

func (s *interp) flatten(p0, c1, c2, p3 point) {
	const steps = 8
	for i := 1; i <= steps; i++ {
		t := float64(i) / steps
		mt := 1 - t
		x := mt*mt*mt*p0.x + 3*mt*mt*t*c1.x + 3*mt*t*t*c2.x + t*t*t*p3.x
		y := mt*mt*mt*p0.y + 3*mt*mt*t*c1.y + 3*mt*t*t*c2.y + t*t*t*p3.y
		s.cur = append(s.cur, point{x, y})
	}
	s.penX, s.penY = p3.x, p3.y
}

// rect adds an axis-aligned rectangle subpath (the `re` operator).
func (s *interp) rect(x, y, w, h float64) {
	s.flushSub()
	p0x, p0y := s.ctm.apply(x, y)
	p1x, p1y := s.ctm.apply(x+w, y)
	p2x, p2y := s.ctm.apply(x+w, y+h)
	p3x, p3y := s.ctm.apply(x, y+h)
	s.cur = []point{{p0x, p0y}, {p1x, p1y}, {p2x, p2y}, {p3x, p3y}}
	s.flushSub()
	s.penX, s.penY = p0x, p0y
}

func (s *interp) closeSub() { s.flushSub() }

// flushSub moves the in-progress subpath into the path list.
func (s *interp) flushSub() {
	if len(s.cur) > 1 {
		s.path = append(s.path, s.cur)
	}
	s.cur = nil
}

// fill rasterises the current path onto ras when the fill colour is dark enough
// to read as QR ink.
func (s *interp) fill(ras *vector.Rasterizer, device func(x, y float64) (float32, float32)) {
	s.flushSub()
	if s.fillGray > 0.5 {
		return // light fill: leave the white canvas as-is
	}

	for _, sub := range s.path {
		if len(sub) < 2 {
			continue
		}
		x, y := device(sub[0].x, sub[0].y)
		ras.MoveTo(x, y)
		for _, p := range sub[1:] {
			px, py := device(p.x, p.y)
			ras.LineTo(px, py)
		}
		ras.ClosePath()
	}
}

func (s *interp) reset() {
	s.path = nil
	s.cur = nil
}

// doXObject paints a referenced Form XObject by interpreting its content stream
// under the current CTM (times the form's own Matrix). Image XObjects are left
// to carveImages.
func (s *interp) doXObject(name string, resources pdf.Value, ras *vector.Rasterizer, device func(x, y float64) (float32, float32), depth int) {
	xobjects := resources.Key("XObject")
	form := xobjects.Key(name)
	if form.IsNull() || form.Key("Subtype").Name() != "Form" {
		return
	}

	saved := s.ctm
	savedStack := s.stack
	if m, ok := readMatrix(form.Key("Matrix")); ok {
		s.ctm = m.mul(s.ctm)
	}

	formRes := form.Key("Resources")
	if formRes.IsNull() {
		formRes = resources
	}

	content := streamBytes(form)
	s.run(content, formRes, ras, device, depth+1)

	s.ctm = saved
	s.stack = savedStack
}

// --- helpers -------------------------------------------------------------

func lum(r, g, b float64) float64 { return 0.299*r + 0.587*g + 0.114*b }

func mediaBox(p pdf.Page) (x0, y0, x1, y1 float64) {
	mb := p.V.Key("MediaBox")
	if mb.Len() == 4 {
		return mb.Index(0).Float64(), mb.Index(1).Float64(), mb.Index(2).Float64(), mb.Index(3).Float64()
	}
	return 0, 0, 612, 792 // US Letter fallback
}

func readMatrix(v pdf.Value) (matrix, bool) {
	if v.Len() != 6 {
		return matrix{}, false
	}
	return matrix{
		v.Index(0).Float64(), v.Index(1).Float64(), v.Index(2).Float64(),
		v.Index(3).Float64(), v.Index(4).Float64(), v.Index(5).Float64(),
	}, true
}

// pageContent concatenates a page's content stream(s).
func pageContent(p pdf.Page) []byte {
	c := p.V.Key("Contents")
	if c.Kind() == pdf.Array {
		var buf bytes.Buffer
		for i := 0; i < c.Len(); i++ {
			buf.Write(streamBytes(c.Index(i)))
			buf.WriteByte('\n')
		}
		return buf.Bytes()
	}
	return streamBytes(c)
}

// streamBytes returns a stream Value's decoded bytes, recovering from the pdf
// reader's panic on filters it does not support.
func streamBytes(v pdf.Value) (out []byte) {
	defer func() {
		if recover() != nil {
			out = nil
		}
	}()

	rc := v.Reader()
	defer rc.Close() //nolint:errcheck

	b, err := io.ReadAll(rc)
	if err != nil {
		return nil
	}
	return b
}

// token is one lexed content-stream token: a number, a name (/Foo), a string, or
// an operator.
type token struct {
	op   string
	name string
	num  float64
	isOp bool
}

// tokenize splits a content stream into numbers, names and operators. Strings,
// dictionaries and inline-image data are skipped since fills never depend on
// them; this keeps the lexer small without misreading path/paint operators.
func tokenize(b []byte) []token {
	var toks []token
	i := 0
	n := len(b)

	for i < n {
		ch := b[i]
		switch {
		case ch == '%':
			for i < n && b[i] != '\n' && b[i] != '\r' {
				i++
			}
		case isWS(ch):
			i++
		case ch == '/':
			j := i + 1
			for j < n && !isDelim(b[j]) && !isWS(b[j]) {
				j++
			}
			toks = append(toks, token{name: string(b[i+1 : j])})
			i = j
		case ch == '(':
			// skip balanced string literal
			depth := 1
			i++
			for i < n && depth > 0 {
				switch b[i] {
				case '\\':
					i++
				case '(':
					depth++
				case ')':
					depth--
				}
				i++
			}
		case ch == '<':
			// hex string or dict; skip to matching close, dictionaries included
			if i+1 < n && b[i+1] == '<' {
				i += 2
				for i+1 < n && (b[i] != '>' || b[i+1] != '>') {
					i++
				}
				i += 2
			} else {
				for i < n && b[i] != '>' {
					i++
				}
				i++
			}
		case ch == '[' || ch == ']' || ch == '{' || ch == '}':
			i++
		case ch == '+' || ch == '-' || ch == '.' || (ch >= '0' && ch <= '9'):
			j := i
			for j < n && (b[j] == '+' || b[j] == '-' || b[j] == '.' || (b[j] >= '0' && b[j] <= '9') || b[j] == 'e' || b[j] == 'E') {
				j++
			}
			f, err := strconv.ParseFloat(string(b[i:j]), 64)
			if err == nil {
				toks = append(toks, token{num: f})
			}
			i = j
		default:
			j := i
			for j < n && !isWS(b[j]) && !isDelim(b[j]) {
				j++
			}
			opStr := string(b[i:j])
			if opStr == "" {
				i++
				continue
			}
			toks = append(toks, token{op: opStr, isOp: true})
			i = j
		}
	}

	return toks
}

func isWS(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' || c == 0
}

func isDelim(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}
