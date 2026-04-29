package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newAddCmd() *cobra.Command {
	var (
		typeFlag string
		note     string
		envFlag  []string
	)
	cmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Capture the currently-logged-in Claude Code account into a named profile",
		Long: `Capture the currently-logged-in Claude Code account into a named profile.

If no name is given, ccswitch reads ~/.claude.json to suggest one based on the
account's email or organization name. Pass --yes to silently accept the
suggestion.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			bridge := claude.NewDefaultBridge()
			blob, err := bridge.ReadLive(ctx)
			if err != nil {
				return err
			}

			info, infoErr := claude.ReadAccountInfo()
			// We don't fail on infoErr — the credential is the source of
			// truth, the .claude.json metadata is best-effort polish.

			name, err := resolveName(cmd, args, info)
			if err != nil {
				return err
			}
			if err := profile.ValidateName(name); err != nil {
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
			if info != nil {
				p.Email = info.Email
				p.OrgName = info.OrgName
			}
			if err := store.Add(p); err != nil {
				_ = secStore.Delete(ctx, profile.SecretKey(name))
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), formatCaptureLine(name, finalType, info, infoErr))
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

// resolveName picks the profile name, in priority order:
//  1. Explicit positional argument.
//  2. profile.SuggestName(email, orgName) if AccountInfo is available.
//  3. Interactive prompt with the suggestion as the default.
//
// When --yes is set, the suggestion is accepted silently. When stdin is
// not a terminal AND no name was given AND we have no suggestion, we
// fail rather than guess.
func resolveName(cmd *cobra.Command, args []string, info *claude.AccountInfo) (string, error) {
	if len(args) == 1 && args[0] != "" {
		return args[0], nil
	}
	suggested := ""
	if info != nil {
		suggested = profile.SuggestName(info.Email, info.OrgName)
	}
	yes, _ := cmd.Root().PersistentFlags().GetBool("yes")
	if yes {
		if suggested == "" {
			return "", errors.New("no name given and unable to derive one from ~/.claude.json — pass an explicit name")
		}
		return suggested, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		if suggested == "" {
			return "", errors.New("no name given, no terminal to prompt, and no .claude.json hint — pass an explicit name or --yes")
		}
		return suggested, nil
	}

	// Interactive prompt with the suggestion as default.
	prompt := "Profile name"
	if suggested != "" {
		prompt = fmt.Sprintf("%s [%s]", prompt, suggested)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s: ", prompt)
	answer := strings.TrimSpace(readLine(os.Stdin))
	if answer == "" {
		if suggested == "" {
			return "", errors.New("a profile name is required")
		}
		return suggested, nil
	}
	return answer, nil
}

// formatCaptureLine renders the success message. Mirrors the style
// claude-swap uses ("Updated credentials for Account 2
// (tn@erai.dev [ERAI DEV])") so users coming from
// that tool see familiar output.
func formatCaptureLine(name string, t claude.Type, info *claude.AccountInfo, infoErr error) string {
	if info == nil {
		hint := ""
		if infoErr != nil {
			hint = fmt.Sprintf(" — couldn't read ~/.claude.json: %v", infoErr)
		}
		return fmt.Sprintf("Captured %q (%s)%s.", name, t, hint)
	}
	identity := info.Email
	if info.OrgName != "" {
		identity = fmt.Sprintf("%s [%s]", info.Email, info.OrgName)
	}
	return fmt.Sprintf("Captured %q (%s, %s).", name, t, identity)
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
