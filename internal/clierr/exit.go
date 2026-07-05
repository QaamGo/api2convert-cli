// Package clierr maps the SDK's typed error hierarchy to friendly, actionable
// CLI messages and stable exit codes, and renders a machine-readable error
// envelope for --json.
package clierr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	api2convert "github.com/QaamGo/api2convert-go"
)

// ExitCode is a stable, documented process exit status.
type ExitCode int

const (
	ExitOK          ExitCode = 0
	ExitGeneric     ExitCode = 1
	ExitUsage       ExitCode = 2
	ExitAuth        ExitCode = 3
	ExitQuota       ExitCode = 4
	ExitValidation  ExitCode = 5
	ExitConversion  ExitCode = 6
	ExitTimeout     ExitCode = 7
	ExitNetwork     ExitCode = 8
	ExitInterrupted ExitCode = 130
)

// UsageError marks a bad-invocation error (unknown flag, missing/invalid
// argument) so it maps to ExitUsage rather than a generic failure.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// Coded carries a pre-chosen exit code (and an optional message). Commands that
// render their own output — e.g. a batch summary — return a Coded with an empty
// Msg so the top-level handler exits with the right code without printing again.
type Coded struct {
	Msg  string
	Code ExitCode
}

func (e *Coded) Error() string { return e.Msg }

// Classify turns any error into a user-facing message and its exit code,
// matching most-specific SDK type first via errors.As.
func Classify(err error) (string, ExitCode) {
	if err == nil {
		return "", ExitOK
	}
	// Only a genuine cancellation (Ctrl-C / SIGTERM) is an "interrupt". A
	// per-request DeadlineExceeded is a network timeout and is classified below
	// via the SDK's NetworkError.
	if errors.Is(err, context.Canceled) {
		return "Interrupted.", ExitInterrupted
	}

	var coded *Coded
	if errors.As(err, &coded) {
		return coded.Msg, coded.Code
	}

	var usage *UsageError
	if errors.As(err, &usage) {
		return usage.Error(), ExitUsage
	}

	var cfg *api2convert.ConfigError
	if errors.As(err, &cfg) {
		return "No API key found.\n  Run 'api2convert login' to save one, or set API2CONVERT_API_KEY.\n  Get a key at https://www.api2convert.com/", ExitAuth
	}
	var auth *api2convert.AuthenticationError
	if errors.As(err, &auth) {
		return "Your API key was rejected. Run 'api2convert login' to update it.", ExitAuth
	}
	var pay *api2convert.PaymentRequiredError
	if errors.As(err, &pay) {
		return "You're out of conversion credits.\n  Check 'api2convert credits' or upgrade at https://www.api2convert.com/", ExitQuota
	}
	var rl *api2convert.RateLimitError
	if errors.As(err, &rl) {
		if rl.RetryAfter != nil {
			return fmt.Sprintf("Too many requests. Try again in %d seconds.", *rl.RetryAfter), ExitQuota
		}
		return "Too many requests. Please try again shortly.", ExitQuota
	}
	var val *api2convert.ValidationError
	if errors.As(err, &val) {
		return val.Error() + "\n  See 'api2convert formats' for targets, or 'api2convert options <target>' for options.", ExitValidation
	}
	var nf *api2convert.NotFoundError
	if errors.As(err, &nf) {
		return "Not found: " + nf.Error(), ExitValidation
	}
	var cf *api2convert.ConversionFailedError
	if errors.As(err, &cf) {
		return conversionFailedMessage(cf), ExitConversion
	}
	var tmo *api2convert.ConversionTimeoutError
	if errors.As(err, &tmo) {
		return fmt.Sprintf("Still processing after the timeout.\n  Job %s is running on the server — check 'api2convert jobs status %s' later.", tmo.Job.ID, tmo.Job.ID), ExitTimeout
	}
	var se *api2convert.ServerError
	if errors.As(err, &se) {
		return "api2convert had a server error. Please retry shortly.", ExitNetwork
	}
	var ne *api2convert.NetworkError
	if errors.As(err, &ne) {
		return "Couldn't reach api2convert. Check your internet connection.", ExitNetwork
	}
	var sig *api2convert.SignatureVerificationError
	if errors.As(err, &sig) {
		return "Webhook verification failed: " + sig.Error(), ExitValidation
	}
	// Cobra structural errors (only reached once no SDK error matched) are usage
	// problems, not runtime failures.
	if isCobraUsageError(err) {
		return err.Error(), ExitUsage
	}
	return err.Error(), ExitGeneric
}

// isCobraUsageError recognizes cobra's argument/flag/command validation messages
// so they map to ExitUsage. Checked only after SDK types, so a genuine API error
// whose message happens to contain one of these phrases is not misclassified.
func isCobraUsageError(err error) bool {
	m := err.Error()
	for _, p := range []string{
		"unknown command",
		"unknown flag",
		"unknown shorthand flag",
		"unknown subcommand",
		"requires at least",
		"requires exactly",
		"accepts ",
		"invalid argument",
		"flag needs an argument",
		"bad flag syntax",
	} {
		if strings.Contains(m, p) {
			return true
		}
	}
	return false
}

func conversionFailedMessage(cf *api2convert.ConversionFailedError) string {
	var b strings.Builder
	b.WriteString("Conversion failed.")
	for _, m := range cf.Errors() {
		b.WriteString("\n  - ")
		if m.Code != nil {
			fmt.Fprintf(&b, "[%d] ", *m.Code)
		}
		b.WriteString(m.Message)
	}
	return b.String()
}

// WriteJSONError emits the --json error envelope to w.
func WriteJSONError(w io.Writer, err error, code ExitCode) {
	e := map[string]any{
		"type":      typeName(err),
		"message":   err.Error(),
		"exit_code": int(code),
	}
	var he api2convert.HTTPError
	if errors.As(err, &he) {
		e["status"] = he.Status()
		if id := he.RequestID(); id != "" {
			e["request_id"] = id
		}
	}
	var rl *api2convert.RateLimitError
	if errors.As(err, &rl) && rl.RetryAfter != nil {
		e["retry_after"] = *rl.RetryAfter
	}
	var cf *api2convert.ConversionFailedError
	if errors.As(err, &cf) {
		e["job_id"] = cf.Job.ID
		msgs := make([]map[string]any, 0, len(cf.Errors()))
		for _, m := range cf.Errors() {
			jm := map[string]any{"message": m.Message}
			if m.Code != nil {
				jm["code"] = *m.Code
			}
			msgs = append(msgs, jm)
		}
		e["job_errors"] = msgs
	}
	var tmo *api2convert.ConversionTimeoutError
	if errors.As(err, &tmo) {
		e["job_id"] = tmo.Job.ID
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{"ok": false, "error": e})
}

func typeName(err error) string {
	var a api2convert.Api2ConvertError
	if errors.As(err, &a) {
		s := fmt.Sprintf("%T", a)
		if i := strings.LastIndex(s, "."); i >= 0 {
			s = s[i+1:]
		}
		return s
	}
	return "Error"
}
