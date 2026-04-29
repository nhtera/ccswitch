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

	"github.com/nhtera/ccswitch/internal/cryptobox"
)

// fileMagic identifies the encrypted vault payload to cryptobox.
var fileMagic = cryptobox.Magic{'C', 'C', 'F', 'B'}

// PassphraseFunc returns a passphrase for the file vault. It receives a
// "first-time" hint so callers can prompt for confirm-on-first-run.
type PassphraseFunc func(firstRun bool) ([]byte, error)

// fileStore implements Store with a single AES-GCM encrypted JSON file. The
// vault is loaded eagerly on first access; mutations rewrite the entire file.
type fileStore struct {
	mu     sync.Mutex
	path   string
	pass   PassphraseFunc
	loaded bool
	cache  map[string][]byte // base64-decoded
	saved  []byte            // last passphrase, kept in process memory only
	params cryptobox.Params
}

// NewFileStore returns a Store that persists to a single encrypted file.
// pass is called the first time a passphrase is needed.
func NewFileStore(pass PassphraseFunc) (Store, error) {
	p, err := fileVaultPath()
	if err != nil {
		return nil, err
	}
	return &fileStore{path: p, pass: pass, cache: map[string][]byte{}, params: cryptobox.DefaultParams}, nil
}

func (f *fileStore) Backend() Backend { return BackendFile }

func (f *fileStore) load() error {
	if f.loaded {
		return nil
	}
	data, ok, err := readFileIfExists(f.path)
	if err != nil {
		return err
	}
	if !ok {
		// First run: prompt for new passphrase.
		pw, err := f.pass(true)
		if err != nil {
			return err
		}
		f.saved = pw
		f.loaded = true
		return nil
	}
	pw, err := f.pass(false)
	if err != nil {
		return err
	}
	plain, err := cryptobox.Open(&fileMagic, pw, data)
	if err != nil {
		return err
	}
	var enc map[string]string
	if err := json.Unmarshal(plain, &enc); err != nil {
		return fmt.Errorf("parse vault: %w", err)
	}
	for k, v := range enc {
		raw, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return fmt.Errorf("vault entry %q: %w", k, err)
		}
		f.cache[k] = raw
	}
	f.saved = pw
	f.loaded = true
	return nil
}

func (f *fileStore) save() error {
	enc := make(map[string]string, len(f.cache))
	for k, v := range f.cache {
		enc[k] = base64.StdEncoding.EncodeToString(v)
	}
	plain, err := json.Marshal(enc)
	if err != nil {
		return err
	}
	if f.saved == nil {
		return errors.New("file vault: no passphrase available (programmer bug)")
	}
	out, err := cryptobox.Seal(fileMagic, f.saved, plain, f.params)
	if err != nil {
		return err
	}
	return writeAtomic(f.path, out, 0o600)
}

func (f *fileStore) Set(_ context.Context, key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return err
	}
	f.cache[key] = append([]byte(nil), value...)
	return f.save()
}

func (f *fileStore) Get(_ context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return nil, err
	}
	v, ok := f.cache[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), v...), nil
}

func (f *fileStore) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return err
	}
	if _, ok := f.cache[key]; !ok {
		return ErrNotFound
	}
	delete(f.cache, key)
	return f.save()
}

func (f *fileStore) List(_ context.Context, prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.load(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(f.cache))
	for k := range f.cache {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}
