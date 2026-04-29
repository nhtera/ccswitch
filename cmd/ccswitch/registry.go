package main

import "github.com/spf13/cobra"

// registerCommands wires all subcommands onto root.
func registerCommands(root *cobra.Command) {
	root.AddCommand(newVersionCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newAddCmd())
	root.AddCommand(newUseCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newCurrentCmd())
	root.AddCommand(newRenameCmd())
	root.AddCommand(newRemoveCmd())
	root.AddCommand(newEnvCmd())
	root.AddCommand(newExportCmd())
	root.AddCommand(newImportCmd())
}
