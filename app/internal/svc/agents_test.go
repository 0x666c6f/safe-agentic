package svc

import (
	"context"
	"strings"
	"testing"

	"github.com/0x666c6f/safe-agentic/app/internal/cli"
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

func TestSpawnArgs(t *testing.T) {
	req := SpawnRequest{Agent: "claude", Repo: "git@github.com:o/r.git",
		Prompt: "do it", SSH: true, ReuseAuth: true, Worktree: false,
		Network: "agent-isolated", Memory: "16g", CPUs: "8", DryRun: true}
	got := strings.Join(spawnArgs(req), " ")
	want := "spawn claude --repo git@github.com:o/r.git --prompt do it --ssh --reuse-auth --network agent-isolated --memory 16g --cpus 8 --background --dry-run"
	if got != want {
		t.Fatalf("\ngot:  %s\nwant: %s", got, want)
	}
}
