package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// AccountInfo is the human-readable account identity Claude Code stores
// in its global config file. Fields are best-effort: any subset may be
// missing depending on whether the account is personal vs org and which
// Claude Code version wrote the file. Callers must handle nil and zero
// fields gracefully.
type AccountInfo struct {
	Email       string `json:"emailAddress"`
	OrgName     string `json:"organizationName,omitempty"`
	OrgUUID     string `json:"organizationUuid,omitempty"`
	AccountUUID string `json:"accountUuid,omitempty"`
}

// IsPersonal reports whether the account has no organization metadata.
func (a *AccountInfo) IsPersonal() bool {
	if a == nil {
		return false
	}
	return a.OrgUUID == "" && a.OrgName == ""
}

// StableFingerprint returns a refresh-safe identity hash derived from
// the account-stable claims (accountUuid + orgUuid). Unlike
// Fingerprint() which hashes the volatile credential blob (changes on
// every token refresh), this stays constant across token refreshes —
// so callers can match a refreshed live credential against profiles
// captured before the refresh.
//
// Returns "" if neither accountUuid nor orgUuid is present, in which
// case callers should fall back to the volatile Fingerprint().
func (a *AccountInfo) StableFingerprint() string {
	if a == nil {
		return ""
	}
	if a.AccountUUID == "" && a.OrgUUID == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(a.AccountUUID))
	h.Write([]byte{'|'})
	h.Write([]byte(a.OrgUUID))
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// GlobalConfigPath returns the path to Claude Code's global config file.
// Mirrors claude-code's own resolution (utils/env.ts getGlobalClaudeFile):
//
//   - $CLAUDE_CONFIG_DIR/.config.json (legacy) if it exists, else
//   - $CLAUDE_CONFIG_DIR/.claude.json if CLAUDE_CONFIG_DIR is set, else
//   - ~/.claude.json
//
// Note the asymmetry with the credential file: .claude.json sits at the
// homedir by default, NOT inside the .claude/ directory.
func GlobalConfigPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		legacy := filepath.Join(dir, ".config.json")
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
		return filepath.Join(dir, ".claude.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude.json"
	}
	legacy := filepath.Join(home, ".claude", ".config.json")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return filepath.Join(home, ".claude.json")
}

// ReadAccountInfo parses Claude Code's global config file and extracts
// the oauthAccount block. Returns (nil, nil) — NOT an error — when:
//   - the config file is missing (user hasn't logged in yet)
//   - the file exists but contains no oauthAccount block
//   - the oauthAccount block has no email
//
// We deliberately swallow these "soft" cases so callers can fall back
// to asking the user for a name. A genuine I/O or JSON-parse error
// still bubbles up so doctor can flag it.
func ReadAccountInfo() (*AccountInfo, error) {
	return readAccountInfoFrom(GlobalConfigPath())
}

func readAccountInfoFrom(path string) (*AccountInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var outer struct {
		OAuthAccount *AccountInfo `json:"oauthAccount"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		return nil, err
	}
	if outer.OAuthAccount == nil || outer.OAuthAccount.Email == "" {
		return nil, nil
	}
	return outer.OAuthAccount, nil
}

// ReadOAuthAccountRaw returns the raw bytes of ~/.claude.json's
// oauthAccount field. Used at capture time to snapshot ALL fields
// (including ones we don't model in AccountInfo, like
// organizationRole or displayName) so they can be restored on switch.
//
// Returns (nil, nil) when the file is missing, has no oauthAccount
// block, or the block is empty.
func ReadOAuthAccountRaw() (json.RawMessage, error) {
	data, err := os.ReadFile(GlobalConfigPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var outer struct {
		OAuthAccount json.RawMessage `json:"oauthAccount"`
	}
	if err := json.Unmarshal(data, &outer); err != nil {
		return nil, err
	}
	if len(outer.OAuthAccount) == 0 || string(outer.OAuthAccount) == "null" {
		return nil, nil
	}
	return outer.OAuthAccount, nil
}

// WriteOAuthAccount atomically replaces just the `oauthAccount`
// field of ~/.claude.json with snapshot, preserving every other
// top-level field unchanged. Used after a profile switch so Claude
// Code's `/status` (which reads email/org/login-method from
// .claude.json) reflects the new account.
//
// Concurrent writes from Claude Code itself are theoretically
// possible but extremely rare — Claude Code only writes
// .claude.json on /login, /logout, and account-mutating
// operations, none of which run concurrently with a CLI switch.
//
// If snapshot is nil, the field is removed (Claude Code will
// repopulate from the access token on next /status).
func WriteOAuthAccount(snapshot json.RawMessage) error {
	path := GlobalConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Bootstrap: create a minimal file with just the
			// oauthAccount block so Claude Code can pick it up on
			// next read.
			if snapshot == nil {
				return nil
			}
			doc := map[string]json.RawMessage{"oauthAccount": snapshot}
			out, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			return atomicWrite(path, out, 0o600)
		}
		return err
	}

	// Decode top-level fields as raw messages so we preserve the
	// exact bytes of every field we don't touch — important
	// because Claude Code may have added new fields we don't know
	// about, and we shouldn't drop or reformat them.
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	if doc == nil {
		doc = map[string]json.RawMessage{}
	}
	if snapshot == nil {
		delete(doc, "oauthAccount")
	} else {
		doc["oauthAccount"] = snapshot
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return atomicWrite(path, out, 0o600)
}

// atomicWrite writes data to path via temp+rename so a crash mid-write
// can never leave a half-written file.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".ccswitch-claude-config-*.tmp")
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	return os.Rename(tmp.Name(), path)
}
