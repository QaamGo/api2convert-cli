package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	api2convert "github.com/QaamGo/api2convert-go/v10"
	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/catalog"
	"github.com/QaamGo/api2convert-cli/internal/clierr"
	"github.com/QaamGo/api2convert-cli/internal/run"
	"github.com/QaamGo/api2convert-cli/internal/tasks"
)

func newTaskCmds() []*cobra.Command {
	verbs := tasks.Registry()
	cmds := make([]*cobra.Command, 0, len(verbs))
	for _, v := range verbs {
		cmds = append(cmds, newTaskCmd(v))
	}
	return cmds
}

func newTaskCmd(v tasks.Verb) *cobra.Command {
	var f convertFlags
	c := &cobra.Command{
		Use:     v.Name + " <input>...",
		Aliases: v.Aliases,
		Short:   v.Short,
		Long:    v.Long,
		Example: v.Example,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTask(cmd, args, f, v)
		},
	}
	addConvertFlags(c, &f)
	return c
}

func runTask(cmd *cobra.Command, args []string, f convertFlags, v tasks.Verb) error {
	ctx := cmd.Context()

	opts, err := parseOptions(f.optionKVs, f.optionsFile)
	if err != nil {
		return &clierr.UsageError{Err: err}
	}
	for k, val := range v.DefaultOptions {
		if _, set := opts[k]; !set {
			opts[k] = val
		}
	}
	onConflict, err := resolveOnConflict(f.onConflict, f.force, f.noClobber)
	if err != nil {
		return &clierr.UsageError{Err: err}
	}
	inputs, err := expandInputs(args, f.recursive)
	if err != nil {
		return &clierr.UsageError{Err: err}
	}
	if len(inputs) == 0 {
		return &clierr.UsageError{Err: errors.New("no inputs matched")}
	}

	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	category := f.category
	if category == "" {
		category = v.Category
	}
	ro := run.Options{
		ConversionOptions: opts,
		Category:          category,
		Password:          f.password,
		OutputIndex:       f.outputIndex,
		Timeout:           f.timeout,
		OnConflict:        onConflict,
	}
	out := chooseOut(f.out, f.outDir)

	switch v.Kind {
	case tasks.Merge:
		if f.async || f.callback != "" {
			return &clierr.UsageError{Err: errors.New("merge does not support --async")}
		}
		target := targetOr(f.to, v.DefaultTarget)
		if target == "" {
			return &clierr.UsageError{Err: errors.New("specify --to <format>")}
		}
		if err := gate(ctx, cl, v, target); err != nil {
			return err
		}
		res, err := run.Merge(ctx, cl, inputs, target, out, ro, newProgress())
		if err != nil {
			return err
		}
		return emitConvert(cmd, res)

	case tasks.SameFormat:
		items := make([]run.Item, 0, len(inputs))
		for _, in := range inputs {
			t := f.to
			if t == "" {
				t = extOf(in)
			}
			if t == "" {
				return &clierr.UsageError{Err: fmt.Errorf("cannot determine target for %q — pass --to", in)}
			}
			items = append(items, run.Item{Input: in, Target: t})
		}
		return runOrAsync(cmd, cl, items, out, ro, f)

	default: // tasks.ToTarget
		target := targetOr(f.to, v.DefaultTarget)
		if target == "" {
			return &clierr.UsageError{Err: errors.New("specify --to <format>")}
		}
		if err := gate(ctx, cl, v, target); err != nil {
			return err
		}
		items := make([]run.Item, 0, len(inputs))
		for _, in := range inputs {
			items = append(items, run.Item{Input: in, Target: target})
		}
		return runOrAsync(cmd, cl, items, out, ro, f)
	}
}

// runItems runs one or many items, emitting a single result or a batch summary.
func runItems(cmd *cobra.Command, cl *api2convert.Client, items []run.Item, out string, ro run.Options, failFast bool) error {
	if len(items) == 1 {
		res, err := run.ConvertOne(cmd.Context(), cl, items[0].Input, items[0].Target, out, ro, newProgress())
		if err != nil {
			return err
		}
		return emitConvert(cmd, res)
	}
	summary := run.BatchItems(cmd.Context(), cl, items, out, resolvedFrom(cmd.Context()).Concurrency, ro, newProgress(), failFast)
	return emitBatch(cmd, summary)
}

// gate checks that a gated verb's capability is actually offered by the
// account's catalog, and reports a friendly message otherwise.
func gate(ctx context.Context, cl *api2convert.Client, v tasks.Verb, target string) error {
	if !v.Gated {
		return nil
	}
	capTarget := v.CapTarget
	if capTarget == "" {
		capTarget = target
	}
	cat, err := catalog.Load(ctx, cl, false, catalogKey(ctx))
	if err != nil {
		return err // surface the underlying (likely auth/network) error
	}
	if !cat.HasTarget(capTarget) {
		return fmt.Errorf("'%s' isn't available on your account/plan — see 'api2convert formats'", v.Name)
	}
	return nil
}

func targetOr(to, def string) string {
	if to != "" {
		return to
	}
	return def
}

func extOf(input string) string {
	if input == "-" || isURLArg(input) {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(input), "."))
}
