package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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

// UpdateMetadata replaces the metadata fields of the profile named
// `name` with values from `incoming`. Preserves the existing
// CreatedAt and Env (when incoming.Env is empty) so a `--replace`
// re-capture doesn't reset things the user customized. Persists to
// disk.
func (s *Store) UpdateMetadata(name string, incoming Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Profiles {
		if s.data.Profiles[i].Name != name {
			continue
		}
		updated := s.data.Profiles[i]
		updated.Type = incoming.Type
		updated.Fingerprint = incoming.Fingerprint
		updated.StableFingerprint = incoming.StableFingerprint
		updated.Email = incoming.Email
		updated.OrgName = incoming.OrgName
		updated.OAuthAccount = incoming.OAuthAccount
		if incoming.Note != "" {
			updated.Note = incoming.Note
		}
		if len(incoming.Env) > 0 {
			updated.Env = incoming.Env
		}
		s.data.Profiles[i] = updated
		return s.save()
	}
	return ErrNotFound
}

// FindByEmail returns the profile whose Email exactly equals email
// (case-insensitive). Empty email never matches.
func (s *Store) FindByEmail(email string) (Profile, bool) {
	if email == "" {
		return Profile{}, false
	}
	target := strings.ToLower(strings.TrimSpace(email))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Profiles {
		if strings.ToLower(p.Email) == target {
			return p, true
		}
	}
	return Profile{}, false
}

// SortedByName returns a copy of all profiles sorted alphabetically
// by name. Used by Lookup-by-index and rotation commands so the
// ordering is stable across invocations.
func (s *Store) SortedByName() []Profile {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Profile, len(s.data.Profiles))
	copy(out, s.data.Profiles)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lookup resolves a flexible identifier to a profile:
//
//   - "@" in the string → match by Profile.Email
//   - all-digit string → 1-based index into SortedByName()
//   - otherwise → exact name match
//
// Returns ErrNotFound with a hint on miss.
func (s *Store) Lookup(ident string) (Profile, error) {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return Profile{}, ErrNotFound
	}
	if strings.Contains(ident, "@") {
		if p, ok := s.FindByEmail(ident); ok {
			return p, nil
		}
		return Profile{}, fmt.Errorf("no profile with email %q: %w", ident, ErrNotFound)
	}
	if isAllDigits(ident) {
		n, err := strconv.Atoi(ident)
		if err != nil || n < 1 {
			return Profile{}, fmt.Errorf("invalid index %q: %w", ident, ErrNotFound)
		}
		all := s.SortedByName()
		if n > len(all) {
			return Profile{}, fmt.Errorf("index %d out of range (have %d profiles): %w", n, len(all), ErrNotFound)
		}
		return all[n-1], nil
	}
	if p, ok := s.Find(ident); ok {
		return p, nil
	}
	return Profile{}, fmt.Errorf("no profile named %q: %w", ident, ErrNotFound)
}

// MostRecentlyUsedName returns the name of the profile with the
// latest LastUsedAt, or "" if no profile has ever been used. This is
// what `next` should rotate from — it's the profile the user most
// recently asked us to switch to, regardless of whether the live
// keychain or ~/.claude.json still agree.
func (s *Store) MostRecentlyUsedName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := ""
	var latest time.Time
	for _, p := range s.data.Profiles {
		if p.LastUsedAt == nil {
			continue
		}
		if name == "" || p.LastUsedAt.After(latest) {
			name = p.Name
			latest = *p.LastUsedAt
		}
	}
	return name
}

// NextProfile returns the profile that follows currentName in
// SortedByName() order, wrapping at the end. If currentName is empty
// or not found, the first profile is returned. Errors only when
// there are zero profiles.
func (s *Store) NextProfile(currentName string) (Profile, error) {
	all := s.SortedByName()
	if len(all) == 0 {
		return Profile{}, ErrNotFound
	}
	if len(all) == 1 {
		return all[0], nil
	}
	for i, p := range all {
		if p.Name == currentName {
			return all[(i+1)%len(all)], nil
		}
	}
	return all[0], nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
