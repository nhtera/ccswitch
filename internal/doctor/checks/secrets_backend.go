// Package checks holds doctor checks that depend on other internal packages.
// Keeping them here avoids creating an import cycle between internal/doctor
// (which only declares the framework) and the secrets/claude/profile pkgs.
package checks

import (
	"context"
	"fmt"

	"github.com/nhtera/ccswitch/internal/doctor"
	"github.com/nhtera/ccswitch/internal/secrets"
)

// SecretsBackendCheck reports which backend is active and whether it can
// round-trip a value. It uses a probe key under the same Service so it
// doesn't pollute the namespace.
func SecretsBackendCheck(open func(ctx context.Context) (secrets.Store, error)) doctor.Check {
	return doctor.CheckFunc{
		N: "SecretsBackend",
		Fn: func(ctx context.Context) doctor.Result {
			store, err := open(ctx)
			if err != nil {
				return doctor.Result{
					Status:  doctor.StatusFail,
					Message: "could not open secret store",
					FixHint: err.Error(),
				}
			}
			const k = "doctor.probe"
			if err := store.Set(ctx, k, []byte("ok")); err != nil {
				return doctor.Result{
					Status:  doctor.StatusFail,
					Message: fmt.Sprintf("backend=%s; set failed", store.Backend()),
					FixHint: err.Error(),
				}
			}
			got, err := store.Get(ctx, k)
			if err != nil || string(got) != "ok" {
				_ = store.Delete(ctx, k)
				return doctor.Result{
					Status:  doctor.StatusFail,
					Message: fmt.Sprintf("backend=%s; get failed", store.Backend()),
					FixHint: "backend round-trip mismatch",
				}
			}
			_ = store.Delete(ctx, k)
			return doctor.Result{
				Status:  doctor.StatusPass,
				Message: fmt.Sprintf("backend=%s", store.Backend()),
			}
		},
	}
}
