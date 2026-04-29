package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "ccswitch",
		Short: "Switch between multiple Claude Code accounts in <100 ms",
		Long: `ccswitch captures your currently-logged-in Claude Code account into a named
profile, then lets you swap between profiles with one command.

It never replaces 'claude /login' — it just snapshots and restores the
credential envelope your platform's secret store already holds.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Persistent flags are stored on the cobra command, not as package
	// globals — so each call to newRootCmd() (e.g. from tests) gets fresh
	// state. confirm() and main() read them via PersistentFlags().GetBool().
	root.PersistentFlags().Bool("debug", false, "verbose error output")
	root.PersistentFlags().BoolP("yes", "y", false, "assume yes for confirmation prompts")

	registerCommands(root)
	return root
}

func main() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		debug, _ := root.PersistentFlags().GetBool("debug")
		if debug {
			fmt.Fprintf(os.Stderr, "error: %+v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
}
