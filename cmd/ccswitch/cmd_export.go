package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nhtera/ccswitch/internal/cryptobox"
	"github.com/nhtera/ccswitch/internal/export"
	"github.com/nhtera/ccswitch/internal/profile"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newExportCmd() *cobra.Command {
	var (
		outFlag     string
		profileFlag string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Encrypt and write all (or one) profile to a portable bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			store, err := profile.LoadStore()
			if err != nil {
				return err
			}
			secStore, err := openSecrets(ctx)
			if err != nil {
				return err
			}

			var names []string
			if profileFlag != "" {
				names = []string{profileFlag}
			}
			pt, err := export.Build(ctx, store, secStore, names)
			if err != nil {
				return err
			}

			pass, err := readBundlePassphrase(true)
			if err != nil {
				return err
			}

			if err := warnWeakPassphrase(cmd, pass); err != nil {
				return err
			}

			sealed, err := export.Seal(pt, pass, cryptobox.DefaultParams)
			if err != nil {
				return err
			}

			path := outFlag
			if path == "" {
				path = fmt.Sprintf("ccswitch-export-%s.cce", time.Now().UTC().Format("20060102-150405"))
			}
			// Refuse to overwrite without an explicit --out (matches user
			// intent for the timestamped default name; for explicit --out
			// the user has already named the target).
			if outFlag == "" {
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("bundle file already exists: %s", path)
				}
			}
			if err := os.WriteFile(path, sealed, 0o600); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Exported %d profile(s) to %s\n", len(pt.Profiles), path)
			return nil
		},
	}
	cmd.Flags().StringVar(&outFlag, "out", "", "output file (default: ccswitch-export-<timestamp>.cce)")
	cmd.Flags().StringVar(&profileFlag, "profile", "", "export only this profile (default: all)")
	return cmd
}

func readBundlePassphrase(confirmRequired bool) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, fmt.Errorf("a passphrase is required but stdin is not a terminal")
	}
	fmt.Fprint(os.Stderr, "Bundle passphrase: ")
	pw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, err
	}
	if len(pw) == 0 {
		return nil, fmt.Errorf("passphrase cannot be empty")
	}
	if confirmRequired {
		fmt.Fprint(os.Stderr, "Confirm:           ")
		pw2, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, err
		}
		if string(pw) != string(pw2) {
			return nil, fmt.Errorf("passphrases do not match")
		}
	}
	return pw, nil
}

func warnWeakPassphrase(cmd *cobra.Command, pw []byte) error {
	if len(pw) < 12 || strings.ToLower(string(pw)) == string(pw) {
		fmt.Fprintln(cmd.ErrOrStderr(), "warn: passphrase is short or all-lowercase. A long random string is much harder to guess.")
	}
	return nil
}
