package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	api2convert "github.com/QaamGo/api2convert-go"
	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/clierr"
	"github.com/QaamGo/api2convert-cli/internal/output"
	"github.com/QaamGo/api2convert-cli/internal/run"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

type convertFlags struct {
	to          string
	out         string
	outDir      string
	category    string
	optionKVs   []string
	optionsFile string
	password    string
	outputIndex int
	timeout     time.Duration
	recursive   bool
	onConflict  string
	force       bool
	noClobber   bool
	failFast    bool
	async       bool
	callback    string
}

func newConvertCmd() *cobra.Command {
	var f convertFlags
	c := &cobra.Command{
		Use:   "convert <input>...",
		Short: "Convert files, URLs or stdin to another format",
		Long: "Convert one or more inputs to a target format.\n\n" +
			"Inputs may be local file paths, https URLs, directories (with --recursive), or '-' for stdin.\n" +
			"The target comes from --to, or is inferred from the --out file extension.",
		Example: "  api2convert convert report.docx --to pdf\n" +
			"  api2convert convert report.docx -o report.pdf\n" +
			"  api2convert convert https://example.com/deck.pptx --to pdf -o out/\n" +
			"  api2convert convert *.png --to webp --option quality=80 --out-dir compressed/",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConvert(cmd, args, f)
		},
	}
	addConvertFlags(c, &f)
	return c
}

func addConvertFlags(c *cobra.Command, f *convertFlags) {
	fl := c.Flags()
	fl.StringVarP(&f.to, "to", "t", "", "target format (else inferred from --out extension)")
	fl.StringVarP(&f.out, "out", "o", "", "output file or directory")
	fl.StringVar(&f.outDir, "out-dir", "", "output directory")
	fl.StringVar(&f.category, "category", "", "disambiguate an ambiguous target")
	fl.StringArrayVar(&f.optionKVs, "option", nil, "per-format option key=value (repeatable)")
	fl.StringVar(&f.optionsFile, "options-file", "", "JSON file of conversion options")
	fl.StringVar(&f.password, "password", "", "protect outputs with a download password")
	fl.IntVar(&f.outputIndex, "output-index", 0, "which output file to select")
	fl.DurationVar(&f.timeout, "convert-timeout", 0, "per-conversion poll timeout (e.g. 5m)")
	fl.BoolVarP(&f.recursive, "recursive", "r", false, "descend into directory inputs")
	fl.StringVar(&f.onConflict, "on-conflict", "error", "when output exists: error|skip|overwrite|rename")
	fl.BoolVarP(&f.force, "force", "f", false, "overwrite existing outputs (alias for --on-conflict overwrite)")
	fl.BoolVar(&f.noClobber, "no-clobber", false, "skip existing outputs (alias for --on-conflict skip)")
	fl.BoolVar(&f.failFast, "fail-fast", false, "stop the batch on the first failure")
	fl.BoolVar(&f.async, "async", false, "start the conversion(s) without waiting; print job id(s)")
	fl.StringVar(&f.callback, "callback", "", "webhook URL to notify on status change (implies --async)")

	_ = c.RegisterFlagCompletionFunc("to", completeTargets)
	_ = c.RegisterFlagCompletionFunc("category", completeCategories)
}

func runConvert(cmd *cobra.Command, args []string, f convertFlags) error {
	ctx := cmd.Context()

	opts, err := parseOptions(f.optionKVs, f.optionsFile)
	if err != nil {
		return &clierr.UsageError{Err: err}
	}
	onConflict, err := resolveOnConflict(f.onConflict, f.force, f.noClobber)
	if err != nil {
		return &clierr.UsageError{Err: err}
	}
	target := f.to
	if target == "" {
		target = inferTarget(f.out)
	}

	inputs, err := expandInputs(args, f.recursive)
	if err != nil {
		return &clierr.UsageError{Err: err}
	}
	if len(inputs) == 0 {
		return &clierr.UsageError{Err: errors.New("no inputs matched")}
	}
	if target == "" {
		return &clierr.UsageError{Err: errors.New("specify --to <format> (or an --out with an extension); see 'api2convert formats'")}
	}

	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}

	ro := run.Options{
		ConversionOptions: opts,
		Category:          f.category,
		Password:          f.password,
		OutputIndex:       f.outputIndex,
		Timeout:           f.timeout,
		OnConflict:        onConflict,
	}
	out := chooseOut(f.out, f.outDir)

	items := make([]run.Item, len(inputs))
	for i, in := range inputs {
		items[i] = run.Item{Input: in, Target: target}
	}
	return runOrAsync(cmd, cl, items, out, ro, f)
}

// runOrAsync dispatches items to the async starter or the synchronous
// convert/batch path depending on the flags.
func runOrAsync(cmd *cobra.Command, cl *api2convert.Client, items []run.Item, out string, ro run.Options, f convertFlags) error {
	if err := multiOutGuard(f, len(items)); err != nil {
		return err
	}
	if f.async || f.callback != "" {
		return runAsyncItems(cmd, cl, items, f.callback, ro)
	}
	return runItems(cmd, cl, items, out, ro, f.failFast)
}

// multiOutGuard rejects converting many inputs into a single --out FILE, which
// would collapse every output onto one path. (Merge, which intentionally
// produces one file from many inputs, does not go through here.)
func multiOutGuard(f convertFlags, n int) error {
	if n > 1 && f.outDir == "" && outIsFile(f.out) {
		return &clierr.UsageError{Err: errors.New("multiple inputs need a directory output — use --out-dir <dir> (not --out <file>)")}
	}
	return nil
}

func outIsFile(out string) bool {
	if out == "" {
		return false
	}
	if strings.HasSuffix(out, "/") || strings.HasSuffix(out, `\`) {
		return false
	}
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		return false
	}
	return true
}

func runAsyncItems(cmd *cobra.Command, cl *api2convert.Client, items []run.Item, callback string, ro run.Options) error {
	type started struct{ input, jobID string }
	var jobs []started
	for _, it := range items {
		job, err := run.StartAsync(cmd.Context(), cl, it.Input, it.Target, callback, ro)
		if err != nil {
			return err
		}
		jobs = append(jobs, started{it.Input, job.ID})
	}

	if gf.json {
		arr := make([]map[string]any, 0, len(jobs))
		for _, j := range jobs {
			arr = append(arr, map[string]any{"input": j.input, "job_id": j.jobID})
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"ok": true, "async": true, "jobs": arr})
	}
	for _, j := range jobs {
		fmt.Fprintf(cmd.OutOrStdout(), "Started %s (not waiting)\n", j.jobID)
		fmt.Fprintln(cmd.OutOrStdout(), ui.Dim("  check:  api2convert jobs status "+j.jobID))
	}
	return nil
}

// --- output ---------------------------------------------------------------

func emitConvert(cmd *cobra.Command, res run.Result) error {
	if gf.json {
		return output.Emit(cmd.OutOrStdout(), true, convertView{res})
	}
	if gf.quiet {
		fmt.Fprintln(cmd.OutOrStdout(), res.Path)
		return nil
	}
	if res.Skipped {
		fmt.Fprintln(cmd.OutOrStdout(), ui.Dim("• skipped (exists): "+res.Path))
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), ui.Success(res.Path))
	return nil
}

type convertView struct{ r run.Result }

func (v convertView) Human(w io.Writer) error {
	fmt.Fprintln(w, ui.Success(v.r.Path))
	return nil
}

func (v convertView) JSON() any {
	return map[string]any{
		"ok":          true,
		"input":       v.r.Input,
		"target":      v.r.Target,
		"output_path": v.r.Path,
		"skipped":     v.r.Skipped,
	}
}

func emitBatch(cmd *cobra.Command, s run.Summary) error {
	if gf.json {
		results := make([]map[string]any, 0, len(s.Results))
		for _, r := range s.Results {
			results = append(results, convertView{r}.JSON().(map[string]any))
		}
		fails := make([]map[string]any, 0, len(s.Errors))
		for _, fe := range s.Errors {
			fails = append(fails, map[string]any{"input": fe.Input, "error": fe.Err.Error()})
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"ok": len(s.Errors) == 0,
			"summary": map[string]any{
				"total": s.Total(), "succeeded": len(s.Results), "failed": len(s.Errors),
			},
			"results": results,
			"errors":  fails,
		})
	} else {
		for _, r := range s.Results {
			status := "ok"
			if r.Skipped {
				status = "skipped"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s %s → %s\n", ui.Check(), r.Input, status)
		}
		for _, fe := range s.Errors {
			msg, _ := clierr.Classify(fe.Err)
			fmt.Fprintf(cmd.ErrOrStderr(), "  %s %s: %s\n", ui.Cross(), fe.Input, firstLine(msg))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d done, %d failed\n", len(s.Results), len(s.Errors))
	}

	if len(s.Errors) == 0 {
		return nil
	}
	return &clierr.Coded{Code: maxSeverity(s.Errors)}
}

func maxSeverity(errs []run.FileError) clierr.ExitCode {
	max := clierr.ExitGeneric
	for _, fe := range errs {
		if _, code := clierr.Classify(fe.Err); code != clierr.ExitInterrupted && code > max {
			max = code
		}
	}
	return max
}

// --- helpers --------------------------------------------------------------

func parseOptions(kvs []string, file string) (map[string]any, error) {
	opts := map[string]any{}
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(file, ".json") {
			return nil, fmt.Errorf("--options-file currently supports .json only")
		}
		if err := json.Unmarshal(b, &opts); err != nil {
			return nil, fmt.Errorf("invalid options file: %w", err)
		}
	}
	for _, kv := range kvs {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("--option must be key=value, got %q", kv)
		}
		opts[k] = coerce(v)
	}
	return opts, nil
}

func coerce(s string) any {
	switch s {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if fv, err := strconv.ParseFloat(s, 64); err == nil {
		return fv
	}
	return s
}

func resolveOnConflict(onConflict string, force, noClobber bool) (string, error) {
	if force && noClobber {
		return "", errors.New("--force and --no-clobber are mutually exclusive")
	}
	switch {
	case force:
		return "overwrite", nil
	case noClobber:
		return "skip", nil
	}
	switch onConflict {
	case "error", "skip", "overwrite", "rename":
		return onConflict, nil
	default:
		return "", fmt.Errorf("invalid --on-conflict value %q (want error|skip|overwrite|rename)", onConflict)
	}
}

func inferTarget(out string) string {
	if out == "" {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(out), "."))
}

func chooseOut(out, outDir string) string {
	if outDir != "" {
		return strings.TrimRight(outDir, `/\`) + string(os.PathSeparator)
	}
	return out
}

// expandInputs turns raw args into a flat list of inputs: URLs and "-" pass
// through; a directory is walked (with recursive) or errors; glob patterns are
// expanded for shells that don't (e.g. Windows cmd).
func expandInputs(args []string, recursive bool) ([]string, error) {
	var out []string
	for _, a := range args {
		if a == "-" || isURLArg(a) {
			out = append(out, a)
			continue
		}
		info, err := os.Stat(a)
		if err != nil {
			if matches, gerr := filepath.Glob(a); gerr == nil && len(matches) > 0 {
				out = append(out, matches...)
				continue
			}
			return nil, fmt.Errorf("no such file: %s", a)
		}
		if info.IsDir() {
			if !recursive {
				return nil, fmt.Errorf("%s is a directory — pass --recursive to convert its contents", a)
			}
			err := filepath.WalkDir(a, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					out = append(out, p)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

func isURLArg(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "http://") || strings.HasPrefix(strings.ToLower(s), "https://")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
