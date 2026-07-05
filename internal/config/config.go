// Package config loads, resolves and persists CLI configuration. Values are
// resolved with the precedence flag > environment > config file > default.
package config

import (
	"fmt"
	"strconv"
	"time"
)

// Config is the on-disk configuration (config.toml). Durations are stored as
// human-editable Go duration strings (e.g. "30s", "5m"). MaxRetries is a pointer
// so an explicit 0 (disable retries) can be distinguished from "unset".
type Config struct {
	APIKey      string `toml:"api_key,omitempty"`
	BaseURL     string `toml:"base_url,omitempty"`
	Timeout     string `toml:"timeout,omitempty"`
	PollTimeout string `toml:"poll_timeout,omitempty"`
	MaxRetries  *int   `toml:"max_retries,omitempty"`
	Output      string `toml:"output,omitempty"`
	Concurrency int    `toml:"concurrency,omitempty"`
}

// Flags carries the raw CLI flag values used during resolution. Empty strings,
// -1 (MaxRetries) and 0 (Concurrency) mean "unset".
type Flags struct {
	APIKey      string
	BaseURL     string
	Timeout     string
	PollTimeout string
	MaxRetries  int
	Output      string
	Concurrency int
}

// Resolved is the effective configuration after merging all sources.
type Resolved struct {
	APIKey      string
	BaseURL     string
	Timeout     time.Duration
	PollTimeout time.Duration
	MaxRetries  int // -1 means "use SDK default"
	Output      string
	Concurrency int // 0 means "auto"
}

// Resolve merges a file Config, CLI Flags and the environment into the effective
// Resolved settings. It returns an error for invalid user input (bad duration,
// non-integer retries, unknown output mode) rather than silently ignoring it.
func Resolve(file Config, fl Flags, getenv func(string) string) (Resolved, error) {
	pick := func(vals ...string) string {
		for _, v := range vals {
			if v != "" {
				return v
			}
		}
		return ""
	}

	r := Resolved{MaxRetries: -1}
	r.APIKey = pick(fl.APIKey, getenv("API2CONVERT_API_KEY"), file.APIKey)
	r.BaseURL = pick(fl.BaseURL, getenv("API2CONVERT_BASE_URL"), file.BaseURL)

	var err error
	if r.Timeout, err = pickDur("--timeout", fl.Timeout, getenv("API2CONVERT_TIMEOUT"), file.Timeout); err != nil {
		return Resolved{}, err
	}
	if r.PollTimeout, err = pickDur("--poll-timeout", fl.PollTimeout, getenv("API2CONVERT_POLL_TIMEOUT"), file.PollTimeout); err != nil {
		return Resolved{}, err
	}

	switch {
	case fl.MaxRetries >= 0:
		r.MaxRetries = fl.MaxRetries
	case getenv("API2CONVERT_MAX_RETRIES") != "":
		n, e := strconv.Atoi(getenv("API2CONVERT_MAX_RETRIES"))
		if e != nil {
			return Resolved{}, fmt.Errorf("API2CONVERT_MAX_RETRIES must be an integer")
		}
		r.MaxRetries = n
	case file.MaxRetries != nil:
		r.MaxRetries = *file.MaxRetries
	}

	r.Output = pick(fl.Output, getenv("API2CONVERT_OUTPUT"), file.Output)
	if r.Output == "" {
		r.Output = "human"
	}
	if r.Output != "human" && r.Output != "json" {
		return Resolved{}, fmt.Errorf("output must be 'human' or 'json', got %q", r.Output)
	}

	switch {
	case fl.Concurrency > 0:
		r.Concurrency = fl.Concurrency
	case file.Concurrency > 0:
		r.Concurrency = file.Concurrency
	}
	return r, nil
}

// pickDur parses the first non-empty duration string, erroring on an invalid one
// (so a bad --timeout is reported instead of silently ignored).
func pickDur(name string, vals ...string) (time.Duration, error) {
	for _, v := range vals {
		if v == "" {
			continue
		}
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, fmt.Errorf("%s: invalid duration %q (use e.g. 30s, 5m)", name, v)
		}
		return d, nil
	}
	return 0, nil
}
