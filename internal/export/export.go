package export

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nhtera/ccswitch/internal/cryptobox"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
)

// Build assembles a Plaintext bundle from the given profile names. Pass an
// empty slice to export every profile in store. Each profile's credential
// blob is fetched from secstore.
func Build(ctx context.Context, store *profile.Store, secstore secrets.Store, names []string) (Plaintext, error) {
	all := store.All()
	want := map[string]bool{}
	if len(names) == 0 {
		for _, p := range all {
			want[p.Name] = true
		}
	} else {
		for _, n := range names {
			want[n] = true
		}
	}

	out := Plaintext{Version: PlaintextVersion, ExportedAt: time.Now().UTC()}
	for _, p := range all {
		if !want[p.Name] {
			continue
		}
		blob, err := secstore.Get(ctx, profile.SecretKey(p.Name))
		if err != nil {
			return Plaintext{}, fmt.Errorf("read credential for %q: %w", p.Name, err)
		}
		out.Profiles = append(out.Profiles, FromProfile(p, blob))
		delete(want, p.Name)
	}
	if len(want) > 0 {
		var missing []string
		for n := range want {
			missing = append(missing, n)
		}
		return Plaintext{}, fmt.Errorf("unknown profile(s): %v", missing)
	}
	if len(out.Profiles) == 0 {
		return Plaintext{}, errors.New("no profiles to export")
	}
	return out, nil
}

// Seal serializes and encrypts a Plaintext for shipping. params controls
// Argon2id work factor; pass DefaultParams for the standard setting.
func Seal(pt Plaintext, passphrase []byte, params cryptobox.Params) ([]byte, error) {
	if params == (cryptobox.Params{}) {
		params = cryptobox.DefaultParams
	}
	return SealPlaintext(pt, passphrase, params)
}
