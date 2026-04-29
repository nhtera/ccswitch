package claude

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// memoryIO is an in-memory LiveIO for unit tests.
type memoryIO struct {
	mu        sync.Mutex
	cur       []byte
	hasCur    bool
	failWrite error
	failRead  error
	// silentWrite, when true, makes Write a no-op (cur stays the same).
	// Used to simulate a keychain that "succeeds" but doesn't actually
	// update — the post-write verify will then read the unchanged value
	// and trigger rollback.
	silentWrite bool
}

func (m *memoryIO) Read(_ context.Context) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failRead != nil {
		return nil, m.failRead
	}
	if !m.hasCur {
		return nil, ErrLiveNotPresent
	}
	return append([]byte(nil), m.cur...), nil
}

func (m *memoryIO) Write(_ context.Context, blob []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failWrite != nil {
		return m.failWrite
	}
	if m.silentWrite {
		return nil
	}
	m.cur = append([]byte(nil), blob...)
	m.hasCur = true
	return nil
}

func loadTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

func TestDetectType_OAuthPro(t *testing.T) {
	if got := DetectType(loadTestdata(t, "envelope-pro.json")); got != TypeOAuth {
		t.Fatalf("got %q, want oauth", got)
	}
}

func TestDetectType_OAuthMax(t *testing.T) {
	if got := DetectType(loadTestdata(t, "envelope-max.json")); got != TypeOAuth {
		t.Fatalf("got %q, want oauth", got)
	}
}

func TestDetectType_SSO(t *testing.T) {
	if got := DetectType(loadTestdata(t, "envelope-sso.json")); got != TypeSSO {
		t.Fatalf("got %q, want sso", got)
	}
}

func TestDetectType_BareAPI(t *testing.T) {
	if got := DetectType(loadTestdata(t, "envelope-api.txt")); got != TypeAPI {
		t.Fatalf("got %q, want api", got)
	}
}

func TestDetectType_Garbage(t *testing.T) {
	if got := DetectType([]byte("not a credential")); got != TypeUnknown {
		t.Fatalf("got %q, want unknown", got)
	}
}

func TestDetectType_BareAPIRejectsOverlong(t *testing.T) {
	// A "key" that exceeds the bare-API length cap should not be
	// classified as TypeAPI — concatenation artifacts shouldn't pass.
	overlong := append([]byte("sk-ant-api"), bytes.Repeat([]byte("x"), 1024)...)
	if got := DetectType(overlong); got == TypeAPI {
		t.Fatalf("overlong sk-ant-api should not classify as TypeAPI, got %q", got)
	}
}

func TestValidate_RejectsGarbage(t *testing.T) {
	if err := Validate([]byte("garbage")); !errors.Is(err, ErrMalformed) {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

func TestFingerprint_StableAcrossWhitespace(t *testing.T) {
	a := loadTestdata(t, "envelope-pro.json")
	b := bytes.ReplaceAll(a, []byte("  "), []byte(" "))
	b = bytes.ReplaceAll(b, []byte("\n"), []byte(""))

	if Fingerprint(a) != Fingerprint(b) {
		t.Fatalf("fingerprint should be stable across whitespace differences")
	}
}

func TestFingerprint_DiffersByContent(t *testing.T) {
	a := loadTestdata(t, "envelope-pro.json")
	b := loadTestdata(t, "envelope-max.json")
	if Fingerprint(a) == Fingerprint(b) {
		t.Fatalf("different envelopes should have different fingerprints")
	}
}

func TestFingerprint_FormatPrefix(t *testing.T) {
	got := Fingerprint(loadTestdata(t, "envelope-pro.json"))
	if !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("expected sha256: prefix, got %q", got)
	}
}

func TestBridge_WriteLive_RoundTrip(t *testing.T) {
	io := &memoryIO{cur: []byte("old"), hasCur: true}
	b := &Bridge{IO: io}
	want := []byte("new value")
	if err := b.WriteLive(context.Background(), want); err != nil {
		t.Fatalf("WriteLive: %v", err)
	}
	got, _ := b.ReadLive(context.Background())
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBridge_WriteLive_RollbackOnVerifyMismatch(t *testing.T) {
	// Simulate a keychain that "accepts" the write silently but never
	// actually updates the stored value. The verify read sees the original
	// bytes, mismatch detected, rollback restores the original.
	io := &memoryIO{cur: []byte("original"), hasCur: true, silentWrite: true}
	b := &Bridge{IO: io}
	err := b.WriteLive(context.Background(), []byte("attempted"))
	if err == nil {
		t.Fatalf("expected verify-mismatch error")
	}
	got, _ := b.ReadLive(context.Background())
	if !bytes.Equal(got, []byte("original")) {
		t.Fatalf("rollback failed: live = %q", got)
	}
}

func TestBridge_WriteLive_NoPriorCredential(t *testing.T) {
	io := &memoryIO{}
	b := &Bridge{IO: io}
	if err := b.WriteLive(context.Background(), []byte("first")); err != nil {
		t.Fatalf("WriteLive into empty: %v", err)
	}
}
