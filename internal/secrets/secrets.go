// Package secrets is the keyring abstraction for ccswitch.
//
// It hides the platform secret backend (macOS Keychain via go-keyring,
// Linux libsecret, Windows Credential Manager) behind a single Store
// interface, with an AES-256-GCM file fallback for headless / SSH sessions
// where the system keyring is unreachable.
//
// Bytes stored here are opaque — callers (internal/claude, internal/profile)
// pass credential blobs and never inspect them inside this package.
package secrets

import (
	"context"
	"errors"
)

// Backend identifies which storage implementation is active.
type Backend string

const (
	BackendKeyring Backend = "keyring"
	BackendFile    Backend = "file"
)

// Service is the keyring service name used for every entry ccswitch writes.
// Per-key separation is achieved by composing keys (e.g. "profile.<name>",
// "live", "live.bak").
const Service = "ccswitch"

// ErrNotFound is returned by Get/Delete when the key is absent.
var ErrNotFound = errors.New("secrets: key not found")

// Store is the abstraction every command uses. Implementations MUST be safe
// for sequential use within a single ccswitch invocation; cross-process
// coordination is the caller's job (see internal/profile file lock).
type Store interface {
	Set(ctx context.Context, key string, value []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
	Backend() Backend
}
