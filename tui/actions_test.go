package main

import (
	"reflect"
	"testing"
)

func TestBuildAttachArgs(t *testing.T) {
	got := buildAttachArgs("agent-codex-demo")
	want := []string{"orb", "run", "-m", vmName, "docker", "attach", "--sig-proxy=false", "agent-codex-demo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildAttachArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildTmuxAttachArgs(t *testing.T) {
	got := buildTmuxAttachArgs("agent-codex-demo")
	want := []string{"orb", "run", "-m", vmName, "docker", "exec", "-it", "agent-codex-demo", "tmux", "attach", "-t", tmuxSessionName}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildTmuxAttachArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildResumeExecArgsCodex(t *testing.T) {
	got, err := buildResumeExecArgs("codex", "agent-codex-demo", "/workspace/org/repo")
	if err != nil {
		t.Fatalf("buildResumeExecArgs() error = %v", err)
	}
	want := []string{
		"orb", "run", "-m", vmName,
		"docker", "exec", "-it", "agent-codex-demo",
		"bash", "-lc",
		"cd /workspace/org/repo && exec codex --yolo resume --last",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildResumeExecArgs() = %#v, want %#v", got, want)
	}
}

func TestBuildResumeExecArgsDefaultsCWD(t *testing.T) {
	got, err := buildResumeExecArgs("claude", "agent-claude-demo", "")
	if err != nil {
		t.Fatalf("buildResumeExecArgs() error = %v", err)
	}
	script := got[len(got)-1]
	want := "cd /workspace && exec claude --dangerously-skip-permissions --continue"
	if script != want {
		t.Fatalf("resume script = %q, want %q", script, want)
	}
}

func TestParseSessionMeta(t *testing.T) {
	data := []byte(`{"timestamp":"2026-04-09T07:32:05.051Z","type":"session_meta","payload":{"cwd":"/workspace/myorg/myrepo","cli_version":"0.118.0","originator":"codex-tui"}}` + "\n")
	got, err := parseSessionMeta(data)
	if err != nil {
		t.Fatalf("parseSessionMeta() error = %v", err)
	}
	if got.CWD != "/workspace/myorg/myrepo" {
		t.Fatalf("parseSessionMeta().CWD = %q", got.CWD)
	}
	if got.CLIVersion != "0.118.0" {
		t.Fatalf("parseSessionMeta().CLIVersion = %q", got.CLIVersion)
	}
}
