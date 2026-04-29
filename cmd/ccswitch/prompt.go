package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// confirm prints prompt, reads a y/n answer from stdin, and returns the
// boolean result. The default is used on empty input. Honors the root
// --yes flag (which short-circuits to true). When stdin is not a
// terminal, emits a hint to stderr and returns the default — silently
// falling through to the default left users with an opaque "Aborted."
// in non-interactive contexts (CI, headless ssh).
func confirm(cmd *cobra.Command, prompt string, def bool) bool {
	if yes, _ := cmd.Root().PersistentFlags().GetBool("yes"); yes {
		return true
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(cmd.ErrOrStderr(), "ccswitch: stdin is not a terminal — pass --yes to confirm non-interactively.")
		return def
	}
	suffix := " [y/N]: "
	if def {
		suffix = " [Y/n]: "
	}
	fmt.Fprint(cmd.ErrOrStderr(), prompt+suffix)
	answer := strings.ToLower(strings.TrimSpace(readLine(os.Stdin)))
	switch answer {
	case "":
		return def
	case "y", "yes":
		return true
	default:
		return false
	}
}

func readLine(r io.Reader) string {
	s := bufio.NewScanner(r)
	if s.Scan() {
		return s.Text()
	}
	return ""
}
