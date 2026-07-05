package main

import (
	"testing"

	"github.com/0x666c6f/safe-agentic/app/internal/poll"
)

func TestTrayHeaderAndChatLine(t *testing.T) {
	agents := []poll.Agent{
		{Name: "agent-a", Running: true, Activity: "Working"},
		{Name: "agent-b", Running: true, State: "blocked", StateReason: "permission prompt"},
	}
	if got := trayHeader(agents, nil); got != "1 working · 1 need you · 0 idle" {
		t.Fatalf("header: %q", got)
	}
	if got := trayHeader(nil, nil); got != "No active chats" {
		t.Fatalf("empty header: %q", got)
	}
	if got := chatMenuLine(agents[1], nil); got != "🟡 b — permission prompt" {
		t.Fatalf("chat line: %q", got)
	}
	if got := chatMenuLine(agents[0], nil); got != "🟢 a — working" {
		t.Fatalf("chat line: %q", got)
	}
}
