package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
	"github.com/spf13/cobra"
)

func newRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename a profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			old, newName := args[0], args[1]
			if err := profile.ValidateName(newName); err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			store, err := profile.LoadStore()
			if err != nil {
				return err
			}
			if _, ok := store.Find(old); !ok {
				if hint := suggestName(store, old); hint != "" {
					return fmt.Errorf("no profile named %q. Did you mean %q?", old, hint)
				}
				return fmt.Errorf("no profile named %q", old)
			}
			if _, ok := store.Find(newName); ok {
				return fmt.Errorf("a profile named %q already exists", newName)
			}

			secStore, err := openSecrets(ctx)
			if err != nil {
				return err
			}
			blob, err := secStore.Get(ctx, profile.SecretKey(old))
			if err != nil {
				return fmt.Errorf("read old credential: %w", err)
			}

			if err := secStore.Set(ctx, profile.SecretKey(newName), blob); err != nil {
				return fmt.Errorf("write new credential: %w", err)
			}
			// store.Rename re-validates collision under its mutex, so a
			// concurrent ccswitch run that registered `newName` between
			// the Find check above and here will land us in the rollback
			// path below — not a corrupted store.
			if err := store.Rename(old, newName); err != nil {
				_ = secStore.Delete(ctx, profile.SecretKey(newName))
				return fmt.Errorf("update profiles.json: %w", err)
			}
			if err := secStore.Delete(ctx, profile.SecretKey(old)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: failed to delete old keyring entry for %q: %v\n", old, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Renamed %q → %q.\n", old, newName)
			return nil
		},
	}
}
