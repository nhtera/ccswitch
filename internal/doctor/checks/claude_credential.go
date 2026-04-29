package checks

import (
	"context"
	"errors"
	"fmt"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/doctor"
)

// ClaudeCredentialCheck reads the live Claude Code credential and reports
// whether it's present and structurally a known envelope shape.
func ClaudeCredentialCheck(b *claude.Bridge) doctor.Check {
	return doctor.CheckFunc{
		N: "ClaudeCredential",
		Fn: func(ctx context.Context) doctor.Result {
			blob, err := b.ReadLive(ctx)
			if errors.Is(err, claude.ErrLiveNotPresent) {
				return doctor.Result{
					Status:  doctor.StatusWarn,
					Message: "no live credential present",
					FixHint: "run `claude /login` first, then `ccswitch add <name>`",
				}
			}
			if err != nil {
				return doctor.Result{
					Status:  doctor.StatusFail,
					Message: "live credential read failed",
					FixHint: err.Error(),
				}
			}
			if err := claude.Validate(blob); err != nil {
				return doctor.Result{
					Status:  doctor.StatusWarn,
					Message: "live credential present but envelope shape unknown",
					FixHint: "Claude Code may have changed its credential format; opaque-byte snapshot still works",
				}
			}
			t := claude.DetectType(blob)
			return doctor.Result{
				Status:  doctor.StatusPass,
				Message: fmt.Sprintf("present, type=%s", t),
			}
		},
	}
}
