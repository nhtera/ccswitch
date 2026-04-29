// Package export builds and parses encrypted, portable bundles of profiles.
//
// Bundles use the cryptobox wire format with magic "CCEX" so they can't be
// confused with the secrets file vault.
package export

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nhtera/ccswitch/internal/cryptobox"
	"github.com/nhtera/ccswitch/internal/profile"
)

// BundleMagic identifies an export bundle to cryptobox.
var BundleMagic = cryptobox.Magic{'C', 'C', 'E', 'X'}

// PlaintextVersion is the schema version of the JSON inside the bundle. v1
// matches the layout below; importers reject unknown versions.
const PlaintextVersion = 1

// Plaintext is the JSON-serialized contents of a decrypted bundle.
type Plaintext struct {
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Profiles   []EncodedProfile `json:"profiles"`
}

// EncodedProfile carries one profile's metadata plus its credential blob
// base64-encoded so JSON survives the round trip.
type EncodedProfile struct {
	Name              string            `json:"name"`
	Type              string            `json:"type"`
	CreatedAt         time.Time         `json:"created_at"`
	Note              string            `json:"note,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	Fingerprint       string            `json:"fingerprint"`
	CredentialBlobB64 string            `json:"credential_blob_b64"`
}

// FromProfile builds an EncodedProfile by base64-encoding the supplied
// credential blob. The Profile fields are copied verbatim.
func FromProfile(p profile.Profile, blob []byte) EncodedProfile {
	return EncodedProfile{
		Name:              p.Name,
		Type:              p.Type,
		CreatedAt:         p.CreatedAt,
		Note:              p.Note,
		Env:               p.Env,
		Fingerprint:       p.Fingerprint,
		CredentialBlobB64: base64.StdEncoding.EncodeToString(blob),
	}
}

// ToProfile converts back to a profile.Profile; the raw credential bytes
// are returned separately for storage in the secret backend.
func (e EncodedProfile) ToProfile() (profile.Profile, []byte, error) {
	blob, err := base64.StdEncoding.DecodeString(e.CredentialBlobB64)
	if err != nil {
		return profile.Profile{}, nil, fmt.Errorf("decode credential for %q: %w", e.Name, err)
	}
	return profile.Profile{
		Name:        e.Name,
		Type:        e.Type,
		CreatedAt:   e.CreatedAt,
		Note:        e.Note,
		Env:         e.Env,
		Fingerprint: e.Fingerprint,
	}, blob, nil
}

// SealPlaintext encrypts pt with passphrase and returns the bundle bytes.
func SealPlaintext(pt Plaintext, passphrase []byte, params cryptobox.Params) ([]byte, error) {
	raw, err := json.Marshal(pt)
	if err != nil {
		return nil, fmt.Errorf("marshal plaintext: %w", err)
	}
	return cryptobox.Seal(BundleMagic, passphrase, raw, params)
}

// OpenPlaintext decrypts a bundle and parses its JSON.
func OpenPlaintext(payload, passphrase []byte) (Plaintext, error) {
	raw, err := cryptobox.Open(&BundleMagic, passphrase, payload)
	if err != nil {
		return Plaintext{}, err
	}
	var pt Plaintext
	if err := json.Unmarshal(raw, &pt); err != nil {
		return Plaintext{}, fmt.Errorf("parse plaintext: %w", err)
	}
	if pt.Version != PlaintextVersion {
		return Plaintext{}, fmt.Errorf("unsupported bundle version v%d (this binary supports v%d)", pt.Version, PlaintextVersion)
	}
	return pt, nil
}
