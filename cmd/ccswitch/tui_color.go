package main

import (
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Minimal ANSI styling helpers for CLI output. Honors:
//   - NO_COLOR env var (https://no-color.org) — if set to anything, no
//     escapes are emitted regardless of the writer.
//   - TTY detection — colors only when the destination is a real
//     terminal, so piped output stays clean for grep/awk/jq.
//
// We deliberately avoid pulling in lipgloss / fatih/color — a few
// SGR constants is all the polish we need for now.

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
	ansiOrange = "\x1b[38;5;208m" // 256-color orange
)

// colorEnabledFor returns true when ANSI escapes should be emitted to
// w. Conservative: any non-os.File writer (test buffers etc.) gets no
// colors so test assertions stay stable. NO_COLOR overrides everything.
func colorEnabledFor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// stylize wraps s in start/end SGR codes when w is a TTY, otherwise
// returns s unchanged.
func stylize(w io.Writer, codes string, s string) string {
	if !colorEnabledFor(w) {
		return s
	}
	return codes + s + ansiReset
}

func styleAccent(w io.Writer, s string) string  { return stylize(w, ansiOrange+ansiBold, s) }
func styleMuted(w io.Writer, s string) string   { return stylize(w, ansiDim, s) }
func styleSuccess(w io.Writer, s string) string { return stylize(w, ansiGreen, s) }
func styleWarn(w io.Writer, s string) string    { return stylize(w, ansiYellow, s) }
func styleDanger(w io.Writer, s string) string  { return stylize(w, ansiRed, s) }
func styleCyan(w io.Writer, s string) string    { return stylize(w, ansiCyan, s) }

// stylePercent colors a percentage value by usage band:
//
//	0–50  → green (plenty of headroom)
//	50–80 → yellow (warning)
//	>80   → red (about to throttle)
//
// The numeric threshold is conservative; users who hit 80% in either
// the 5-hour or 7-day window are likely to bump into the limit soon.
func stylePercent(w io.Writer, pct float64, formatted string) string {
	switch {
	case pct >= 80:
		return styleDanger(w, formatted)
	case pct >= 50:
		return styleWarn(w, formatted)
	default:
		return styleSuccess(w, formatted)
	}
}

// padVisible returns s padded with trailing spaces to reach width
// `width` of VISIBLE characters. ANSI escape sequences are not
// counted, so colored strings still align in tabular layouts.
func padVisible(s string, width int) string {
	visible := stripANSI(s)
	if len(visible) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(visible))
}

// stripANSI removes SGR escape sequences from s. Used for visible-width
// calculations only — never to mutate user-facing output.
func stripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
