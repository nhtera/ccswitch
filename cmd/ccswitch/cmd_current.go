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
				if id := formatIdentity(p); id != "-" {
					fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", p.Name, id)
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), p.Name)
				}
				return nil
			}
			// Untracked credential — show whatever account info Claude
			// Code's global config knows about so the user can see who's
			// logged in even without a matching profile.
			line := "(untracked)"
			if info, _ := claude.ReadAccountInfo(); info != nil {
				if info.OrgName != "" {
					line = fmt.Sprintf("(untracked: %s [%s])", info.Email, info.OrgName)
				} else if info.Email != "" {
					line = fmt.Sprintf("(untracked: %s)", info.Email)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), line)
			os.Exit(1)
			return nil
		},
	}
}
