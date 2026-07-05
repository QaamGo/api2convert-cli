// Package ui handles terminal detection, colored output gating, spinners and
// interactive prompts. All animation goes to stderr so stdout stays clean.
package ui

import (
	"io"
	"os"

	"golang.org/x/term"
)

// NoColor is set from --no-color; combined with NO_COLOR / dumb terminals it
// suppresses all ANSI styling.
var NoColor bool

// IsTTY reports whether w is an interactive terminal.
func IsTTY(w any) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// Interactive reports whether we may draw interactive UI to w: it must be a TTY,
// color/animation must not be disabled, and CI must not be set.
func Interactive(w io.Writer) bool {
	if NoColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || os.Getenv("CI") != "" {
		return false
	}
	return IsTTY(w)
}
