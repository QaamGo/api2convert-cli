package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/QaamGo/api2convert-cli/internal/config"
	"github.com/QaamGo/api2convert-cli/internal/selfupdate"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

const (
	// updateCheckInterval bounds how often the background update check runs.
	updateCheckInterval = 7 * 24 * time.Hour
	// updateCheckTimeout bounds the GitHub check so a slow or unreachable network
	// never delays the command the user actually ran.
	updateCheckTimeout = 3 * time.Second
)

// updateCheckSkipCommands lists the top-level commands after which an update
// prompt is redundant or would corrupt output: self-update does the upgrade
// itself, version has its own --check, completion output is parsed by the shell,
// and login/help are setup/help-only flows.
var updateCheckSkipCommands = map[string]bool{
	"self-update":      true,
	"version":          true,
	"completion":       true,
	"login":            true,
	"help":             true,
	"__complete":       true,
	"__completeNoDesc": true,
}

// maybePromptUpdate runs at most once every updateCheckInterval in an interactive
// session: it asks GitHub whether a newer release exists and, if so, offers to
// self-update now. It never returns an error and never fails the command the user
// ran — a network failure, a declined prompt or an unwritable state file all just
// continue silently.
func maybePromptUpdate(cmd *cobra.Command) {
	if !updateCheckAllowed(cmd) {
		return
	}
	st, err := config.LoadState()
	if err != nil {
		return
	}
	if !dueForCheck(st.LastUpdateCheck, time.Now(), updateCheckInterval) {
		return
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), updateCheckTimeout)
	defer cancel()
	res, checkErr := selfupdate.Available(ctx, buildInfo.Version)

	// Record the attempt whatever the outcome: one check per interval, so a down
	// GitHub is never hammered and the user is never renagged before the interval.
	st.LastUpdateCheck = time.Now()
	_ = config.SaveState(st)

	if checkErr != nil || !selfupdate.IsNewer(res.To, res.From) {
		return
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, ui.Bold("A new api2convert version is available: "+res.From+" → "+res.To))
	if !ui.Confirm("Update now? [y/N] ", os.Stdin, out, false) {
		fmt.Fprintln(out, ui.Dim("Skipped — run 'api2convert self-update' anytime (or --no-update-check to silence this)."))
		return
	}
	upd, err := selfupdate.Run(cmd.Context(), buildInfo.Version)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.Dim(err.Error()))
		return
	}
	if upd.Updated {
		fmt.Fprintln(out, ui.Success("Updated "+upd.From+" → "+upd.To))
	}
}

// updateCheckAllowed reports whether we may run the interactive update check for
// this command: not suppressed, in a fully interactive terminal, not JSON/quiet,
// and not one of the skip-list commands.
func updateCheckAllowed(cmd *cobra.Command) bool {
	if gf.noUpdateCheck || updateCheckSuppressedByEnv(os.Getenv) {
		return false
	}
	if gf.json || gf.quiet {
		return false
	}
	// The prompt needs a real terminal on both ends; ui.Interactive also declines
	// under CI / NO_COLOR / dumb terminals.
	if !ui.Interactive(os.Stdout) || !ui.IsTTY(os.Stdin) {
		return false
	}
	return !updateCheckSkipCommands[topLevelName(cmd)]
}

// updateCheckSuppressedByEnv reports whether API2CONVERT_NO_UPDATE_CHECK is set to
// a truthy value.
func updateCheckSuppressedByEnv(getenv func(string) string) bool {
	switch strings.ToLower(strings.TrimSpace(getenv("API2CONVERT_NO_UPDATE_CHECK"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// dueForCheck reports whether interval has elapsed since the last check. A zero
// last (never checked) is always due.
func dueForCheck(last, now time.Time, interval time.Duration) bool {
	return now.Sub(last) >= interval
}

// topLevelName returns the name of cmd's ancestor directly under the root, or ""
// when cmd is the root itself (the no-arg wizard).
func topLevelName(cmd *cobra.Command) string {
	root := cmd.Root()
	c := cmd
	for c.HasParent() && c.Parent() != root {
		c = c.Parent()
	}
	if c == root {
		return ""
	}
	return c.Name()
}
