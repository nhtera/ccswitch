package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var jsonOut bool
	var withUsage bool
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

			// Resolve which profile (if any) is currently active. We
			// match by stable fingerprint first (refresh-safe) and fall
			// back to volatile blob fingerprint, so the active marker
			// stays correct even after Claude Code rotates the token.
			bridge := claude.NewDefaultBridge()
			activeName := ""
			if active, ok, _, _ := findActiveProfile(ctx, bridge, store); ok {
				activeName = active.Name
			}
			// currentFp kept for the JSON output's
			// "current_fingerprint" field — script consumers may already
			// rely on it.
			currentFp := ""
			if blob, err := bridge.ReadLive(ctx); err == nil {
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

			if withUsage {
				return renderListWithUsage(ctx, cmd, profiles, activeName)
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
			out := cmd.OutOrStdout()
			for _, p := range profiles {
				marker := " "
				name := p.Name
				if p.Name == activeName {
					marker = styleAccent(out, "*")
					name = styleAccent(out, p.Name)
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
					fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\t%s\n",
						marker, name, p.Type,
						styleMuted(out, identity),
						styleMuted(out, lastUsed),
						styleMuted(out, note))
				} else {
					fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\n",
						marker, name, p.Type,
						styleMuted(out, lastUsed),
						styleMuted(out, note))
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
	cmd.Flags().BoolVar(&withUsage, "usage", false, "fetch 5h/7d quota usage from Anthropic API (network call per profile)")
	return cmd
}

// renderListWithUsage produces a per-account block layout: an "Accounts:"
// section with one block per profile (identity row + 5h/7d lines),
// followed by the running-instance footer. Network calls are made
// here only.
func renderListWithUsage(ctx context.Context, cmd *cobra.Command, profiles []profile.Profile, activeName string) error {
	out := cmd.OutOrStdout()
	secStore, err := openSecrets(ctx)
	if err != nil {
		return err
	}

	// Sort by name so the displayed "N:" prefix matches what
	// `ccswitch use <N>` resolves to (Store.Lookup uses sorted order).
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })

	rows := fetchUsageForProfiles(ctx, profiles, secStore)

	fmt.Fprintln(out, styleAccent(out, "Accounts:"))
	for i, p := range profiles {
		identity := formatIdentity(p)
		marker := ""
		if p.Name == activeName {
			marker = " " + styleAccent(out, "(active)")
		}
		fmt.Fprintf(out, "  %s %s %s%s\n",
			styleMuted(out, fmt.Sprintf("%d:", i+1)),
			p.Name,
			styleMuted(out, "— "+identity),
			marker)
		renderUsageRows(out, []usageRow{rows[i]}, []profile.Profile{p})
		fmt.Fprintln(out)
	}
	renderRunningInstances(cmd)
	return nil
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
