package config

import (
	"testing"
	"time"
)

func TestStateRoundTrip(t *testing.T) {
	// os.UserConfigDir honors XDG_CONFIG_HOME on Linux, so point it at a temp dir
	// and keep the test hermetic (no real ~/.config writes).
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// A missing state file is a zero State, not an error.
	s, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState (missing): %v", err)
	}
	if !s.LastUpdateCheck.IsZero() {
		t.Errorf("missing state should have a zero timestamp, got %v", s.LastUpdateCheck)
	}

	want := time.Now().UTC().Truncate(time.Second)
	if err := SaveState(State{LastUpdateCheck: want}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !got.LastUpdateCheck.Equal(want) {
		t.Errorf("round-tripped timestamp = %v, want %v", got.LastUpdateCheck, want)
	}
}
