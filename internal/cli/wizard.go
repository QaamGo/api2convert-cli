package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/catalog"
	"github.com/QaamGo/api2convert-cli/internal/run"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

// runWizard is the guided flow shown when api2convert is run with no arguments
// in a terminal: pick a file, pick a format, choose a destination, convert.
func runWizard(cmd *cobra.Command) error {
	ctx := cmd.Context()
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}

	var inputPath string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewFilePicker().
			Title("Pick a file to convert").
			CurrentDirectory(".").
			Value(&inputPath),
	)).Run(); err != nil {
		return wizardErr(err)
	}
	if strings.TrimSpace(inputPath) == "" {
		return nil
	}

	cat, err := catalog.Load(ctx, cl, false, catalogKey(ctx))
	if err != nil {
		return err
	}
	targets := cat.Targets()
	if len(targets) == 0 {
		return fmt.Errorf("no target formats available")
	}

	var target string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Convert " + filepath.Base(inputPath) + " to…").
			Options(huh.NewOptions(targets...)...).
			Value(&target),
	)).Run(); err != nil {
		return wizardErr(err)
	}

	defaultOut := stripExtBase(inputPath) + "." + target
	outPath := defaultOut
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Save to").
			Placeholder(defaultOut).
			Value(&outPath),
	)).Run(); err != nil {
		return wizardErr(err)
	}
	if strings.TrimSpace(outPath) == "" {
		outPath = defaultOut
	}

	res, err := run.ConvertOne(ctx, cl, inputPath, target, outPath, run.Options{OnConflict: "rename"}, newProgress())
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), ui.Success(res.Path))
	fmt.Fprintln(cmd.OutOrStdout(), ui.Dim(fmt.Sprintf("Tip: api2convert convert %q --to %s", inputPath, target)))
	return nil
}

func stripExtBase(p string) string {
	b := filepath.Base(p)
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// wizardErr swallows a user abort (Esc/Ctrl-C in a form) as a clean exit.
func wizardErr(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return nil
	}
	return err
}
