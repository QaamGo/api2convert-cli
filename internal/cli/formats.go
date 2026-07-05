package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/catalog"
	"github.com/QaamGo/api2convert-cli/internal/output"
)

func newFormatsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "formats",
		Short: "Explore supported conversions",
	}
	c.AddCommand(newFormatsListCmd(), newFormatsSearchCmd(), newFormatsCategoriesCmd())
	return c
}

func loadCatalog(cmd *cobra.Command, refresh bool) (*catalog.Catalog, error) {
	cl, err := clientFrom(cmd.Context())
	if err != nil {
		return nil, err
	}
	return catalog.Load(cmd.Context(), cl, refresh, catalogKey(cmd.Context()))
}

func newFormatsListCmd() *cobra.Command {
	var category string
	var refresh bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List supported target formats",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := loadCatalog(cmd, refresh)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), gf.json, conversionsView(cat.Filter(category)))
		},
	}
	c.Flags().StringVar(&category, "category", "", "filter by category")
	c.Flags().BoolVar(&refresh, "refresh", false, "bypass the local cache")
	return c
}

func newFormatsSearchCmd() *cobra.Command {
	var refresh bool
	c := &cobra.Command{
		Use:   "search <query>",
		Short: "Search targets and categories",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := loadCatalog(cmd, refresh)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), gf.json, conversionsView(cat.Search(args[0])))
		},
	}
	c.Flags().BoolVar(&refresh, "refresh", false, "bypass the local cache")
	return c
}

func newFormatsCategoriesCmd() *cobra.Command {
	var refresh bool
	c := &cobra.Command{
		Use:   "categories",
		Short: "List available categories",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cat, err := loadCatalog(cmd, refresh)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), gf.json, stringsView(cat.Categories()))
		},
	}
	c.Flags().BoolVar(&refresh, "refresh", false, "bypass the local cache")
	return c
}

type conversionsView []catalog.Conversion

func (v conversionsView) Human(w io.Writer) error {
	t := output.Table{Headers: []string{"CATEGORY", "TARGET"}}
	for _, cv := range v {
		t.Rows = append(t.Rows, []string{cv.Category, cv.Target})
	}
	return t.Write(w)
}

func (v conversionsView) JSON() any {
	out := make([]map[string]any, 0, len(v))
	for _, cv := range v {
		out = append(out, map[string]any{"id": cv.ID, "category": cv.Category, "target": cv.Target})
	}
	return out
}

type stringsView []string

func (v stringsView) Human(w io.Writer) error {
	for _, s := range v {
		fmt.Fprintln(w, s)
	}
	return nil
}

func (v stringsView) JSON() any { return []string(v) }
