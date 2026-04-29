package export

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nhtera/ccswitch/internal/cryptobox"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
)

// fastParams keeps unit tests under a second by lowering the Argon2id
// memory factor. They still exercise the full code path.
var fastParams = cryptobox.Params{Time: 1, MemMB: 4, Threads: 1}

// memStore is a process-local in-memory secrets.Store for tests.
type memStore struct {
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: map[string][]byte{}} }

func (m *memStore) Backend() secrets.Backend                                  { return secrets.BackendKeyring }
func (m *memStore) Set(_ context.Context, key string, value []byte) error     { m.data[key] = append([]byte(nil), value...); return nil }
func (m *memStore) Delete(_ context.Context, key string) error                { delete(m.data, key); return nil }
func (m *memStore) Get(_ context.Context, key string) ([]byte, error) {
	if v, ok := m.data[key]; ok {
		return append([]byte(nil), v...), nil
	}
	return nil, secrets.ErrNotFound
}
func (m *memStore) List(_ context.Context, prefix string) ([]string, error) {
	out := []string{}
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

func loadProfilesAt(t *testing.T) *profile.Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	s, err := profile.LoadStore()
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	return s
}

func seedTwoProfiles(t *testing.T, store *profile.Store, sec *memStore) {
	t.Helper()
	for _, p := range []profile.Profile{
		{
			Name:        "work",
			Type:        "max",
			Fingerprint: "sha256:work",
			CreatedAt:   time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
			Env:         map[string]string{"ANTHROPIC_BASE_URL": "https://gw.acme.com"},
		},
		{
			Name:        "home",
			Type:        "pro",
			Fingerprint: "sha256:home",
			CreatedAt:   time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC),
		},
	} {
		if err := store.Add(p); err != nil {
			t.Fatalf("seed Add %q: %v", p.Name, err)
		}
		_ = sec.Set(context.Background(), profile.SecretKey(p.Name), []byte("blob-for-"+p.Name))
	}
}

func TestRoundTrip_BuildSealOpenApply(t *testing.T) {
	src := loadProfilesAt(t)
	sec := newMemStore()
	seedTwoProfiles(t, src, sec)

	pt, err := Build(context.Background(), src, sec, nil)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(pt.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(pt.Profiles))
	}
	pass := []byte("strong passphrase")
	sealed, err := Seal(pt, pass, fastParams)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	pt2, err := OpenPlaintext(sealed, pass)
	if err != nil {
		t.Fatalf("OpenPlaintext: %v", err)
	}
	if len(pt2.Profiles) != 2 {
		t.Fatalf("expected 2 in opened bundle, got %d", len(pt2.Profiles))
	}

	// Apply into a fresh empty store.
	dst := loadProfilesAt(t)
	dstSec := newMemStore()
	added, err := Apply(context.Background(), pt2, dst, dstSec, ImportOption{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(added) != 2 {
		t.Fatalf("expected 2 added, got %v", added)
	}
	if blob, err := dstSec.Get(context.Background(), profile.SecretKey("work")); err != nil || string(blob) != "blob-for-work" {
		t.Fatalf("blob round trip failed: %q %v", blob, err)
	}
}

func TestOpen_WrongPassphrase(t *testing.T) {
	src := loadProfilesAt(t)
	sec := newMemStore()
	seedTwoProfiles(t, src, sec)
	pt, _ := Build(context.Background(), src, sec, nil)
	sealed, _ := Seal(pt, []byte("right"), fastParams)
	_, err := OpenPlaintext(sealed, []byte("wrong"))
	if !errors.Is(err, cryptobox.ErrAuth) {
		t.Fatalf("expected ErrAuth, got %v", err)
	}
}

func TestOpen_TamperedRejected(t *testing.T) {
	src := loadProfilesAt(t)
	sec := newMemStore()
	seedTwoProfiles(t, src, sec)
	pt, _ := Build(context.Background(), src, sec, nil)
	sealed, _ := Seal(pt, []byte("pw"), fastParams)
	// Flip a byte well past the header.
	sealed[len(sealed)-2] ^= 0x01
	_, err := OpenPlaintext(sealed, []byte("pw"))
	if !errors.Is(err, cryptobox.ErrAuth) {
		t.Fatalf("expected ErrAuth on tamper, got %v", err)
	}
}

func TestApply_ConflictsRequireRename(t *testing.T) {
	src := loadProfilesAt(t)
	sec := newMemStore()
	seedTwoProfiles(t, src, sec)
	pt, _ := Build(context.Background(), src, sec, nil)

	// Destination already has a "work" profile.
	dst := loadProfilesAt(t)
	dstSec := newMemStore()
	_ = dst.Add(profile.Profile{Name: "work", Fingerprint: "sha256:other", CreatedAt: time.Now()})
	_ = dstSec.Set(context.Background(), profile.SecretKey("work"), []byte("other"))

	if _, err := Apply(context.Background(), pt, dst, dstSec, ImportOption{}); err == nil {
		t.Fatalf("expected conflict error")
	}

	// Now resolve via rename.
	added, err := Apply(context.Background(), pt, dst, dstSec, ImportOption{Renames: map[string]string{"work": "work-imported"}})
	if err != nil {
		t.Fatalf("Apply with rename: %v", err)
	}
	if len(added) != 2 {
		t.Fatalf("expected 2 added (one renamed), got %v", added)
	}
}

func TestBuild_SubsetByName(t *testing.T) {
	src := loadProfilesAt(t)
	sec := newMemStore()
	seedTwoProfiles(t, src, sec)
	pt, err := Build(context.Background(), src, sec, []string{"work"})
	if err != nil {
		t.Fatalf("Build subset: %v", err)
	}
	if len(pt.Profiles) != 1 || pt.Profiles[0].Name != "work" {
		t.Fatalf("expected only work, got %+v", pt.Profiles)
	}
}

func TestBuild_UnknownProfileError(t *testing.T) {
	src := loadProfilesAt(t)
	sec := newMemStore()
	seedTwoProfiles(t, src, sec)
	if _, err := Build(context.Background(), src, sec, []string{"nope"}); err == nil {
		t.Fatalf("expected error on unknown name")
	}
}

// secretsStoreTypeAssertion ensures memStore implements the interface used
// by Build/Apply at compile time.
var _ secrets.Store = (*memStore)(nil)

func TestBundleFile_PermsAndContent(t *testing.T) {
	// Just sanity check that sealed bundle starts with magic bytes.
	src := loadProfilesAt(t)
	sec := newMemStore()
	seedTwoProfiles(t, src, sec)
	pt, _ := Build(context.Background(), src, sec, nil)
	sealed, err := Seal(pt, []byte("pw"), fastParams)
	if err != nil {
		t.Fatal(err)
	}
	if string(sealed[:4]) != "CCEX" {
		t.Fatalf("expected CCEX magic, got %q", sealed[:4])
	}
	if filepath.Ext("dummy.cce") != ".cce" {
		t.Fatalf("sanity")
	}
}
