//go:build livespike

// Live E2E QA harness: exercises the real berth CLI + VM + a disposable
// shell container through the same service layer the GUI calls.
// Run: BERTH_LIVE_QA=1 go test -tags livespike -run TestLiveQA ./internal/svc/ -v -timeout 10m
package svc

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/0x666c6f/berth/app/internal/cli"
	"github.com/0x666c6f/berth/app/internal/poll"
	"github.com/0x666c6f/berth/pkg/vmexec"
)

const (
	qaName      = "qa-e2e"
	qaContainer = "agent-shell-qa-e2e"
	qaRepo      = "https://github.com/octocat/Hello-World.git"
	qaWorkdir   = "/workspace/octocat/Hello-World"
)

func vmRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ex := &vmexec.MachineExecutor{VMName: "berth"}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := ex.Run(ctx, args...)
	return string(out), err
}

func waitFor(t *testing.T, within time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestLiveQA(t *testing.T) {
	if os.Getenv("BERTH_LIVE_QA") == "" {
		t.Skip("set BERTH_LIVE_QA=1 to run the live E2E QA harness")
	}
	svc := &AgentService{Runner: cli.NewRunner()}

	// Cleanup any prior run, then spawn the disposable target.
	exec.Command("berth", "stop", qaContainer).Run()
	t.Cleanup(func() { exec.Command("berth", "stop", qaContainer).Run() })

	out, err := svc.Spawn(SpawnRequest{Agent: "shell", Name: qaName, Repo: qaRepo, SSH: false})
	if err != nil {
		t.Fatalf("spawn: %v\n%s", err, out)
	}
	waitFor(t, 120*time.Second, "container Up + tmux session", func() bool {
		ps, _ := vmRun(t, "docker", "ps", "--filter", "name="+qaContainer, "--format", "{{.Status}}")
		if !strings.HasPrefix(strings.TrimSpace(ps), "Up") {
			return false
		}
		_, err := vmRun(t, "docker", "exec", qaContainer, "tmux", "has-session", "-t", "berth")
		return err == nil
	})
	// Wait for the clone to land (entrypoint clones after container start).
	waitFor(t, 120*time.Second, "repo cloned", func() bool {
		_, err := vmRun(t, "docker", "exec", qaContainer, "test", "-d", qaWorkdir+"/.git")
		return err == nil
	})

	t.Run("A6_poller_parses_real_docker", func(t *testing.T) {
		ex := &vmexec.MachineExecutor{VMName: "berth"}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		raw, err := ex.Run(ctx, "docker", "ps", "-a", "--filter", "name=^agent-", "--format", poll.PSFormat())
		if err != nil {
			t.Fatalf("ps: %v", err)
		}
		agents := poll.ParsePS(raw)
		var found *poll.Agent
		for i := range agents {
			if agents[i].Name == qaContainer {
				found = &agents[i]
			}
		}
		if found == nil {
			t.Fatalf("qa container not parsed; got %d agents", len(agents))
		}
		if found.Terminal != "tmux" || !found.Running || found.Type != "shell" {
			t.Fatalf("bad parse: %+v", *found)
		}
	})

	t.Run("C1_output_json", func(t *testing.T) {
		info, err := svc.Output(qaContainer)
		if err != nil {
			t.Fatalf("output: %v", err)
		}
		if info.Status == "" {
			t.Fatalf("empty status: %+v", info)
		}
	})

	t.Run("C3_steer_reaches_tmux", func(t *testing.T) {
		if err := svc.Steer(qaContainer, "echo qa-steer-$((40+2))"); err != nil {
			t.Fatalf("steer: %v", err)
		}
		waitFor(t, 20*time.Second, "steer output in pane", func() bool {
			pane, _ := vmRun(t, "docker", "exec", qaContainer, "tmux", "capture-pane", "-t", "berth", "-p", "-S", "-50")
			return strings.Contains(pane, "qa-steer-42")
		})
	})

	t.Run("D2_diff_sees_change", func(t *testing.T) {
		if _, err := vmRun(t, "docker", "exec", qaContainer, "bash", "-c",
			"echo qa-change >> "+qaWorkdir+"/README"); err != nil {
			t.Fatalf("mutate workspace: %v", err)
		}
		diff, err := svc.Diff(qaContainer)
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		if !strings.Contains(diff, "README") || !strings.Contains(diff, "qa-change") {
			t.Fatalf("diff missing change:\n%s", diff)
		}
	})

	t.Run("D3_checkpoint_create_list", func(t *testing.T) {
		if err := svc.CheckpointCreate(qaContainer, "qa-checkpoint"); err != nil {
			t.Fatalf("checkpoint create: %v", err)
		}
		list, err := svc.CheckpointList(qaContainer)
		if err != nil {
			t.Fatalf("checkpoint list: %v", err)
		}
		if strings.TrimSpace(list) == "" {
			t.Fatal("empty checkpoint list after create")
		}
	})

	t.Run("D4_revert_cleans_diff", func(t *testing.T) {
		if err := svc.WorkspaceRevert(qaContainer); err != nil {
			t.Fatalf("revert: %v", err)
		}
		diff, err := svc.Diff(qaContainer)
		if err != nil {
			t.Fatalf("diff after revert: %v", err)
		}
		if strings.Contains(diff, "qa-change") {
			t.Fatalf("revert did not clean change:\n%s", diff)
		}
	})

	t.Run("E2_dryrun_norepo", func(t *testing.T) {
		out, err := svc.Spawn(SpawnRequest{Agent: "shell", DryRun: true})
		if err != nil {
			t.Fatalf("dry-run: %v\n%s", err, out)
		}
		if !strings.Contains(out, "docker run") {
			t.Fatalf("no docker run in dry-run output:\n%s", out)
		}
		if strings.Contains(out, "REPOS=") {
			t.Fatalf("no-repo dry-run still sets REPOS:\n%s", out)
		}
	})

	t.Run("E4_template_list", func(t *testing.T) {
		out, err := svc.TemplateList()
		if err != nil {
			t.Fatalf("template list: %v", err)
		}
		if !strings.Contains(out, "security-audit") {
			t.Fatalf("built-in template missing:\n%s", out)
		}
	})

	t.Run("H1_cost_history", func(t *testing.T) {
		if _, err := svc.CostHistory("7d"); err != nil {
			t.Fatalf("cost history: %v", err)
		}
	})

	t.Run("C4_stop", func(t *testing.T) {
		if err := svc.Stop(qaContainer); err != nil {
			t.Fatalf("stop: %v", err)
		}
		waitFor(t, 30*time.Second, "container gone/stopped", func() bool {
			ps, _ := vmRun(t, "docker", "ps", "--filter", "name="+qaContainer, "--format", "{{.Names}}")
			return !strings.Contains(ps, qaContainer)
		})
	})
}
