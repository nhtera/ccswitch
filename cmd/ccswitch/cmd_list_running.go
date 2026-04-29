package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nhtera/ccswitch/internal/claude"
	"github.com/spf13/cobra"
)

// renderRunningInstances appends a "Running instances:" section to the
// list output. Best-effort — a read error is silently swallowed so a
// missing ~/.claude/sessions directory doesn't break `list`.
func renderRunningInstances(cmd *cobra.Command) {
	sessions, _ := claude.ListSessions()
	ides, _ := claude.ListIdeInstances()
	if len(sessions) == 0 && len(ides) == 0 {
		return
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, styleAccent(out, "Running instances:"))

	// Group CLI sessions by cwd so multiple sessions in the same project
	// show as one row with a session count.
	cliByCWD := map[string]int{}
	cliCWDs := []string{}
	for _, s := range sessions {
		if s.Entrypoint != "cli" && s.Entrypoint != "sdk-cli" && s.Entrypoint != "" {
			continue
		}
		if _, seen := cliByCWD[s.CWD]; !seen {
			cliCWDs = append(cliCWDs, s.CWD)
		}
		cliByCWD[s.CWD]++
	}
	sort.Strings(cliCWDs)
	for _, cwd := range cliCWDs {
		count := cliByCWD[cwd]
		suffix := ""
		if count > 1 {
			suffix = fmt.Sprintf("  (%d sessions)", count)
		}
		fmt.Fprintf(out, "  %s CLI       %s%s\n",
			styleSuccess(out, "●"),
			contractHome(cwd),
			styleMuted(out, suffix))
	}

	// IDEs from .lock files: dedupe by IDE+workspace.
	type ideRow struct {
		ide  string
		path string
	}
	seenIde := map[ideRow]bool{}
	var ideRows []ideRow
	for _, inst := range ides {
		ide := inst.IDEName
		if ide == "" {
			ide = "IDE"
		}
		if len(inst.WorkspaceFolders) == 0 {
			row := ideRow{ide: ide, path: ""}
			if !seenIde[row] {
				seenIde[row] = true
				ideRows = append(ideRows, row)
			}
			continue
		}
		for _, w := range inst.WorkspaceFolders {
			row := ideRow{ide: ide, path: w}
			if !seenIde[row] {
				seenIde[row] = true
				ideRows = append(ideRows, row)
			}
		}
	}
	sort.Slice(ideRows, func(i, j int) bool {
		if ideRows[i].ide != ideRows[j].ide {
			return ideRows[i].ide < ideRows[j].ide
		}
		return ideRows[i].path < ideRows[j].path
	})
	for _, r := range ideRows {
		path := contractHome(r.path)
		if path == "" {
			path = "(no workspace)"
		}
		fmt.Fprintf(out, "  %s %-9s %s  %s\n",
			styleSuccess(out, "●"),
			shortIdeName(r.ide),
			path,
			styleMuted(out, "(IDE)"))
	}
}

// shortIdeName collapses long IDE names ("Visual Studio Code") to
// compact labels ("VS Code") for tighter list output. Unknown names
// pass through unchanged.
func shortIdeName(name string) string {
	switch strings.ToLower(name) {
	case "visual studio code":
		return "VS Code"
	case "cursor":
		return "Cursor"
	case "windsurf":
		return "Windsurf"
	default:
		return name
	}
}

// contractHome replaces $HOME with ~ so paths read at a glance. No-op
// on error.
func contractHome(path string) string {
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}
