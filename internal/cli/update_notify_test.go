package cli

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestDueForCheck(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		last time.Time
		want bool
	}{
		{"never checked (zero)", time.Time{}, true},
		{"just now", now, false},
		{"6 days ago", now.Add(-6 * 24 * time.Hour), false},
		{"exactly 7 days ago", now.Add(-7 * 24 * time.Hour), true},
		{"8 days ago", now.Add(-8 * 24 * time.Hour), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dueForCheck(c.last, now, updateCheckInterval); got != c.want {
				t.Errorf("dueForCheck(%v) = %v, want %v", c.last, got, c.want)
			}
		})
	}
}

func TestUpdateCheckSuppressedByEnv(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
		{"  on  ", true},
		{"", false},
		{"0", false},
		{"false", false},
		{"no", false},
	}
	for _, c := range cases {
		got := updateCheckSuppressedByEnv(func(string) string { return c.val })
		if got != c.want {
			t.Errorf("updateCheckSuppressedByEnv(%q) = %v, want %v", c.val, got, c.want)
		}
	}
}

func TestTopLevelName(t *testing.T) {
	root := &cobra.Command{Use: "api2convert"}
	selfUpdate := &cobra.Command{Use: "self-update"}
	completion := &cobra.Command{Use: "completion"}
	completionBash := &cobra.Command{Use: "bash"}
	config := &cobra.Command{Use: "config"}
	configSet := &cobra.Command{Use: "set"}

	completion.AddCommand(completionBash)
	config.AddCommand(configSet)
	root.AddCommand(selfUpdate, completion, config)

	cases := []struct {
		name string
		cmd  *cobra.Command
		want string
	}{
		{"root itself (wizard)", root, ""},
		{"top-level leaf", selfUpdate, "self-update"},
		{"nested completion subcommand maps to parent", completionBash, "completion"},
		{"nested config subcommand maps to parent", configSet, "config"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := topLevelName(c.cmd); got != c.want {
				t.Errorf("topLevelName(%q) = %q, want %q", c.cmd.Name(), got, c.want)
			}
		})
	}
}

// TestUpdateCheckSkipsSuppressCommands is a light guard that the skip-list keys
// stay in sync with the commands we mean to exclude.
func TestUpdateCheckSkipsSuppressCommands(t *testing.T) {
	for _, name := range []string{"self-update", "version", "completion", "login", "help", "__complete"} {
		if !updateCheckSkipCommands[name] {
			t.Errorf("%q should be in the update-check skip list", name)
		}
	}
	if updateCheckSkipCommands["convert"] {
		t.Error("convert must NOT be in the skip list")
	}
}
