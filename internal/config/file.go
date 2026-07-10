package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
)

// Load reads config.toml. A missing file is not an error — it returns a zero
// Config so first-run works without any file present.
func Load() (Config, error) {
	p, err := File()
	if err != nil {
		return Config{}, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if _, err := toml.Decode(string(b), &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save writes config.toml with 0600 permissions (dir 0700), so a stored API key
// is never group/other-readable. The write is atomic (temp file + rename) so a
// crash mid-write can't corrupt an existing config, and the 0600 mode is applied
// explicitly — O_CREATE alone would leave a pre-existing 0644 file world-readable.
func Save(c Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p, err := File()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup: a no-op once the rename below succeeds.
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := toml.NewEncoder(tmp).Encode(c); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, p)
}
