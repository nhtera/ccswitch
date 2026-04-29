package checks

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nhtera/ccswitch/internal/doctor"
)

// ShellHookCheck inspects ~/.zshrc and ~/.bashrc looking for a line that
// sources the active.env file. It returns SKIP unless enabled is true.
func ShellHookCheck(enabled bool) doctor.Check {
	return doctor.CheckFunc{
		N: "ShellHook",
		Fn: func(ctx context.Context) doctor.Result {
			if !enabled {
				return doctor.Result{Status: doctor.StatusSkip, Message: "use --check-shell to inspect rc files"}
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return doctor.Result{Status: doctor.StatusFail, Message: err.Error()}
			}
			found := []string{}
			for _, f := range []string{".zshrc", ".bashrc"} {
				p := filepath.Join(home, f)
				data, err := os.ReadFile(p)
				if err != nil {
					if errors.Is(err, fs.ErrNotExist) {
						continue
					}
					continue
				}
				if strings.Contains(string(data), "active.env") {
					found = append(found, f)
				}
			}
			if len(found) == 0 {
				return doctor.Result{
					Status:  doctor.StatusWarn,
					Message: "no shell rc file sources active.env",
					FixHint: `add: [ -f ~/.config/ccswitch/active.env ] && . ~/.config/ccswitch/active.env  # to ~/.zshrc or ~/.bashrc`,
				}
			}
			return doctor.Result{Status: doctor.StatusPass, Message: "found in: " + strings.Join(found, ", ")}
		},
	}
}
