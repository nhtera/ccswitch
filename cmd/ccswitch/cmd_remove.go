package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Delete a profile",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			store, err := profile.LoadStore()
			if err != nil {
				return err
			}
			target, ok := store.Find(name)
			if !ok {
				return fmt.Errorf("no profile named %q", name)
			}

			active := isActiveProfile(ctx, target)

			if !force && !confirm(cmd, fmt.Sprintf("Remove %q? This cannot be undone.", name), false) {
				fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
				return nil
			}

			secStore, err := openSecrets(ctx)
			if err != nil {
				return err
			}
			if err := secStore.Delete(ctx, profile.SecretKey(name)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
				return fmt.Errorf("delete keyring entry: %w", err)
			}
			if err := store.Remove(name); err != nil {
				return fmt.Errorf("update profiles.json: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed %q.\n", name)
			if active {
				fmt.Fprintln(cmd.ErrOrStderr(), "warn: this was the active account; Claude Code will keep using it until you /logout.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation")
	return cmd
}

// isActiveProfile reports whether the profile's fingerprint matches the
// live credential. Errors (e.g. ErrLiveNotPresent) silently report false.
func isActiveProfile(ctx context.Context, p profile.Profile) bool {
	bridge := claude.NewDefaultBridge()
	blob, err := bridge.ReadLive(ctx)
	if err != nil {
		return false
	}
	return bridge.Fingerprint(blob) == p.Fingerprint
}
