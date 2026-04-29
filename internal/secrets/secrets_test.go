package secrets

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// fakeKeyring is an in-memory stand-in for the OS keyring.
type fakeKeyring struct {
	mu   sync.Mutex
	data map[string]string
	fail map[string]error
}

func newFakeKeyring() *fakeKeyring { return &fakeKeyring{data: map[string]string{}, fail: map[string]error{}} }

func (f *fakeKeyring) Set(_, user, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.fail["set:"+user]; ok {
		return err
	}
	f.data[user] = password
	return nil
}

func (f *fakeKeyring) Get(_, user string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.fail["get:"+user]; ok {
		return "", err
	}
	v, ok := f.data[user]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *fakeKeyring) Delete(_, user string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.fail["del:"+user]; ok {
		return err
	}
	if _, ok := f.data[user]; !ok {
		return ErrNotFound
	}
	delete(f.data, user)
	return nil
}

func newKeyringStoreForTest(t *testing.T) (*keyringStore, *fakeKeyring) {
	t.Helper()
	t.Setenv("CCSWITCH_CONFIG_DIR", t.TempDir())
	idx, err := loadKeyIndex()
	if err != nil {
		t.Fatalf("loadKeyIndex: %v", err)
	}
	fk := newFakeKeyring()
	return &keyringStore{keyring: fk, index: idx}, fk
}

func TestKeyringStore_RoundTrip(t *testing.T) {
	s, _ := newKeyringStoreForTest(t)
	ctx := context.Background()

	if err := s.Set(ctx, "profile.work", []byte("secret-blob")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get(ctx, "profile.work")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "secret-blob" {
		t.Fatalf("got %q", got)
	}
}

func TestKeyringStore_GetNotFound(t *testing.T) {
	s, _ := newKeyringStoreForTest(t)
	if _, err := s.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestKeyringStore_Delete(t *testing.T) {
	s, _ := newKeyringStoreForTest(t)
	ctx := context.Background()
	_ = s.Set(ctx, "k", []byte("v"))
	if err := s.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Delete, got %v", err)
	}
}

func TestKeyringStore_ListByPrefix(t *testing.T) {
	s, _ := newKeyringStoreForTest(t)
	ctx := context.Background()
	for _, k := range []string{"profile.a", "profile.b", "live", "live.bak"} {
		_ = s.Set(ctx, k, []byte("x"))
	}
	got, err := s.List(ctx, "profile.")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"profile.a", "profile.b"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestKeyringStore_IndexPersists(t *testing.T) {
	t.Setenv("CCSWITCH_CONFIG_DIR", t.TempDir())
	idx, _ := loadKeyIndex()
	fk := newFakeKeyring()
	s := &keyringStore{keyring: fk, index: idx}
	ctx := context.Background()
	_ = s.Set(ctx, "k1", []byte("v"))
	_ = s.Set(ctx, "k2", []byte("v"))

	// Reload from disk → should still see both keys.
	idx2, err := loadKeyIndex()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	keys := idx2.keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %v", keys)
	}
}

func TestFileStore_RoundTripAndPersistence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	pass := PassphraseFunc(func(firstRun bool) ([]byte, error) {
		return []byte("test-passphrase-1"), nil
	})
	store, err := NewFileStore(pass)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ctx := context.Background()
	if err := store.Set(ctx, "profile.x", []byte("hello")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Set(ctx, "profile.y", []byte("world")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Reopen with a fresh store; ensure the data persists.
	store2, err := NewFileStore(pass)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := store2.Get(ctx, "profile.y")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "world" {
		t.Fatalf("got %q", got)
	}

	keys, err := store2.List(ctx, "profile.")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %v", keys)
	}
}

func TestFileStore_WrongPassphraseRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	good := PassphraseFunc(func(firstRun bool) ([]byte, error) {
		return []byte("right"), nil
	})
	bad := PassphraseFunc(func(firstRun bool) ([]byte, error) {
		return []byte("wrong"), nil
	})
	store, err := NewFileStore(good)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if err := store.Set(context.Background(), "k", []byte("v")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	store2, _ := NewFileStore(bad)
	_, err = store2.Get(context.Background(), "k")
	if err == nil {
		t.Fatalf("expected error on wrong passphrase")
	}
}
