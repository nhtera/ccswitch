package profile

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	s, err := loadStoreAt(filepath.Join(dir, "profiles.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return s
}

func sample(name, fp string) Profile {
	return Profile{
		Name:        name,
		Type:        "oauth",
		CreatedAt:   time.Date(2026, 4, 29, 3, 13, 5, 0, time.UTC),
		Fingerprint: fp,
	}
}

func TestStore_AddAndFind(t *testing.T) {
	s := newTempStore(t)
	if err := s.Add(sample("work", "sha256:aaa")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, ok := s.Find("work")
	if !ok || got.Name != "work" {
		t.Fatalf("Find missed: %v %v", got, ok)
	}
}

func TestStore_AddDuplicateNameRejected(t *testing.T) {
	s := newTempStore(t)
	_ = s.Add(sample("work", "sha256:aaa"))
	if err := s.Add(sample("work", "sha256:bbb")); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestStore_AddDuplicateFingerprintRejected(t *testing.T) {
	s := newTempStore(t)
	_ = s.Add(sample("work", "sha256:dup"))
	if err := s.Add(sample("personal", "sha256:dup")); !errors.Is(err, ErrFingerprintDup) {
		t.Fatalf("expected ErrFingerprintDup, got %v", err)
	}
}

func TestStore_FindByFingerprint(t *testing.T) {
	s := newTempStore(t)
	_ = s.Add(sample("work", "sha256:work"))
	_ = s.Add(sample("home", "sha256:home"))
	got, ok := s.FindByFingerprint("sha256:home")
	if !ok || got.Name != "home" {
		t.Fatalf("FindByFingerprint: %+v %v", got, ok)
	}
}

func TestStore_RoundTripPersistence(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CCSWITCH_CONFIG_DIR", dir)
	p := filepath.Join(dir, "profiles.json")
	s, _ := loadStoreAt(p)
	_ = s.Add(sample("work", "sha256:aaa"))
	s2, err := loadStoreAt(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got, ok := s2.Find("work"); !ok || got.Name != "work" {
		t.Fatalf("after reload, Find work failed: %v %v", got, ok)
	}
}

func TestStore_Rename(t *testing.T) {
	s := newTempStore(t)
	_ = s.Add(sample("a", "sha256:a"))
	if err := s.Rename("a", "b"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, ok := s.Find("a"); ok {
		t.Fatalf("old name still present")
	}
	if _, ok := s.Find("b"); !ok {
		t.Fatalf("new name missing")
	}
}

func TestStore_RenameTargetExists(t *testing.T) {
	s := newTempStore(t)
	_ = s.Add(sample("a", "sha256:a"))
	_ = s.Add(sample("b", "sha256:b"))
	if err := s.Rename("a", "b"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
}

func TestStore_Remove(t *testing.T) {
	s := newTempStore(t)
	_ = s.Add(sample("a", "sha256:a"))
	if err := s.Remove("a"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, ok := s.Find("a"); ok {
		t.Fatalf("still found after Remove")
	}
}

func TestStore_RemoveNotFound(t *testing.T) {
	s := newTempStore(t)
	if err := s.Remove("ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStore_UpdateEnv(t *testing.T) {
	s := newTempStore(t)
	_ = s.Add(sample("a", "sha256:a"))
	got, err := s.UpdateEnv("a", map[string]string{"FOO": "bar", "BAZ": "qux"}, nil)
	if err != nil {
		t.Fatalf("UpdateEnv: %v", err)
	}
	if got.Env["FOO"] != "bar" || got.Env["BAZ"] != "qux" {
		t.Fatalf("env not set: %+v", got.Env)
	}
	got, err = s.UpdateEnv("a", nil, []string{"FOO"})
	if err != nil {
		t.Fatalf("UpdateEnv unset: %v", err)
	}
	if _, present := got.Env["FOO"]; present {
		t.Fatalf("FOO should be removed")
	}
}

func TestStore_TouchSetsLastUsed(t *testing.T) {
	s := newTempStore(t)
	_ = s.Add(sample("a", "sha256:a"))
	now := time.Date(2026, 4, 29, 10, 42, 11, 0, time.UTC)
	if err := s.Touch("a", now); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	got, _ := s.Find("a")
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(now) {
		t.Fatalf("LastUsedAt = %v, want %v", got.LastUsedAt, now)
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"a", "ab_cd", "with-dash", "1234"} {
		if err := ValidateName(ok); err != nil {
			t.Errorf("expected %q valid, got %v", ok, err)
		}
	}
	for _, bad := range []string{"", "with space", "../x", "way-too-long-name-that-exceeds-32-characters"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("expected %q invalid", bad)
		}
	}
}

func TestValidateEnvKey(t *testing.T) {
	for _, ok := range []string{"FOO", "ANTHROPIC_BASE_URL", "_X", "A1"} {
		if err := ValidateEnvKey(ok); err != nil {
			t.Errorf("expected %q valid, got %v", ok, err)
		}
	}
	for _, bad := range []string{"", "lower", "1FIRST", "WITH-DASH", "with space"} {
		if err := ValidateEnvKey(bad); err == nil {
			t.Errorf("expected %q invalid", bad)
		}
	}
}

func TestSuggestName_OrgPreferredOverEmail(t *testing.T) {
	got := SuggestName("tn@erai.dev", "ERAI")
	if got != "erai" {
		t.Fatalf("got %q, want erai", got)
	}
}

func TestSuggestName_FallsBackToEmailLocalPart(t *testing.T) {
	got := SuggestName("alice@example.com", "")
	if got != "alice" {
		t.Fatalf("got %q, want alice", got)
	}
}

func TestSuggestName_SanitizesSpecials(t *testing.T) {
	got := SuggestName("user.name+tag@a.com", "")
	if got != "user-name-tag" {
		t.Fatalf("got %q, want user-name-tag", got)
	}
}

func TestSuggestName_OrgWithApostrophesAndAmpersand(t *testing.T) {
	got := SuggestName("x@x", "Acme & Co.'s Org")
	// Goal: produce a valid kebab-case name; ampersand/apostrophe/period
	// all collapse into single dashes. Exact result is implementation
	// detail but ValidateName must accept it.
	if err := ValidateName(got); err != nil {
		t.Fatalf("SuggestName produced invalid name %q: %v", got, err)
	}
}

func TestSuggestName_TruncatesAt32(t *testing.T) {
	got := SuggestName("x@x", "this-is-a-very-long-organization-name-exceeding-thirty-two-characters")
	if len(got) > 32 {
		t.Fatalf("not truncated: %q (%d chars)", got, len(got))
	}
	if err := ValidateName(got); err != nil {
		t.Fatalf("truncated name failed validation: %v", err)
	}
}

func TestSuggestName_EmptyInputs(t *testing.T) {
	if got := SuggestName("", ""); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
