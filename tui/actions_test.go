package main

import (
	"reflect"
	"strings"
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

func TestSessionSearchDirsPrefersProjectThenSessions(t *testing.T) {
	got := sessionSearchDirs("/home/agent/.codex", "morpho-org/hermes-agent-sre")
	want := []string{
		"/home/agent/.codex/projects/-workspace-morpho-org-hermes-agent-sre",
		"/home/agent/.codex/sessions",
		"/home/agent/.codex",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sessionSearchDirs() = %#v, want %#v", got, want)
	}
}

func TestSessionSearchDirsWithoutRepo(t *testing.T) {
	got := sessionSearchDirs("/home/agent/.claude", "-")
	want := []string{
		"/home/agent/.claude/sessions",
		"/home/agent/.claude",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sessionSearchDirs() = %#v, want %#v", got, want)
	}
}

func TestMcpLoginArgs(t *testing.T) {
	got := mcpLoginArgs("linear", "agent-claude-demo")
	want := []string{"mcp-login", "linear", "agent-claude-demo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mcpLoginArgs() = %#v, want %#v", got, want)
	}
}

func TestSSHLabelAllowsPush(t *testing.T) {
	cases := map[string]bool{
		"yes":   true,
		"true":  true,
		" no ":  false,
		"false": false,
		"":      false,
	}
	for input, want := range cases {
		if got := sshLabelAllowsPush(input); got != want {
			t.Fatalf("sshLabelAllowsPush(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestRenderSessionLogCurrentCodexFormat(t *testing.T) {
	data := []byte(
		`{"timestamp":"2026-04-10T07:46:06.385Z","type":"session_meta","payload":{"timestamp":"2026-04-10T07:46:01.762Z","cwd":"/workspace/morpho-org/hermes-agent-sre","cli_version":"0.118.0","originator":"codex-tui"}}` + "\n" +
			`{"timestamp":"2026-04-10T07:46:14.627Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Using github skills first."}]}}` + "\n" +
			`{"timestamp":"2026-04-10T07:46:22.648Z","type":"event_msg","payload":{"type":"agent_message","message":"Checking PR context now.","phase":"commentary"}}` + "\n",
	)

	rendered := renderSessionLog(data)
	if !strings.Contains(rendered, "Using github skills first.") {
		t.Fatalf("rendered log missing assistant response_item text:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Checking PR context now.") {
		t.Fatalf("rendered log missing event_msg agent text:\n%s", rendered)
	}
}
