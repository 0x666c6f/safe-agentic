package main

import (
	"strings"
	"testing"
)

func TestHeaderLoadingAndUpdate(t *testing.T) {
	h := NewHeader()

	h.ShowLoading()
	loading := h.view.GetText(true)
	if !strings.Contains(loading, "safe-agentic") || !strings.Contains(loading, "Loading...") || !strings.Contains(loading, vmName) {
		t.Fatalf("loading header = %q", loading)
	}

	h.Update(2, 3, true)
	updated := h.view.GetText(true)
	if !strings.Contains(updated, "agents:") || !strings.Contains(updated, "2") || !strings.Contains(updated, "3") || !strings.Contains(updated, "STALE") {
		t.Fatalf("updated header = %q", updated)
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

func TestAgentTableLoadingSelectionSortAndFilter(t *testing.T) {
	at := NewAgentTable()

	at.ShowLoading()
	if got := at.table.GetCell(0, 0).Text; !strings.Contains(got, "Connecting to VM") {
		t.Fatalf("loading cell = %q", got)
	}

	agents := testAgents()
	at.Update(append([]Agent(nil), agents...))

	if at.RunningCount() != 2 {
		t.Fatalf("RunningCount() = %d, want 2", at.RunningCount())
	}
	if at.TotalCount() != 3 {
		t.Fatalf("TotalCount() = %d, want 3", at.TotalCount())
	}
	if got := at.table.GetCell(0, 0).Text; got != "NAME▲" {
		t.Fatalf("default header = %q, want %q", got, "NAME▲")
	}
	if sel := at.SelectedAgent(); sel == nil {
		t.Fatalf("default selection = %#v", sel)
	}

	at.SetSort(0)
	if got := at.table.GetCell(0, 0).Text; got != "NAME▼" {
		t.Fatalf("desc header = %q, want %q", got, "NAME▼")
	}
	if sel := at.SelectedAgent(); sel == nil {
		t.Fatalf("desc selection = %#v", sel)
	}

	at.table.Select(2, 0)
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

	if got := padRight("abc", 5); got != "abc  " {
		t.Fatalf("padRight() = %q, want %q", got, "abc  ")
	}
	if got := padRight("abcdef", 3); got != "abcdef" {
		t.Fatalf("padRight() with short width = %q", got)
	}
}
