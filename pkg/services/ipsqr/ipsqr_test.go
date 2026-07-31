package ipsqr

import (
	"bytes"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readSampleBill loads a real-bill fixture, skipping the test when it is absent.
// The fixtures carry personal data and are gitignored, so they exist only on a
// machine that has actually downloaded bills — locally they give strong
// end-to-end coverage; in CI the test simply skips.
func readSampleBill(t *testing.T, name string) []byte {
	t.Helper()

	pdf, err := os.ReadFile(filepath.Join("testdata", name))
	if os.IsNotExist(err) {
		t.Skipf("fixture %s not present (gitignored real bill); skipping", name)
	}
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}

	return pdf
}

// TestExtractFromRealBills runs extraction against the sample bills, locking in
// per-provider coverage:
//   - mts: CCITT-G4 image XObject
//   - esanduce: Flate/PNG-predictor 1-bit image XObject
//   - eupravnik: vector-drawn QR (no raster)
//   - eps: inline BI/ID/EI ASCII85+Flate image in the content stream
func TestExtractFromRealBills(t *testing.T) {
	cases := map[string]struct {
		wantQR      bool
		wantAccount string // "R:" field, present only when wantQR
	}{
		"mts.pdf":       {wantQR: true, wantAccount: "160000000000060216"},
		"esanduce.pdf":  {wantQR: true, wantAccount: "200220618010100048"},
		"eupravnik.pdf": {wantQR: true, wantAccount: "200372566010184426"},
		"eps.pdf":       {wantQR: true, wantAccount: "190000000009987010"},
	}

	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			pdf := readSampleBill(t, name)

			payload, ok := Extract(pdf)
			t.Logf("%s: ipsQR=%v payload=%q", name, ok, payload)

			if ok != want.wantQR {
				t.Fatalf("%s: Extract ok = %v, want %v", name, ok, want.wantQR)
			}
			if !want.wantQR {
				return
			}
			if !IsIPSPayload(payload) {
				t.Fatalf("%s: payload is not valid IPS: %q", name, payload)
			}
			if !strings.Contains(payload, "|R:"+want.wantAccount) {
				t.Errorf("%s: payload missing account R:%s\n got: %q", name, want.wantAccount, payload)
			}
		})
	}
}

// TestRenderRoundTrip re-encodes a real extracted payload and decodes it back,
// proving the rendered QR carries the exact payment string the bill did.
func TestRenderRoundTrip(t *testing.T) {
	pdf := readSampleBill(t, "mts.pdf")
	payload, ok := Extract(pdf)
	if !ok {
		t.Fatal("expected mts.pdf to yield an IPS QR")
	}

	png, err := RenderPNG(payload, DefaultSize)
	if err != nil {
		t.Fatalf("RenderPNG: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(png))
	if err != nil {
		t.Fatalf("decode rendered PNG: %v", err)
	}
	got, err := decodeQR(img)
	if err != nil {
		t.Fatalf("decode rendered QR: %v", err)
	}
	if strings.TrimSpace(got) != strings.TrimSpace(payload) {
		t.Errorf("round-trip mismatch:\n got: %q\nwant: %q", got, payload)
	}
}

func TestIsIPSPayload(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"K:PR|V:01|C:1|R:845000000040484284|N:EPS SNABDEVANJE|I:RSD1234,56|SF:189|S:Racun", true},
		{"k:pr|v:01|r:123", true},
		{"https://a1.rs/pay/123", false},
		{"", false},
		{"K:PR|V:01|C:1|N:no account field", false},
	}
	for _, c := range cases {
		if got := IsIPSPayload(c.in); got != c.want {
			t.Errorf("IsIPSPayload(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
