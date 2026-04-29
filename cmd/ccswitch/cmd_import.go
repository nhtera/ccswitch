package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nhtera/ccswitch/internal/export"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	var renames []string
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "Decrypt and import a previously-exported bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			pass, err := readBundlePassphrase(false)
			if err != nil {
				return err
			}

			pt, err := export.OpenPlaintext(data, pass)
			if err != nil {
				return err
			}

			renameMap, err := parseRenames(renames)
			if err != nil {
				return err
			}

			store, err := profile.LoadStore()
			if err != nil {
				return err
			}
			secStore, err := openSecrets(ctx)
			if err != nil {
				return err
			}

			plan := export.Inspect(pt, store, export.ImportOption{Renames: renameMap})
			fmt.Fprintf(cmd.OutOrStdout(), "Bundle: %d profile(s), exported %s.\n", len(pt.Profiles), pt.ExportedAt.Local().Format("2006-01-02 15:04"))
			if len(plan.ToImport) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  to import: %s\n", strings.Join(plan.ToImport, ", "))
			}
			if len(plan.Renamed) > 0 {
				for old, neu := range plan.Renamed {
					fmt.Fprintf(cmd.OutOrStdout(), "  rename:    %s → %s\n", old, neu)
				}
			}
			if len(plan.Conflict) > 0 {
				return fmt.Errorf("name conflict on %v — pass --rename old=new", plan.Conflict)
			}

			if !confirm(cmd, "Apply this import?", true) {
				fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
				return nil
			}

			added, err := export.Apply(ctx, pt, store, secStore, export.ImportOption{Renames: renameMap})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Imported %d profile(s): %s\n", len(added), strings.Join(added, ", "))
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&renames, "rename", nil, "resolve a name conflict (repeatable: old=new)")
	return cmd
}

func parseRenames(in []string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, raw := range in {
		eq := strings.IndexByte(raw, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("invalid --rename %q: expected old=new", raw)
		}
		out[raw[:eq]] = raw[eq+1:]
	}
	return out, nil
}
