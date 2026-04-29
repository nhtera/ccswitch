package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
	"github.com/spf13/cobra"
)

// newPruneCmd wipes every trace of ccswitch from the machine: all
// per-profile keychain entries, profiles.json, the file-vault, the
// keyring index, the usage cache, and active.env. The live Claude Code
// login (the system-keychain "Claude Code-credentials" item and
// ~/.claude.json) is intentionally left alone — `claude /status`
// should still work after a prune.
func newPruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "prune",
		Aliases: []string{"purge"},
		Short:   "Remove ALL ccswitch data (profiles, secrets, cache) — leaves Claude Code itself untouched",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			cfgDir, err := secrets.ConfigDir()
			if err != nil {
				return err
			}

			// Snapshot what we're about to nuke before asking, so the
			// confirmation prompt is concrete.
			var profileNames []string
			if store, err := profile.LoadStore(); err == nil {
				for _, p := range store.All() {
					profileNames = append(profileNames, p.Name)
				}
			}

			fmt.Fprintln(out, styleWarn(out, "This will remove ALL ccswitch data from your system:"))
			fmt.Fprintf(out, "  - Config directory: %s\n", contractHome(cfgDir))
			if len(profileNames) > 0 {
				fmt.Fprintf(out, "  - %d stored credential(s) from the system keyring\n", len(profileNames))
			} else {
				fmt.Fprintln(out, "  - All stored credentials from the system keyring (if any)")
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, styleMuted(out, "Note: this does NOT affect your current Claude Code login."))
			fmt.Fprintln(out)

			if !confirm(cmd, "Are you sure you want to prune all data?", false) {
				fmt.Fprintln(out, styleMuted(out, "Cancelled"))
				return nil
			}

			var removed []string

			// Delete keychain entries. We use List("profile.") as the
			// authoritative set of keys to wipe — this covers both the
			// profiles in profiles.json AND any orphans whose metadata
			// rows were deleted but whose secrets lingered.
			//
			// If Open fails (e.g. file-vault passphrase mismatch), we
			// still proceed to remove the on-disk config — but warn the
			// user that keychain entries may linger.
			secStore, openErr := openSecrets(ctx)
			if openErr == nil {
				existing, _ := secStore.List(ctx, "profile.")
				existingSet := map[string]bool{}
				for _, k := range existing {
					existingSet[k] = true
				}
				for _, name := range profileNames {
					k := profile.SecretKey(name)
					if !existingSet[k] {
						continue
					}
					if err := secStore.Delete(ctx, k); err == nil {
						removed = append(removed, "Credential: "+name)
					} else if !errors.Is(err, secrets.ErrNotFound) {
						fmt.Fprintf(errOut, "warn: delete keychain entry %q: %v\n", k, err)
					}
					delete(existingSet, k)
				}
				for k := range existingSet {
					orphanName := strings.TrimPrefix(k, "profile.")
					if err := secStore.Delete(ctx, k); err == nil {
						removed = append(removed, "Orphan credential: "+orphanName)
					}
				}
			} else {
				fmt.Fprintf(errOut, "warn: cannot open secret store (%v) — keychain entries may need manual cleanup\n", openErr)
			}

			// Nuke the config directory (profiles.json, keyring-index.json,
			// secrets.enc, cache/, active.env). Skip if it doesn't exist.
			if info, err := os.Stat(cfgDir); err == nil && info.IsDir() {
				if err := os.RemoveAll(cfgDir); err != nil {
					return fmt.Errorf("remove config dir %s: %w", cfgDir, err)
				}
				removed = append(removed, "Directory: "+contractHome(cfgDir))
			}

			fmt.Fprintln(out)
			if len(removed) == 0 {
				fmt.Fprintln(out, styleMuted(out, "No ccswitch data found to remove."))
			} else {
				fmt.Fprintln(out, styleAccent(out, "Removed:"))
				for _, item := range removed {
					fmt.Fprintf(out, "  %s %s\n", styleMuted(out, "-"), item)
				}
			}
			fmt.Fprintln(out)
			fmt.Fprintln(out, styleAccent(out, "Prune complete."))
			return nil
		},
	}
	return cmd
}

