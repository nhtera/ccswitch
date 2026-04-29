package secrets

import (
	"os"
	"path/filepath"
)

// configDir returns the directory where ccswitch stores its non-secret
// metadata (profiles.json, keyring index, encrypted file fallback).
//
// Honors XDG_CONFIG_HOME on POSIX and %AppData% on Windows.
func configDir() (string, error) {
	if v := os.Getenv("CCSWITCH_CONFIG_DIR"); v != "" {
		return v, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ccswitch"), nil
}

// ConfigDir is the public accessor for the config directory.
// Other packages (profile, envfile) need it.
func ConfigDir() (string, error) { return configDir() }

func indexPath() (string, error) {
	d, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "keyring-index.json"), nil
}

func fileVaultPath() (string, error) {
	d, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "secrets.enc"), nil
}
