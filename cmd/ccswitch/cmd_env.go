package main

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/nhtera/ccswitch/internal/envfile"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/nhtera/ccswitch/internal/secrets"
	"github.com/spf13/cobra"
)

func newEnvCmd() *cobra.Command {
	var (
		setFlag   []string
		unsetFlag []string
		listFlag  bool
	)
	cmd := &cobra.Command{
		Use:   "env <name>",
		Short: "Manage per-profile environment variables",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			modeCount := 0
			for _, has := range []bool{len(setFlag) > 0, len(unsetFlag) > 0, listFlag} {
				if has {
					modeCount++
				}
			}
			if modeCount == 0 {
				listFlag = true
			}

			store, err := profile.LoadStore()
			if err != nil {
				return err
			}
			target, ok := store.Find(name)
			if !ok {
				return fmt.Errorf("no profile named %q", name)
			}

			if listFlag && len(setFlag) == 0 && len(unsetFlag) == 0 {
				keys := make([]string, 0, len(target.Env))
				for k := range target.Env {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", k, target.Env[k])
				}
				return nil
			}

			set, err := parseEnvAssignments(setFlag)
			if err != nil {
				return err
			}
			for _, k := range unsetFlag {
				if err := profile.ValidateEnvKey(k); err != nil {
					return err
				}
				if _, conflicts := set[k]; conflicts {
					return fmt.Errorf("--set and --unset cannot both target %q", k)
				}
			}

			updated, err := store.UpdateEnv(name, set, unsetFlag)
			if err != nil {
				if errors.Is(err, profile.ErrNotFound) {
					return fmt.Errorf("no profile named %q", name)
				}
				return err
			}

			// If this profile is currently active, rewrite active.env so the
			// next shell sourcing reflects the change immediately.
			if isActiveProfile(ctx, updated) {
				cfgDir, derr := secrets.ConfigDir()
				if derr != nil {
					return derr
				}
				if err := envfile.WriteActiveEnv(cfgDir, updated.Env); err != nil {
					return fmt.Errorf("rewrite active.env: %w", err)
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Updated env for %q (%d var(s)).\n", name, len(updated.Env))
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&setFlag, "set", nil, "set KEY=VALUE (repeatable)")
	cmd.Flags().StringSliceVar(&unsetFlag, "unset", nil, "remove KEY (repeatable)")
	cmd.Flags().BoolVar(&listFlag, "list", false, "list current env vars")
	return cmd
}
