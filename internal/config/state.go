package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// State is machine-managed CLI state, kept separate from the user-edited
// config.toml so a routine write (e.g. the update-check timestamp) never touches
// the API-key-bearing config file. It lives beside it as state.json.
type State struct {
	// LastUpdateCheck is when the background "newer release?" check last ran. A
	// zero value means it has never run.
	LastUpdateCheck time.Time `json:"last_update_check,omitempty"`
}

// StateFile returns the full path to state.json.
func StateFile() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "state.json"), nil
}

// LoadState reads state.json. A missing file is not an error — it returns a zero
// State so first-run works without any file present.
func LoadState() (State, error) {
	p, err := StateFile()
	if err != nil {
		return State{}, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	return s, nil
}

// SaveState writes state.json atomically (temp file + rename) so a crash
// mid-write can't corrupt an existing file.
func SaveState(s State) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p, err := StateFile()
	if err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup: a no-op once the rename below succeeds.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, p)
}
