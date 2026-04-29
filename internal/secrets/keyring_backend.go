package secrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	keyring "github.com/zalando/go-keyring"
)

// keyringStore wraps zalando/go-keyring with a sidecar JSON index so we can
// support List portably. The index lives at $CONFIG/keyring-index.json and
// records every key we've Set; it's a hint, not authoritative — Get/Delete
// always go to the OS keyring.
type keyringStore struct {
	mu      sync.Mutex
	keyring keyringClient // injectable for tests
	index   *keyIndex
}

// keyringClient is the small subset of zalando/go-keyring we depend on.
// The package-level vars below default to the real implementation; tests can
// swap them.
type keyringClient interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type defaultKeyring struct{}

func (defaultKeyring) Set(s, u, p string) error { return keyring.Set(s, u, p) }
func (defaultKeyring) Get(s, u string) (string, error) {
	v, err := keyring.Get(s, u)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return v, nil
}
func (defaultKeyring) Delete(s, u string) error {
	if err := keyring.Delete(s, u); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// NewKeyringStore returns a Store that uses the OS keyring.
func NewKeyringStore() (Store, error) {
	idx, err := loadKeyIndex()
	if err != nil {
		return nil, err
	}
	return &keyringStore{keyring: defaultKeyring{}, index: idx}, nil
}

func (k *keyringStore) Backend() Backend { return BackendKeyring }

func (k *keyringStore) Set(_ context.Context, key string, value []byte) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	enc := base64.StdEncoding.EncodeToString(value)
	if err := k.keyring.Set(Service, key, enc); err != nil {
		return fmt.Errorf("keyring set %q: %w", key, err)
	}
	k.index.add(key)
	return k.index.save()
}

func (k *keyringStore) Get(_ context.Context, key string) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	enc, err := k.keyring.Get(Service, key)
	if err != nil {
		return nil, err
	}
	dec, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, fmt.Errorf("keyring get %q: corrupt base64: %w", key, err)
	}
	// Backfill the index in case a write happened on an older version.
	k.index.add(key)
	_ = k.index.save()
	return dec, nil
}

func (k *keyringStore) Delete(_ context.Context, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	err := k.keyring.Delete(Service, key)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("keyring delete %q: %w", key, err)
	}
	k.index.remove(key)
	return k.index.save()
}

func (k *keyringStore) List(_ context.Context, prefix string) ([]string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]string, 0)
	for _, key := range k.index.keys() {
		if strings.HasPrefix(key, prefix) {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out, nil
}

// keyIndex is a JSON-backed sorted set of keys we've written.
type keyIndex struct {
	mu   sync.Mutex
	path string
	set  map[string]struct{}
}

func loadKeyIndex() (*keyIndex, error) {
	p, err := indexPath()
	if err != nil {
		return nil, err
	}
	idx := &keyIndex{path: p, set: map[string]struct{}{}}
	data, ok, err := readFileIfExists(p)
	if err != nil {
		return nil, err
	}
	if ok && len(data) > 0 {
		var keys []string
		if err := json.Unmarshal(data, &keys); err != nil {
			return nil, fmt.Errorf("parse keyring index: %w", err)
		}
		for _, k := range keys {
			idx.set[k] = struct{}{}
		}
	}
	return idx, nil
}

func (i *keyIndex) add(key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.set[key] = struct{}{}
}

func (i *keyIndex) remove(key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.set, key)
}

func (i *keyIndex) keys() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]string, 0, len(i.set))
	for k := range i.set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (i *keyIndex) save() error {
	keys := i.keys()
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(i.path, data, 0o600)
}
