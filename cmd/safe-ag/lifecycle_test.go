package main

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"safe-agentic/pkg/orb"
)

// ─── containerState helper ─────────────────────────────────────────────────

func TestContainerState(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantState string
		wantErr   bool
	}{
		{
			name:      "running container",
			response:  "running\n",
			wantState: "running",
		},
		{
			name:      "exited container",
			response:  "exited\n",
			wantState: "exited",
		},
		{
			name:      "created container",
			response:  "created\n",
			wantState: "created",
		},
		{
			name:      "trimmed whitespace",
			response:  "  running  \n",
			wantState: "running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := orb.NewFake()
			exec.SetResponse("docker inspect --format {{.State.Status}}", tt.response)

			state, err := containerState(context.Background(), exec, "agent-claude-test")
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if state != tt.wantState {
				t.Fatalf("containerState() = %q, want %q", state, tt.wantState)
			}
		})
	}
}

func TestContainerStateError(t *testing.T) {
	exec := orb.NewFake()
	exec.SetError("docker inspect", "no such container")

	_, err := containerState(context.Background(), exec, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing container")
	}
}

// ─── splitLines helper ─────────────────────────────────────────────────────

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"a\n\nb\n", []string{"a", "b"}},
		{"  a  \n  b  ", []string{"a", "b"}},
		{"single", []string{"single"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitLines(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitLines()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ─── reconstructSpawnOpts ──────────────────────────────────────────────────

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// buildFakeExec sets up a FakeExecutor that responds to label and env var
// inspections for the given container.
func buildFakeExec(containerName string, labelMap map[string]string, envLines []string) *orb.FakeExecutor {
	exec := orb.NewFake()

	// Labels: docker inspect --format {{index .Config.Labels "key"}} <name>
	for key, val := range labelMap {
		prefix := "docker inspect --format {{index .Config.Labels \"" + key + "\"}} " + containerName
		exec.SetResponse(prefix, val+"\n")
	}

	// Env vars: docker inspect --format {{range .Config.Env}}{{println .}}{{end}} <name>
	envOutput := strings.Join(envLines, "\n") + "\n"
	exec.SetResponse(
		"docker inspect --format {{range .Config.Env}}{{println .}}{{end}} "+containerName,
		envOutput,
	)

	return exec
}

func TestReconstructSpawnOpts_Basic(t *testing.T) {
	containerName := "agent-claude-myrepo"

	labelMap := map[string]string{
		"safe-agentic.agent-type": "claude",
		"safe-agentic.ssh":        "true",
		"safe-agentic.auth":       "shared",
		"safe-agentic.gh-auth":    "",
		"safe-agentic.docker":     "off",
		"safe-agentic.max-cost":   "5.00",
		"safe-agentic.aws":        "",
		// b64-encoded labels
		"safe-agentic.on-complete-b64": "",
		"safe-agentic.on-fail-b64":     "",
		"safe-agentic.notify-b64":      "",
	}

	envLines := []string{
		"REPOS=https://github.com/org/repo.git",
		"SAFE_AGENTIC_PROMPT_B64=" + b64("Fix the tests"),
		"GIT_AUTHOR_NAME=Alice",
		"GIT_AUTHOR_EMAIL=alice@example.com",
	}

	exec := buildFakeExec(containerName, labelMap, envLines)

	opts, err := reconstructSpawnOpts(context.Background(), exec, containerName)
	if err != nil {
		t.Fatalf("reconstructSpawnOpts() error: %v", err)
	}

	if opts.AgentType != "claude" {
		t.Errorf("AgentType = %q, want %q", opts.AgentType, "claude")
	}
	if !opts.SSH {
		t.Error("SSH should be true")
	}
	if !opts.ReuseAuth {
		t.Error("ReuseAuth should be true")
	}
	if opts.EphemeralAuth {
		t.Error("EphemeralAuth should be false")
	}
	if opts.MaxCost != "5.00" {
		t.Errorf("MaxCost = %q, want %q", opts.MaxCost, "5.00")
	}
	if len(opts.Repos) != 1 || opts.Repos[0] != "https://github.com/org/repo.git" {
		t.Errorf("Repos = %v, want single https repo", opts.Repos)
	}
	if opts.Prompt != "Fix the tests" {
		t.Errorf("Prompt = %q, want %q", opts.Prompt, "Fix the tests")
	}
	if opts.Identity != "Alice <alice@example.com>" {
		t.Errorf("Identity = %q, want %q", opts.Identity, "Alice <alice@example.com>")
	}
}

func TestReconstructSpawnOpts_EphemeralAuth(t *testing.T) {
	containerName := "agent-codex-task"

	labelMap := map[string]string{
		"safe-agentic.agent-type":      "codex",
		"safe-agentic.ssh":             "false",
		"safe-agentic.auth":            "ephemeral",
		"safe-agentic.gh-auth":         "",
		"safe-agentic.docker":          "dind",
		"safe-agentic.max-cost":        "",
		"safe-agentic.aws":             "",
		"safe-agentic.on-complete-b64": "",
		"safe-agentic.on-fail-b64":     "",
		"safe-agentic.notify-b64":      "",
	}

	envLines := []string{
		"REPOS=git@github.com:org/private.git",
	}

	exec := buildFakeExec(containerName, labelMap, envLines)

	opts, err := reconstructSpawnOpts(context.Background(), exec, containerName)
	if err != nil {
		t.Fatalf("reconstructSpawnOpts() error: %v", err)
	}

	if opts.AgentType != "codex" {
		t.Errorf("AgentType = %q", opts.AgentType)
	}
	if opts.SSH {
		t.Error("SSH should be false")
	}
	if !opts.EphemeralAuth {
		t.Error("EphemeralAuth should be true")
	}
	if !opts.DockerAccess {
		t.Error("DockerAccess should be true for dind")
	}
	if len(opts.Repos) == 0 || !strings.Contains(opts.Repos[0], "private") {
		t.Errorf("Repos = %v", opts.Repos)
	}
}

func TestReconstructSpawnOpts_Callbacks(t *testing.T) {
	containerName := "agent-claude-callbacks"

	labelMap := map[string]string{
		"safe-agentic.agent-type":      "claude",
		"safe-agentic.ssh":             "false",
		"safe-agentic.auth":            "shared",
		"safe-agentic.gh-auth":         "",
		"safe-agentic.docker":          "off",
		"safe-agentic.max-cost":        "",
		"safe-agentic.aws":             "myprofile",
		"safe-agentic.on-complete-b64": b64("echo done"),
		"safe-agentic.on-fail-b64":     b64("echo fail"),
		"safe-agentic.notify-b64":      b64("slack"),
	}

	envLines := []string{
		"SAFE_AGENTIC_ON_EXIT_B64=" + b64("echo exit"),
		"SAFE_AGENTIC_INSTRUCTIONS_B64=" + b64("Focus on auth"),
	}

	exec := buildFakeExec(containerName, labelMap, envLines)

	opts, err := reconstructSpawnOpts(context.Background(), exec, containerName)
	if err != nil {
		t.Fatalf("reconstructSpawnOpts() error: %v", err)
	}

	if opts.AWSProfile != "myprofile" {
		t.Errorf("AWSProfile = %q, want %q", opts.AWSProfile, "myprofile")
	}
	if opts.OnComplete != "echo done" {
		t.Errorf("OnComplete = %q, want %q", opts.OnComplete, "echo done")
	}
	if opts.OnFail != "echo fail" {
		t.Errorf("OnFail = %q, want %q", opts.OnFail, "echo fail")
	}
	if opts.Notify != "slack" {
		t.Errorf("Notify = %q, want %q", opts.Notify, "slack")
	}
	if opts.OnExit != "echo exit" {
		t.Errorf("OnExit = %q, want %q", opts.OnExit, "echo exit")
	}
	if opts.Instructions != "Focus on auth" {
		t.Errorf("Instructions = %q, want %q", opts.Instructions, "Focus on auth")
	}
}

func TestReconstructSpawnOpts_MissingAgentType(t *testing.T) {
	exec := orb.NewFake()
	// No responses set — all return empty
	_, err := reconstructSpawnOpts(context.Background(), exec, "agent-unknown")
	if err == nil {
		t.Fatal("expected error when agent-type label is missing")
	}
	if !strings.Contains(err.Error(), "missing label") {
		t.Errorf("error should mention missing label, got: %v", err)
	}
}

// ─── retry feedback appending ──────────────────────────────────────────────

func TestRetryFeedbackAppended(t *testing.T) {
	opts := SpawnOpts{
		AgentType: "claude",
		Prompt:    "Fix the tests",
	}
	feedback := "the tests require --verbose flag"
	opts.Prompt = opts.Prompt + "\n\nPrevious attempt failed. Feedback: " + feedback + ". Try a different approach."

	if !strings.Contains(opts.Prompt, "Fix the tests") {
		t.Error("original prompt should be preserved")
	}
	if !strings.Contains(opts.Prompt, "Previous attempt failed") {
		t.Error("feedback prefix should be present")
	}
	if !strings.Contains(opts.Prompt, feedback) {
		t.Error("feedback should be present in prompt")
	}
}

func TestRetryFeedbackNoOriginalPrompt(t *testing.T) {
	feedback := "read the README first"
	prompt := "Previous attempt failed. Feedback: " + feedback + ". Try a different approach."

	if !strings.HasPrefix(prompt, "Previous attempt failed") {
		t.Error("should start with feedback message when no original prompt")
	}
}
