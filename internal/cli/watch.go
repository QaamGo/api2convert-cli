package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/clierr"
	"github.com/QaamGo/api2convert-cli/internal/run"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

func newWatchCmd() *cobra.Command {
	var (
		to          string
		outDir      string
		category    string
		password    string
		onConflict  string
		recursive   bool
		include     []string
		exclude     []string
		optionKVs   []string
		optionsFile string
	)
	c := &cobra.Command{
		Use:   "watch <dir>",
		Short: "Auto-convert files added to a folder",
		Long: "Watch a directory and convert each file added or written to it into --out-dir.\n" +
			"Runs until interrupted (Ctrl-C). Files present before starting are not converted.",
		Example: "  api2convert watch ./inbox --to pdf --out-dir ./done\n" +
			"  api2convert watch ./inbox --to webp --out-dir ./web --recursive --include '*.png'",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if to == "" {
				return &clierr.UsageError{Err: errors.New("specify --to <format>")}
			}
			opts, err := parseOptions(optionKVs, optionsFile)
			if err != nil {
				return &clierr.UsageError{Err: err}
			}
			oc, err := resolveOnConflict(onConflict, false, false)
			if err != nil {
				return &clierr.UsageError{Err: err}
			}
			cl, err := clientFrom(ctx)
			if err != nil {
				return err
			}

			cfg := run.WatchConfig{
				Dir:       args[0],
				Target:    to,
				OutDir:    outDir,
				Recursive: recursive,
				Include:   include,
				Exclude:   exclude,
				Options: run.Options{
					ConversionOptions: opts,
					Category:          category,
					Password:          password,
					OnConflict:        oc,
				},
			}

			if !gf.json {
				fmt.Fprintf(cmd.ErrOrStderr(), "Watching %s → %s  (Ctrl-C to stop)\n", args[0], to)
			}
			enc := json.NewEncoder(cmd.OutOrStdout())

			// The callback fires from concurrent debounce goroutines; serialize
			// its writes to the shared encoder/stdout.
			var mu sync.Mutex
			err = run.Watch(ctx, cl, cfg, func(res run.Result, rerr error) {
				mu.Lock()
				defer mu.Unlock()
				switch {
				case rerr != nil:
					if gf.json {
						_ = enc.Encode(map[string]any{"ok": false, "error": rerr.Error()})
					} else {
						msg, _ := clierr.Classify(rerr)
						fmt.Fprintf(cmd.ErrOrStderr(), "%s %s\n", ui.Cross(), firstLine(msg))
					}
				case gf.json:
					_ = enc.Encode(map[string]any{"ok": true, "input": res.Input, "output_path": res.Path, "skipped": res.Skipped})
				default:
					fmt.Fprintf(cmd.OutOrStdout(), "%s %s → %s\n", ui.Check(), res.Input, res.Path)
				}
			})
			if errors.Is(err, context.Canceled) {
				return nil // clean Ctrl-C shutdown
			}
			return err
		},
	}
	fl := c.Flags()
	fl.StringVarP(&to, "to", "t", "", "target format (required)")
	fl.StringVar(&outDir, "out-dir", "", "output directory")
	fl.StringVar(&category, "category", "", "disambiguate an ambiguous target")
	fl.StringVar(&password, "password", "", "protect outputs with a download password")
	fl.StringVar(&onConflict, "on-conflict", "skip", "when output exists: error|skip|overwrite|rename")
	fl.BoolVarP(&recursive, "recursive", "r", false, "watch subdirectories too")
	fl.StringArrayVar(&include, "include", nil, "only convert files matching this glob (repeatable)")
	fl.StringArrayVar(&exclude, "exclude", nil, "skip files matching this glob (repeatable)")
	fl.StringArrayVar(&optionKVs, "option", nil, "per-format option key=value (repeatable)")
	fl.StringVar(&optionsFile, "options-file", "", "JSON file of conversion options")
	_ = c.RegisterFlagCompletionFunc("to", completeTargets)
	_ = c.RegisterFlagCompletionFunc("category", completeCategories)
	return c
}
