package main

import (
	"context"
	"os"

	"github.com/nhtera/ccswitch/internal/doctor"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var (
		jsonOut    bool
		verbose    bool
		checkShell bool
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run read-only diagnostics",
		Long: `Doctor inspects the local environment and reports problems with hints
on how to fix them. It never mutates state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			runner := buildDoctorRunner(checkShell)
			rep := runner.Run(ctx)

			if jsonOut {
				if err := doctor.RenderJSON(cmd.OutOrStdout(), rep); err != nil {
					return err
				}
			} else {
				doctor.RenderText(cmd.OutOrStdout(), rep, verbose)
			}

			if rep.Summary.Fail > 0 {
				os.Exit(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON instead of text")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "include fix hints on PASS lines")
	cmd.Flags().BoolVar(&checkShell, "check-shell", false, "inspect shell rc files for active.env sourcing line")
	return cmd
}

// buildDoctorRunner is overridden in pkg files that have additional checks
// (e.g. cmd_doctor_checks.go wires the cross-package checks). Keeping the
// indirection lets doctor.go stay free of secrets/profile imports.
var buildDoctorRunner = func(checkShell bool) *doctor.Runner {
	r := doctor.NewRunner()
	r.Register(doctor.PlatformCheck())
	return r
}
