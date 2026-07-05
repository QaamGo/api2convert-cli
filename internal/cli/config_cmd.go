package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/QaamGo/api2convert-cli/internal/clierr"
	"github.com/QaamGo/api2convert-cli/internal/config"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

func newLoginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Save and validate your API key",
		Long:  "Prompts for your api2convert API key (input hidden), validates it against the API, and saves it to the config file.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			key := gf.apiKey
			if key == "" {
				fmt.Fprint(cmd.ErrOrStderr(), "Enter your api2convert API key: ")
				raw, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(cmd.ErrOrStderr())
				if err != nil {
					return &clierr.UsageError{Err: errors.New("could not read API key")}
				}
				key = strings.TrimSpace(string(raw))
			}
			if key == "" {
				return &clierr.UsageError{Err: errors.New("no API key provided")}
			}

			res := resolvedFrom(cmd.Context())
			res.APIKey = key
			c, err := buildClient(res)
			if err != nil {
				return err
			}
			prog := newProgress()
			prog.Start("Validating…")
			_, err = c.Contracts().Get(cmd.Context())
			prog.Stop()
			if err != nil {
				return err
			}

			fileCfg, err := config.Load()
			if err != nil {
				return err
			}
			fileCfg.APIKey = key
			if err := config.Save(fileCfg); err != nil {
				return err
			}
			p, _ := config.File()
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success("Signed in. Key saved to "+p))
			fmt.Fprintln(cmd.OutOrStdout(), ui.Dim("Try:  api2convert convert myfile.docx --to pdf"))
			return nil
		},
	}
}

func newConfigCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
	}
	c.AddCommand(newConfigSetCmd(), newConfigGetCmd(), newConfigPathCmd(), newConfigUnsetCmd())
	return c
}

// validConfigKeys maps the friendly key names to a short description.
var validConfigKeys = []string{"api-key", "base-url", "timeout", "poll-timeout", "max-retries", "output", "concurrency"}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "set <key> <value>",
		Short:     "Set a configuration value",
		Args:      cobra.ExactArgs(2),
		ValidArgs: validConfigKeys,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := setConfigKey(&cfg, args[0], args[1]); err != nil {
				return &clierr.UsageError{Err: err}
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success("Set "+args[0]))
			return nil
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "get <key>",
		Short:     "Print a configuration value",
		Args:      cobra.ExactArgs(1),
		ValidArgs: validConfigKeys,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			v, err := getConfigKey(cfg, args[0])
			if err != nil {
				return &clierr.UsageError{Err: err}
			}
			fmt.Fprintln(cmd.OutOrStdout(), v)
			return nil
		},
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config file location",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := config.File()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), p)
			return nil
		},
	}
}

func newConfigUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "unset <key>",
		Short:     "Remove a configuration value",
		Args:      cobra.ExactArgs(1),
		ValidArgs: validConfigKeys,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := setConfigKey(&cfg, args[0], ""); err != nil {
				return &clierr.UsageError{Err: err}
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), ui.Success("Unset "+args[0]))
			return nil
		},
	}
}

func setConfigKey(c *config.Config, key, value string) error {
	switch key {
	case "api-key":
		c.APIKey = value
	case "base-url":
		c.BaseURL = value
	case "timeout":
		c.Timeout = value
	case "poll-timeout":
		c.PollTimeout = value
	case "max-retries":
		if value == "" {
			c.MaxRetries = nil
			return nil
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("max-retries must be an integer")
		}
		c.MaxRetries = &n
	case "output":
		if value != "" && value != "human" && value != "json" {
			return fmt.Errorf("output must be 'human' or 'json'")
		}
		c.Output = value
	case "concurrency":
		if value == "" {
			c.Concurrency = 0
			return nil
		}
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("concurrency must be an integer")
		}
		c.Concurrency = n
	default:
		return fmt.Errorf("unknown config key %q (valid: %s)", key, strings.Join(validConfigKeys, ", "))
	}
	return nil
}

func getConfigKey(c config.Config, key string) (string, error) {
	switch key {
	case "api-key":
		return maskKey(c.APIKey), nil
	case "base-url":
		return c.BaseURL, nil
	case "timeout":
		return c.Timeout, nil
	case "poll-timeout":
		return c.PollTimeout, nil
	case "max-retries":
		if c.MaxRetries == nil {
			return "", nil
		}
		return strconv.Itoa(*c.MaxRetries), nil
	case "output":
		return c.Output, nil
	case "concurrency":
		return strconv.Itoa(c.Concurrency), nil
	default:
		return "", fmt.Errorf("unknown config key %q (valid: %s)", key, strings.Join(validConfigKeys, ", "))
	}
}

func maskKey(k string) string {
	if k == "" {
		return ""
	}
	if len(k) <= 4 {
		return "****"
	}
	return "…" + k[len(k)-4:]
}
