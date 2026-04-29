package claude

import (
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
