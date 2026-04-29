package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/envfile"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
	"github.com/spf13/cobra"
)

func newUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch the active Claude Code account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return runUse(ctx, cmd, args[0])
		},
	}
}

func runUse(ctx context.Context, cmd *cobra.Command, name string) error {
	if err := profile.ValidateName(name); err != nil {
		return err
	}

	store, err := profile.LoadStore()
	if err != nil {
		return err
	}
	target, ok := store.Find(name)
	if !ok {
		if hint := suggestName(store, name); hint != "" {
			return fmt.Errorf("no profile named %q. Did you mean %q?", name, hint)
		}
		return fmt.Errorf("no profile named %q. Run `ccswitch list` to see profiles", name)
	}

	bridge := claude.NewDefaultBridge()

	// Untracked-credential detection: if a live cred exists and doesn't
	// match any known profile, warn and offer capture.
	if liveBlob, err := bridge.ReadLive(ctx); err == nil {
		fp := bridge.Fingerprint(liveBlob)
		if _, found := store.FindByFingerprint(fp); !found && fp != target.Fingerprint {
			if !confirm(cmd, "Current credential isn't tracked — switching will overwrite it. Proceed?", false) {
				fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
				return nil
			}
		}
	} else if !errors.Is(err, claude.ErrLiveNotPresent) {
		return fmt.Errorf("read live credential: %w", err)
	}

	secStore, err := openSecrets(ctx)
	if err != nil {
		return err
	}
	blob, err := secStore.Get(ctx, profile.SecretKey(name))
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return fmt.Errorf("profile %q is in profiles.json but its credential is missing from the secret store; run `ccswitch doctor`", name)
		}
		return err
	}

	if err := bridge.WriteLive(ctx, blob); err != nil {
		return fmt.Errorf("write live credential: %w", err)
	}

	cfgDir, err := secrets.ConfigDir()
	if err != nil {
		return err
	}
	if err := envfile.WriteActiveEnv(cfgDir, target.Env); err != nil {
		// Live credential is already switched at this point — surface that
		// fact so the user knows Claude Code itself is on the new account
		// even though the shell-sourced env overlay is stale.
		return fmt.Errorf("switched live credential to %q but failed to write active.env (shell env vars may be stale until you re-run `ccswitch use %s`): %w", name, name, err)
	}
	if err := store.Touch(name, time.Now().UTC()); err != nil && !errors.Is(err, profile.ErrNotFound) {
		// Non-fatal — switch already succeeded.
		fmt.Fprintf(cmd.ErrOrStderr(), "warn: failed to update last_used_at: %v\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Switched to %q (%s).", name, target.Type)
	if len(target.Env) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), " active.env: %d var(s).", len(target.Env))
	}
	fmt.Fprintln(cmd.OutOrStdout())
	return nil
}

// suggestName returns the closest profile name within edit distance 2 of
// query, or "" if none qualify.
func suggestName(store *profile.Store, query string) string {
	best := ""
	bestD := 3
	for _, p := range store.All() {
		if d := levenshtein(query, p.Name); d < bestD {
			best, bestD = p.Name, d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = curr
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
