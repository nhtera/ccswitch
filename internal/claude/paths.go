package claude

import (
	"os"
	"path/filepath"
	"runtime"
)

// CredentialFilePath returns the path to Claude Code's credential file on
// platforms that store it on disk (Linux/Windows). Honors $CLAUDE_CONFIG_DIR.
//
// On macOS the live credential lives in the Keychain (service "Claude
// Code-credentials"), so this helper still returns a path but readers should
// prefer the Keychain backend.
func CredentialFilePath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".credentials.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".credentials.json"
	}
	return filepath.Join(home, ".claude", ".credentials.json")
}

// LiveBackendKind tells callers what storage backend will be used to
// read/write the live credential on this OS.
type LiveBackendKind string

const (
	LiveBackendKeychain LiveBackendKind = "keychain"  // macOS
	LiveBackendFile     LiveBackendKind = "file"      // Linux / Windows
)

// LiveBackend returns the active live-credential backend kind for this OS.
func LiveBackend() LiveBackendKind {
	if runtime.GOOS == "darwin" {
		return LiveBackendKeychain
	}
	return LiveBackendFile
}

// MacKeychainService is the service name Claude Code uses for its
// generic-password Keychain item on macOS.
const MacKeychainService = "Claude Code-credentials"
