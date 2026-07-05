package cli

import "github.com/spf13/cobra"

// newBatchCmd is a bulk-oriented alias of convert: same flags and pipeline, but
// documented for converting whole folders / globs (typically with --recursive).
func newBatchCmd() *cobra.Command {
	var f convertFlags
	c := &cobra.Command{
		Use:   "batch <glob|dir>...",
		Short: "Bulk-convert many files or directories to one target",
		Example: "  api2convert batch ./images --to webp --out-dir out/ --recursive\n" +
			"  api2convert batch *.docx --to pdf --out-dir pdfs/",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConvert(cmd, args, f)
		},
	}
	addConvertFlags(c, &f)
	return c
}
