package config

import (
	"testing"
	"time"
)

func mustResolve(t *testing.T, file Config, fl Flags, getenv func(string) string) Resolved {
	t.Helper()
	r, err := Resolve(file, fl, getenv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return r
}

func TestResolvePrecedence(t *testing.T) {
	env := map[string]string{"API2CONVERT_API_KEY": "envkey"}
	getenv := func(k string) string { return env[k] }

	// flag > env > file
	r := mustResolve(t, Config{APIKey: "filekey"}, Flags{APIKey: "flagkey", MaxRetries: -1}, getenv)
	if r.APIKey != "flagkey" {
		t.Errorf("flag should win, got %q", r.APIKey)
	}
	r = mustResolve(t, Config{APIKey: "filekey"}, Flags{MaxRetries: -1}, getenv)
	if r.APIKey != "envkey" {
		t.Errorf("env should beat file, got %q", r.APIKey)
	}
	r = mustResolve(t, Config{APIKey: "filekey"}, Flags{MaxRetries: -1}, func(string) string { return "" })
	if r.APIKey != "filekey" {
		t.Errorf("file should be used, got %q", r.APIKey)
	}
}

func TestResolveDefaultsAndDurations(t *testing.T) {
	none := func(string) string { return "" }
	r := mustResolve(t, Config{Timeout: "30s", PollTimeout: "5m"}, Flags{MaxRetries: -1}, none)
	if r.Timeout != 30*time.Second {
		t.Errorf("timeout = %v", r.Timeout)
	}
	if r.PollTimeout != 5*time.Minute {
		t.Errorf("poll timeout = %v", r.PollTimeout)
	}
	if r.Output != "human" {
		t.Errorf("output default = %q, want human", r.Output)
	}
	if r.MaxRetries != -1 {
		t.Errorf("max retries default = %d, want -1 (unset)", r.MaxRetries)
	}

	// Bad duration string is now a reported error, not silently ignored.
	if _, err := Resolve(Config{Timeout: "not-a-duration"}, Flags{MaxRetries: -1}, none); err == nil {
		t.Error("bad duration should return an error")
	}
}

func TestResolveRejectsNonsensicalNumbers(t *testing.T) {
	none := func(string) string { return "" }

	// Negative durations are rejected instead of being silently dropped to the
	// SDK default by the downstream `if r.Timeout > 0` guard.
	if _, err := Resolve(Config{Timeout: "-5s"}, Flags{MaxRetries: -1}, none); err == nil {
		t.Error("negative --timeout should return an error")
	}
	if _, err := Resolve(Config{PollTimeout: "-1m"}, Flags{MaxRetries: -1}, none); err == nil {
		t.Error("negative poll timeout should return an error")
	}

	// A negative --max-retries flag value other than the -1 "unset" sentinel is
	// rejected rather than silently falling through (mirrors --timeout).
	if _, err := Resolve(Config{}, Flags{MaxRetries: -5}, none); err == nil {
		t.Error("negative --max-retries flag should return an error")
	}
	// The -1 sentinel means "unset" and must still resolve cleanly.
	if r := mustResolve(t, Config{}, Flags{MaxRetries: -1}, none); r.MaxRetries != -1 {
		t.Errorf("--max-retries -1 sentinel should stay unset, got %d", r.MaxRetries)
	}

	// Negative max_retries from the config file is rejected.
	neg := -3
	if _, err := Resolve(Config{MaxRetries: &neg}, Flags{MaxRetries: -1}, none); err == nil {
		t.Error("negative max_retries from file should return an error")
	}

	// Negative max_retries from the environment is rejected.
	env := func(k string) string {
		if k == "API2CONVERT_MAX_RETRIES" {
			return "-2"
		}
		return ""
	}
	if _, err := Resolve(Config{}, Flags{MaxRetries: -1}, env); err == nil {
		t.Error("negative API2CONVERT_MAX_RETRIES should return an error")
	}
}

func TestResolveMaxRetriesZeroFromFile(t *testing.T) {
	none := func(string) string { return "" }
	zero := 0
	r := mustResolve(t, Config{MaxRetries: &zero}, Flags{MaxRetries: -1}, none)
	if r.MaxRetries != 0 {
		t.Errorf("explicit 0 from file should resolve to 0, got %d", r.MaxRetries)
	}
}
