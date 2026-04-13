package main

import (
	"testing"
)

func TestResumeHelpers(t *testing.T) {
	if !resumeSupported("codex") || !resumeSupported("claude") || resumeSupported("shell") {
		t.Fatal("resumeSupported() returned unexpected values")
	}

	got, err := resumeCLIArgs("codex")
	if err != nil {
		t.Fatalf("resumeCLIArgs(codex) error = %v", err)
	}
	if len(got) != 4 || got[0] != "codex" || got[2] != "resume" {
		t.Fatalf("resumeCLIArgs(codex) = %#v", got)
	}

	got, err = resumeCLIArgs("claude")
	if err != nil {
		t.Fatalf("resumeCLIArgs(claude) error = %v", err)
	}
	if len(got) != 3 || got[0] != "claude" || got[2] != "--continue" {
		t.Fatalf("resumeCLIArgs(claude) = %#v", got)
	}

	if _, err := resumeCLIArgs("shell"); err == nil {
		t.Fatal("resumeCLIArgs(shell) should fail")
	}
}

func TestParseSessionMetaErrorsAndLogsTitle(t *testing.T) {
	if _, err := parseSessionMeta([]byte("not-json\n")); err == nil {
		t.Fatal("parseSessionMeta should fail when no metadata exists")
	}

	data := []byte(`{"timestamp":"2026-04-09T07:32:05.051Z","type":"session_meta","payload":"oops"}` + "\n")
	if _, err := parseSessionMeta(data); err == nil {
		t.Fatal("parseSessionMeta should fail on invalid payload")
	}

	ac := NewActions(NewApp())
	title := ac.logsTitle("agent-beta", &logsState{autoRefresh: true, tailLines: "0"})
	if title != "Logs: agent-beta | [r]efresh:3s [5]00/[2]000/[a]ll:all | Esc close" {
		t.Fatalf("logsTitle() = %q", title)
	}
}

func TestWaitForTmuxSession(t *testing.T) {
	installFakeOrb(t)

	if !waitForTmuxSession("agent-beta", 1) {
		t.Fatal("waitForTmuxSession(agent-beta) = false, want true")
	}
	if waitForTmuxSession("agent-alpha", 1) {
		t.Fatal("waitForTmuxSession(agent-alpha) = true, want false")
	}
}

func TestSelectedOrWarn(t *testing.T) {
	a := NewApp()
	ac := NewActions(a)

	if got := ac.selectedOrWarn(); got != nil {
		t.Fatalf("selectedOrWarn() = %#v, want nil", got)
	}
	if a.footer.Mode() != FooterModeStatus {
		t.Fatalf("footer mode = %v, want status", a.footer.Mode())
	}
}
