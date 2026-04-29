package claude

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrLiveNotPresent indicates Claude Code has no credential stored on this
// machine yet. Callers (e.g. `add`) should treat this as user-actionable.
var ErrLiveNotPresent = errors.New("claude: no live credential found (run `claude /login` first)")

// LiveIO is the read/write surface against the live Claude Code credential
// store on this platform. Tests can inject their own implementation.
type LiveIO interface {
	Read(ctx context.Context) ([]byte, error)
	Write(ctx context.Context, blob []byte) error
}

// Bridge ties LiveIO to the higher-level operations the CLI needs:
// fingerprinting, type detection, and atomic write-with-rollback.
type Bridge struct {
	IO LiveIO
}

// NewDefaultBridge returns a Bridge using the platform-default LiveIO:
//   - macOS: a `security` CLI wrapper against the Claude Code-credentials
//     keychain entry.
//   - Linux/Windows: a file-based reader against
//     ${CLAUDE_CONFIG_DIR:-~/.claude}/.credentials.json.
func NewDefaultBridge() *Bridge {
	if runtime.GOOS == "darwin" {
		return &Bridge{IO: &macSecurityIO{Service: MacKeychainService}}
	}
	return &Bridge{IO: &fileIO{Path: CredentialFilePath()}}
}

// ReadLive returns the current live credential or ErrLiveNotPresent.
func (b *Bridge) ReadLive(ctx context.Context) ([]byte, error) {
	return b.IO.Read(ctx)
}

// WriteLive replaces the live credential with newBlob. The previous value is
// snapshotted first; if the post-write read-back doesn't match, we roll back
// to the snapshot and return an error. The blob is not interpreted.
func (b *Bridge) WriteLive(ctx context.Context, newBlob []byte) error {
	prev, err := b.IO.Read(ctx)
	hadPrev := err == nil
	if err != nil && !errors.Is(err, ErrLiveNotPresent) {
		return fmt.Errorf("read live for backup: %w", err)
	}

	if err := b.IO.Write(ctx, newBlob); err != nil {
		return fmt.Errorf("write live: %w", err)
	}
	verify, err := b.IO.Read(ctx)
	if err != nil || !bytes.Equal(verify, newBlob) {
		// Rollback.
		if hadPrev {
			_ = b.IO.Write(ctx, prev)
		}
		if err != nil {
			return fmt.Errorf("verify live: %w", err)
		}
		return errors.New("verify live: written bytes did not match read-back")
	}
	return nil
}

// Fingerprint canonicalizes and SHA-256-hashes the blob.
func (b *Bridge) Fingerprint(blob []byte) string { return Fingerprint(blob) }

// DetectType classifies a blob.
func (b *Bridge) DetectType(blob []byte) Type { return DetectType(blob) }

// fileIO reads/writes the credential file on Linux/Windows.
type fileIO struct {
	Path string
}

func (f *fileIO) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(f.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrLiveNotPresent
		}
		return nil, fmt.Errorf("read %s: %w", f.Path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, ErrLiveNotPresent
	}
	return data, nil
}

func (f *fileIO) Write(ctx context.Context, blob []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(blob); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
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
	return os.Rename(tmp.Name(), f.Path)
}

// macSecurityIO shells out to `security` for the Claude Code keychain entry.
// We use the CLI rather than go-keyring here because we don't always know
// the per-user account name Claude Code wrote with — `security
// find-generic-password -s <service>` matches by service alone.
//
// Keychain, when non-empty, is appended as the trailing positional arg so
// integration tests can exercise the contract against an isolated
// temporary keychain instead of the user's default keychain. Production
// callers leave it empty.
type macSecurityIO struct {
	Service  string
	Keychain string
}

func (m *macSecurityIO) Read(ctx context.Context) ([]byte, error) {
	args := []string{"find-generic-password", "-s", m.Service, "-w"}
	if m.Keychain != "" {
		args = append(args, m.Keychain)
	}
	cmd := exec.CommandContext(ctx, "security", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		es := strings.ToLower(stderr.String())
		if strings.Contains(es, "could not be found") || strings.Contains(es, "specified item could not") {
			return nil, ErrLiveNotPresent
		}
		return nil, fmt.Errorf("security find-generic-password: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	out := bytes.TrimRight(stdout.Bytes(), "\n")
	if len(out) == 0 {
		return nil, ErrLiveNotPresent
	}
	return out, nil
}

func (m *macSecurityIO) Write(ctx context.Context, blob []byte) error {
	// `-U` updates if present, else creates. We deliberately do NOT pass
	// `-A` (allow access from any app) — that would widen the keychain
	// ACL on every switch. The expected one-time "Always Allow" UX is
	// documented in docs/troubleshooting.md.
	args := []string{
		"add-generic-password",
		"-U",
		"-s", m.Service,
		"-a", currentAccount(),
		"-w", string(blob),
	}
	if m.Keychain != "" {
		args = append(args, m.Keychain)
	}
	cmd := exec.CommandContext(ctx, "security", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("security add-generic-password: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func currentAccount() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "ccswitch"
}
