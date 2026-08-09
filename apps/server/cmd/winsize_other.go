//go:build !unix

package cmd

import (
	"os"
	"strconv"
)

// terminalWidth returns the terminal's column count on platforms without the
// TIOCGWINSZ ioctl, from $COLUMNS with a conventional 80-column fallback.
func terminalWidth() int {
	if c, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && c > 0 {
		return c
	}

	return 80
}
