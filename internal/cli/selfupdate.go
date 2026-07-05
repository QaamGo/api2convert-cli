package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/selfupdate"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

func newSelfUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "self-update",
		Short: "Update api2convert to the latest release",
		Long:  "Download the latest release for your OS/arch, verify its checksum, and replace this binary in place.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := selfupdate.Run(cmd.Context(), buildInfo.Version)
			if err != nil {
				return err
			}
			if !res.Updated {
				fmt.Fprintln(cmd.OutOrStdout(), ui.Success("Already up to date ("+res.From+")"))
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success("Updated "+res.From+" → "+res.To))
			return nil
		},
	}
}
