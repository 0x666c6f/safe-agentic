package main

import "testing"

func TestAgentTableDeletingOverlayPersistsAcrossPolls(t *testing.T) {
	table := NewAgentTable()
	agent := Agent{
		Name:     "agent-codex-demo",
		Type:     "codex",
		Status:   "Up 2 minutes",
		Activity: "Working",
		Running:  true,
	}

	table.Update([]Agent{agent})
	table.MarkDeleting(agent)

	if !table.HasDeleting() {
		t.Fatalf("expected deleting overlay to be active")
	}
	if len(table.agents) != 1 {
		t.Fatalf("agents len = %d, want 1", len(table.agents))
	}
	if got := fieldByColumn(table.agents[0], 8); got != "Deleting" {
		t.Fatalf("status field = %q, want %q", got, "Deleting")
	}
	if got := fieldByColumn(table.agents[0], 9); got != spinnerFrames[0] {
		t.Fatalf("activity field = %q, want %q", got, spinnerFrames[0])
	}

	table.Update(nil)
	if len(table.allAgents) != 1 {
		t.Fatalf("allAgents len = %d, want 1", len(table.allAgents))
	}
	if !table.allAgents[0].Deleting {
		t.Fatalf("expected deleting agent to persist after poll update")
	}

	table.SetDeletingFrame(spinnerFrames[1])
	if got := fieldByColumn(table.agents[0], 9); got != spinnerFrames[1] {
		t.Fatalf("activity field after tick = %q, want %q", got, spinnerFrames[1])
	}

	table.FinishDeleting(agent.Name)
	if table.HasDeleting() {
		t.Fatalf("expected deleting overlay to clear after finish")
	}
	if len(table.allAgents) != 0 {
		t.Fatalf("allAgents len after finish = %d, want 0", len(table.allAgents))
	}
}

func TestFieldByColumnPrefersDeletingState(t *testing.T) {
	agent := Agent{
		Name:     "agent-claude-demo",
		Status:   "Up 1 minute",
		Activity: "Working",
		Deleting: true,
		Progress: "⠙",
	}

	if got := fieldByColumn(agent, 8); got != "Deleting" {
		t.Fatalf("status field = %q, want %q", got, "Deleting")
	}
	if got := fieldByColumn(agent, 9); got != "⠙" {
		t.Fatalf("activity field = %q, want %q", got, "⠙")
	}
}

func TestBuildTableRowsNestedHierarchy(t *testing.T) {
	agents := []Agent{
		{Name: "root-leaf", Type: "codex", Fleet: "display-nested", Hierarchy: "display-nested"},
		{Name: "child-leaf", Type: "codex", Fleet: "display-nested", Hierarchy: "display-nested/display-nested-child"},
		{Name: "grandchild-leaf", Type: "codex", Fleet: "display-nested", Hierarchy: "display-nested/display-nested-child/display-nested-grandchild"},
	}

	rows := buildTableRows(agents)
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.isGroup {
			got = append(got, row.prefix+"🔄 "+row.groupName)
			continue
		}
		got = append(got, row.prefix+agents[row.agentIndex].Name)
	}

	want := []string{
		" 🔄 display-nested",
		" ├── root-leaf",
		" └── 🔄 display-nested-child",
		"     ├── child-leaf",
		"     └── 🔄 display-nested-grandchild",
		"         └── grandchild-leaf",
	}

	if len(got) != len(want) {
		t.Fatalf("rows len = %d, want %d\nrows=%q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %q, want %q\nrows=%q", i, got[i], want[i], got)
		}
	}
}
