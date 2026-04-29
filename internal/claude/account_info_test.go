package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestWriteOAuthAccount_PreservesOtherFields proves we only swap the
// `oauthAccount` field and don't disturb the rest of ~/.claude.json
// (projects, settings, theme, unknown future fields).
func TestWriteOAuthAccount_PreservesOtherFields(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, ".claude.json")

	original := map[string]interface{}{
		"oauthAccount": map[string]string{"emailAddress": "old@example.com", "accountUuid": "OLD"},
		"projects":     map[string]interface{}{"/Users/x/foo": map[string]bool{"trustDialogAccepted": true}},
		"theme":        "dark",
		"customField":  "must-be-preserved",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	newOAuth := json.RawMessage(`{"emailAddress":"new@example.com","accountUuid":"NEW"}`)
	if err := WriteOAuthAccount(newOAuth); err != nil {
		t.Fatalf("WriteOAuthAccount: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	oauth, ok := parsed["oauthAccount"].(map[string]interface{})
	if !ok {
		t.Fatalf("oauthAccount missing or wrong type: %v", parsed["oauthAccount"])
	}
	if oauth["emailAddress"] != "new@example.com" {
		t.Errorf("emailAddress = %v, want new@example.com", oauth["emailAddress"])
	}
	if parsed["theme"] != "dark" {
		t.Errorf("theme not preserved: got %v", parsed["theme"])
	}
	if parsed["customField"] != "must-be-preserved" {
		t.Errorf("unknown field not preserved: got %v", parsed["customField"])
	}
	if parsed["projects"] == nil {
		t.Error("projects field dropped")
	}
}

func TestWriteOAuthAccount_BootstrapsMissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	newOAuth := json.RawMessage(`{"emailAddress":"new@example.com"}`)
	if err := WriteOAuthAccount(newOAuth); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatal(err)
	}
	oauth := parsed["oauthAccount"].(map[string]interface{})
	if oauth["emailAddress"] != "new@example.com" {
		t.Errorf("got %v", oauth)
	}
}

func TestReadOAuthAccountRaw_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, ".claude.json")
	doc := []byte(`{"oauthAccount":{"emailAddress":"a@b.c","customField":"x"}}`)
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := ReadOAuthAccountRaw()
	if err != nil || raw == nil {
		t.Fatalf("got %v %v", raw, err)
	}
	// The raw bytes should be parseable and contain our custom field
	// (proving we capture the WHOLE block, not just the modeled fields).
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["customField"] != "x" {
		t.Errorf("custom field lost: %v", got)
	}
}
