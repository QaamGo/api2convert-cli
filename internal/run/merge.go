package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	api2convert "github.com/QaamGo/api2convert-go/v10"

	"github.com/QaamGo/api2convert-cli/internal/ui"
)

// Merge combines multiple inputs into a single job producing one (merged)
// output — e.g. several PDFs into one. It drives the public job lifecycle
// (create staged → add/upload each input → start → wait → download).
func Merge(ctx context.Context, c *api2convert.Client, inputs []string, target, out string, o Options, prog ui.Progress) (Result, error) {
	conv := map[string]any{"target": target}
	if o.Category != "" {
		conv["category"] = o.Category
	}
	if len(o.ConversionOptions) > 0 {
		conv["options"] = o.ConversionOptions
	}
	payload := map[string]any{"process": false, "conversion": []any{conv}}
	if o.Password != "" {
		payload["download_passwords"] = []any{o.Password}
	}

	prog.Start("Merging " + fmt.Sprintf("%d inputs", len(inputs)) + " → " + target)
	defer prog.Stop()

	created, err := c.Jobs().Create(ctx, payload)
	if err != nil {
		return Result{}, err
	}
	for _, in := range inputs {
		if isURL(in) {
			if _, err := c.Jobs().AddInput(ctx, created.ID, map[string]any{"type": "remote", "source": in}); err != nil {
				return Result{}, err
			}
			continue
		}
		if _, err := c.Jobs().Upload(ctx, *created, in); err != nil {
			return Result{}, err
		}
	}
	if _, err := c.Jobs().Start(ctx, created.ID); err != nil {
		return Result{}, err
	}
	done, err := c.Jobs().Wait(ctx, created.ID, o.Timeout, true)
	if err != nil {
		return Result{}, err
	}
	if len(done.Output) == 0 {
		return Result{}, fmt.Errorf("merge produced no output files")
	}

	idx := o.OutputIndex
	if idx < 0 || idx >= len(done.Output) {
		idx = 0
	}
	outFile := done.Output[idx]

	planned := mergeOutPath(out, target, outFile.Filename)
	final, skip, err := claimPath(planned, o.OnConflict)
	if err != nil {
		return Result{}, err
	}
	if skip {
		return Result{Target: target, Path: planned, Skipped: true}, nil
	}

	var pw []string
	if o.Password != "" {
		pw = []string{o.Password}
	}
	written, err := c.Download(outFile, pw...).Save(ctx, final)
	if err != nil {
		if o.OnConflict != "overwrite" {
			_ = os.Remove(final) // remove the empty claim placeholder
		}
		return Result{}, err
	}
	return Result{Target: target, Path: written}, nil
}

func mergeOutPath(out, target, apiName string) string {
	if out != "" && !isDirLike(out) {
		return out
	}
	dir := "."
	if out != "" {
		dir = out
	}
	name := SafeBase(apiName)
	if name == "" {
		name = "merged." + target
	}
	return filepath.Join(dir, name)
}
