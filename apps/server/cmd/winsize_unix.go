//go:build unix

package cmd

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// terminalWidth returns the terminal's column count, read from the tty via
// TIOCGWINSZ. It falls back to $COLUMNS and finally to a conventional 80 when the
// size cannot be determined (e.g. output is not a terminal).
func terminalWidth() int {
	if ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ); err == nil && ws.Col > 0 {
		return int(ws.Col)
	}
	if c, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && c > 0 {
		return c
	}

	return 80
}
