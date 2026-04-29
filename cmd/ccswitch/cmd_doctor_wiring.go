package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/nhtera/ccswitch/internal/doctor"
	"github.com/nhtera/ccswitch/internal/doctor/checks"
	"github.com/nhtera/ccswitch/internal/secrets"

	"golang.org/x/term"
)

// init replaces the default doctor runner with one that includes the
// cross-package checks. It runs once at program start, before cobra parses.
func init() {
	buildDoctorRunner = func(checkShell bool) *doctor.Runner {
		r := doctor.NewRunner()
		r.Register(doctor.PlatformCheck())
		r.Register(checks.SecretsBackendCheck(openSecrets))
		r.Register(checks.ClaudeCredentialCheck(claude.NewDefaultBridge()))
		r.Register(checks.ProfilesStoreCheck())
		r.Register(checks.FingerprintMatchCheck(claude.NewDefaultBridge()))
		r.Register(checks.OrphanSecretsCheck(openSecrets))
		r.Register(checks.ShellHookCheck(checkShell))
		return r
	}
}

// openSecrets is the central point where the rest of the CLI obtains a
// Store. Defining it here (rather than per-command) keeps the passphrase
// prompt consistent regardless of which command triggered the file backend.
func openSecrets(ctx context.Context) (secrets.Store, error) {
	return secrets.Open(ctx, promptVaultPassphrase)
}

// promptVaultPassphrase reads a passphrase from the controlling TTY without
// echoing. On firstRun we read it twice to confirm.
func promptVaultPassphrase(firstRun bool) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, fmt.Errorf("file vault needs a passphrase but stdin is not a terminal")
	}
	if firstRun {
		fmt.Fprintln(os.Stderr, "ccswitch needs a passphrase to encrypt the local file vault.")
		fmt.Fprintln(os.Stderr, "This passphrase is asked once per session and never written to disk.")
	}
	fmt.Fprint(os.Stderr, "Passphrase: ")
	pw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("read passphrase: %w", err)
	}
	if len(pw) == 0 {
		return nil, fmt.Errorf("passphrase cannot be empty")
	}
	if firstRun {
		fmt.Fprint(os.Stderr, "Confirm:    ")
		pw2, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, fmt.Errorf("read confirm: %w", err)
		}
		if string(pw) != string(pw2) {
			return nil, fmt.Errorf("passphrases do not match")
		}
	}
	return pw, nil
}
