package main

import (
	"reflect"
	"testing"
)

func testAgents() []Agent {
	return []Agent{
		{
			Name:        "agent-beta",
			Type:        "codex",
			Repo:        "org/private-api",
			SSH:         "yes",
			Auth:        "reuse",
			GHAuth:      "app",
			Docker:      "host",
			NetworkMode: "private",
			Status:      "running",
			Running:     true,
			State:       "blocked",
			Activity:    "Working",
			CPU:         "20%",
			Memory:      "512MiB",
			NetIO:       "2MB / 1MB",
			PIDs:        "8",
		},
		{
			Name:        "agent-alpha",
			Type:        "claude",
			Repo:        "org/docs",
			SSH:         "no",
			Auth:        "session",
			GHAuth:      "none",
			Docker:      "sandbox",
			NetworkMode: "bridge",
			Status:      "stopped",
			Running:     false,
			State:       "done",
			Activity:    "Stopped",
			CPU:         "0%",
			Memory:      "0MiB",
			NetIO:       "0B / 0B",
			PIDs:        "0",
		},
		{
			Name:        "agent-gamma",
			Type:        "codex",
			Repo:        "org/frontend",
			SSH:         "yes",
			Auth:        "session",
			GHAuth:      "app",
			Docker:      "sandbox",
			NetworkMode: "bridge",
			Status:      "running",
			Running:     true,
			State:       "working",
			Activity:    "Idle",
			CPU:         "5%",
			Memory:      "256MiB",
			NetIO:       "10MB / 4MB",
			PIDs:        "6",
		},
	}
}

func TestSortAgentsByColumn(t *testing.T) {
	agents := testAgents()

	SortAgents(agents, 0, true)
	if got := []string{agents[0].Name, agents[1].Name, agents[2].Name}; !reflect.DeepEqual(got, []string{"agent-alpha", "agent-beta", "agent-gamma"}) {
		t.Fatalf("ascending sort = %#v", got)
	}

	// ACTIVITY is column 10 (STATE was inserted at 8, shifting STATUS→9,
	// ACTIVITY→10). Descending puts Working before Idle before Stopped.
	SortAgents(agents, 10, false)
	if got := []string{agents[0].Activity, agents[1].Activity, agents[2].Activity}; !reflect.DeepEqual(got, []string{"Working", "Stopped", "Idle"}) {
		t.Fatalf("descending activity sort = %#v", got)
	}

	// STATE (column 8) sorts by urgency rank, not alphabetically:
	// blocked (beta) > done (alpha) > working (gamma).
	SortAgents(agents, stateColumnIndex, true)
	if got := []string{agents[0].Name, agents[1].Name, agents[2].Name}; !reflect.DeepEqual(got, []string{"agent-beta", "agent-alpha", "agent-gamma"}) {
		t.Fatalf("state-rank sort = %#v", got)
	}

	if got := fieldByColumn(agents[0], 99); got != "" {
		t.Fatalf("fieldByColumn invalid col = %q, want empty", got)
	}
}

func TestStateRankOrder(t *testing.T) {
	order := []string{"blocked", "done", "exited", "working", "idle", "unknown"}
	for i := 0; i < len(order)-1; i++ {
		if stateRank(order[i]) <= stateRank(order[i+1]) {
			t.Fatalf("stateRank(%q)=%d should outrank stateRank(%q)=%d",
				order[i], stateRank(order[i]), order[i+1], stateRank(order[i+1]))
		}
	}
	if stateRank("") != 0 || stateRank("bogus") != 0 {
		t.Fatalf("unknown states should rank 0")
	}
}

func TestFilterAgentsCaseInsensitiveAcrossFields(t *testing.T) {
	agents := testAgents()

	if got := FilterAgents(agents, "PRIVATE"); len(got) != 1 || got[0].Name != "agent-beta" {
		t.Fatalf("filter by network = %#v", got)
	}
	if got := FilterAgents(agents, "10mb / 4mb"); len(got) != 1 || got[0].Name != "agent-gamma" {
		t.Fatalf("filter by net io = %#v", got)
	}
	if got := FilterAgents(agents, ""); !reflect.DeepEqual(got, agents) {
		t.Fatalf("empty filter should return original slice contents")
	}
}

func TestVisibleColumnsAndTotalUsed(t *testing.T) {
	// Highest-priority columns that fit width 50: NAME, TYPE, STATE, ACTIVITY.
	if got := VisibleColumns(50); !reflect.DeepEqual(got, []int{0, 1, 8, 10}) {
		t.Fatalf("VisibleColumns(50) = %#v, want %#v", got, []int{0, 1, 8, 10})
	}

	all := VisibleColumns(1000)
	if len(all) != len(columns) {
		t.Fatalf("VisibleColumns(1000) len = %d, want %d", len(all), len(columns))
	}
	for i := range columns {
		if all[i] != i {
			t.Fatalf("VisibleColumns(1000)[%d] = %d, want %d", i, all[i], i)
		}
	}

	if got := totalUsed([]colEntry{{width: 3}, {width: 4}}); got != 9 {
		t.Fatalf("totalUsed() = %d, want 9", got)
	}
}
