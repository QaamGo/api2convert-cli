package cli

import (
	"context"

	api2convert "github.com/QaamGo/api2convert-go"

	"github.com/QaamGo/api2convert-cli/internal/client"
	"github.com/QaamGo/api2convert-cli/internal/config"
)

type ctxKey int

const keyResolved ctxKey = iota

func withResolved(ctx context.Context, r config.Resolved) context.Context {
	return context.WithValue(ctx, keyResolved, r)
}

func resolvedFrom(ctx context.Context) config.Resolved {
	r, _ := ctx.Value(keyResolved).(config.Resolved)
	return r
}

// clientFrom builds an SDK client on demand from the resolved config stashed on
// the context. Commands that don't need the network (version, config) never
// call it, so an unconfigured user can still run them.
func clientFrom(ctx context.Context) (*api2convert.Client, error) {
	return client.Build(resolvedFrom(ctx))
}

// buildClient builds an SDK client from an explicit Resolved (used by login,
// which validates a just-entered key before persisting it).
func buildClient(r config.Resolved) (*api2convert.Client, error) {
	return client.Build(r)
}

// catalogKey scopes the on-disk catalog cache to the active account, so one
// account's formats/options are never served to another.
func catalogKey(ctx context.Context) string {
	r := resolvedFrom(ctx)
	return r.APIKey + "|" + r.BaseURL
}
