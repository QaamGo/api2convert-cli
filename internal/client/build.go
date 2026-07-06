// Package client constructs the api2convert SDK client from resolved config.
package client

import (
	api2convert "github.com/QaamGo/api2convert-go/v10"

	"github.com/QaamGo/api2convert-cli/internal/config"
)

// Build constructs an SDK client from resolved settings. The API key is passed
// explicitly (resolved by the CLI), so precedence is deterministic. A missing
// key yields a *api2convert.ConfigError, which clierr maps to a friendly hint.
func Build(r config.Resolved) (*api2convert.Client, error) {
	var opts []api2convert.Option
	if r.BaseURL != "" {
		opts = append(opts, api2convert.WithBaseURL(r.BaseURL))
	}
	if r.Timeout > 0 {
		opts = append(opts, api2convert.WithTimeout(r.Timeout))
	}
	if r.PollTimeout > 0 {
		opts = append(opts, api2convert.WithPollTimeout(r.PollTimeout))
	}
	if r.MaxRetries >= 0 {
		opts = append(opts, api2convert.WithMaxRetries(r.MaxRetries))
	}
	return api2convert.New(r.APIKey, opts...)
}
