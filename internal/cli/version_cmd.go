package cli

import (
	"fmt"

	api2convert "github.com/QaamGo/api2convert-go/v10"
	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/selfupdate"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

func newVersionCmd() *cobra.Command {
	var check bool
	c := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintf(cmd.OutOrStdout(),
				"api2convert %s (SDK %s, commit %s, built %s)\n",
				buildInfo.Version, api2convert.Version, buildInfo.Commit, buildInfo.Date)
			if !check {
				return nil
			}
			res, err := selfupdate.Available(cmd.Context(), buildInfo.Version)
			if err != nil {
				// A failed update check must not fail the command.
				fmt.Fprintln(cmd.ErrOrStderr(), ui.Dim("(could not check for updates: "+err.Error()+")"))
				return nil
			}
			if res.To != "" && res.To != res.From {
				fmt.Fprintln(cmd.OutOrStdout(), ui.Bold("A newer version is available: "+res.To))
				fmt.Fprintln(cmd.OutOrStdout(), ui.Dim("  run 'api2convert self-update' to upgrade"))
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), ui.Success("You're on the latest version."))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&check, "check", false, "check whether a newer release is available")
	return c
}
