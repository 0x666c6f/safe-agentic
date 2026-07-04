package main

import (
	"testing"

	"github.com/0x666c6f/safe-agentic/app/internal/poll"
)

func TestTrayLabel(t *testing.T) {
	agents := []poll.Agent{
		{Name: "a", Running: true, Activity: "Working"},
		{Name: "b", Running: true, Activity: "Idle"},
		{Name: "c", Running: false},
		{Name: "d", Running: true, Activity: "Idle", State: "blocked"},
	}
	needs := map[string]bool{"b": true}
	if got := trayLabel(agents, needs); got != "🟢1 🟡2 ⚪0" {
		t.Fatalf("label: %q", got)
	}
}
