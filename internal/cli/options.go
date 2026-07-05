package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/output"
)

func newOptionsCmd() *cobra.Command {
	var category string
	c := &cobra.Command{
		Use:   "options <target>",
		Short: "Show the options a target format accepts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := clientFrom(cmd.Context())
			if err != nil {
				return err
			}
			var schema map[string]any
			if category != "" {
				schema, err = cl.Options(cmd.Context(), args[0], category)
			} else {
				schema, err = cl.Options(cmd.Context(), args[0])
			}
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), gf.json, optionsView(schema))
		},
	}
	c.Flags().StringVar(&category, "category", "", "disambiguate an ambiguous target")
	return c
}

type optionsView map[string]any

func (v optionsView) Human(w io.Writer) error {
	if len(v) == 0 {
		fmt.Fprintln(w, "No options for this target.")
		return nil
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	t := output.Table{Headers: []string{"OPTION", "TYPE", "DEFAULT", "ALLOWED"}}
	for _, k := range keys {
		spec, _ := v[k].(map[string]any)
		t.Rows = append(t.Rows, []string{k, optType(spec), optDefault(spec), optAllowed(spec)})
	}
	return t.Write(w)
}

func (v optionsView) JSON() any { return map[string]any(v) }

func optType(spec map[string]any) string {
	if spec == nil {
		return ""
	}
	return fmt.Sprintf("%v", firstOf(spec, "type"))
}

func optDefault(spec map[string]any) string {
	if spec == nil {
		return ""
	}
	if d := firstOf(spec, "default"); d != nil {
		return fmt.Sprintf("%v", d)
	}
	return ""
}

func optAllowed(spec map[string]any) string {
	if spec == nil {
		return ""
	}
	if enum := firstOf(spec, "enum", "allowed", "values"); enum != nil {
		if list, ok := enum.([]any); ok {
			parts := make([]string, 0, len(list))
			for _, e := range list {
				parts = append(parts, fmt.Sprintf("%v", e))
			}
			return strings.Join(parts, "|")
		}
	}
	min := firstOf(spec, "min")
	max := firstOf(spec, "max")
	if min != nil || max != nil {
		return fmt.Sprintf("%v–%v", orDash(min), orDash(max))
	}
	return ""
}

func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func orDash(v any) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%v", v)
}
