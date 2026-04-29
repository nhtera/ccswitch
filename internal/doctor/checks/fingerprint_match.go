package checks

import (
	"context"
	"errors"
	"fmt"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/doctor"
	"github.com/nhtera/ccswitch/internal/profile"
)

// FingerprintMatchCheck verifies the live credential fingerprint matches at
// most one known profile. Multiple matches mean a fingerprint collision in
// profiles.json, which `add` should have prevented.
func FingerprintMatchCheck(b *claude.Bridge) doctor.Check {
	return doctor.CheckFunc{
		N: "FingerprintMatch",
		Fn: func(ctx context.Context) doctor.Result {
			blob, err := b.ReadLive(ctx)
			if errors.Is(err, claude.ErrLiveNotPresent) {
				return doctor.Result{Status: doctor.StatusSkip, Message: "no live credential"}
			}
			if err != nil {
				return doctor.Result{Status: doctor.StatusFail, Message: err.Error()}
			}
			fp := claude.Fingerprint(blob)

			store, err := profile.LoadStore()
			if err != nil {
				return doctor.Result{Status: doctor.StatusFail, Message: "profiles.json: " + err.Error()}
			}
			matches := []string{}
			for _, p := range store.All() {
				if p.Fingerprint == fp {
					matches = append(matches, p.Name)
				}
			}
			switch len(matches) {
			case 0:
				return doctor.Result{
					Status:  doctor.StatusWarn,
					Message: "live credential is untracked",
					FixHint: "run `ccswitch add <name>` to capture it",
				}
			case 1:
				return doctor.Result{Status: doctor.StatusPass, Message: "active = " + matches[0]}
			default:
				return doctor.Result{
					Status:  doctor.StatusFail,
					Message: fmt.Sprintf("fingerprint matches %d profiles: %v", len(matches), matches),
					FixHint: "remove duplicate(s) with `ccswitch remove <name>`",
				}
			}
		},
	}
}
