package main

import (
	"context"
	"strings"
	"testing"

	"github.com/0x666c6f/berth/pkg/vmexec"
)

// setStateResponses wires the docker inspect / capture-pane calls that
// gatherStatus makes for a single container using broad prefixes (which match
// any container name suffix).
func setStateResponses(fake *vmexec.FakeExecutor, running bool, agentType, terminal string) {
	runningStr := "false"
	statusStr := "exited"
	if running {
		runningStr = "true"
		statusStr = "running"
	}
	fake.SetResponse("docker inspect --format {{.State.Running}}", runningStr+"\n")
	fake.SetResponse("docker inspect --format {{.State.Status}}", statusStr+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "berth.agent-type"}}`, agentType+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "berth.terminal"}}`, terminal+"\n")
}

const blockedPane = `Do you want to proceed?
❯ 1. Yes
  3. No, and tell Claude what to do differently`

const workingPane = `● Editing src/main.go
✳ Working… (esc to interrupt)`

func TestStatusRunningBlocked(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-x\n")
	setStateResponses(fake, true, "claude", "tmux")
	fake.SetResponse("docker exec agent-claude-x tmux capture-pane", blockedPane+"\n")

	out := captureOutput(func() {
		if err := runStatus(statusCmd, []string{"agent-claude-x"}); err != nil {
			t.Fatalf("runStatus() error = %v", err)
		}
	})
	if !strings.Contains(out, "blocked") {
		t.Fatalf("expected blocked state, got: %q", out)
	}
	if !strings.Contains(out, "agent-claude-x") {
		t.Fatalf("expected container name, got: %q", out)
	}
}

func TestStatusRunningWorking(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-codex-y\n")
	setStateResponses(fake, true, "codex", "tmux")
	fake.SetResponse("docker exec agent-codex-y tmux capture-pane", workingPane+"\n")

	out := captureOutput(func() {
		if err := runStatus(statusCmd, []string{"agent-codex-y"}); err != nil {
			t.Fatalf("runStatus() error = %v", err)
		}
	})
	if !strings.Contains(out, "working") {
		t.Fatalf("expected working state, got: %q", out)
	}
}

func TestStatusStoppedCleanExitIsDone(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-z\n")
	setStateResponses(fake, false, "claude", "tmux")
	fake.SetResponse("docker inspect --format {{.State.ExitCode}}", "0\n")

	out := captureOutput(func() {
		if err := runStatus(statusCmd, []string{"agent-claude-z"}); err != nil {
			t.Fatalf("runStatus() error = %v", err)
		}
	})
	if !strings.Contains(out, "done") {
		t.Fatalf("expected done state for clean exit, got: %q", out)
	}
}

func TestStatusStoppedNonZeroIsExited(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-z\n")
	setStateResponses(fake, false, "claude", "tmux")
	fake.SetResponse("docker inspect --format {{.State.ExitCode}}", "137\n")

	out := captureOutput(func() {
		if err := runStatus(statusCmd, []string{"agent-claude-z"}); err != nil {
			t.Fatalf("runStatus() error = %v", err)
		}
	})
	if !strings.Contains(out, "exited") || !strings.Contains(out, "137") {
		t.Fatalf("expected exited state with code 137, got: %q", out)
	}
}

func TestStatusNonTmuxRunningIsWorking(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-bg\n")
	setStateResponses(fake, true, "claude", "background")

	out := captureOutput(func() {
		if err := runStatus(statusCmd, []string{"agent-bg"}); err != nil {
			t.Fatalf("runStatus() error = %v", err)
		}
	})
	if !strings.Contains(out, "working") || !strings.Contains(out, "no tmux pane") {
		t.Fatalf("expected working/no-pane for non-tmux, got: %q", out)
	}
}

// A running container whose tmux pane can't be captured (headless/background
// mode, a shell running bash directly, or a session still starting) must report
// working, NOT unknown.
func TestStatusRunningNoPaneIsWorking(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-nopane\n")
	setStateResponses(fake, true, "claude", "tmux")
	fake.SetError("docker exec agent-nopane tmux capture-pane", "no server running on /tmp/tmux")

	out := captureOutput(func() {
		if err := runStatus(statusCmd, []string{"agent-nopane"}); err != nil {
			t.Fatalf("runStatus() error = %v", err)
		}
	})
	if !strings.Contains(out, "working") || strings.Contains(out, "unknown") {
		t.Fatalf("expected working (not unknown) for no-pane running container, got: %q", out)
	}
	if !strings.Contains(out, "no tmux pane") {
		t.Fatalf("expected 'no tmux pane' reason, got: %q", out)
	}
}

func TestStatusJSON(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-x\n")
	setStateResponses(fake, true, "claude", "tmux")
	fake.SetResponse("docker exec agent-claude-x tmux capture-pane", blockedPane+"\n")

	oldJSON := statusJSON
	statusJSON = true
	defer func() { statusJSON = oldJSON }()

	out := captureOutput(func() {
		if err := runStatus(statusCmd, []string{"agent-claude-x"}); err != nil {
			t.Fatalf("runStatus() error = %v", err)
		}
	})
	if !strings.Contains(out, `"state": "blocked"`) {
		t.Fatalf("expected JSON state field, got: %q", out)
	}
	if !strings.Contains(out, `"agent_type": "claude"`) {
		t.Fatalf("expected JSON agent_type field, got: %q", out)
	}
}

func TestStatusAll(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-a\nagent-b\n")
	setStateResponses(fake, true, "claude", "tmux")
	fake.SetResponse("docker exec agent-a tmux capture-pane", blockedPane+"\n")
	fake.SetResponse("docker exec agent-b tmux capture-pane", workingPane+"\n")

	oldAll := statusAll
	statusAll = true
	defer func() { statusAll = oldAll }()

	out := captureOutput(func() {
		if err := runStatus(statusCmd, nil); err != nil {
			t.Fatalf("runStatus() error = %v", err)
		}
	})
	if !strings.Contains(out, "agent-a") || !strings.Contains(out, "agent-b") {
		t.Fatalf("expected both containers, got: %q", out)
	}
	if !strings.Contains(out, "blocked") || !strings.Contains(out, "working") {
		t.Fatalf("expected blocked and working states, got: %q", out)
	}
}

func TestStatusAllEmpty(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "")

	oldAll := statusAll
	statusAll = true
	defer func() { statusAll = oldAll }()

	out := captureOutput(func() {
		if err := runStatus(statusCmd, nil); err != nil {
			t.Fatalf("runStatus() error = %v", err)
		}
	})
	if !strings.Contains(out, "No agent containers") {
		t.Fatalf("expected empty message, got: %q", out)
	}
}

// liveBlockedEntries feeds the inbox; a blocked running agent must produce one
// needs-attention entry.
func TestLiveBlockedEntries(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-a\n")
	setStateResponses(fake, true, "claude", "tmux")
	fake.SetResponse("docker exec agent-a tmux capture-pane", blockedPane+"\n")

	entries := liveBlockedEntries(context.Background(), fake)
	if len(entries) != 1 {
		t.Fatalf("expected 1 blocked entry, got %d", len(entries))
	}
	if entries[0].Container != "agent-a" || entries[0].Status != "blocked" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

// A working agent must NOT create an inbox entry.
func TestLiveBlockedEntriesIgnoresWorking(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-a\n")
	setStateResponses(fake, true, "claude", "tmux")
	fake.SetResponse("docker exec agent-a tmux capture-pane", workingPane+"\n")

	if entries := liveBlockedEntries(context.Background(), fake); len(entries) != 0 {
		t.Fatalf("expected no entries for working agent, got %+v", entries)
	}
}

// runInbox must surface a live blocked agent as a needs-attention item.
func TestInboxSurfacesBlockedAgent(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-a\n")
	setStateResponses(fake, true, "claude", "tmux")
	fake.SetResponse("docker exec agent-a tmux capture-pane", blockedPane+"\n")

	oldAll := inboxAll
	inboxAll = false
	defer func() { inboxAll = oldAll }()

	out := captureOutput(func() {
		if err := runInbox(inboxCmd, nil); err != nil {
			t.Fatalf("runInbox() error = %v", err)
		}
	})
	if !strings.Contains(out, "blocked") || !strings.Contains(out, "agent-a") {
		t.Fatalf("expected blocked agent in inbox, got: %q", out)
	}
}

// summary must include a State: line driven by the same detection.
func TestSummaryShowsState(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-x\n")
	fake.SetResponse("docker inspect --format {{.State.Status}}", "running\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "berth.agent-type"}}`, "claude\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "berth.terminal"}}`, "tmux\n")
	fake.SetResponse("docker exec agent-claude-x tmux capture-pane", blockedPane+"\n")

	out := captureOutput(func() {
		if err := runSummary(summaryCmd, []string{"agent-claude-x"}); err != nil {
			t.Fatalf("runSummary() error = %v", err)
		}
	})
	if !strings.Contains(out, "State:") || !strings.Contains(out, "blocked") {
		t.Fatalf("expected State: blocked line in summary, got: %q", out)
	}
}

func TestStateIcon(t *testing.T) {
	for state, want := range map[string]string{
		"blocked": "🔴",
		"working": "🟢",
		"idle":    "🟡",
		"done":    "✅",
		"exited":  "⏹",
		"unknown": "❔",
	} {
		if got := stateIcon(state); got != want {
			t.Errorf("stateIcon(%q) = %q, want %q", state, got, want)
		}
	}
}
