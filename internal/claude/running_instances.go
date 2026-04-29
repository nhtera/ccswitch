package claude

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

// ClaudeSession is one row from ~/.claude/sessions/{pid}.json.
// Claude Code itself writes these files; we read them.
type ClaudeSession struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	CWD        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"`  // epoch milliseconds
	Kind       string `json:"kind"`       // interactive | bg | daemon | daemon-worker
	Entrypoint string `json:"entrypoint"` // cli | claude-vscode | claude-desktop | sdk-cli | mcp
	Status     string `json:"status,omitempty"`
}

// IdeInstance is one row from ~/.claude/ide/{port}.lock describing a
// running IDE that has the Claude extension active.
type IdeInstance struct {
	Port             int      `json:"-"` // parsed from filename
	PID              int      `json:"pid"`
	IDEName          string   `json:"ideName"`
	WorkspaceFolders []string `json:"workspaceFolders,omitempty"`
}

// claudeConfigHome resolves the Claude config home directory. Reuses
// the same env var Claude Code itself respects: $CLAUDE_CONFIG_DIR or
// $HOME/.claude.
func claudeConfigHome() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

// ListSessions reads ~/.claude/sessions/*.json and returns the entries
// whose PIDs are still alive. A missing or unreadable directory yields
// an empty slice with no error — the user simply has no sessions.
func ListSessions() ([]ClaudeSession, error) {
	dir := filepath.Join(claudeConfigHome(), "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []ClaudeSession
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s ClaudeSession
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if !isPIDAlive(s.PID) {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// ListIdeInstances reads ~/.claude/ide/*.lock and returns the entries
// whose PIDs are still alive.
func ListIdeInstances() ([]IdeInstance, error) {
	dir := filepath.Join(claudeConfigHome(), "ide")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []IdeInstance
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".lock" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var inst IdeInstance
		if err := json.Unmarshal(data, &inst); err != nil {
			continue
		}
		if !isPIDAlive(inst.PID) {
			continue
		}
		// Filename is "<port>.lock".
		base := e.Name()
		stem := base[:len(base)-len(filepath.Ext(base))]
		if port, err := strconv.Atoi(stem); err == nil {
			inst.Port = port
		}
		out = append(out, inst)
	}
	return out, nil
}

// isPIDAlive is implemented per-OS in running_instances_{unix,windows}.go.
