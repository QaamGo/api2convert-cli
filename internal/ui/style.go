package ui

import "os"

func colorize(code, s string) string {
	if NoColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

// Check is a green success glyph.
func Check() string { return colorize("32", "✓") }

// Cross is a red failure glyph.
func Cross() string { return colorize("31", "✗") }

// Success formats a success line.
func Success(s string) string { return Check() + " " + s }

// Failure formats an error line.
func Failure(s string) string { return Cross() + " " + s }

// Dim renders faint secondary text.
func Dim(s string) string { return colorize("2", s) }

// Bold renders emphasized text.
func Bold(s string) string { return colorize("1", s) }
