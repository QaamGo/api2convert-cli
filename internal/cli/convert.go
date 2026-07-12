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

	api2convert "github.com/QaamGo/api2convert-go/v10"
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

	// cloud input
	inputCloud     string
	inputParams    []string
	inputCredsFile string
	// cloud output
	outputTarget    string
	outputParams    []string
	outputCredsFile string
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
			"  api2convert convert https://example-files.online-convert.com/raster%20image/jpg/example_small.jpg --to pdf -o out/\n" +
			"  api2convert convert *.png --to webp --option quality=80 --out-dir compressed/\n" +
			"  api2convert convert --input-cloud amazons3 --input-param bucket=my-bucket --input-param file=in.docx --input-credentials-file s3.json --to pdf\n" +
			"  api2convert convert report.docx --to pdf --output-target amazons3 --output-param bucket=out --output-credentials-file s3.json",
		Args: func(cmd *cobra.Command, args []string) error {
			// With --input-cloud the source comes from cloud storage, so there is
			// no positional input; otherwise at least one input is required.
			if cmd.Flags().Changed("input-cloud") {
				if len(args) > 0 {
					return errors.New("--input-cloud takes no positional input (the source is the cloud location)")
				}
				return nil
			}
			return cobra.MinimumNArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConvert(cmd, args, f)
		},
	}
	addConvertFlags(c, &f)
	addCloudFlags(c, &f)
	return c
}

// addCloudFlags registers the cloud input/output flags. They live only on the
// top-level `convert` command (not batch/tasks). Credentials are sourced from a
// file or an env var — deliberately never a flag whose value would sit in argv
// (shell history / process list leak).
func addCloudFlags(c *cobra.Command, f *convertFlags) {
	fl := c.Flags()
	fl.StringVar(&f.inputCloud, "input-cloud", "", "import the source from cloud storage: amazons3|azure|ftp|googlecloud")
	fl.StringArrayVar(&f.inputParams, "input-param", nil, "cloud input locator key=value (repeatable; e.g. bucket=, file=, host=, container=, projectid=)")
	fl.StringVar(&f.inputCredsFile, "input-credentials-file", "", "JSON file of cloud input credentials (or env A2C_INPUT_CREDENTIALS)")
	fl.StringVar(&f.outputTarget, "output-target", "", "deliver the output to cloud storage: amazons3|azure|ftp|googlecloud|gdrive|youtube")
	fl.StringArrayVar(&f.outputParams, "output-param", nil, "cloud output delivery key=value (repeatable)")
	fl.StringVar(&f.outputCredsFile, "output-credentials-file", "", "JSON file of cloud output credentials (or env A2C_OUTPUT_CREDENTIALS)")
}

func addConvertFlags(c *cobra.Command, f *convertFlags) {
	fl := c.Flags()
	fl.StringVarP(&f.to, "to", "t", "", "target format (else inferred from --out extension)")
	fl.StringVarP(&f.out, "out", "o", "", "output file or directory")
	fl.StringVar(&f.outDir, "out-dir", "", "output directory")
	fl.StringVar(&f.category, "category", "", "disambiguate an ambiguous target")
	fl.StringArrayVar(&f.optionKVs, "option", nil, "per-format option key=value (repeatable; key:=value forces a literal string)")
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

	cloudInput, err := buildCloudInput(f)
	if err != nil {
		return &clierr.UsageError{Err: err}
	}
	outputTargets, err := buildOutputTargets(f)
	if err != nil {
		return &clierr.UsageError{Err: err}
	}

	ro := run.Options{
		ConversionOptions: opts,
		Category:          f.category,
		Password:          f.password,
		OutputIndex:       f.outputIndex,
		Timeout:           f.timeout,
		OnConflict:        onConflict,
		CloudInput:        cloudInput,
		OutputTargets:     outputTargets,
	}
	out := chooseOut(f.out, f.outDir)

	var items []run.Item
	if cloudInput != nil {
		// A cloud input is a single synthetic item — there is no positional input
		// and no local file to expand.
		items = []run.Item{{Input: "cloud:" + cloudInput.Provider, Target: target}}
	} else {
		inputs, ierr := expandInputs(args, f.recursive)
		if ierr != nil {
			return &clierr.UsageError{Err: ierr}
		}
		if len(inputs) == 0 {
			return &clierr.UsageError{Err: errors.New("no inputs matched")}
		}
		items = itemsForTarget(inputs, target, out)
	}
	if target == "" {
		return &clierr.UsageError{Err: errors.New("specify --to <format> (or an --out with an extension); see 'api2convert formats'")}
	}

	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}

	return runOrAsync(cmd, cl, items, out, ro, f)
}

// inputCloudProviders are the four providers that offer an input downloader
// (gdrive/youtube are output-only and rejected as an input).
var inputCloudProviders = map[string]bool{"amazons3": true, "azure": true, "ftp": true, "googlecloud": true}

// outputCloudProviders are the providers that accept a delivery target.
var outputCloudProviders = map[string]bool{"amazons3": true, "azure": true, "ftp": true, "googlecloud": true, "gdrive": true, "youtube": true}

// buildCloudInput assembles the cloud input spec from the --input-* flags, or
// returns nil when --input-cloud is not set. Credentials come only from a file or
// the A2C_INPUT_CREDENTIALS env var — never a flag value in argv.
func buildCloudInput(f convertFlags) (*run.CloudSource, error) {
	if f.inputCloud == "" {
		return nil, nil
	}
	provider := strings.ToLower(f.inputCloud)
	if !inputCloudProviders[provider] {
		if provider == "gdrive" || provider == "youtube" {
			return nil, fmt.Errorf("--input-cloud %s is not supported as an input (output-only); use one of amazons3|azure|ftp|googlecloud", provider)
		}
		return nil, fmt.Errorf("invalid --input-cloud %q (want amazons3|azure|ftp|googlecloud)", f.inputCloud)
	}
	params, err := parseCloudParams(f.inputParams, "--input-param")
	if err != nil {
		return nil, err
	}
	creds, err := loadCredentials(f.inputCredsFile, "A2C_INPUT_CREDENTIALS", "--input-credentials-file")
	if err != nil {
		return nil, err
	}
	return &run.CloudSource{Provider: provider, Parameters: params, Credentials: creds}, nil
}

// buildOutputTargets assembles the cloud delivery target from the --output-*
// flags, or returns nil when --output-target is not set. Credentials come only
// from a file or the A2C_OUTPUT_CREDENTIALS env var — never a flag value in argv.
func buildOutputTargets(f convertFlags) ([]run.CloudTarget, error) {
	if f.outputTarget == "" {
		if len(f.outputParams) > 0 || f.outputCredsFile != "" {
			return nil, errors.New("--output-param/--output-credentials-file require --output-target <provider>")
		}
		return nil, nil
	}
	provider := strings.ToLower(f.outputTarget)
	if !outputCloudProviders[provider] {
		return nil, fmt.Errorf("invalid --output-target %q (want amazons3|azure|ftp|googlecloud|gdrive|youtube)", f.outputTarget)
	}
	params, err := parseCloudParams(f.outputParams, "--output-param")
	if err != nil {
		return nil, err
	}
	creds, err := loadCredentials(f.outputCredsFile, "A2C_OUTPUT_CREDENTIALS", "--output-credentials-file")
	if err != nil {
		return nil, err
	}
	return []run.CloudTarget{{Provider: provider, Parameters: params, Credentials: creds}}, nil
}

// parseCloudParams turns repeated key=value flags into a parameters map. Values
// stay verbatim strings (cloud locator keys like bucket/file/host are strings; no
// bool/number coercion that could mangle a numeric-looking bucket name).
func parseCloudParams(kvs []string, flag string) (map[string]any, error) {
	m := map[string]any{}
	for _, kv := range kvs {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("%s must be key=value, got %q", flag, kv)
		}
		m[k] = v
	}
	return m, nil
}

// loadCredentials reads a JSON object of credentials from a file (preferred) or an
// env var. Credentials are never accepted as an inline flag value, which would
// leak them into shell history and the process list. Returns nil when neither
// source is set (an empty credentials object is sent).
func loadCredentials(file, envVar, flag string) (map[string]any, error) {
	var raw []byte
	switch {
	case file != "":
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", flag, err)
		}
		raw = b
	case os.Getenv(envVar) != "":
		raw = []byte(os.Getenv(envVar))
	default:
		return nil, nil
	}
	creds := map[string]any{}
	if err := json.Unmarshal(raw, &creds); err != nil {
		src := flag
		if file == "" {
			src = "env " + envVar
		}
		return nil, fmt.Errorf("invalid credentials JSON (%s): %w", src, err)
	}
	return creds, nil
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
	if res.Cloud {
		if gf.quiet {
			fmt.Fprintln(cmd.OutOrStdout(), cloudDeliverySummary(res))
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), ui.Success("delivered to cloud storage"))
		for _, d := range res.Deliveries {
			fmt.Fprintln(cmd.OutOrStdout(), ui.Dim("  → "+d.Provider+" ("+deliveryStatus(d.Status)+")"))
		}
		return nil
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

func deliveryStatus(s string) string {
	if s == "" {
		return "pending"
	}
	return s
}

// cloudDeliverySummary is the terse one-line form for --quiet cloud output.
func cloudDeliverySummary(res run.Result) string {
	parts := make([]string, 0, len(res.Deliveries))
	for _, d := range res.Deliveries {
		parts = append(parts, d.Provider+"="+deliveryStatus(d.Status))
	}
	if len(parts) == 0 {
		return "cloud"
	}
	return strings.Join(parts, ",")
}

type convertView struct{ r run.Result }

func (v convertView) Human(w io.Writer) error {
	if v.r.Cloud {
		fmt.Fprintln(w, ui.Success("delivered to cloud storage"))
		for _, d := range v.r.Deliveries {
			fmt.Fprintln(w, ui.Dim("  → "+d.Provider+" ("+deliveryStatus(d.Status)+")"))
		}
		return nil
	}
	fmt.Fprintln(w, ui.Success(v.r.Path))
	return nil
}

func (v convertView) JSON() any {
	if v.r.Cloud {
		// Cloud delivery: no local output path; report each target's provider +
		// status. Credentials are never carried on the result and never emitted.
		targets := make([]map[string]any, 0, len(v.r.Deliveries))
		for _, d := range v.r.Deliveries {
			targets = append(targets, map[string]any{"provider": d.Provider, "status": d.Status})
		}
		return map[string]any{
			"ok":             true,
			"input":          v.r.Input,
			"target":         v.r.Target,
			"cloud":          true,
			"output_targets": targets,
		}
	}
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
			return nil, fmt.Errorf("--option must be key=value (or key:=value for a literal string), got %q", kv)
		}
		// key:=value forces a literal string, bypassing coercion — needed for
		// values that look numeric/boolean but must stay strings (e.g. 080).
		if strings.HasSuffix(k, ":") {
			opts[strings.TrimSuffix(k, ":")] = v
			continue
		}
		opts[k] = coerce(v)
	}
	return opts, nil
}

// coerce maps a bare --option value to a bool/int/float when it unambiguously
// looks like one, else keeps it a string. Use key:=value to force a string.
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

// resolvedInput is one input to convert. For files discovered by walking a
// directory argument, rel is the input's path relative to that walked root, so
// the output can mirror the source tree (photos/2021/a.jpg → out/2021/a.webp)
// instead of flattening every match onto one directory. rel is "" for direct
// file args, glob matches, URLs and stdin, which keep basename behavior.
type resolvedInput struct {
	path string
	rel  string
}

// expandInputs turns raw args into a flat list of inputs: URLs and "-" pass
// through; a directory is walked (with recursive) or errors; glob patterns are
// expanded for shells that don't (e.g. Windows cmd).
func expandInputs(args []string, recursive bool) ([]resolvedInput, error) {
	var out []resolvedInput
	for _, a := range args {
		if a == "-" || isURLArg(a) {
			out = append(out, resolvedInput{path: a})
			continue
		}
		info, err := os.Stat(a)
		if err != nil {
			if matches, gerr := filepath.Glob(a); gerr == nil && len(matches) > 0 {
				for _, m := range matches {
					out = append(out, resolvedInput{path: m})
				}
				continue
			}
			return nil, fmt.Errorf("no such file: %s", a)
		}
		if info.IsDir() {
			if !recursive {
				return nil, fmt.Errorf("%s is a directory — pass --recursive to convert its contents", a)
			}
			root := a
			err := filepath.WalkDir(a, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				rel, rerr := filepath.Rel(root, p)
				if rerr != nil {
					rel = filepath.Base(p)
				}
				out = append(out, resolvedInput{path: p, rel: rel})
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		out = append(out, resolvedInput{path: a})
	}
	return out, nil
}

// itemsForTarget builds conversion items for one shared target.
func itemsForTarget(inputs []resolvedInput, target, out string) []run.Item {
	items := make([]run.Item, 0, len(inputs))
	for _, in := range inputs {
		items = append(items, itemFor(in, target, out))
	}
	return items
}

// itemFor builds a single conversion item, giving directory-walked inputs an
// explicit per-item output that mirrors the source tree — under --out-dir when a
// directory destination is given, else alongside the source file. Non-walked
// inputs (rel == "") and an explicit --out FILE keep the default naming.
func itemFor(in resolvedInput, target, out string) run.Item {
	it := run.Item{Input: in.path, Target: target}
	if in.rel == "" {
		return it
	}
	if dir, ok := outDirOf(out); ok {
		it.Out = filepath.Join(dir, replaceExt(in.rel, target))
	} else if out == "" {
		it.Out = replaceExt(in.path, target)
	}
	return it
}

// outDirOf reports the destination directory when out denotes one (a trailing
// separator, or an existing directory), so tree-preserving outputs land inside it.
func outDirOf(out string) (string, bool) {
	if out == "" {
		return "", false
	}
	if strings.HasSuffix(out, "/") || strings.HasSuffix(out, `\`) {
		return strings.TrimRight(out, `/\`), true
	}
	if info, err := os.Stat(out); err == nil && info.IsDir() {
		return out, true
	}
	return "", false
}

// replaceExt swaps a path's extension for ext, preserving any directory part.
func replaceExt(p, ext string) string {
	return strings.TrimSuffix(p, filepath.Ext(p)) + "." + ext
}

// inputPaths drops the tree metadata, for callers (merge) that take plain paths.
func inputPaths(ins []resolvedInput) []string {
	ps := make([]string, len(ins))
	for i, in := range ins {
		ps[i] = in.path
	}
	return ps
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
