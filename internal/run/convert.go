// Package run orchestrates conversions on top of the SDK: single-file convert
// (with output-path resolution and an atomic conflict policy), the batch worker
// pool, and the folder watcher.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	api2convert "github.com/QaamGo/api2convert-go"

	"github.com/QaamGo/api2convert-cli/internal/ui"
)

var urlRE = regexp.MustCompile(`(?i)^https?://`)

// Options are the per-conversion controls shared by convert, batch, watch and
// the task verbs.
type Options struct {
	ConversionOptions map[string]any
	Category          string
	Password          string
	OutputIndex       int
	Timeout           time.Duration
	OnConflict        string // error | skip | overwrite | rename
}

// Result describes one completed (or skipped) conversion.
type Result struct {
	Input   string
	Target  string
	Path    string
	Skipped bool
}

// ConvertOne converts a single input to target, writing to out (a file path, a
// directory, or "" to auto-name), honoring the conflict policy atomically.
//
// When the output path is knowable before the conversion (an explicit --out
// file, or a local input auto-named into a directory), the conflict policy is
// applied FIRST — so `skip` and `error` never spend a conversion, which makes
// re-runs (batch/watch) idempotent and quota-cheap. Path claiming uses
// O_CREATE|O_EXCL, so concurrent batch workers can never collide on one path.
func ConvertOne(ctx context.Context, c *api2convert.Client, input, target, out string, o Options, prog ui.Progress) (Result, error) {
	planned, deterministic := plannedPath(input, out, target)

	if deterministic {
		final, skip, err := claimPath(planned, o.OnConflict)
		if err != nil {
			return Result{}, err
		}
		if skip {
			return Result{Input: input, Target: target, Path: planned, Skipped: true}, nil
		}
		res, cerr := doConvert(ctx, c, input, target, out, o, prog)
		if cerr != nil {
			if o.OnConflict != "overwrite" {
				_ = os.Remove(final) // remove the empty claim placeholder
			}
			return Result{}, cerr
		}
		if _, err := res.Save(ctx, final); err != nil {
			return Result{}, err
		}
		return Result{Input: input, Target: target, Path: final}, nil
	}

	// Non-deterministic (URL/stdin into a directory): must convert to learn the
	// server-assigned filename before we can resolve the path.
	res, cerr := doConvert(ctx, c, input, target, out, o, prog)
	if cerr != nil {
		return Result{}, cerr
	}
	planned, err := resolveAPIPath(res, out, target)
	if err != nil {
		return Result{}, err
	}
	final, skip, err := claimPath(planned, o.OnConflict)
	if err != nil {
		return Result{}, err
	}
	if skip {
		return Result{Input: input, Target: target, Path: planned, Skipped: true}, nil
	}
	if _, err := res.Save(ctx, final); err != nil {
		return Result{}, err
	}
	return Result{Input: input, Target: target, Path: final}, nil
}

func doConvert(ctx context.Context, c *api2convert.Client, input, target, out string, o Options, prog ui.Progress) (*api2convert.ConversionResult, error) {
	copts := buildConvertOptions(o)
	var in any = input
	if input == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		in = data
		copts = append(copts, api2convert.WithFilename(stdinName(out, target)))
	}
	prog.Start("Converting " + displayName(input) + " → " + target)
	res, err := c.Convert(ctx, in, target, copts...)
	prog.Stop()
	return res, err
}

// silentProgress is a disabled Progress used by batch/watch workers so their
// per-file spinners don't interleave.
func silentProgress() ui.Progress { return ui.NewProgress(nil, false) }

// StartAsync starts a conversion without waiting and returns the created job.
// When callback is set, the API notifies it on status change.
func StartAsync(ctx context.Context, c *api2convert.Client, input, target, callback string, o Options) (*api2convert.Job, error) {
	copts := buildConvertOptions(o)
	if callback != "" {
		copts = append(copts, api2convert.WithCallback(callback))
	}
	var in any = input
	if input == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		in = data
		copts = append(copts, api2convert.WithFilename(stdinName("", target)))
	}
	return c.ConvertAsync(ctx, in, target, copts...)
}

func buildConvertOptions(o Options) []api2convert.ConvertOption {
	var c []api2convert.ConvertOption
	if len(o.ConversionOptions) > 0 {
		c = append(c, api2convert.WithConversionOptions(o.ConversionOptions))
	}
	if o.Category != "" {
		c = append(c, api2convert.WithCategory(o.Category))
	}
	if o.Password != "" {
		c = append(c, api2convert.WithDownloadPassword(o.Password))
	}
	if o.OutputIndex != 0 {
		c = append(c, api2convert.WithOutputIndex(o.OutputIndex))
	}
	if o.Timeout > 0 {
		c = append(c, api2convert.WithConvertTimeout(o.Timeout))
	}
	return c
}

// plannedPath returns the intended output path and whether it is knowable
// before conversion. Deterministic paths (explicit file, or a local input into a
// directory named from its basename) let us apply the conflict policy up front.
func plannedPath(input, out, target string) (path string, deterministic bool) {
	if out != "" && !isDirLike(out) {
		return out, true // explicit file path
	}
	if isLocalFile(input) {
		dir := "."
		if out != "" {
			dir = strings.TrimRight(out, `/\`)
		}
		return filepath.Join(dir, stripExt(filepath.Base(input))+"."+target), true
	}
	return "", false // URL/stdin into a directory: name comes from the API filename
}

// resolveAPIPath builds the output path from the server-assigned filename (for
// URL/stdin inputs written into a directory).
func resolveAPIPath(res *api2convert.ConversionResult, out, target string) (string, error) {
	dir := "."
	if out != "" {
		dir = strings.TrimRight(out, `/\`)
	}
	o, err := res.Output()
	if err != nil {
		return "", err
	}
	name := safeBase(o.Filename)
	if name == "" {
		name = "output." + target
	}
	return filepath.Join(dir, name), nil
}

// claimPath applies the conflict policy by atomically claiming the output path
// with O_CREATE|O_EXCL, so two concurrent workers can never both take one path.
// For overwrite it just returns the target. For skip it reports skip=true when
// the path is taken. For rename it returns the first free "name (n).ext".
func claimPath(target, policy string) (final string, skip bool, err error) {
	if policy == "overwrite" {
		return target, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", false, err
	}
	switch policy {
	case "skip":
		if f, e := create(target); e == nil {
			_ = f.Close()
			return target, false, nil
		} else if errors.Is(e, os.ErrExist) {
			return target, true, nil
		} else {
			return "", false, e
		}
	case "", "error":
		if f, e := create(target); e == nil {
			_ = f.Close()
			return target, false, nil
		} else if errors.Is(e, os.ErrExist) {
			return "", false, fmt.Errorf("%s already exists — use --on-conflict overwrite|skip|rename, or --out", target)
		} else {
			return "", false, e
		}
	case "rename":
		ext := filepath.Ext(target)
		stem := strings.TrimSuffix(target, ext)
		cand := target
		for i := 1; ; i++ {
			f, e := create(cand)
			if e == nil {
				_ = f.Close()
				return cand, false, nil
			}
			if !errors.Is(e, os.ErrExist) {
				return "", false, e
			}
			cand = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
	default:
		return "", false, fmt.Errorf("invalid --on-conflict value %q (want error|skip|overwrite|rename)", policy)
	}
}

func create(p string) (*os.File, error) {
	return os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
}

func isURL(s string) bool       { return urlRE.MatchString(s) }
func isLocalFile(s string) bool { return s != "-" && !isURL(s) }

func isDirLike(p string) bool {
	if strings.HasSuffix(p, "/") || strings.HasSuffix(p, `\`) {
		return true
	}
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func stripExt(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// safeBase reduces an API-supplied filename to a bare basename safe to join to a
// directory (mirrors the SDK's own sanitization).
func safeBase(name string) string {
	name = strings.ReplaceAll(name, "\x00", "")
	name = strings.ReplaceAll(name, `\`, "/")
	b := strings.TrimSpace(path.Base(name))
	if b == "" || b == "." || b == ".." {
		return ""
	}
	return b
}

func displayName(input string) string {
	switch {
	case input == "-":
		return "stdin"
	case isURL(input):
		return input
	default:
		return filepath.Base(input)
	}
}

func stdinName(out, target string) string {
	if out != "" && !isDirLike(out) {
		return filepath.Base(out)
	}
	return "input." + target
}
