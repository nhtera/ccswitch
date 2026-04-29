package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var (
		typeFlag string
		note     string
		envFlag  []string
	)
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Capture the currently-logged-in Claude Code account into a named profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := profile.ValidateName(name); err != nil {
				return err
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			bridge := claude.NewDefaultBridge()
			blob, err := bridge.ReadLive(ctx)
			if err != nil {
				return err
			}

			detected := bridge.DetectType(blob)
			finalType := normalizeType(typeFlag, detected)

			fp := bridge.Fingerprint(blob)

			env, err := parseEnvAssignments(envFlag)
			if err != nil {
				return err
			}

			store, err := profile.LoadStore()
			if err != nil {
				return err
			}
			if existing, ok := store.FindByFingerprint(fp); ok {
				return fmt.Errorf("this credential is already captured as profile %q (try `ccswitch rename`)", existing.Name)
			}

			secStore, err := openSecrets(ctx)
			if err != nil {
				return err
			}
			if err := secStore.Set(ctx, profile.SecretKey(name), blob); err != nil {
				return err
			}

			now := time.Now().UTC()
			p := profile.Profile{
				Name:        name,
				Type:        string(finalType),
				CreatedAt:   now,
				Note:        note,
				Fingerprint: fp,
				Env:         env,
			}
			if err := store.Add(p); err != nil {
				// Always roll back the keyring write so we don't leave an
				// orphan blob that doctor can warn about but no command can
				// clean up.
				_ = secStore.Delete(ctx, profile.SecretKey(name))
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Captured %q (%s).\n", name, finalType)
			if len(env) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  env: %d var(s)\n", len(env))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeFlag, "type", "", "override detected type (oauth|api|sso)")
	cmd.Flags().StringVar(&note, "note", "", "free-form note attached to the profile")
	cmd.Flags().StringSliceVar(&envFlag, "env", nil, "per-profile env var (repeatable: KEY=VALUE)")
	return cmd
}

func normalizeType(override string, detected claude.Type) claude.Type {
	if override == "" {
		if detected == claude.TypeUnknown {
			return claude.TypeOAuth
		}
		return detected
	}
	switch strings.ToLower(strings.TrimSpace(override)) {
	case "oauth":
		return claude.TypeOAuth
	case "api":
		return claude.TypeAPI
	case "sso":
		return claude.TypeSSO
	}
	return detected
}

func parseEnvAssignments(in []string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, raw := range in {
		eq := strings.IndexByte(raw, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("invalid --env %q: expected KEY=VALUE", raw)
		}
		k, v := raw[:eq], raw[eq+1:]
		if err := profile.ValidateEnvKey(k); err != nil {
			return nil, err
		}
		if strings.ContainsAny(v, "\n\x00") {
			return nil, fmt.Errorf("invalid --env value for %s: contains newline or NUL", k)
		}
		out[k] = v
	}
	return out, nil
}
