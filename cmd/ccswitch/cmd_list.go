package main

import (
	"context"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "Show all profiles, marking the active one",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			store, err := profile.LoadStore()
			if err != nil {
				return err
			}
			profiles := store.All()

			currentFp := ""
			if blob, err := claude.NewDefaultBridge().ReadLive(ctx); err == nil {
				currentFp = claude.Fingerprint(blob)
			}

			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					CurrentFingerprint string            `json:"current_fingerprint,omitempty"`
					Profiles           []profile.Profile `json:"profiles"`
				}{currentFp, profiles})
			}

			if len(profiles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No profiles. Run `ccswitch add <name>` after `claude /login`.")
				return nil
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tTYPE\tACTIVE\tLAST USED\tNOTE")
			for _, p := range profiles {
				active := ""
				if p.Fingerprint == currentFp {
					active = "*"
				}
				lastUsed := "-"
				if p.LastUsedAt != nil {
					lastUsed = p.LastUsedAt.Local().Format("2006-01-02 15:04")
				}
				note := p.Note
				if note == "" {
					note = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.Name, p.Type, active, lastUsed, note)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON instead of a table")
	return cmd
}
