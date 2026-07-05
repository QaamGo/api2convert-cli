package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/output"
)

func newCreditsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "credits",
		Short: "Show your remaining conversion credits and plan",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, err := clientFrom(cmd.Context())
			if err != nil {
				return err
			}
			data, err := cl.Contracts().Get(cmd.Context())
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), gf.json, freeformView{data})
		},
	}
}

// freeformView renders an arbitrary API response: a flat key/value table for a
// top-level object, or pretty JSON otherwise.
type freeformView struct{ data any }

func (v freeformView) JSON() any { return v.data }

func (v freeformView) Human(w io.Writer) error {
	if m, ok := v.data.(map[string]any); ok && len(m) > 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t := output.Table{Headers: []string{"FIELD", "VALUE"}}
		for _, k := range keys {
			t.Rows = append(t.Rows, []string{k, fmt.Sprintf("%v", m[k])})
		}
		return t.Write(w)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v.data)
}
