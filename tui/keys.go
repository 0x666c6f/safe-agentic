package main

import (
	"fmt"
	"strings"
)

// keyBinding is one row of the single keybinding table. It is the sole source of
// truth for the footer strip, the ? help overlay, the --help text, and the
// rune-input dispatch (via Handler). Add or change a binding in one place here
// and every surface updates.
type keyBinding struct {
	Key     string // display token, e.g. "a", "^K", "Enter", "1–9"
	Label   string // short footer label; empty means "not shown in the footer"
	Desc    string // longer description for the help overlay / --help
	Section string // grouping in the help overlay
	Handler string // handler id dispatched for single-rune keys; empty = none
}

// keySections is the display order of help-overlay groups.
var keySections = []string{"Navigation", "Actions", "Inspect", "Other"}

// keyBindings lists every binding in display order. Single-character Key values
// with a Handler are dispatched as rune actions; multi-character keys (Enter,
// ^K, Esc, …) are dispatched by the special-key switch in handleGlobalInput and
// listed here for help/footer rendering only.
var keyBindings = []keyBinding{
	// Navigation
	{Key: "j/k", Desc: "Move selection up/down", Section: "Navigation"},
	{Key: "g", Desc: "Go to top", Section: "Navigation", Handler: "top"},
	{Key: "G", Desc: "Go to bottom", Section: "Navigation", Handler: "bottom"},
	{Key: "1–9", Desc: "Sort by visible column number", Section: "Navigation"},
	{Key: "/", Label: "Filter", Desc: "Filter agents by keyword", Section: "Navigation", Handler: "filter"},
	{Key: ":", Label: "Cmd", Desc: "Command mode (quit, fleet, pipeline, profile, action, comments, timeline, inbox, pr-review, pr-fix, audit)", Section: "Navigation", Handler: "command"},

	// Actions
	{Key: "Enter", Desc: "Attach to selected agent (tmux)", Section: "Actions"},
	{Key: "a", Label: "Attach", Desc: "Attach to selected agent (tmux)", Section: "Actions", Handler: "attach"},
	{Key: "r", Desc: "Resume agent session", Section: "Actions", Handler: "resume"},
	{Key: "s", Label: "Stop", Desc: "Stop selected agent", Section: "Actions", Handler: "stop"},
	{Key: "^D", Desc: "Stop selected agent", Section: "Actions"},
	{Key: "^K", Label: "KillAll", Desc: "Stop all agents", Section: "Actions"},
	{Key: "i", Label: "Steer", Desc: "Send a follow-up message to the selected agent", Section: "Actions", Handler: "steer"},
	{Key: "n", Label: "New", Desc: "Spawn a new agent (form)", Section: "Actions", Handler: "new"},
	{Key: "P", Label: "PR", Desc: "Create GitHub PR from agent branch", Section: "Actions", Handler: "pr"},
	{Key: "S", Desc: "Run 'safe-ag vm start' to recover an unreachable VM", Section: "Actions", Handler: "vmstart"},
	{Key: "^R", Label: "Refresh", Desc: "Force an immediate refresh", Section: "Actions"},

	// Inspect
	{Key: "p", Label: "Preview", Desc: "Toggle preview pane (last output)", Section: "Inspect", Handler: "preview"},
	{Key: "l", Label: "Logs", Desc: "Logs (safe-ag logs)", Section: "Inspect", Handler: "logs"},
	{Key: "d", Label: "Describe", Desc: "Describe container (docker inspect)", Section: "Inspect", Handler: "describe"},
	{Key: "f", Label: "Diff", Desc: "Diff (safe-ag diff)", Section: "Inspect", Handler: "diff"},
	{Key: "x", Label: "Chkpt", Desc: "Checkpoint create", Section: "Inspect", Handler: "checkpoint"},
	{Key: "t", Label: "Todos", Desc: "Todo list", Section: "Inspect", Handler: "todo"},
	{Key: "e", Label: "Export", Desc: "Export sessions", Section: "Inspect", Handler: "export"},
	{Key: "c", Label: "Copy", Desc: "Transfer files VM ↔ agent", Section: "Inspect", Handler: "copy"},
	{Key: "$", Label: "Cost", Desc: "Cost estimate", Section: "Inspect", Handler: "cost"},
	{Key: "A", Label: "Audit", Desc: "Audit log", Section: "Inspect", Handler: "audit"},
	{Key: "R", Label: "Review", Desc: "Code review (safe-ag review)", Section: "Inspect", Handler: "review"},
	{Key: "m", Label: "MCP", Desc: "MCP OAuth login", Section: "Inspect", Handler: "mcp"},

	// Other
	{Key: "?", Label: "Help", Desc: "This help overlay", Section: "Other", Handler: "help"},
	{Key: "q", Label: "Quit", Desc: "Quit", Section: "Other", Handler: "quit"},
	{Key: "^C", Desc: "Quit", Section: "Other"},
	{Key: "Esc", Desc: "Close overlay / reset filter", Section: "Other"},
}

// runeKey returns the single rune a binding dispatches on, or 0 for
// multi-character (special) keys.
func (b keyBinding) runeKey() rune {
	r := []rune(b.Key)
	if len(r) == 1 {
		return r[0]
	}
	return 0
}

// footerShortcutList derives the footer strip entries (those with a Label).
func footerShortcutList() []shortcut {
	out := make([]shortcut, 0, len(keyBindings))
	for _, kb := range keyBindings {
		if kb.Label == "" {
			continue
		}
		out = append(out, shortcut{kb.Key, kb.Label})
	}
	return out
}

// helpText renders the full keybinding reference, grouped by section, for both
// the ? overlay and the --help output.
func helpText() string {
	var b strings.Builder
	b.WriteString("Keybindings")
	for _, section := range keySections {
		b.WriteString("\n\n" + section + "\n")
		for _, kb := range keyBindings {
			if kb.Section == section {
				fmt.Fprintf(&b, "  %-16s %s\n", kb.Key, kb.Desc)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
