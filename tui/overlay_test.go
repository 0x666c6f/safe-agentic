package main

import (
	"strings"
	"testing"
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
