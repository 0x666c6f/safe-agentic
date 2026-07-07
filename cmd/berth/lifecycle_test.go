package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0x666c6f/berth/pkg/vmexec"
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
			exec := vmexec.NewFake()
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
	exec := vmexec.NewFake()
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
func buildFakeExec(containerName string, labelMap map[string]string, envLines []string) *vmexec.FakeExecutor {
	exec := vmexec.NewFake()

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
		"berth.agent-type": "claude",
		"berth.ssh":        "true",
		"berth.auth":       "shared",
		"berth.gh-auth":    "",
		"berth.seed-auth":  "true",
		"berth.docker":     "off",
		"berth.max-cost":   "5.00",
		"berth.aws":        "",
		// b64-encoded labels
		"berth.on-complete-b64": "",
		"berth.on-fail-b64":     "",
		"berth.notify-b64":      "",
	}

	envLines := []string{
		"REPOS=https://github.com/org/repo.git",
		"BERTH_PROMPT_B64=" + b64("Fix the tests"),
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
	if opts.NoSSH {
		t.Error("NoSSH should be false when original had SSH")
	}
	if !opts.ReuseAuth {
		t.Error("ReuseAuth should be true")
	}
	if opts.EphemeralAuth {
		t.Error("EphemeralAuth should be false")
	}
	if !opts.SeedAuth {
		t.Error("SeedAuth should be true")
	}
	if opts.NoSeedAuth {
		t.Error("NoSeedAuth should be false when original had seed auth")
	}
	if !opts.NoReuseGHAuth {
		t.Error("NoReuseGHAuth should be true when original lacked GH auth reuse")
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
		"berth.agent-type":      "codex",
		"berth.ssh":             "false",
		"berth.auth":            "ephemeral",
		"berth.gh-auth":         "",
		"berth.docker":          "dind",
		"berth.max-cost":        "",
		"berth.aws":             "",
		"berth.on-complete-b64": "",
		"berth.on-fail-b64":     "",
		"berth.notify-b64":      "",
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
	if !opts.NoSSH {
		t.Error("NoSSH should be true when original lacked SSH")
	}
	if !opts.EphemeralAuth {
		t.Error("EphemeralAuth should be true")
	}
	if !opts.NoReuseAuth {
		t.Error("NoReuseAuth should be true when original used ephemeral auth")
	}
	if !opts.DockerAccess {
		t.Error("DockerAccess should be true for dind")
	}
	if !opts.NoDockerSocket {
		t.Error("NoDockerSocket should be true for dind retry")
	}
	if len(opts.Repos) == 0 || !strings.Contains(opts.Repos[0], "private") {
		t.Errorf("Repos = %v", opts.Repos)
	}
}

func TestReconstructSpawnOpts_Callbacks(t *testing.T) {
	containerName := "agent-claude-callbacks"

	labelMap := map[string]string{
		"berth.agent-type":      "claude",
		"berth.ssh":             "false",
		"berth.auth":            "shared",
		"berth.gh-auth":         "",
		"berth.docker":          "off",
		"berth.max-cost":        "",
		"berth.aws":             "myprofile",
		"berth.on-complete-b64": b64("echo done"),
		"berth.on-fail-b64":     b64("echo fail"),
		"berth.notify-b64":      b64("slack"),
	}

	envLines := []string{
		"BERTH_ON_EXIT_B64=" + b64("echo exit"),
		"BERTH_INSTRUCTIONS_B64=" + b64("Focus on auth"),
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
	exec := vmexec.NewFake()
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

// ─── session resume ────────────────────────────────────────────────────────

func cmdsContain(cmds [][]string, want string) bool {
	for _, c := range cmds {
		if strings.Join(c, " ") == want || strings.Contains(strings.Join(c, " "), want) {
			return true
		}
	}
	return false
}

func TestAgentSupportsResume(t *testing.T) {
	for _, tt := range []struct {
		agent string
		want  bool
	}{
		{"claude", true},
		{"codex", true},
		{"shell", false},
		{"", false},
	} {
		if got := agentSupportsResume(tt.agent); got != tt.want {
			t.Errorf("agentSupportsResume(%q) = %v, want %v", tt.agent, got, tt.want)
		}
	}
}

func TestAuthVolumePersists(t *testing.T) {
	for _, tt := range []struct {
		authType string
		want     bool
	}{
		{"shared", true},
		{"fleet-isolated", true},
		{"ephemeral", false},
		{"", false},
	} {
		if got := authVolumePersists(tt.authType); got != tt.want {
			t.Errorf("authVolumePersists(%q) = %v, want %v", tt.authType, got, tt.want)
		}
	}
}

// A running container with a live tmux session is already mid-conversation:
// resume just attaches, without restarting or relaunching.
func TestResumeAttach_RunningLiveSession_AttachesOnly(t *testing.T) {
	name := "agent-claude-live"
	exec := buildFakeExec(name, map[string]string{"berth.agent-type": "claude"}, nil)

	if err := resumeAttach(context.Background(), exec, name, "running", true); err != nil {
		t.Fatalf("resumeAttach() error: %v", err)
	}
	if cmdsContain(exec.Log, "docker start") {
		t.Error("should not restart a running container")
	}
	if cmdsContain(exec.Log, "new-session") {
		t.Error("should not relaunch when a live session exists")
	}
	if !cmdsContain(exec.CommandsMatching("tmux attach"), "tmux attach") {
		t.Error("should attach to the live session")
	}
}

// Regression (P2): a RUNNING container with no live tmux session must NOT be
// relaunched — a --background/headless agent may still be alive, so relaunching
// would start a second agent against the same workspace and auth volume. Resume
// must refuse instead, and must not `docker start` or `tmux new-session`.
func TestResumeAttach_RunningNoSession_RefusesRelaunch(t *testing.T) {
	name := "agent-claude-headless"
	exec := buildFakeExec(name, map[string]string{"berth.agent-type": "claude"}, nil)
	// HasSession returns false when `tmux has-session` errors.
	exec.SetError("docker exec "+name+" tmux has-session", "no server running")

	err := resumeAttach(context.Background(), exec, name, "running", true)
	if err == nil || !strings.Contains(err.Error(), "no attachable tmux session") {
		t.Fatalf("expected refusal error, got: %v", err)
	}
	if cmdsContain(exec.Log, "new-session") {
		t.Error("must NOT relaunch a second agent on a running headless container")
	}
	if cmdsContain(exec.Log, "docker start") {
		t.Error("must not restart a running container")
	}
	if cmdsContain(exec.CommandsMatching("tmux attach"), "tmux attach") {
		t.Error("must not attach when refusing")
	}
}

// A stopped container with a persistent auth volume is restarted; the entrypoint
// auto-resumes from its session-state file, then we attach.
func TestResumeAttach_StoppedSharedAuth_StartsThenAttaches(t *testing.T) {
	name := "agent-claude-stopped"
	exec := buildFakeExec(name, map[string]string{
		"berth.agent-type": "claude",
		"berth.auth":       "shared",
	}, nil)

	if err := resumeAttach(context.Background(), exec, name, "exited", true); err != nil {
		t.Fatalf("resumeAttach() error: %v", err)
	}
	if !cmdsContain(exec.CommandsMatching("docker start"), "docker start "+name) {
		t.Errorf("should restart the stopped container; log: %v", exec.Log)
	}
	if !cmdsContain(exec.CommandsMatching("tmux attach"), "tmux attach") {
		t.Error("should attach after restart")
	}
}

// Regression (P2 round 2): attach --resume on a STOPPED ephemeral-auth container
// must refuse. Its tmpfs transcript is gone, and restarting can't start fresh —
// the entrypoint's persisted session-state marker would auto-continue against an
// empty auth dir and error. Refuse without `docker start`.
func TestResumeAttach_EphemeralStopped_RefusesRestart(t *testing.T) {
	name := "agent-claude-eph-stopped"
	exec := buildFakeExec(name, map[string]string{
		"berth.agent-type": "claude",
		"berth.auth":       "ephemeral",
	}, nil)

	err := resumeAttach(context.Background(), exec, name, "exited", true)
	if err == nil || !strings.Contains(err.Error(), "ephemeral") || !strings.Contains(err.Error(), "cannot recover") {
		t.Fatalf("expected ephemeral-auth refusal, got: %v", err)
	}
	if cmdsContain(exec.Log, "docker start") {
		t.Error("must NOT restart: it would auto-continue against an empty auth dir")
	}
	if cmdsContain(exec.CommandsMatching("tmux attach"), "tmux attach") {
		t.Error("must not attach when refusing")
	}
}

func TestResumeAttach_ShellAgent_Errors(t *testing.T) {
	name := "agent-shell-x"
	exec := buildFakeExec(name, map[string]string{"berth.agent-type": "shell"}, nil)

	err := resumeAttach(context.Background(), exec, name, "running", true)
	if err == nil || !strings.Contains(err.Error(), "claude and codex") {
		t.Fatalf("expected unsupported-agent error, got: %v", err)
	}
}

func TestResumeAttach_NonTmux_Errors(t *testing.T) {
	name := "agent-claude-plain"
	exec := buildFakeExec(name, map[string]string{"berth.agent-type": "claude"}, nil)

	err := resumeAttach(context.Background(), exec, name, "running", false)
	if err == nil || !strings.Contains(err.Error(), "tmux") {
		t.Fatalf("expected tmux-required error, got: %v", err)
	}
}

// Ephemeral-auth transcripts do not survive a stop, so resuming a stopped
// ephemeral container must fail with an actionable error rather than silently
// starting fresh.
func TestResumeRetry_EphemeralStopped_Errors(t *testing.T) {
	name := "agent-codex-eph"
	exec := buildFakeExec(name, map[string]string{
		"berth.agent-type": "codex",
		"berth.terminal":   "tmux",
		"berth.auth":       "ephemeral",
	}, nil)
	exec.SetResponse("docker inspect --format {{.State.Running}} "+name, "false\n")

	err := resumeRetry(context.Background(), exec, name, "")
	if err == nil || !strings.Contains(err.Error(), "ephemeral") || !strings.Contains(err.Error(), "cannot recover") {
		t.Fatalf("expected ephemeral-auth error, got: %v", err)
	}
	if cmdsContain(exec.Log, "docker start") {
		t.Error("must not restart when the transcript is unrecoverable")
	}
}

// A stopped container with a persistent volume is restarted in resume mode; the
// original prompt is NOT re-injected (agent-session.sh resumes) and feedback is
// delivered through the tmux input path, never a fresh `docker run`.
func TestResumeRetry_SharedStopped_StartsAndSendsFeedback(t *testing.T) {
	name := "agent-claude-retry"
	exec := buildFakeExec(name, map[string]string{
		"berth.agent-type": "claude",
		"berth.terminal":   "tmux",
		"berth.auth":       "shared",
	}, nil)
	exec.SetResponse("docker inspect --format {{.State.Running}} "+name, "false\n")

	feedback := "focus on the failing auth test"
	if err := resumeRetry(context.Background(), exec, name, feedback); err != nil {
		t.Fatalf("resumeRetry() error: %v", err)
	}
	if !cmdsContain(exec.CommandsMatching("docker start"), "docker start "+name) {
		t.Errorf("should restart the source container; log: %v", exec.Log)
	}
	if cmdsContain(exec.Log, "docker run") {
		t.Error("resume must reuse the source container, not spawn a fresh one")
	}
	sendKeys := exec.CommandsMatching("send-keys")
	if len(sendKeys) == 0 || !cmdsContain(sendKeys, feedback) {
		t.Errorf("feedback should be delivered via tmux send-keys; got: %v", sendKeys)
	}
}

func TestResumeRetry_ShellAgent_Errors(t *testing.T) {
	name := "agent-shell-retry"
	exec := buildFakeExec(name, map[string]string{"berth.agent-type": "shell"}, nil)

	err := resumeRetry(context.Background(), exec, name, "")
	if err == nil || !strings.Contains(err.Error(), "claude and codex") {
		t.Fatalf("expected unsupported-agent error, got: %v", err)
	}
}

// Regression (P2): retry --resume on a RUNNING container with no live session
// must refuse rather than relaunch, so it can't spawn a second headless agent.
func TestResumeRetry_RunningNoSession_RefusesRelaunch(t *testing.T) {
	name := "agent-codex-headless"
	exec := buildFakeExec(name, map[string]string{
		"berth.agent-type": "codex",
		"berth.terminal":   "tmux",
		"berth.auth":       "shared",
	}, nil)
	exec.SetResponse("docker inspect --format {{.State.Running}} "+name, "true\n")
	exec.SetError("docker exec "+name+" tmux has-session", "no server running")

	err := resumeRetry(context.Background(), exec, name, "fix the flake")
	if err == nil || !strings.Contains(err.Error(), "headless") {
		t.Fatalf("expected refusal error, got: %v", err)
	}
	if cmdsContain(exec.Log, "new-session") {
		t.Error("must NOT relaunch a second agent on a running headless container")
	}
	if cmdsContain(exec.Log, "send-keys") {
		t.Error("must not steer feedback when refusing")
	}
	if cmdsContain(exec.Log, "docker start") {
		t.Error("must not restart an already-running container")
	}
}

func TestPushClaudeConfigCommandShape(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"model":"opus"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	fake := vmexec.NewFake()
	pushClaudeConfig(context.Background(), fake, "agent-x", true)
	cmds := fake.CommandsMatching("docker exec agent-x bash -c")
	if len(cmds) != 1 {
		t.Fatalf("want 1 exec, got %d", len(cmds))
	}
	joined := strings.Join(cmds[0], " ")
	want := base64.StdEncoding.EncodeToString([]byte(`{"model":"opus"}`))
	if !strings.Contains(joined, want) || !strings.Contains(joined, "settings.host.json") {
		t.Fatalf("bad push cmd: %s", joined)
	}
}
