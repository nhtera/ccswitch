package export

import (
	"context"
	"errors"
	"fmt"

	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
)

// ImportOption controls how Apply handles a bundle entry.
type ImportOption struct {
	// Renames maps source-name → target-name when the source name conflicts
	// with an existing profile.
	Renames map[string]string
}

// Plan describes what Apply would do without actually doing it. Useful for
// the --yes confirm prompt.
type Plan struct {
	ToImport []string // final names that will be added
	Conflict []string // source names that have no rename and conflict
	Renamed  map[string]string
}

// Inspect reports conflicts vs the current store.
func Inspect(pt Plaintext, store *profile.Store, opt ImportOption) Plan {
	plan := Plan{Renamed: map[string]string{}}
	for _, ep := range pt.Profiles {
		final := ep.Name
		if r, ok := opt.Renames[ep.Name]; ok {
			final = r
			plan.Renamed[ep.Name] = final
		}
		if _, exists := store.Find(final); exists {
			plan.Conflict = append(plan.Conflict, ep.Name)
			continue
		}
		plan.ToImport = append(plan.ToImport, final)
	}
	return plan
}

// Apply writes every non-conflicting profile in pt into store + secstore.
// If any conflict remains and opt.Renames doesn't resolve it, returns an
// error before mutating anything.
func Apply(ctx context.Context, pt Plaintext, store *profile.Store, secstore secrets.Store, opt ImportOption) ([]string, error) {
	plan := Inspect(pt, store, opt)
	if len(plan.Conflict) > 0 {
		return nil, fmt.Errorf("conflict on names %v — pass --rename old=new to resolve", plan.Conflict)
	}

	added := make([]string, 0, len(pt.Profiles))
	for _, ep := range pt.Profiles {
		final := ep.Name
		if r, ok := opt.Renames[ep.Name]; ok {
			final = r
		}
		if err := profile.ValidateName(final); err != nil {
			return added, fmt.Errorf("invalid name after rename for %q: %w", ep.Name, err)
		}

		p, blob, err := ep.ToProfile()
		if err != nil {
			return added, err
		}
		p.Name = final

		if err := secstore.Set(ctx, profile.SecretKey(final), blob); err != nil {
			return added, fmt.Errorf("write credential for %q: %w", final, err)
		}
		if err := store.Add(p); err != nil {
			// Roll back the keyring write to avoid orphans.
			_ = secstore.Delete(ctx, profile.SecretKey(final))
			if errors.Is(err, profile.ErrFingerprintDup) {
				return added, fmt.Errorf("fingerprint of %q already used by another local profile", final)
			}
			return added, fmt.Errorf("update profiles.json for %q: %w", final, err)
		}
		added = append(added, final)
	}
	return added, nil
}
