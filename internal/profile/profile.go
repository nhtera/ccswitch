// Package profile manages ccswitch's metadata side: profiles.json on disk,
// per-profile environment overlays, and fingerprint-based active detection.
//
// Sensitive credential bytes never live in this package — those go through
// internal/secrets. We only deal in metadata, names, and fingerprints.
package profile

import (
	"fmt"
	"regexp"
	"time"
)

// CurrentSchemaVersion is the on-disk profiles.json schema version. Bumped
// whenever a non-additive change is required so older binaries refuse to
// silently mis-parse.
const CurrentSchemaVersion = 1

// Profile is one named account snapshot.
type Profile struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"` // oauth | api | sso
	CreatedAt   time.Time         `json:"created_at"`
	LastUsedAt  *time.Time        `json:"last_used_at,omitempty"`
	Note        string            `json:"note,omitempty"`
	Fingerprint string            `json:"fingerprint"`
	Env         map[string]string `json:"env,omitempty"`
}

// File is the persisted shape of profiles.json.
type File struct {
	Version  int       `json:"version"`
	Profiles []Profile `json:"profiles"`
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)
var envKeyRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// ValidateName rejects empty / overlong / unsafe profile names.
func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must match %s", name, nameRe.String())
	}
	return nil
}

// ValidateEnvKey enforces POSIX-ish env var naming.
func ValidateEnvKey(key string) error {
	if !envKeyRe.MatchString(key) {
		return fmt.Errorf("invalid env key %q: must match %s", key, envKeyRe.String())
	}
	return nil
}

// SecretKey returns the keyring key under which the named profile's
// credential blob is stored.
func SecretKey(name string) string { return "profile." + name }
