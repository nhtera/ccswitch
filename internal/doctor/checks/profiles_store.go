package checks

import (
	"context"
	"fmt"

	"github.com/nhtera/ccswitch/internal/doctor"
	"github.com/nhtera/ccswitch/internal/profile"
)

// ProfilesStoreCheck reports whether profiles.json parses (or is absent —
// also OK) and how many profiles it contains.
func ProfilesStoreCheck() doctor.Check {
	return doctor.CheckFunc{
		N: "ProfilesStore",
		Fn: func(ctx context.Context) doctor.Result {
			store, err := profile.LoadStore()
			if err != nil {
				return doctor.Result{
					Status:  doctor.StatusFail,
					Message: "profiles.json failed to parse",
					FixHint: err.Error(),
				}
			}
			n := len(store.All())
			return doctor.Result{
				Status:  doctor.StatusPass,
				Message: fmt.Sprintf("%d profile(s), schema v%d", n, profile.CurrentSchemaVersion),
			}
		},
	}
}
