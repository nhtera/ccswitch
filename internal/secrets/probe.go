package secrets

import (
	"context"
	"errors"

	keyring "github.com/zalando/go-keyring"
)

// Open returns the best available Store for this environment. It tries the
// system keyring first; on failure (headless Linux, missing libsecret, etc.)
// it falls back to an encrypted file vault. pass is only consulted if the
// file backend is selected.
func Open(ctx context.Context, pass PassphraseFunc) (Store, error) {
	if probeKeyring() {
		return NewKeyringStore()
	}
	return NewFileStore(pass)
}

// probeKeyring round-trips a tiny value to confirm the system keyring is
// reachable. We use a fixed probe key under the same Service so we don't
// pollute the namespace and so we can recognize / clean up leftovers.
func probeKeyring() bool {
	const probeKey = "probe.startup"
	const probeVal = "ok"
	if err := keyring.Set(Service, probeKey, probeVal); err != nil {
		return false
	}
	got, err := keyring.Get(Service, probeKey)
	if err != nil || got != probeVal {
		_ = keyring.Delete(Service, probeKey)
		return false
	}
	if err := keyring.Delete(Service, probeKey); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		// Probe succeeded enough to read; failure to delete is non-fatal.
	}
	return true
}
