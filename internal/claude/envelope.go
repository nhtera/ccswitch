// Package claude reads and writes the live Claude Code credential envelope.
//
// The envelope is treated as opaque bytes for storage. We parse it only
// enough to (a) compute a stable fingerprint and (b) detect the account
// type. New fields Anthropic adds flow through verbatim — we never
// reconstruct the envelope.
package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// Type categorizes a credential's account type.
type Type string

const (
	TypeOAuth Type = "oauth"
	TypeAPI   Type = "api"
	TypeSSO   Type = "sso"

	// TypeUnknown is reported when the envelope shape is recognized but the
	// caller couldn't classify it (e.g. an OAuth-shape blob with no
	// subscriptionType).
	TypeUnknown Type = "unknown"
)

// ErrMalformed indicates the bytes don't match any known credential shape.
var ErrMalformed = errors.New("claude: envelope is not OAuth JSON or a bare api key")

// outerEnvelope is the JSON shape Claude Code stores. We capture
// claudeAiOauth as RawMessage so we can canonicalize for fingerprinting
// while preserving unknown fields.
type outerEnvelope struct {
	ClaudeAiOauth json.RawMessage `json:"claudeAiOauth"`
}

// oauthInner pulls only the fields we need for type detection.
type oauthInner struct {
	SubscriptionType string `json:"subscriptionType"`
}

// DetectType returns the account type for a credential blob without
// modifying it. A blob that looks like a bare `sk-ant-api...` string is
// classified as TypeAPI; an OAuth envelope's subscriptionType decides
// between oauth/sso (enterprise hint) and the default oauth.
func DetectType(blob []byte) Type {
	trimmed := strings.TrimSpace(string(blob))
	if isBareAPIKey(trimmed) {
		return TypeAPI
	}
	var outer outerEnvelope
	if err := json.Unmarshal(blob, &outer); err != nil || len(outer.ClaudeAiOauth) == 0 {
		return TypeUnknown
	}
	var inner oauthInner
	if err := json.Unmarshal(outer.ClaudeAiOauth, &inner); err != nil {
		return TypeOAuth
	}
	sub := strings.ToLower(strings.TrimSpace(inner.SubscriptionType))
	switch {
	case strings.Contains(sub, "enterprise"), strings.Contains(sub, "sso"):
		return TypeSSO
	case sub == "pro", sub == "max":
		return TypeOAuth
	default:
		return TypeOAuth
	}
}

// Validate returns nil if the blob is structurally a recognized credential
// (OAuth envelope or bare API key). It does NOT verify the token works.
func Validate(blob []byte) error {
	if DetectType(blob) == TypeUnknown {
		return ErrMalformed
	}
	return nil
}

// Fingerprint returns "sha256:<hex>" for the blob. OAuth envelopes are
// canonicalized (sorted keys, no whitespace) so two structurally-identical
// envelopes always hash the same. Bare API keys are trimmed and hashed
// directly.
func Fingerprint(blob []byte) string {
	trimmed := strings.TrimSpace(string(blob))
	if isBareAPIKey(trimmed) {
		sum := sha256.Sum256([]byte(trimmed))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	canon, err := canonicalJSON(blob)
	if err != nil {
		// Fall back to raw bytes if we can't parse — better a stable hash
		// than a panic. doctor will flag a malformed envelope separately.
		sum := sha256.Sum256(blob)
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(canon)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// canonicalJSON returns a deterministic encoding of the input: object keys
// sorted lexicographically, no whitespace, arrays preserved in order.
func canonicalJSON(in []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, err
	}
	return marshalCanonical(v)
}

func marshalCanonical(v any) ([]byte, error) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var buf strings.Builder
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			ek, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf.Write(ek)
			buf.WriteByte(':')
			ev, err := marshalCanonical(x[k])
			if err != nil {
				return nil, err
			}
			buf.Write(ev)
		}
		buf.WriteByte('}')
		return []byte(buf.String()), nil
	case []any:
		var buf strings.Builder
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			ev, err := marshalCanonical(e)
			if err != nil {
				return nil, err
			}
			buf.Write(ev)
		}
		buf.WriteByte(']')
		return []byte(buf.String()), nil
	default:
		return json.Marshal(x)
	}
}

// maxBareAPIKeyLen caps the length of a string we'll classify as a bare
// API key. Anthropic's keys are well under this bound; anything larger
// is far more likely to be a concatenation artifact or pasted JSON than
// a real key, so we punt to the JSON-shape branch.
const maxBareAPIKeyLen = 512

func isBareAPIKey(s string) bool {
	if len(s) == 0 || len(s) > maxBareAPIKeyLen {
		return false
	}
	return strings.HasPrefix(s, "sk-ant-api") && !strings.HasPrefix(s, "{")
}
