package svc

import (
	"context"
	"strings"
	"testing"

	"github.com/0x666c6f/safe-agentic/app/internal/cli"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
)

func recorderRunner() (*cli.Runner, *[][]string) {
	var calls [][]string
	r := &cli.Runner{Bin: "safe-ag",
		Exec: func(context.Context, string, ...string) ([]byte, []byte, error) {
			return []byte("{}"), nil, nil
		},
		OnCommand: func(argv []string) { calls = append(calls, argv) }}
	return r, &calls
}

func TestSteerBuildsArgs(t *testing.T) {
	r, calls := recorderRunner()
	s := &AgentService{Runner: r}
	if err := s.Steer("agent-x", "focus on tests"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join((*calls)[0], " ")
	if got != "safe-ag steer agent-x focus on tests" {
		t.Fatalf("argv: %q", got)
	}
}

func TestRetryWithFeedback(t *testing.T) {
	r, calls := recorderRunner()
	s := &AgentService{Runner: r}
	if err := s.Retry("agent-x", "fix lint"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join((*calls)[0], " ")
	if got != "safe-ag retry agent-x --feedback fix lint" {
		t.Fatalf("argv: %q", got)
	}
}

func TestPipelineRunRepoUsesRepoFlag(t *testing.T) {
	r, calls := recorderRunner()
	s := &AgentService{Runner: r}
	if _, err := s.PipelineRun("claude-pr-review",
		map[string]string{"repo": "https://github.com/org/repo.git", "pr": "223"}, false); err != nil {
		t.Fatal(err)
	}
	got := strings.Join((*calls)[0], " ")
	if strings.Contains(got, "--var repo=") {
		t.Fatalf("repo is reserved and must not be a --var: %q", got)
	}
	if !strings.Contains(got, "--repo https://github.com/org/repo.git") {
		t.Fatalf("repo must be passed via --repo: %q", got)
	}
	if !strings.Contains(got, "--var pr=223") || !strings.Contains(got, "--background") {
		t.Fatalf("expected pr var + --background: %q", got)
	}
}

func TestPipelineRunSkipsEmptyVars(t *testing.T) {
	r, calls := recorderRunner()
	s := &AgentService{Runner: r}
	if _, err := s.PipelineRun("p", map[string]string{"repo": "", "pr": ""}, true); err != nil {
		t.Fatal(err)
	}
	got := strings.Join((*calls)[0], " ")
	if strings.Contains(got, "--repo") || strings.Contains(got, "--var") {
		t.Fatalf("empty vars should be skipped: %q", got)
	}
	if !strings.Contains(got, "--dry-run") {
		t.Fatalf("expected --dry-run: %q", got)
	}
}

func TestSpawnArgsNoRepo(t *testing.T) {
	req := SpawnRequest{Agent: "shell", DryRun: false}
	got := strings.Join(spawnArgs(req), " ")
	if got != "spawn shell --seed-auth --reuse-gh-auth --auto-trust --background" {
		t.Fatalf("argv: %q", got)
	}
}

func TestSpawnArgsWithName(t *testing.T) {
	req := SpawnRequest{Agent: "shell", Name: "my-task"}
	got := strings.Join(spawnArgs(req), " ")
	if got != "spawn shell --name my-task --seed-auth --reuse-gh-auth --auto-trust --background" {
		t.Fatalf("argv: %q", got)
	}
}

func TestSpawnArgs(t *testing.T) {
	req := SpawnRequest{Agent: "claude", Repo: "git@github.com:o/r.git",
		Prompt: "do it", SSH: true, ReuseAuth: true, Worktree: false,
		Network: "agent-isolated", Memory: "16g", CPUs: "8", DryRun: true}
	got := strings.Join(spawnArgs(req), " ")
	want := "spawn claude --repo git@github.com:o/r.git --prompt do it --ssh --reuse-auth --seed-auth --reuse-gh-auth --auto-trust --network agent-isolated --memory 16g --cpus 8 --background --dry-run"
	if got != want {
		t.Fatalf("\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSpawnArgsSanitizesName(t *testing.T) {
	got := strings.Join(spawnArgs(SpawnRequest{Agent: "claude", Name: "test local"}), " ")
	if got != "spawn claude --name test-local --seed-auth --reuse-gh-auth --auto-trust --background" {
		t.Fatalf("argv: %q", got)
	}
	if got := strings.Join(spawnArgs(SpawnRequest{Agent: "claude", Name: "  !!  "}), " "); got != "spawn claude --seed-auth --reuse-gh-auth --auto-trust --background" {
		t.Fatalf("all-invalid name must be dropped: %q", got)
	}
}

func TestCloneReconstructsSpawn(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker inspect",
		"claude|true|PATH=/usr/bin\nREPOS=git@github.com:o/r.git https://github.com/o/x.git\nHOME=/home/agent\n")
	r, calls := recorderRunner()
	s := &AgentService{Runner: r, Exec: fake}
	if _, err := s.Clone("agent-claude-orig"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join((*calls)[0], " ")
	want := "safe-ag spawn claude --repo git@github.com:o/r.git --repo https://github.com/o/x.git --ssh --seed-auth --reuse-gh-auth --auto-trust --background"
	if got != want {
		t.Fatalf("\ngot:  %s\nwant: %s", got, want)
	}
}

func TestSpawnArgsMaxCost(t *testing.T) {
	got := strings.Join(spawnArgs(SpawnRequest{Agent: "claude", MaxCost: "2.50"}), " ")
	if got != "spawn claude --seed-auth --reuse-gh-auth --auto-trust --max-cost 2.50 --background" {
		t.Fatalf("argv: %q", got)
	}
}
