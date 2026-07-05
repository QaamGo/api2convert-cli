package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Table is a simple elastic-column table rendered with text/tabwriter.
type Table struct {
	Headers []string
	Rows    [][]string
}

// Write renders the table to w.
func (t Table) Write(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	if len(t.Headers) > 0 {
		fmt.Fprintln(tw, strings.Join(t.Headers, "\t"))
	}
	for _, row := range t.Rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}
