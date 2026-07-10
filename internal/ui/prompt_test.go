package ui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name string
		in   string
		def  bool
		want bool
	}{
		{name: "y is yes", in: "y\n", def: false, want: true},
		{name: "yes is yes", in: "YES\n", def: false, want: true},
		{name: "n is no", in: "n\n", def: true, want: false},
		{name: "no is no", in: "No\n", def: true, want: false},
		{name: "empty line uses default (no)", in: "\n", def: false, want: false},
		{name: "empty line uses default (yes)", in: "\n", def: true, want: true},
		{name: "eof with nothing typed uses default", in: "", def: false, want: false},
		{name: "unrecognized answer uses default", in: "maybe\n", def: false, want: false},
		{name: "surrounding whitespace is trimmed", in: "  y  \n", def: false, want: true},
		{name: "no trailing newline still parses", in: "y", def: false, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			got := Confirm("Update? ", strings.NewReader(tt.in), &out, tt.def)
			if got != tt.want {
				t.Errorf("Confirm(%q, def=%v) = %v, want %v", tt.in, tt.def, got, tt.want)
			}
			if out.String() != "Update? " {
				t.Errorf("prompt = %q, want %q", out.String(), "Update? ")
			}
		})
	}
}

func TestMaskedInput(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
		wantOut string
	}{
		{name: "each char echoes a star", in: "abc\r", want: "abc", wantOut: "***\r\n"},
		{name: "newline also submits", in: "hello\n", want: "hello", wantOut: "*****\r\n"},
		{name: "DEL erases the last star", in: "ab\x7fc\r", want: "ac", wantOut: "**\b \b*\r\n"},
		{name: "backspace erases the last star", in: "ab\x08c\r", want: "ac", wantOut: "**\b \b*\r\n"},
		{name: "backspace on empty is a no-op", in: "\x7fa\r", want: "a", wantOut: "*\r\n"},
		{name: "ctrl-u clears the line", in: "ab\x15c\r", want: "c", wantOut: "**\b \b\b \b*\r\n"},
		{name: "ctrl-c aborts", in: "ab\x03", want: "", wantErr: ErrInterrupted, wantOut: "**\r\n"},
		{name: "ctrl-d on empty is EOF", in: "\x04", want: "", wantErr: io.EOF, wantOut: "\r\n"},
		{name: "ctrl-d with buffer submits it", in: "ab\x04", want: "ab", wantOut: "**\r\n"},
		{name: "eof without enter submits the buffer", in: "ab", want: "ab", wantOut: "**\r\n"},
		{name: "arrow keys are swallowed", in: "a\x1b[Db\r", want: "ab", wantOut: "**\r\n"},
		{name: "other control chars are ignored", in: "a\tb\r", want: "ab", wantOut: "**\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			got, err := maskedInput(bytes.NewReader([]byte(tt.in)), &out)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			case err != nil:
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tt.want {
				t.Errorf("value = %q, want %q", got, tt.want)
			}
			if out.String() != tt.wantOut {
				t.Errorf("echoed = %q, want %q", out.String(), tt.wantOut)
			}
		})
	}
}

// TestReadSecretPiped covers the non-terminal path: input is read as a single
// unmasked line (os.Pipe's read end is not a TTY).
func TestReadSecretPiped(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = io.WriteString(w, "my-secret-key\n")
		_ = w.Close()
	}()

	var out bytes.Buffer
	got, err := ReadSecret("Key: ", r, &out)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "my-secret-key" {
		t.Errorf("value = %q, want %q", got, "my-secret-key")
	}
	if out.String() != "Key: " {
		t.Errorf("prompt = %q, want %q", out.String(), "Key: ")
	}
}
