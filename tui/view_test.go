package main

import (
	"strings"
	"testing"
	"time"
)

func TestHeaderLoadingAndUpdate(t *testing.T) {
	h := NewHeader()

	h.ShowLoading()
	loading := h.view.GetText(true)
	if !strings.Contains(loading, "safe-agentic") || !strings.Contains(loading, "Loading...") || !strings.Contains(loading, vmName) {
		t.Fatalf("loading header = %q", loading)
	}

	h.Update(2, 3, time.Time{})
	updated := h.view.GetText(true)
	if !strings.Contains(updated, "agents:") || !strings.Contains(updated, "2") || !strings.Contains(updated, "3") {
		t.Fatalf("updated header = %q", updated)
	}
	if strings.Contains(updated, "VM UNREACHABLE") {
		t.Fatalf("healthy header should not show alert: %q", updated)
	}

	// A non-zero staleSince turns the whole bar into the red VM-unreachable alert.
	h.Update(2, 3, time.Unix(0, 0))
	alert := h.view.GetText(true)
	if !strings.Contains(alert, "VM UNREACHABLE") || !strings.Contains(alert, "vm start") {
		t.Fatalf("stale header = %q", alert)
	}

	if got := colorToTag(colorTitle); got != "#00ffff" {
		t.Fatalf("colorToTag(colorTitle) = %q, want %q", got, "#00ffff")
	}
}

func TestPreviewPaneUpdateToggleAndUnavailable(t *testing.T) {
	p := NewPreviewPane()
	if p.Visible() {
		t.Fatal("new preview should start hidden")
	}
	if p.Lines() != defaultPreviewLines {
		t.Fatalf("Lines() = %d, want %d", p.Lines(), defaultPreviewLines)
	}

	p.Toggle()
	p.Update("agent-beta", "line one\nline two\n\n")

	if !p.Visible() {
		t.Fatal("preview should be visible after toggle")
	}
	if p.AgentName() != "agent-beta" {
		t.Fatalf("AgentName() = %q", p.AgentName())
	}
	if got := p.textView.GetText(false); got != "line one\nline two" {
		t.Fatalf("preview text = %q", got)
	}
	if got := p.textView.GetTitle(); !strings.Contains(got, "agent-beta") {
		t.Fatalf("preview title = %q", got)
	}

	p.SetUnavailable("agent-gamma", "tmux missing")
	if got := p.textView.GetText(false); got != "tmux missing" {
		t.Fatalf("unavailable text = %q", got)
	}

	p.Toggle()
	if p.Visible() {
		t.Fatal("preview should be hidden after second toggle")
	}
	if p.AgentName() != "" || p.textView.GetText(false) != "" || p.textView.GetTitle() != "" {
		t.Fatalf("preview should reset when hidden: name=%q title=%q text=%q", p.AgentName(), p.textView.GetTitle(), p.textView.GetText(false))
	}
}

func TestAgentTableLoadingState(t *testing.T) {
	at := NewAgentTable()
	at.ShowLoading()
	if got := at.table.GetCell(0, 0).Text; !strings.Contains(got, "Connecting to VM") {
		t.Fatalf("loading cell = %q", got)
	}
}

func TestAgentTableSelectionAndSorting(t *testing.T) {
	at := newLoadedAgentTable()
	if at.RunningCount() != 2 {
		t.Fatalf("RunningCount() = %d, want 2", at.RunningCount())
	}
	if at.TotalCount() != 3 {
		t.Fatalf("TotalCount() = %d, want 3", at.TotalCount())
	}
	// Headers carry the number key that sorts them (by visible position) and the
	// sorted column gets an arrow. The default sort is the STATE priority sort
	// (column 8), so the arrow sits on STATE and NAME is unmarked. NAME is the
	// first visible column → "1 NAME". (headerTitle is checked directly for STATE
	// because it may be dropped from the header at other table widths.)
	if got := at.table.GetCell(0, 0).Text; got != "1 NAME" {
		t.Fatalf("default NAME header = %q, want %q", got, "1 NAME")
	}
	if got := at.headerTitle(stateColumnIndex, stateColumnIndex); got != "9 STATE▲" {
		t.Fatalf("default STATE header = %q, want %q", got, "9 STATE▲")
	}
	if sel := at.SelectedAgent(); sel == nil {
		t.Fatalf("default selection = %#v", sel)
	}
	at.SetSort(0) // switch to a NAME column sort (ascending)
	if got := at.table.GetCell(0, 0).Text; got != "1 NAME▲" {
		t.Fatalf("asc header = %q, want %q", got, "1 NAME▲")
	}
	at.SetSort(0) // toggle NAME to descending
	if got := at.table.GetCell(0, 0).Text; got != "1 NAME▼" {
		t.Fatalf("desc header = %q, want %q", got, "1 NAME▼")
	}
	if sel := at.SelectedAgent(); sel == nil {
		t.Fatalf("desc selection = %#v", sel)
	}
}

func TestAgentTableFilteringAndEmptyState(t *testing.T) {
	at := newLoadedAgentTable()
	// Default STATE priority sort orders rows blocked>done>working, i.e.
	// beta, alpha, gamma — so row 1 is agent-beta (blocked).
	at.table.Select(1, 0)
	if sel := at.SelectedAgent(); sel == nil || sel.Name != "agent-beta" {
		t.Fatalf("manual selection = %#v", sel)
	}
	at.SetFilter("private")
	if len(at.agents) != 1 || at.agents[0].Name != "agent-beta" {
		t.Fatalf("filtered agents = %#v", at.agents)
	}
	if sel := at.SelectedAgent(); sel == nil || sel.Name != "agent-beta" {
		t.Fatalf("filtered selection = %#v", sel)
	}
	at.SetFilter("missing")
	if got := at.table.GetCell(1, 0).Text; !strings.Contains(got, "No agents found") {
		t.Fatalf("empty state cell = %q", got)
	}
	if at.SelectedAgent() != nil {
		t.Fatal("SelectedAgent() should be nil for empty state")
	}
}

func TestPadRight(t *testing.T) {
	if got := padRight("abc", 5); got != "abc  " {
		t.Fatalf("padRight() = %q, want %q", got, "abc  ")
	}
	if got := padRight("abcdef", 3); got != "abcdef" {
		t.Fatalf("padRight() with short width = %q", got)
	}
	// Display-width aware: the emoji type icon is 2 cells wide and the space 1,
	// so "🟠 x" occupies 4 cells and needs 2 more to reach width 6.
	if got := padRight("🟠 x", 6); got != "🟠 x  " {
		t.Fatalf("padRight(emoji) = %q, want %q", got, "🟠 x  ")
	}
	// CJK runes are 2 cells each: "中文" is 4 cells wide → 2 spaces to reach 6.
	if got := padRight("中文", 6); got != "中文  " {
		t.Fatalf("padRight(CJK) = %q, want %q", got, "中文  ")
	}
	// Already at or over the target width by display cells → returned unchanged.
	if got := padRight("🟠🟠", 4); got != "🟠🟠" {
		t.Fatalf("padRight(wide, no pad) = %q, want %q", got, "🟠🟠")
	}
}

func newLoadedAgentTable() *AgentTable {
	at := NewAgentTable()
	at.Update(append([]Agent(nil), testAgents()...))
	return at
}
