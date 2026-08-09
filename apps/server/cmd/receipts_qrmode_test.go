package cmd

import "testing"

func TestParseQRModeOverride(t *testing.T) {
	cases := []struct {
		in     string
		want   qrMode
		forced bool
	}{
		{"inline", qrInline, true},
		{"image", qrFileOpen, true},
		{"file", qrFileOpen, true},
		{"open", qrFileOpen, true},
		{"path", qrFilePath, true},
		{"blocks", qrBlocks, true},
		{"ascii", qrBlocks, true},
		{"braille", qrBraille, true},
		{"compact", qrBraille, true},
		{"  INLINE  ", qrInline, true}, // trimmed and case-insensitive
		{"", 0, false},                 // unset → auto
		{"auto", 0, false},             // explicit auto
		{"nonsense", 0, false},         // unknown → auto, not an error
	}

	for _, c := range cases {
		got, forced := parseQRModeOverride(c.in)
		if forced != c.forced || (forced && got != c.want) {
			t.Errorf("parseQRModeOverride(%q) = (%v, %v), want (%v, %v)", c.in, got, forced, c.want, c.forced)
		}
	}
}

func TestAutoQRMode(t *testing.T) {
	cases := []struct {
		name                         string
		isTTY, inline, desktop, fits bool
		want                         qrMode
	}{
		{"piped output leaves a file", false, true, true, true, qrFilePath},
		{"inline terminal wins", true, true, true, true, qrInline},
		{"local desktop opens viewer", true, false, true, true, qrFileOpen},
		{"remote wide falls to blocks", true, false, false, true, qrBlocks},
		{"remote narrow falls to braille", true, false, false, false, qrBraille},
	}

	for _, c := range cases {
		if got := autoQRMode(c.isTTY, c.inline, c.desktop, c.fits); got != c.want {
			t.Errorf("%s: autoQRMode(%v,%v,%v,%v) = %v, want %v",
				c.name, c.isTTY, c.inline, c.desktop, c.fits, got, c.want)
		}
	}
}
