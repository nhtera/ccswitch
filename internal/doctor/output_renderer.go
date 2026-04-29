package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// ANSI SGR codes inlined here to keep doctor self-contained (no
// dependency on the cmd-package color helper).
const (
	doctorReset  = "\x1b[0m"
	doctorGreen  = "\x1b[32m"
	doctorYellow = "\x1b[33m"
	doctorRed    = "\x1b[31m"
	doctorDim    = "\x1b[2m"
)

func doctorColorOK(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// RenderText writes a human-readable report to w. Verbose includes fix hints
// even on PASS lines (default: hint only on non-PASS).
func RenderText(w io.Writer, rep Report, verbose bool) {
	color := doctorColorOK(w)
	for _, r := range rep.Checks {
		tag := tagFor(r.Status, color)
		fmt.Fprintf(w, "  %s  %-22s %s\n", tag, r.Name, r.Message)
		if r.FixHint != "" && (verbose || r.Status != StatusPass) {
			hint := "fix: " + r.FixHint
			if color {
				hint = doctorDim + hint + doctorReset
			}
			fmt.Fprintf(w, "        %-22s %s\n", "", hint)
		}
	}
	fmt.Fprintf(w, "\n%d PASS, %d WARN, %d FAIL", rep.Summary.Pass, rep.Summary.Warn, rep.Summary.Fail)
	if rep.Summary.Skip > 0 {
		fmt.Fprintf(w, ", %d SKIP", rep.Summary.Skip)
	}
	fmt.Fprintln(w)
}

// RenderJSON writes a machine-readable report to w.
func RenderJSON(w io.Writer, rep Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func tagFor(s Status, color bool) string {
	tag := ""
	col := ""
	switch s {
	case StatusPass:
		tag, col = "PASS", doctorGreen
	case StatusWarn:
		tag, col = "WARN", doctorYellow
	case StatusFail:
		tag, col = "FAIL", doctorRed
	case StatusSkip:
		tag, col = " -  ", doctorDim
	default:
		return "?   "
	}
	if !color {
		return tag
	}
	return col + tag + doctorReset
}
