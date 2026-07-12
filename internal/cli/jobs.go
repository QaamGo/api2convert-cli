package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	api2convert "github.com/QaamGo/api2convert-go/v10"
	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/output"
	"github.com/QaamGo/api2convert-cli/internal/run"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

func newJobsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "jobs",
		Short: "Work with server-side conversion jobs",
	}
	c.AddCommand(
		newJobsListCmd(),
		newJobsStatusCmd(),
		newJobsWaitCmd(),
		newJobsDownloadCmd(),
		newJobsCancelCmd(),
	)
	return c
}

func newJobsListCmd() *cobra.Command {
	var status string
	var page int
	c := &cobra.Command{
		Use:   "list",
		Short: "List your recent jobs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, err := clientFrom(cmd.Context())
			if err != nil {
				return err
			}
			jobs, err := cl.Jobs().List(cmd.Context(), status, page)
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), gf.json, jobsView(jobs))
		},
	}
	c.Flags().StringVar(&status, "status", "", "filter by status (e.g. completed, failed, processing)")
	c.Flags().IntVar(&page, "page", 1, "page number (50 per page)")
	return c
}

func newJobsStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <job-id>",
		Short: "Show a job's status, inputs, outputs and messages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := clientFrom(cmd.Context())
			if err != nil {
				return err
			}
			job, err := cl.Jobs().Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), gf.json, jobDetailView{*job})
		},
	}
}

func newJobsWaitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wait <job-id>",
		Short: "Wait until a job finishes, with progress",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := clientFrom(cmd.Context())
			if err != nil {
				return err
			}
			prog := newProgress()
			prog.Start("Waiting for " + args[0] + "…")
			job, err := cl.Jobs().Wait(cmd.Context(), args[0], resolvedFrom(cmd.Context()).PollTimeout, true)
			prog.Stop()
			if err != nil {
				return err
			}
			return output.Emit(cmd.OutOrStdout(), gf.json, jobDetailView{*job})
		},
	}
}

func newJobsDownloadCmd() *cobra.Command {
	var outDir, password string
	c := &cobra.Command{
		Use:   "download <job-id>",
		Short: "Download a finished job's output file(s)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := clientFrom(cmd.Context())
			if err != nil {
				return err
			}
			outs, err := cl.Jobs().Outputs(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if len(outs) == 0 {
				return fmt.Errorf("job %s has no output files", args[0])
			}
			dir := outDir
			if dir == "" {
				dir = "."
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			var written []string
			seen := map[string]bool{}
			for _, o := range outs {
				target := uniqueDownloadPath(dir, o.Filename, o.ID, seen)
				seen[target] = true
				dl := cl.Download(o, passwordArgs(password)...)
				p, err := dl.Save(cmd.Context(), target)
				if err != nil {
					return err
				}
				written = append(written, p)
			}
			for _, p := range written {
				if gf.quiet {
					fmt.Fprintln(cmd.OutOrStdout(), p)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), ui.Success(p))
				}
			}
			return nil
		},
	}
	c.Flags().StringVarP(&outDir, "out", "o", "", "output directory (default .)")
	c.Flags().StringVar(&password, "password", "", "download password, if the output is protected")
	return c
}

func newJobsCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <job-id>",
		Short: "Cancel a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := clientFrom(cmd.Context())
			if err != nil {
				return err
			}
			if err := cl.Jobs().Cancel(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success("Canceled "+args[0]))
			return nil
		},
	}
}

func passwordArgs(p string) []string {
	if p == "" {
		return nil
	}
	return []string{p}
}

// uniqueDownloadPath returns a collision-free path for one output, so a job with
// several outputs sharing a filename doesn't overwrite itself. Both the filename
// and the id fallback are reduced to a bare, sanitized basename so a hostile
// server-supplied value (e.g. an id of "../../etc/cron.d/evil") can never escape
// the download directory.
func uniqueDownloadPath(dir, filename, id string, seen map[string]bool) string {
	base := run.SafeBase(filename)
	if base == "" {
		base = run.SafeBase(id)
	}
	if base == "" {
		base = "output"
	}
	target := filepath.Join(dir, base)
	if !seen[target] {
		if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
			return target
		}
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; ; i++ {
		cand := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if !seen[cand] {
			if _, err := os.Stat(cand); errors.Is(err, os.ErrNotExist) {
				return cand
			}
		}
	}
}

// --- views ---------------------------------------------------------------

type jobsView []api2convert.Job

func (v jobsView) Human(w io.Writer) error {
	t := output.Table{Headers: []string{"ID", "STATUS", "OUTPUTS"}}
	for _, j := range v {
		t.Rows = append(t.Rows, []string{j.ID, j.Status.Code, fmt.Sprintf("%d", len(j.Output))})
	}
	return t.Write(w)
}

func (v jobsView) JSON() any {
	out := make([]map[string]any, 0, len(v))
	for _, j := range v {
		out = append(out, map[string]any{"id": j.ID, "status": j.Status.Code, "outputs": len(j.Output)})
	}
	return out
}

type jobDetailView struct{ job api2convert.Job }

func (v jobDetailView) Human(w io.Writer) error {
	j := v.job
	fmt.Fprintf(w, "Job:     %s\n", j.ID)
	fmt.Fprintf(w, "Status:  %s\n", j.Status.Code)
	if j.Status.Info != "" {
		fmt.Fprintf(w, "Info:    %s\n", j.Status.Info)
	}
	for _, in := range j.Input {
		fmt.Fprintf(w, "Input:   %s\n", in.Filename)
	}
	for _, o := range j.Output {
		fmt.Fprintf(w, "Output:  %s (%s)\n", o.Filename, sizeStr(o.Size))
	}
	// Cloud delivery targets: print provider + status only. Credentials are never
	// printed — the SDK strips them on read, and we never emit t.Credentials here.
	for _, t := range jobOutputTargets(j) {
		status := t.Status
		if status == "" {
			status = "pending"
		}
		fmt.Fprintf(w, "Target:  %s (%s)\n", t.Type, status)
	}
	for _, e := range j.Errors {
		fmt.Fprintf(w, "%s %s\n", ui.Cross(), e.Message)
	}
	for _, wm := range j.Warnings {
		fmt.Fprintf(w, "! %s\n", wm.Message)
	}
	return nil
}

func (v jobDetailView) JSON() any {
	j := v.job
	outs := make([]map[string]any, 0, len(j.Output))
	for _, o := range j.Output {
		outs = append(outs, map[string]any{"id": o.ID, "filename": o.Filename, "uri": o.URI, "size": o.Size})
	}
	res := map[string]any{
		"id":     j.ID,
		"status": j.Status.Code,
		"info":   j.Status.Info,
		"output": outs,
	}
	if targets := jobOutputTargets(j); len(targets) > 0 {
		// Cloud delivery targets: emit provider (type), parameters and status —
		// but NEVER credentials, even if the SDK ever surfaced them.
		ts := make([]map[string]any, 0, len(targets))
		for _, t := range targets {
			ts = append(ts, map[string]any{
				"type":       t.Type,
				"parameters": t.Parameters,
				"status":     t.Status,
			})
		}
		res["output_targets"] = ts
	}
	return res
}

// jobOutputTargets flattens the cloud delivery targets attached across a job's
// conversions. Credentials are intentionally never read off these targets.
func jobOutputTargets(j api2convert.Job) []api2convert.OutputTarget {
	var out []api2convert.OutputTarget
	for _, conv := range j.Conversion {
		out = append(out, conv.OutputTargets...)
	}
	return out
}

func sizeStr(n *int64) string {
	if n == nil {
		return "?"
	}
	return fmt.Sprintf("%d bytes", *n)
}
