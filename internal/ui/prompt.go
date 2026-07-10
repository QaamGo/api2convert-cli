package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// ErrInterrupted is returned by ReadSecret when the user aborts the prompt with
// Ctrl-C. Because the terminal is in raw mode while reading, Ctrl-C does not
// raise a signal, so it is surfaced as this error instead. Callers should treat
// it as a clean cancellation.
var ErrInterrupted = errors.New("interrupted")

// Confirm writes prompt to out and reads a single yes/no line from in.
// "y"/"yes" is true and "n"/"no" is false (case-insensitive, surrounding
// whitespace ignored); an empty line, an unrecognized answer, or a read error /
// EOF all return def. That way just pressing Enter — or a piped/interrupted
// prompt with no clear answer — cleanly falls back to the default instead of
// blocking or failing.
func Confirm(prompt string, in io.Reader, out io.Writer, def bool) bool {
	fmt.Fprint(out, prompt)
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

// ReadSecret writes prompt to out, then reads a secret from in.
//
// When in is an interactive terminal the typed characters are not shown, but
// each keystroke echoes a '*' to out so the user gets feedback that the key
// registered (Backspace erases the last '*', Ctrl-U clears the line). When in
// is not a terminal (piped or redirected input) it reads a single line without
// masking. The returned value carries no trailing newline; trimming surrounding
// whitespace is left to the caller.
func ReadSecret(prompt string, in *os.File, out io.Writer) (string, error) {
	fmt.Fprint(out, prompt)

	fd := int(in.Fd())
	if !term.IsTerminal(fd) {
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && (!errors.Is(err, io.EOF) || line == "") {
			return "", err
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	return maskedInput(in, out)
}

// maskedInput reads a secret from r one byte at a time, echoing a '*' to out for
// each printable character. It honors Backspace/DEL (erase last char), Ctrl-U
// (clear line), Enter (submit), Ctrl-C (ErrInterrupted) and Ctrl-D (EOF: submit
// any buffered input, otherwise cancel), and swallows ANSI escape sequences such
// as arrow keys so they never leak into the value.
//
// r is expected to be a terminal already switched to raw mode; the function is
// decoupled from the terminal so it can be unit-tested with an in-memory reader.
// Because raw mode disables output post-processing, line breaks are written as
// an explicit "\r\n".
func maskedInput(r io.Reader, out io.Writer) (string, error) {
	var (
		buf      []byte
		one      = make([]byte, 1)
		escaping bool // inside an ANSI escape sequence (e.g. an arrow key)
		csi      bool // ...specifically a CSI ("ESC [") sequence
	)
	for {
		n, err := r.Read(one)
		if n == 0 {
			if err != nil {
				fmt.Fprint(out, "\r\n")
				if errors.Is(err, io.EOF) && len(buf) > 0 {
					return string(buf), nil
				}
				return "", err
			}
			continue
		}
		c := one[0]

		if escaping {
			switch {
			case csi:
				if c >= 0x40 && c <= 0x7e { // final byte ends a CSI sequence
					escaping, csi = false, false
				}
			case c == '[' || c == 'O':
				csi = true
			default: // a short ESC sequence (or lone ESC): consume one byte
				escaping = false
			}
			continue
		}

		switch c {
		case '\r', '\n': // Enter — submit
			fmt.Fprint(out, "\r\n")
			return string(buf), nil
		case 0x03: // Ctrl-C — abort
			fmt.Fprint(out, "\r\n")
			return "", ErrInterrupted
		case 0x04: // Ctrl-D — EOF (submit buffered input, else cancel)
			fmt.Fprint(out, "\r\n")
			if len(buf) > 0 {
				return string(buf), nil
			}
			return "", io.EOF
		case 0x08, 0x7f: // Backspace / DEL — erase the last character
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Fprint(out, "\b \b")
			}
		case 0x15: // Ctrl-U — clear the whole line
			for range buf {
				fmt.Fprint(out, "\b \b")
			}
			buf = buf[:0]
		case 0x1b: // ESC — start of an escape sequence; swallow it
			escaping = true
		default:
			if c >= 0x20 { // a printable byte
				buf = append(buf, c)
				fmt.Fprint(out, "*")
			}
		}
	}
}
