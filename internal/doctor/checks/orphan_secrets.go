package checks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nhtera/ccswitch/internal/doctor"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
)

// OrphanSecretsCheck reports keyring entries under the profile.* namespace
// that have no corresponding profiles.json entry. These leak space but are
// not actively harmful — WARN, not FAIL.
func OrphanSecretsCheck(open func(ctx context.Context) (secrets.Store, error)) doctor.Check {
	return doctor.CheckFunc{
		N: "OrphanSecrets",
		Fn: func(ctx context.Context) doctor.Result {
			store, err := profile.LoadStore()
			if err != nil {
				return doctor.Result{Status: doctor.StatusFail, Message: err.Error()}
			}
			known := map[string]struct{}{}
			for _, p := range store.All() {
				known[profile.SecretKey(p.Name)] = struct{}{}
			}

			sec, err := open(ctx)
			if err != nil {
				return doctor.Result{Status: doctor.StatusFail, Message: err.Error()}
			}
			keys, err := sec.List(ctx, "profile.")
			if err != nil {
				return doctor.Result{Status: doctor.StatusFail, Message: err.Error()}
			}
			orphans := []string{}
			for _, k := range keys {
				if _, ok := known[k]; !ok {
					orphans = append(orphans, k)
				}
			}
			if len(orphans) == 0 {
				return doctor.Result{Status: doctor.StatusPass, Message: "none"}
			}
			sort.Strings(orphans)
			return doctor.Result{
				Status:  doctor.StatusWarn,
				Message: fmt.Sprintf("%d orphan keyring entry/entries: %s", len(orphans), strings.Join(orphans, ", ")),
				FixHint: "remove via your OS keychain UI; we don't auto-delete to avoid data loss",
			}
		},
	}
}
