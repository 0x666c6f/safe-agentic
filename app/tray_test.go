package main

import (
	"bytes"
	"image/png"
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
	if got, dot := chatMenuLine(agents[1], nil); got != "b — permission prompt" || !bytes.Equal(dot, dotNeeds) {
		t.Fatalf("chat line: %q (needs dot: %v)", got, bytes.Equal(dot, dotNeeds))
	}
	if got, dot := chatMenuLine(agents[0], nil); got != "a — working" || !bytes.Equal(dot, dotWorking) {
		t.Fatalf("chat line: %q (working dot: %v)", got, bytes.Equal(dot, dotWorking))
	}
}

func TestStatusDotsAreValidPNGs(t *testing.T) {
	for name, dot := range map[string][]byte{"idle": dotIdle, "needs": dotNeeds, "working": dotWorking} {
		img, err := png.Decode(bytes.NewReader(dot))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if b := img.Bounds(); b.Dx() != 14 || b.Dy() != 14 {
			t.Fatalf("%s: bounds %v", name, b)
		}
	}
}
