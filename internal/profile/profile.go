// Package profile manages ccswitch's metadata side: profiles.json on disk,
// per-profile environment overlays, and fingerprint-based active detection.
//
// Sensitive credential bytes never live in this package — those go through
// internal/secrets. We only deal in metadata, names, and fingerprints.
package profile

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// CurrentSchemaVersion is the on-disk profiles.json schema version. Bumped
// whenever a non-additive change is required so older binaries refuse to
// silently mis-parse.
const CurrentSchemaVersion = 1

// Profile is one named account snapshot.
//
// Email and OrgName are best-effort identity hints copied from Claude
// Code's global config (~/.claude.json) at capture time. They're
// omitempty for backward compatibility — older profiles.json files
// without these fields still load cleanly.
//
// StableFingerprint is a refresh-safe identity hash (sha256 of
// accountUuid + orgUuid). It survives OAuth token rotation. The
// volatile Fingerprint (over the full credential blob) remains the
// canonical identity key for legacy profiles captured before this
// field existed; callers should try StableFingerprint first and fall
// back to Fingerprint.
type Profile struct {
	Name              string            `json:"name"`
	Type              string            `json:"type"` // oauth | api | sso
	CreatedAt         time.Time         `json:"created_at"`
	LastUsedAt        *time.Time        `json:"last_used_at,omitempty"`
	Note              string            `json:"note,omitempty"`
	Fingerprint       string            `json:"fingerprint"`
	StableFingerprint string            `json:"stable_fingerprint,omitempty"`
	Email             string            `json:"email,omitempty"`
	OrgName           string            `json:"org_name,omitempty"`
	// OAuthAccount is the raw JSON of ~/.claude.json's oauthAccount
	// field at capture time. Restored on switch so Claude Code's
	// `/status` (which reads email/org/login-method from .claude.json)
	// reflects the new account. Omitempty keeps profiles captured
	// before this field existed loading cleanly.
	OAuthAccount json.RawMessage   `json:"oauth_account,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
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

// SuggestName derives a sensible profile name from an account's email
// and (optional) organization name. Org name is preferred when set
// because it's usually more memorable than a personal email handle —
// "erai-dev" is friendlier than "tien-nguyen". Falls back to
// the email local-part. Output is lowercase, kebab-case, capped at 32
// chars, and guaranteed to satisfy ValidateName.
//
// Returns "" if neither input yields any usable characters — caller
// should then prompt the user.
func SuggestName(email, orgName string) string {
	if name := slugify(orgName); name != "" {
		return name
	}
	if at := indexAt(email); at > 0 {
		return slugify(email[:at])
	}
	return slugify(email)
}

func indexAt(s string) int {
	for i, r := range s {
		if r == '@' {
			return i
		}
	}
	return -1
}

// slugify lowercases s, replaces non-[a-z0-9] runs with a single dash,
// trims leading/trailing dashes and underscores, and caps at 32 chars.
// Returns "" if the result would be empty.
func slugify(s string) string {
	if s == "" {
		return ""
	}
	out := make([]rune, 0, len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
			prevDash = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			out = append(out, r)
			prevDash = false
		default:
			if !prevDash && len(out) > 0 {
				out = append(out, '-')
				prevDash = true
			}
		}
	}
	// Trim trailing dash.
	for len(out) > 0 && (out[len(out)-1] == '-' || out[len(out)-1] == '_') {
		out = out[:len(out)-1]
	}
	if len(out) > 32 {
		out = out[:32]
		// Re-trim in case truncation left a trailing dash.
		for len(out) > 0 && (out[len(out)-1] == '-' || out[len(out)-1] == '_') {
			out = out[:len(out)-1]
		}
	}
	return string(out)
}
