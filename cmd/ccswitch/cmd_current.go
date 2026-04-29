package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/spf13/cobra"
)

func newCurrentCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Print the active profile name (or 'untracked' if no match)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			bridge := claude.NewDefaultBridge()
			blob, err := bridge.ReadLive(ctx)
			if err != nil {
				if errors.Is(err, claude.ErrLiveNotPresent) {
					fmt.Fprintln(cmd.ErrOrStderr(), "no live credential — run `claude /login`")
					os.Exit(1)
				}
				return err
			}
			fp := bridge.Fingerprint(blob)

			store, err := profile.LoadStore()
			if err != nil {
				return err
			}
			if p, ok := store.FindByFingerprint(fp); ok {
				fmt.Fprintln(cmd.OutOrStdout(), p.Name)
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "(untracked)")
			os.Exit(1)
			return nil
		},
	}
}
