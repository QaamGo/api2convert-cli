// Package output renders command results either as friendly human text or as
// stable machine JSON, from a single view value per command.
package output

import (
	"encoding/json"
	"io"
)

// Renderer produces both a human rendering and a JSON-serializable value.
type Renderer interface {
	Human(w io.Writer) error
	JSON() any
}

// Emit writes r to out as JSON when jsonMode is set, else as human text.
func Emit(out io.Writer, jsonMode bool, r Renderer) error {
	if jsonMode {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(r.JSON())
	}
	return r.Human(out)
}
