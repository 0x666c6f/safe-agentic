package main

import (
	"strings"
	"testing"

	"github.com/0x666c6f/berth/pkg/risk"
)

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

func TestBuildSpawnFormArgs_SafeDefaultsDoNotAddRiskFlags(t *testing.T) {
	args := buildSpawnFormArgs(spawnFormSpec{
		agentType: "claude",
		repoURL:   "https://github.com/org/repo.git",
	})

	for _, forbidden := range []string{"--ssh", "--reuse-auth", "--reuse-gh-auth", "--seed-auth", "--docker", "--docker-socket"} {
		if hasArg(args, forbidden) {
			t.Fatalf("unexpected risk flag %s in args: %v", forbidden, args)
		}
	}
}

func TestBuildSpawnFormArgs_SSHRepoEnablesSSH(t *testing.T) {
	args := buildSpawnFormArgs(spawnFormSpec{
		agentType: "claude",
		repoURL:   "git@github.com:org/repo.git",
	})
	if !hasArg(args, "--ssh") {
		t.Fatalf("expected --ssh for SSH repo URL, got %v", args)
	}
}

func TestBuildSpawnFormArgs_OverridesRiskyDefaults(t *testing.T) {
	args := buildSpawnFormArgs(spawnFormSpec{
		agentType: "codex",
		repoURL:   "https://github.com/org/repo.git",
		defaults: spawnFormDefaults{
			ssh:          true,
			reuseAuth:    true,
			reuseGHAuth:  true,
			seedAuth:     true,
			docker:       true,
			dockerSocket: true,
		},
	})

	for _, want := range []string{"--no-ssh", "--no-reuse-auth", "--no-reuse-gh-auth", "--no-seed-auth", "--no-docker", "--no-docker-socket"} {
		if !hasArg(args, want) {
			t.Fatalf("expected %s in args: %v", want, args)
		}
	}
}

func TestBuildSpawnFormArgs_ExplicitSeedAuth(t *testing.T) {
	args := buildSpawnFormArgs(spawnFormSpec{
		agentType: "claude",
		seedAuth:  true,
	})
	if !hasArg(args, "--seed-auth") {
		t.Fatalf("expected --seed-auth in args: %v", args)
	}
}

func TestBuildSpawnFormArgs_ExplicitDockerSocket(t *testing.T) {
	args := buildSpawnFormArgs(spawnFormSpec{
		agentType:    "shell",
		dockerSocket: true,
	})
	if !hasArg(args, "--docker-socket") {
		t.Fatalf("expected --docker-socket in args: %v", args)
	}
}

func TestSpawnFormRiskNotices(t *testing.T) {
	notices := spawnFormRiskNotices(spawnFormSpec{
		agentType:    "claude",
		repoURL:      "git@github.com:org/repo.git",
		awsProfile:   "dev",
		dockerSocket: true,
	})
	got := riskFlags(notices)
	for _, want := range []string{"--ssh", "--aws dev", "--docker-socket"} {
		if !containsString(got, want) {
			t.Fatalf("risk flags = %v, missing %s", got, want)
		}
	}
}

func TestSpawnRiskConfirmMessageTruncates(t *testing.T) {
	msg := spawnRiskConfirmMessage(spawnFormRiskNotices(spawnFormSpec{
		agentType:    "shell",
		repoURL:      "git@github.com:org/repo.git",
		reuseAuth:    true,
		reuseGHAuth:  true,
		seedAuth:     true,
		awsProfile:   "dev",
		dockerSocket: true,
	}))
	if !strings.Contains(msg, "Spawn widens sandbox") || !strings.Contains(msg, "+3 more") {
		t.Fatalf("confirm message = %q", msg)
	}
}

func TestCleanAgentCopyPathRestrictsWorkspace(t *testing.T) {
	got, err := cleanAgentCopyPath(" /workspace/../workspace/src/./app.go ")
	if err != nil {
		t.Fatalf("cleanAgentCopyPath() error = %v", err)
	}
	if got != "/workspace/src/app.go" {
		t.Fatalf("cleanAgentCopyPath() = %q", got)
	}

	for _, value := range []string{
		"",
		"workspace/file.txt",
		"/home/agent/.codex/auth.json",
		"/workspace/../../home/agent/.ssh/id_ed25519",
		"/workspace/file:with-colon",
		"/workspace/\x00secret",
	} {
		if _, err := cleanAgentCopyPath(value); err == nil {
			t.Fatalf("cleanAgentCopyPath(%q) expected error", value)
		}
	}
}

func TestCleanVMCopyPathRequiresAbsoluteLocalPath(t *testing.T) {
	got, err := cleanVMCopyPath(" /tmp/out/../session.txt ", "VM source")
	if err != nil {
		t.Fatalf("cleanVMCopyPath() error = %v", err)
	}
	if got != "/tmp/session.txt" {
		t.Fatalf("cleanVMCopyPath() = %q", got)
	}

	for _, value := range []string{
		"",
		"./session.txt",
		"relative/session.txt",
		"other-container:/tmp/session.txt",
		"/tmp/file:with-colon",
		"/tmp/\x00session.txt",
	} {
		if _, err := cleanVMCopyPath(value, "VM source"); err == nil {
			t.Fatalf("cleanVMCopyPath(%q) expected error", value)
		}
	}
}

func TestExecuteSpawnFormRejectsDockerModeConflict(t *testing.T) {
	_, err := executeSpawnForm(spawnFormSpec{
		agentType:    "claude",
		docker:       true,
		dockerSocket: true,
	})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected docker conflict error, got %v", err)
	}
}

func TestSpawnedContainerNameParsesCLIOutput(t *testing.T) {
	got := spawnedContainerName("Agent claude started: agent-claude-repo-20260410-120000\n")
	if got != "agent-claude-repo-20260410-120000" {
		t.Fatalf("spawnedContainerName() = %q", got)
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func riskFlags(notices []risk.Notice) []string {
	var flags []string
	for _, notice := range notices {
		flags = append(flags, notice.Flag)
	}
	return flags
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
