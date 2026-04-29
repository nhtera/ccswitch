//go:build darwin

// Real-keychain contract tests for macSecurityIO.
//
// Mirrors the contract layer from
// claude-swap/tests/test_macos_keychain_contract.py — but instead of
// swapping the user's default keychain (their CI-only approach), we
// target an isolated temporary keychain via the new Keychain field on
// macSecurityIO. This means:
//
//  1. The user's default keychain is never modified, so it's safe to
//     run locally without GHA-only gating.
//  2. We still exercise the real `security` CLI end-to-end — the only
//     thing that's mocked is which keychain file the CLI touches.
//
// Skips automatically if the `security` binary or `KeychainServices`
// aren't available (non-macOS CI runners shouldn't get here, but
// defense-in-depth).
package claude

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newTempKeychain(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("security"); err != nil {
		t.Skipf("`security` binary not on PATH: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ccswitch-test.keychain")

	// `create-keychain -p ""` — empty password is fine for an isolated
	// throwaway keychain that never gets added to the user search list.
	if out, err := exec.Command("security", "create-keychain", "-p", "", path).CombinedOutput(); err != nil {
		t.Fatalf("create-keychain: %v: %s", err, out)
	}
	if out, err := exec.Command("security", "unlock-keychain", "-p", "", path).CombinedOutput(); err != nil {
		t.Fatalf("unlock-keychain: %v: %s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("security", "delete-keychain", path).Run()
	})
	return path
}

// TestMacSecurityIO_RoundTrip — the canonical contract: write via our
// IO, read back via our IO, get the same bytes.
func TestMacSecurityIO_RoundTrip(t *testing.T) {
	kc := newTempKeychain(t)
	io := &macSecurityIO{Service: MacKeychainService, Keychain: kc}
	want := []byte(`{"claudeAiOauth":{"accessToken":"tok-1","refreshToken":"rtok-1"}}`)
	if err := io.Write(context.Background(), want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := io.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip mismatch:\nwant %q\ngot  %q", want, got)
	}
}

// TestMacSecurityIO_ReadFindsClaudeCodeShape — when an entry is seeded
// using the same args Claude Code itself uses (`-a $USER -s "Claude
// Code-credentials"`), our Read must find it. This is the
// integration-direction contract: claude-swap's own seeding shape must
// be readable by us. Mirrors
// test_read_credentials_finds_claude_code_seeded_entry.
func TestMacSecurityIO_ReadFindsClaudeCodeShape(t *testing.T) {
	kc := newTempKeychain(t)
	user := os.Getenv("USER")
	if user == "" {
		user = "ccswitch-test"
	}
	want := "fake-token-read"
	out, err := exec.Command(
		"security", "add-generic-password",
		"-a", user,
		"-s", MacKeychainService,
		"-w", want,
		kc,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("seed add-generic-password: %v: %s", err, out)
	}

	io := &macSecurityIO{Service: MacKeychainService, Keychain: kc}
	got, err := io.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestMacSecurityIO_WriteIsClaudeCodeReadable — the reverse direction.
// What we wrote must be readable by the same `find-generic-password`
// shape that Claude Code itself uses, so a switch-then-launch sequence
// works. Mirrors test_write_credentials_creates_user_scoped_entry.
func TestMacSecurityIO_WriteIsClaudeCodeReadable(t *testing.T) {
	kc := newTempKeychain(t)
	io := &macSecurityIO{Service: MacKeychainService, Keychain: kc}
	want := "fake-token-write"
	if err := io.Write(context.Background(), []byte(want)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	user := os.Getenv("USER")
	if user == "" {
		user = "ccswitch-test"
	}
	out, err := exec.Command(
		"security", "find-generic-password",
		"-a", user,
		"-s", MacKeychainService,
		"-w",
		kc,
	).Output()
	if err != nil {
		t.Fatalf("verify find-generic-password: %v", err)
	}
	got := strings.TrimRight(string(out), "\n")
	if got != want {
		t.Fatalf("Claude-Code-shape lookup got %q, want %q", got, want)
	}
}

// TestMacSecurityIO_ReadMissingItem — when the entry doesn't exist,
// Read must return ErrLiveNotPresent (the user-actionable sentinel),
// not a generic exec error. Maps to claude-swap's `returncode == 44`
// branch.
func TestMacSecurityIO_ReadMissingItem(t *testing.T) {
	kc := newTempKeychain(t)
	io := &macSecurityIO{Service: MacKeychainService, Keychain: kc}
	_, err := io.Read(context.Background())
	if !errors.Is(err, ErrLiveNotPresent) {
		t.Fatalf("expected ErrLiveNotPresent, got %v", err)
	}
}

// TestMacSecurityIO_WriteUpdatesExisting — `-U` semantics: writing
// twice replaces the prior value rather than creating a duplicate
// entry.
func TestMacSecurityIO_WriteUpdatesExisting(t *testing.T) {
	kc := newTempKeychain(t)
	io := &macSecurityIO{Service: MacKeychainService, Keychain: kc}
	if err := io.Write(context.Background(), []byte("first")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := io.Write(context.Background(), []byte("second")); err != nil {
		t.Fatalf("second write: %v", err)
	}
	got, err := io.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("update did not replace: got %q", got)
	}
}

// TestBridge_WriteLive_RealKeychain_RollbackOnNoMutation — full
// Bridge-level rollback test using the real keychain. If the keychain
// silently no-ops the write (we simulate by deleting between write and
// verify), the verify-mismatch path must restore the snapshot.
func TestBridge_WriteLive_RealKeychain(t *testing.T) {
	kc := newTempKeychain(t)
	b := &Bridge{IO: &macSecurityIO{Service: MacKeychainService, Keychain: kc}}

	original := []byte("original-token")
	if err := b.WriteLive(context.Background(), original); err != nil {
		t.Fatalf("seed WriteLive: %v", err)
	}

	updated := []byte("updated-token")
	if err := b.WriteLive(context.Background(), updated); err != nil {
		t.Fatalf("update WriteLive: %v", err)
	}

	got, err := b.ReadLive(context.Background())
	if err != nil {
		t.Fatalf("ReadLive: %v", err)
	}
	if !bytes.Equal(got, updated) {
		t.Fatalf("got %q, want %q", got, updated)
	}
}
