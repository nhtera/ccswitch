package checks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/doctor"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
)

// memStore is an in-memory secrets.Store for tests.
type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: map[string][]byte{}} }

func (m *memStore) Backend() secrets.Backend { return secrets.BackendKeyring }
func (m *memStore) Set(_ context.Context, k string, v []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[k] = append([]byte(nil), v...)
	return nil
}
func (m *memStore) Get(_ context.Context, k string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.data[k]; ok {
		return append([]byte(nil), v...), nil
	}
	return nil, secrets.ErrNotFound
}
func (m *memStore) Delete(_ context.Context, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, k)
	return nil
}
func (m *memStore) List(_ context.Context, prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

// memBridgeIO simulates a live credential.
type memBridgeIO struct {
	cur    []byte
	hasCur bool
}

func (m *memBridgeIO) Read(_ context.Context) ([]byte, error) {
	if !m.hasCur {
		return nil, claude.ErrLiveNotPresent
	}
	return append([]byte(nil), m.cur...), nil
}
func (m *memBridgeIO) Write(_ context.Context, b []byte) error {
	m.cur = append([]byte(nil), b...)
	m.hasCur = true
	return nil
}

func TestSecretsBackendCheck_Pass(t *testing.T) {
	open := func(ctx context.Context) (secrets.Store, error) { return newMemStore(), nil }
	res := SecretsBackendCheck(open).Run(context.Background())
	if res.Status != doctor.StatusPass {
		t.Fatalf("got %s: %s (%s)", res.Status, res.Message, res.FixHint)
	}
}

func TestProfilesStoreCheck_EmptyOK(t *testing.T) {
	t.Setenv("CCSWITCH_CONFIG_DIR", t.TempDir())
	res := ProfilesStoreCheck().Run(context.Background())
	if res.Status != doctor.StatusPass {
		t.Fatalf("got %s: %s", res.Status, res.Message)
	}
}

func TestProfilesStoreCheck_CorruptFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := ProfilesStoreCheck().Run(context.Background())
	if res.Status != doctor.StatusFail {
		t.Fatalf("expected FAIL, got %s: %s", res.Status, res.Message)
	}
}

func TestClaudeCredentialCheck_NoLive(t *testing.T) {
	b := &claude.Bridge{IO: &memBridgeIO{}}
	res := ClaudeCredentialCheck(b).Run(context.Background())
	if res.Status != doctor.StatusWarn {
		t.Fatalf("expected WARN, got %s: %s", res.Status, res.Message)
	}
}

func TestClaudeCredentialCheck_KnownEnvelope(t *testing.T) {
	blob := []byte(`{"claudeAiOauth":{"subscriptionType":"max"}}`)
	b := &claude.Bridge{IO: &memBridgeIO{cur: blob, hasCur: true}}
	res := ClaudeCredentialCheck(b).Run(context.Background())
	if res.Status != doctor.StatusPass {
		t.Fatalf("expected PASS, got %s: %s", res.Status, res.Message)
	}
}

func TestFingerprintMatchCheck_Match(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	store, _ := profile.LoadStore()
	blob := []byte(`{"claudeAiOauth":{"subscriptionType":"max"}}`)
	fp := claude.Fingerprint(blob)
	_ = store.Add(profile.Profile{Name: "work", Type: "max", Fingerprint: fp, CreatedAt: time.Now()})
	b := &claude.Bridge{IO: &memBridgeIO{cur: blob, hasCur: true}}
	res := FingerprintMatchCheck(b).Run(context.Background())
	if res.Status != doctor.StatusPass {
		t.Fatalf("expected PASS, got %s: %s", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "work") {
		t.Fatalf("expected message to name profile, got %q", res.Message)
	}
}

func TestFingerprintMatchCheck_Untracked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	blob := []byte(`{"claudeAiOauth":{"subscriptionType":"max"}}`)
	b := &claude.Bridge{IO: &memBridgeIO{cur: blob, hasCur: true}}
	res := FingerprintMatchCheck(b).Run(context.Background())
	if res.Status != doctor.StatusWarn {
		t.Fatalf("expected WARN for untracked, got %s: %s", res.Status, res.Message)
	}
}

func TestOrphanSecretsCheck_FindsOrphan(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	sec := newMemStore()
	_ = sec.Set(context.Background(), profile.SecretKey("ghost"), []byte("blob"))
	open := func(ctx context.Context) (secrets.Store, error) { return sec, nil }
	res := OrphanSecretsCheck(open).Run(context.Background())
	if res.Status != doctor.StatusWarn {
		t.Fatalf("expected WARN with orphan, got %s: %s", res.Status, res.Message)
	}
	if !strings.Contains(res.Message, "ghost") {
		t.Fatalf("expected message to name orphan key, got %q", res.Message)
	}
}

func TestOrphanSecretsCheck_NoOrphan(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	store, _ := profile.LoadStore()
	_ = store.Add(profile.Profile{Name: "work", Fingerprint: "sha256:x", CreatedAt: time.Now()})
	sec := newMemStore()
	_ = sec.Set(context.Background(), profile.SecretKey("work"), []byte("blob"))
	open := func(ctx context.Context) (secrets.Store, error) { return sec, nil }
	res := OrphanSecretsCheck(open).Run(context.Background())
	if res.Status != doctor.StatusPass {
		t.Fatalf("expected PASS with no orphans, got %s: %s", res.Status, res.Message)
	}
}

func TestShellHookCheck_Disabled(t *testing.T) {
	res := ShellHookCheck(false).Run(context.Background())
	if res.Status != doctor.StatusSkip {
		t.Fatalf("expected SKIP, got %s", res.Status)
	}
}

func TestShellHookCheck_FindsLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"),
		[]byte("# preamble\n[ -f ~/.config/ccswitch/active.env ] && . ~/.config/ccswitch/active.env\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	res := ShellHookCheck(true).Run(context.Background())
	if res.Status != doctor.StatusPass {
		t.Fatalf("expected PASS, got %s: %s", res.Status, res.Message)
	}
}

func TestShellHookCheck_MissingLine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("# nothing relevant\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := ShellHookCheck(true).Run(context.Background())
	if res.Status != doctor.StatusWarn {
		t.Fatalf("expected WARN, got %s: %s", res.Status, res.Message)
	}
}
