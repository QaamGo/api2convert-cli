// Package cli builds the cobra command tree and owns process-level error
// reporting and exit codes.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/clierr"
	"github.com/QaamGo/api2convert-cli/internal/config"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

// BuildInfo carries the injected build metadata.
type BuildInfo struct{ Version, Commit, Date string }

type globalFlags struct {
	apiKey        string
	baseURL       string
	timeout       string
	pollTimeout   string
	maxRetries    int
	output        string
	concurrency   int
	json          bool
	quiet         bool
	noColor       bool
	verbose       bool
	noUpdateCheck bool
}

var (
	gf        = globalFlags{maxRetries: -1}
	buildInfo BuildInfo
)

// Execute builds and runs the root command, returning the process exit code.
func Execute(ctx context.Context, bi BuildInfo) int {
	buildInfo = bi
	// Let a Windows Explorer double-click launch the tool: by default cobra's
	// mousetrap prints "open cmd.exe and run it from there" and exits, which
	// breaks the README's headline "double-click for the guided wizard". Clearing
	// the text disables the mousetrap; the no-arg TTY path runs the wizard, which
	// is exactly what a double-click user wants.
	cobra.MousetrapHelpText = ""
	root := newRootCmd()
	if err := root.ExecuteContext(ctx); err != nil {
		msg, code := clierr.Classify(err)
		var coded *clierr.Coded
		alreadyRendered := errors.As(err, &coded) // command printed its own output
		switch {
		case gf.json && !alreadyRendered:
			clierr.WriteJSONError(os.Stdout, err, code)
		case !gf.json && msg != "":
			fmt.Fprintln(os.Stderr, ui.Failure(msg))
			if gf.verbose {
				fmt.Fprintf(os.Stderr, "\n%+v\n", err)
			}
		}
		return int(code)
	}
	return 0
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "api2convert",
		Short:         "Convert, compress and transform files from the command line",
		Long:          "api2convert — convert, compress and transform files using the api2convert API.\n\nRun with no arguments in a terminal to launch the guided wizard, or use\n'api2convert convert <file> --to <format>' directly.",
		Version:       buildInfo.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// Set color gating first, so even an early config error prints cleanly
			// when stdout is not a terminal.
			ui.NoColor = gf.noColor || !ui.IsTTY(os.Stdout)

			fileCfg, err := config.Load()
			if err != nil {
				return err
			}
			res, err := config.Resolve(fileCfg, config.Flags{
				APIKey:      gf.apiKey,
				BaseURL:     gf.baseURL,
				Timeout:     gf.timeout,
				PollTimeout: gf.pollTimeout,
				MaxRetries:  gf.maxRetries,
				Output:      gf.output,
				Concurrency: gf.concurrency,
			}, os.Getenv)
			if err != nil {
				return &clierr.UsageError{Err: err}
			}
			if res.Output == "json" {
				gf.json = true
			}
			cmd.SetContext(withResolved(cmd.Context(), res))
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			// After a successful command, in an interactive session, check for a
			// newer release at most once a week and offer to update. Never fails
			// the command the user ran.
			maybePromptUpdate(cmd)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if ui.IsTTY(os.Stdin) && ui.IsTTY(os.Stdout) {
				return runWizard(cmd)
			}
			return cmd.Help()
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&gf.apiKey, "api-key", "", "API key (overrides env and config)")
	pf.StringVar(&gf.baseURL, "base-url", "", "API base URL")
	pf.StringVar(&gf.timeout, "timeout", "", "per-request network timeout (e.g. 30s)")
	pf.StringVar(&gf.pollTimeout, "poll-timeout", "", "conversion poll timeout (e.g. 5m)")
	pf.IntVar(&gf.maxRetries, "max-retries", -1, "retries for transient failures")
	pf.StringVar(&gf.output, "output", "", "output format: human|json")
	pf.IntVar(&gf.concurrency, "concurrency", 0, "parallel conversions (0 = auto)")
	pf.BoolVar(&gf.json, "json", false, "machine-readable JSON output")
	pf.BoolVarP(&gf.quiet, "quiet", "q", false, "suppress progress and chatter")
	pf.BoolVar(&gf.noColor, "no-color", false, "disable colored output")
	pf.BoolVarP(&gf.verbose, "verbose", "v", false, "verbose output")
	pf.BoolVar(&gf.noUpdateCheck, "no-update-check", false, "don't check for a newer release")

	root.SetFlagErrorFunc(func(_ *cobra.Command, e error) error {
		return &clierr.UsageError{Err: e}
	})

	root.AddCommand(
		newVersionCmd(),
		newLoginCmd(),
		newConfigCmd(),
		newConvertCmd(),
		newBatchCmd(),
		newWatchCmd(),
		newFormatsCmd(),
		newOptionsCmd(),
		newJobsCmd(),
		newCreditsCmd(),
		newWebhookCmd(),
		newSelfUpdateCmd(),
	)
	root.AddCommand(newTaskCmds()...)
	return root
}

// newProgress returns a stderr spinner enabled only in an interactive,
// non-quiet, non-JSON session.
func newProgress() ui.Progress {
	enabled := !gf.json && !gf.quiet && ui.Interactive(os.Stderr)
	return ui.NewProgress(os.Stderr, enabled)
}
