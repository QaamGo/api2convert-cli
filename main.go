// Command api2convert is the official command-line tool for the api2convert
// file-conversion API — convert, compress and transform files from a single
// self-contained binary.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/QaamGo/api2convert-cli/internal/cli"
)

func main() {
	// A cancellable root context: Ctrl-C / SIGTERM cancels in-flight requests
	// and the poll loop cleanly (every SDK call takes this ctx).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.Execute(ctx, cli.BuildInfo{Version: version, Commit: commit, Date: date}))
}
