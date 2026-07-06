package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	api2convert "github.com/QaamGo/api2convert-go/v10"
	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/ui"
)

func newWebhookCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "webhook",
		Short: "Webhook helpers",
	}
	c.AddCommand(newWebhookVerifyCmd())
	return c
}

func newWebhookVerifyCmd() *cobra.Command {
	var secret, signature string
	c := &cobra.Command{
		Use:   "verify <file|->",
		Short: "Verify and parse a webhook callback payload",
		Long: "Verify an api2convert webhook callback. Pass the raw request body as a file or '-' for stdin.\n" +
			"With --secret set, the HMAC-SHA256 signature (--signature, the X-Oc-Signature header) is checked.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				body []byte
				err  error
			)
			if args[0] == "-" {
				body, err = io.ReadAll(os.Stdin)
			} else {
				body, err = os.ReadFile(args[0])
			}
			if err != nil {
				return err
			}

			event, err := api2convert.Webhooks().ConstructEvent(body, signature, secret)
			if err != nil {
				return err
			}

			if gf.json {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"ok":     true,
					"job_id": event.Job.ID,
					"status": event.Job.Status.Code,
				})
			}
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success(fmt.Sprintf("Verified. Job %s status %s", event.Job.ID, event.Job.Status.Code)))
			return nil
		},
	}
	c.Flags().StringVar(&secret, "secret", "", "webhook signing secret (empty skips signature verification)")
	c.Flags().StringVar(&signature, "signature", "", "the X-Oc-Signature header value")
	return c
}
