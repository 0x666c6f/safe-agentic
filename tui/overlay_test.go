package main

import "testing"

func TestOverlayHelpers(t *testing.T) {
	a := NewApp()

	ShowCopyForm(a, "agent-beta")
	if name, _ := a.pages.GetFrontPage(); name != "copy" {
		t.Fatalf("front page after copy form = %q", name)
	}

	ShowSpawnForm(a)
	if name, _ := a.pages.GetFrontPage(); name != "spawn" {
		t.Fatalf("front page after spawn form = %q", name)
	}

	cmd := newAgentCmd("spawn", "codex")
	if cmd.Path == "" || len(cmd.Args) != 3 || cmd.Args[1] != "spawn" {
		t.Fatalf("newAgentCmd() = %#v", cmd.Args)
	}

	got := shellQuoteArgs([]string{"plain", "two words", "it's", "$HOME"})
	want := "plain 'two words' 'it'\\''s' '$HOME'"
	if got != want {
		t.Fatalf("shellQuoteArgs() = %q, want %q", got, want)
	}
}
