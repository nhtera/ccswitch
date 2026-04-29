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
				fmt.Fprintln(cmd.OutOrStdout(), "No profiles. Run `ccswitch add` after `claude /login`.")
				return nil
			}

			// Show identity columns (EMAIL/ORG) only when at least one
			// profile has them — keeps the table tight for users who
			// captured profiles before the .claude.json reader existed.
			showIdentity := false
			for _, p := range profiles {
				if p.Email != "" || p.OrgName != "" {
					showIdentity = true
					break
				}
			}

			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			if showIdentity {
				fmt.Fprintln(tw, "  NAME\tTYPE\tIDENTITY\tLAST USED\tNOTE")
			} else {
				fmt.Fprintln(tw, "  NAME\tTYPE\tLAST USED\tNOTE")
			}
			for _, p := range profiles {
				marker := " "
				if p.Fingerprint == currentFp {
					marker = "*"
				}
				lastUsed := "-"
				if p.LastUsedAt != nil {
					lastUsed = p.LastUsedAt.Local().Format("2006-01-02 15:04")
				}
				note := p.Note
				if note == "" {
					note = "-"
				}
				if showIdentity {
					identity := formatIdentity(p)
					fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\t%s\n", marker, p.Name, p.Type, identity, lastUsed, note)
				} else {
					fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\n", marker, p.Name, p.Type, lastUsed, note)
				}
			}
			if err := tw.Flush(); err != nil {
				return err
			}

			// Append running-instance footer (best-effort — silent on error).
			renderRunningInstances(cmd)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON instead of a table")
	return cmd
}

// formatIdentity renders "email [org]" or just "email" or "-".
func formatIdentity(p profile.Profile) string {
	switch {
	case p.Email != "" && p.OrgName != "":
		return fmt.Sprintf("%s [%s]", p.Email, p.OrgName)
	case p.Email != "":
		return p.Email
	case p.OrgName != "":
		return fmt.Sprintf("[%s]", p.OrgName)
	default:
		return "-"
	}
}
