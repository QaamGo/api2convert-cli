package config

import (
	"os"
	"path/filepath"
)

const appDir = "api2convert"

// Dir returns the OS-appropriate config directory for the CLI
// (~/.config/api2convert, %AppData%\api2convert, ~/Library/Application Support/api2convert).
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appDir), nil
}

// File returns the full path to config.toml.
func File() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}
