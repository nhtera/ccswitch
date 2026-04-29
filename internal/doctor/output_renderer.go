package doctor

import (
	"encoding/json"
	"fmt"
	"io"
)

// RenderText writes a human-readable report to w. Verbose includes fix hints
// even on PASS lines (default: hint only on non-PASS).
func RenderText(w io.Writer, rep Report, verbose bool) {
	for _, r := range rep.Checks {
		tag := tagFor(r.Status)
		fmt.Fprintf(w, "  %s  %-22s %s\n", tag, r.Name, r.Message)
		if r.FixHint != "" && (verbose || r.Status != StatusPass) {
			fmt.Fprintf(w, "        %-22s fix: %s\n", "", r.FixHint)
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

func tagFor(s Status) string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusWarn:
		return "WARN"
	case StatusFail:
		return "FAIL"
	case StatusSkip:
		return " -  "
	}
	return "?   "
}
