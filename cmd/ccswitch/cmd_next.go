package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/spf13/cobra"
)

// newNextCmd is the rotation command. Picks the next profile in
// alphabetical order from the currently-active one (wraps around).
// Useful when a user just wants to flip between two accounts
// without typing a name.
func newNextCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "next",
		Aliases: []string{"switch", "rotate"},
		Short:   "Rotate to the next profile in alphabetical order",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			store, err := profile.LoadStore()
			if err != nil {
				return err
			}

			// Rotation is driven by store.LastUsedAt — the profile we
			// most recently switched to via `ccswitch use`. We
			// deliberately don't rely on live-credential matching
			// here: if ~/.claude.json is stale (e.g. older profile
			// without an oauth_account snapshot), stable-fingerprint
			// detection can return the wrong profile and `next` ends
			// up bouncing back to the same target every time.
			currentName := store.MostRecentlyUsedName()

			next, err := store.NextProfile(currentName)
			if err != nil {
				if errors.Is(err, profile.ErrNotFound) {
					return errors.New("no profiles to rotate to — run `ccswitch add` first")
				}
				return err
			}
			// Single-profile case: rotation is a no-op.
			if next.Name == currentName {
				fmt.Fprintf(cmd.OutOrStdout(), "Already on the only profile %q.\n", currentName)
				return nil
			}
			return runUse(ctx, cmd, next.Name)
		},
	}
}
