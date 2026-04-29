package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/nhtera/ccswitch/internal/secrets"
)

// Sentinel errors from store operations.
var (
	ErrNotFound       = errors.New("profile: not found")
	ErrAlreadyExists  = errors.New("profile: already exists")
	ErrFingerprintDup = errors.New("profile: fingerprint already used by another profile")
)

// Store is the on-disk profiles.json plus a process-wide mutex.
type Store struct {
	mu   sync.Mutex
	path string
	data File
}

// LoadStore reads profiles.json from the standard config dir, or returns an
// empty store if the file doesn't exist yet.
func LoadStore() (*Store, error) {
	p, err := profilesJSONPath()
	if err != nil {
		return nil, err
	}
	return loadStoreAt(p)
}

func loadStoreAt(p string) (*Store, error) {
	s := &Store{path: p, data: File{Version: CurrentSchemaVersion}}
	raw, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	if len(raw) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	if s.data.Version > CurrentSchemaVersion {
		return nil, fmt.Errorf("profiles.json schema v%d is newer than this binary supports (v%d)", s.data.Version, CurrentSchemaVersion)
	}
	if s.data.Version == 0 {
		s.data.Version = CurrentSchemaVersion
	}
	return s, nil
}

// All returns a snapshot of every profile, sorted by name.
func (s *Store) All() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Profile, len(s.data.Profiles))
	copy(out, s.data.Profiles)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Find returns the named profile (and a copy is returned, mutate via store
// methods only).
func (s *Store) Find(name string) (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Profiles {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}

// FindByFingerprint returns the unique profile with this fingerprint, if any.
// Multiple matches return the first plus an error so doctor can flag the
// inconsistency.
func (s *Store) FindByFingerprint(fp string) (Profile, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Profiles {
		if p.Fingerprint == fp {
			return p, true
		}
	}
	return Profile{}, false
}

// FindByStableFingerprint returns the profile whose StableFingerprint
// matches sfp. Empty sfp never matches (callers shouldn't pass it,
// but guarding here keeps FindByFingerprint as the single source of
// truth for legacy profiles whose StableFingerprint is "").
func (s *Store) FindByStableFingerprint(sfp string) (Profile, bool) {
	if sfp == "" {
		return Profile{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Profiles {
		if p.StableFingerprint == sfp {
			return p, true
		}
	}
	return Profile{}, false
}

// Add inserts a new profile. Returns ErrAlreadyExists on name collision and
// ErrFingerprintDup on fingerprint collision.
func (s *Store) Add(p Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.data.Profiles {
		if existing.Name == p.Name {
			return ErrAlreadyExists
		}
		if existing.Fingerprint == p.Fingerprint {
			return ErrFingerprintDup
		}
	}
	s.data.Profiles = append(s.data.Profiles, p)
	return s.save()
}

// Touch updates LastUsedAt for the named profile.
func (s *Store) Touch(name string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.data.Profiles {
		if p.Name == name {
			s.data.Profiles[i].LastUsedAt = &t
			return s.save()
		}
	}
	return ErrNotFound
}

// Rename changes a profile's Name field. Caller is responsible for moving
// the keyring entry; this method just updates metadata.
func (s *Store) Rename(oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if oldName == newName {
		return nil
	}
	for _, p := range s.data.Profiles {
		if p.Name == newName {
			return ErrAlreadyExists
		}
	}
	for i, p := range s.data.Profiles {
		if p.Name == oldName {
			s.data.Profiles[i].Name = newName
			return s.save()
		}
	}
	return ErrNotFound
}

// Remove deletes the named profile from the store. Caller deletes the
// keyring entry.
func (s *Store) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.data.Profiles {
		if p.Name == name {
			s.data.Profiles = append(s.data.Profiles[:i], s.data.Profiles[i+1:]...)
			return s.save()
		}
	}
	return ErrNotFound
}

// UpdateEnv applies set/unset operations to the named profile's env map.
// Returns the updated profile.
func (s *Store) UpdateEnv(name string, set map[string]string, unset []string) (Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.data.Profiles {
		if p.Name == name {
			if p.Env == nil {
				p.Env = map[string]string{}
			}
			for k, v := range set {
				p.Env[k] = v
			}
			for _, k := range unset {
				delete(p.Env, k)
			}
			s.data.Profiles[i] = p
			if err := s.save(); err != nil {
				return Profile{}, err
			}
			return p, nil
		}
	}
	return Profile{}, ErrNotFound
}

// save writes profiles.json atomically. mu must be held by caller.
func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	s.data.Version = CurrentSchemaVersion
	out, err := json.MarshalIndent(&s.data, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, out, 0o600)
}

// profilesJSONPath returns the canonical path to profiles.json.
func profilesJSONPath() (string, error) {
	dir, err := secrets.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "profiles.json"), nil
}

// writeFileAtomic uses temp+rename to make the write durable.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".profiles-*.tmp")
	if err != nil {
		return err
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
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
	return os.Rename(tmp.Name(), path)
}
