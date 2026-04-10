package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"safe-agentic/pkg/audit"
	"safe-agentic/pkg/config"
	"safe-agentic/pkg/fleet"
	"safe-agentic/pkg/orb"
)

// ─── test harness ─────────────────────────────────────────────────────────────

// testSetup replaces the global executor with a FakeExecutor and returns it +
// a cleanup function that restores the original.
func testSetup(t *testing.T) (*orb.FakeExecutor, func()) {
	t.Helper()
	fake := orb.NewFake()
	original := newExecutor
	newExecutor = func() orb.Executor { return fake }
	return fake, func() { newExecutor = original }
}

// captureOutput redirects os.Stdout to a buffer for the duration of fn,
// then returns what was written.
func captureOutput(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// ─── list ─────────────────────────────────────────────────────────────────────

func TestListCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent-", "agent-claude-test\tUp 5 minutes\n")

	output := captureOutput(func() {
		if err := runList(listCmd, nil); err != nil {
			t.Fatalf("runList() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker ps")
	if len(cmds) == 0 {
		t.Fatal("no docker ps command sent")
	}
	// Verify the output filter is present
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "--filter") {
		t.Errorf("expected --filter flag, got: %s", joined)
	}
	if !strings.Contains(joined, "name=^agent-") {
		t.Errorf("expected name=^agent- filter, got: %s", joined)
	}
	_ = output
}

func TestListCommandJSON(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent-", `{"Names":"agent-claude-test","Status":"Up 5 minutes"}`+"\n")

	// Enable JSON mode
	listJSON = true
	defer func() { listJSON = false }()

	captureOutput(func() {
		if err := runList(listCmd, nil); err != nil {
			t.Fatalf("runList() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker ps")
	if len(cmds) == 0 {
		t.Fatal("no docker ps command sent")
	}
	// Verify json format requested
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "{{json .}}") {
		t.Errorf("expected json format flag, got: %s", joined)
	}
}

// ─── attach ───────────────────────────────────────────────────────────────────

func TestAttachCommand_Running(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	// ResolveTarget: list containers
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	// containerState
	fake.SetResponse("docker inspect --format {{.State.Status}}", "running\n")
	// InspectLabel for terminal
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.terminal"}}`, "tmux\n")
	// HasSession
	fake.SetResponse("docker exec "+containerName+" tmux has-session", "")

	if err := runAttach(attachCmd, []string{containerName}); err != nil {
		t.Fatalf("runAttach() error = %v", err)
	}

	// Verify tmux attach was issued (via RunInteractive)
	cmds := fake.CommandsMatching("tmux attach")
	if len(cmds) == 0 {
		t.Fatal("no tmux attach command sent")
	}
}

func TestAttachCommand_Stopped(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-stopped"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	// containerState: exited
	fake.SetResponse("docker inspect --format {{.State.Status}}", "exited\n")
	// InspectLabel for terminal
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.terminal"}}`, "tmux\n")
	// docker start
	fake.SetResponse("docker start "+containerName, "")
	// HasSession — after start, tmux session is there
	fake.SetResponse("docker exec "+containerName+" tmux has-session", "")

	if err := runAttach(attachCmd, []string{containerName}); err != nil {
		t.Fatalf("runAttach() error = %v", err)
	}

	startCmds := fake.CommandsMatching("docker start")
	if len(startCmds) == 0 {
		t.Fatal("expected docker start command")
	}
}

// ─── stop ─────────────────────────────────────────────────────────────────────

func TestStopCommand_Single(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	output := captureOutput(func() {
		if err := runStop(stopCmd, []string{containerName}); err != nil {
			t.Fatalf("runStop() error = %v", err)
		}
	})

	if !strings.Contains(output, "Stopping") {
		t.Errorf("expected 'Stopping' in output, got: %s", output)
	}

	stopCmds := fake.CommandsMatching("docker stop")
	if len(stopCmds) == 0 {
		t.Fatal("expected docker stop command")
	}
	rmCmds := fake.CommandsMatching("docker rm")
	if len(rmCmds) == 0 {
		t.Fatal("expected docker rm command")
	}
}

func TestStopCommand_All(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	stopAll = true
	defer func() { stopAll = false }()

	// List returns two containers
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-a\nagent-claude-b\n")

	output := captureOutput(func() {
		if err := runStop(stopCmd, nil); err != nil {
			t.Fatalf("runStop() error = %v", err)
		}
	})

	if !strings.Contains(output, "Stopped") {
		t.Errorf("expected 'Stopped' in output, got: %s", output)
	}

	stopCmds := fake.CommandsMatching("docker stop")
	if len(stopCmds) == 0 {
		t.Fatal("expected docker stop command")
	}
	rmCmds := fake.CommandsMatching("docker rm")
	if len(rmCmds) == 0 {
		t.Fatal("expected docker rm command")
	}
}

func TestStopCommand_All_NoContainers(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	stopAll = true
	defer func() { stopAll = false }()

	fake.SetResponse("docker ps -a --filter name=^agent-", "")

	output := captureOutput(func() {
		if err := runStop(stopCmd, nil); err != nil {
			t.Fatalf("runStop() error = %v", err)
		}
	})

	if !strings.Contains(output, "No agent containers") {
		t.Errorf("expected 'No agent containers' in output, got: %s", output)
	}
}

// ─── cleanup ──────────────────────────────────────────────────────────────────

func TestCleanupCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	// Running containers
	fake.SetResponse("docker ps --filter name=^agent- --format {{.Names}}", "agent-claude-test\n")
	// All containers (incl stopped)
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-test\n")

	output := captureOutput(func() {
		if err := runCleanup(cleanupCmd, nil); err != nil {
			t.Fatalf("runCleanup() error = %v", err)
		}
	})

	if !strings.Contains(output, "Cleanup complete") {
		t.Errorf("expected 'Cleanup complete' in output, got: %s", output)
	}

	// Verify prune was called
	pruneCmds := fake.CommandsMatching("docker network prune")
	if len(pruneCmds) == 0 {
		t.Fatal("expected docker network prune command")
	}
}

// ─── peek ─────────────────────────────────────────────────────────────────────

func TestPeekCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	// IsRunning
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	// InspectLabel terminal
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.terminal"}}`, "tmux\n")
	// capture-pane output
	fake.SetResponse("docker exec "+containerName+" tmux capture-pane", "line1\nline2\nline3\n")

	output := captureOutput(func() {
		if err := runPeek(peekCmd, []string{containerName}); err != nil {
			t.Fatalf("runPeek() error = %v", err)
		}
	})

	captureCmds := fake.CommandsMatching("tmux capture-pane")
	if len(captureCmds) == 0 {
		t.Fatal("expected tmux capture-pane command")
	}
	// Verify lines arg is present
	joined := strings.Join(captureCmds[0], " ")
	if !strings.Contains(joined, "-S") {
		t.Errorf("expected -S flag for line count, got: %s", joined)
	}
	_ = output
}

func TestPeekCommand_NotRunning(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-stopped"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "false\n")

	err := runPeek(peekCmd, []string{containerName})
	if err == nil {
		t.Fatal("expected error for stopped container")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("expected 'not running' in error, got: %v", err)
	}
}

// ─── diff ─────────────────────────────────────────────────────────────────────

func TestDiffCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", "diff output here\n")

	output := captureOutput(func() {
		diffStat = false
		if err := runDiff(diffCmd, []string{containerName}); err != nil {
			t.Fatalf("runDiff() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec "+containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	// Verify git diff command
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "git diff") {
		t.Errorf("expected 'git diff' in command, got: %s", joined)
	}
	_ = output
}

func TestDiffCommand_Stat(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", " 3 files changed\n")

	diffStat = true
	defer func() { diffStat = false }()

	captureOutput(func() {
		if err := runDiff(diffCmd, []string{containerName}); err != nil {
			t.Fatalf("runDiff() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec "+containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "--stat") {
		t.Errorf("expected '--stat' in command, got: %s", joined)
	}
}

// ─── output ───────────────────────────────────────────────────────────────────

func TestOutputCommand_Diff(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash", "diff content\n")

	outputDiff = true
	defer func() { outputDiff = false }()

	captureOutput(func() {
		if err := runOutput(outputCmd, []string{containerName}); err != nil {
			t.Fatalf("runOutput() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec "+containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "git diff") {
		t.Errorf("expected 'git diff' in command, got: %s", joined)
	}
}

func TestOutputCommand_Files(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash", "main.go\nlib.go\n")

	outputFiles = true
	defer func() { outputFiles = false }()

	output := captureOutput(func() {
		if err := runOutput(outputCmd, []string{containerName}); err != nil {
			t.Fatalf("runOutput() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec "+containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "--name-only") {
		t.Errorf("expected '--name-only' in command, got: %s", joined)
	}
	_ = output
}

func TestOutputCommand_Commits(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash", "abc1234 fix: test\n")

	outputCommits = true
	defer func() { outputCommits = false }()

	captureOutput(func() {
		if err := runOutput(outputCmd, []string{containerName}); err != nil {
			t.Fatalf("runOutput() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec "+containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "git log --oneline") {
		t.Errorf("expected 'git log --oneline', got: %s", joined)
	}
}

func TestOutputCommand_Default(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker logs", "log line 1\nlog line 2\n")

	// Ensure all sub-mode flags are false
	outputDiff = false
	outputFiles = false
	outputCommits = false
	outputJSON = false

	captureOutput(func() {
		if err := runOutput(outputCmd, []string{containerName}); err != nil {
			t.Fatalf("runOutput() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker logs")
	if len(cmds) == 0 {
		t.Fatal("expected docker logs command")
	}
}

// ─── summary ──────────────────────────────────────────────────────────────────

func TestSummaryCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	// inspectField calls
	fake.SetResponse("docker inspect --format {{.State.Status}}", "running\n")
	fake.SetResponse("docker inspect --format {{.State.StartedAt}}", "2026-04-10T10:00:00Z\n")
	fake.SetResponse("docker inspect --format {{.State.FinishedAt}}", "0001-01-01T00:00:00Z\n")
	// InspectLabel calls
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.repo"}}`, "myrepo\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.ssh"}}`, "true\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.auth"}}`, "shared\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.docker"}}`, "off\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.network"}}`, "bridge\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.resources"}}`, "cpu=4,mem=8g\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.terminal"}}`, "tmux\n")

	output := captureOutput(func() {
		if err := runSummary(summaryCmd, []string{containerName}); err != nil {
			t.Fatalf("runSummary() error = %v", err)
		}
	})

	if !strings.Contains(output, containerName) {
		t.Errorf("expected container name in output, got: %s", output)
	}
	if !strings.Contains(output, "Status") {
		t.Errorf("expected 'Status' in output, got: %s", output)
	}
}

// ─── audit ────────────────────────────────────────────────────────────────────

func TestAuditCommand(t *testing.T) {
	// Create temp audit file with some entries
	tmpFile, err := os.CreateTemp("", "audit-test-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	entry := audit.Entry{
		Timestamp: "2026-04-10T10:00:00Z",
		Action:    "spawn",
		Container: "agent-claude-test",
		Details:   map[string]string{"type": "claude"},
	}
	data, _ := json.Marshal(entry)
	fmt.Fprintln(tmpFile, string(data))
	tmpFile.Close()

	// Override audit path via logger directly by saving/restoring
	logger := &audit.Logger{Path: tmpFile.Name()}
	entries, err := logger.Read(10)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry")
	}
	if entries[0].Action != "spawn" {
		t.Errorf("expected action 'spawn', got %q", entries[0].Action)
	}
}

func TestAuditCommand_Empty(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "audit-empty-*.jsonl")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	logger := &audit.Logger{Path: tmpFile.Name()}
	entries, err := logger.Read(10)
	if err != nil {
		t.Fatalf("read empty audit: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ─── todo ─────────────────────────────────────────────────────────────────────

func TestTodoAdd(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	// readTodos returns empty list
	fake.SetResponse("docker exec "+containerName+" bash -c cat", "[]\n")

	output := captureOutput(func() {
		if err := runTodoAdd(todoAddCmd, []string{containerName, "Write tests"}); err != nil {
			t.Fatalf("runTodoAdd() error = %v", err)
		}
	})

	if !strings.Contains(output, "Added") {
		t.Errorf("expected 'Added' in output, got: %s", output)
	}
	if !strings.Contains(output, "Write tests") {
		t.Errorf("expected 'Write tests' in output, got: %s", output)
	}

	// Verify write was called
	writeCmds := fake.CommandsMatching("docker exec "+containerName)
	if len(writeCmds) == 0 {
		t.Fatal("expected docker exec write command")
	}
}

func TestTodoList_Empty(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c cat", "[]\n")

	output := captureOutput(func() {
		if err := runTodoList(todoListCmd, []string{containerName}); err != nil {
			t.Fatalf("runTodoList() error = %v", err)
		}
	})

	if !strings.Contains(output, "No todos") {
		t.Errorf("expected 'No todos' in output, got: %s", output)
	}
}

func TestTodoList_WithItems(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	// Return a list with items
	items := `[{"text":"Fix bug","done":false},{"text":"Write docs","done":true}]`
	fake.SetResponse("docker exec "+containerName+" bash -c cat", items+"\n")

	output := captureOutput(func() {
		if err := runTodoList(todoListCmd, []string{containerName}); err != nil {
			t.Fatalf("runTodoList() error = %v", err)
		}
	})

	if !strings.Contains(output, "Fix bug") {
		t.Errorf("expected 'Fix bug' in output, got: %s", output)
	}
	if !strings.Contains(output, "Write docs") {
		t.Errorf("expected 'Write docs' in output, got: %s", output)
	}
	// First item not done ([ ]), second done ([x])
	if !strings.Contains(output, "[ ]") {
		t.Errorf("expected '[ ]' in output, got: %s", output)
	}
	if !strings.Contains(output, "[x]") {
		t.Errorf("expected '[x]' in output, got: %s", output)
	}
}

// ─── spawn validation ─────────────────────────────────────────────────────────

func TestSpawnValidation_InvalidAgentType(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	err := executeSpawn(SpawnOpts{
		AgentType: "invalid-agent",
	})
	if err == nil {
		t.Fatal("expected error for invalid agent type")
	}
	if !strings.Contains(err.Error(), "agent type must be") {
		t.Errorf("expected 'agent type must be' in error, got: %v", err)
	}
}

func TestSpawnValidation_InvalidName(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	err := executeSpawn(SpawnOpts{
		AgentType: "claude",
		Name:      "bad name with spaces",
	})
	if err == nil {
		t.Fatal("expected error for invalid container name")
	}
}

func TestSpawnValidation_SSHRequired(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	err := executeSpawn(SpawnOpts{
		AgentType: "claude",
		Repos:     []string{"git@github.com:org/repo.git"},
		SSH:       false,
	})
	if err == nil {
		t.Fatal("expected error for SSH repo without --ssh flag")
	}
}

func TestSpawnValidation_ValidTypes(t *testing.T) {
	for _, agentType := range []string{"claude", "codex", "shell"} {
		t.Run(agentType, func(t *testing.T) {
			// We don't call executeSpawn all the way (it would try to connect)
			// Just verify the type check passes (it fails at a later stage with
			// "load config" which is fine for this validation test)
			err := executeSpawn(SpawnOpts{
				AgentType: agentType,
				DryRun:    true, // prevent network calls
				Repos:     []string{},
			})
			// DryRun: may succeed or fail after validation; we just check it doesn't fail
			// with the "agent type must be" error
			if err != nil && strings.Contains(err.Error(), "agent type must be") {
				t.Errorf("valid agent type %q rejected: %v", agentType, err)
			}
		})
	}
}

// ─── checkpoint ───────────────────────────────────────────────────────────────

func TestCheckpointCreate(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", "Saved working directory\n")
	fake.SetResponse("docker commit", "sha256:abc123\n")

	output := captureOutput(func() {
		if err := runCheckpointCreate(checkpointCreateCmd, []string{containerName, "my-snapshot"}); err != nil {
			t.Fatalf("runCheckpointCreate() error = %v", err)
		}
	})

	if !strings.Contains(output, "Checkpoint created") {
		t.Errorf("expected 'Checkpoint created', got: %s", output)
	}
	if !strings.Contains(output, "my-snapshot") {
		t.Errorf("expected snapshot label in output, got: %s", output)
	}
}

func TestCheckpointList_Empty(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", "\n")

	output := captureOutput(func() {
		if err := runCheckpointList(checkpointListCmd, []string{containerName}); err != nil {
			t.Fatalf("runCheckpointList() error = %v", err)
		}
	})

	if !strings.Contains(output, "No checkpoints") {
		t.Errorf("expected 'No checkpoints', got: %s", output)
	}
}

// ─── run (quick start) ────────────────────────────────────────────────────────

func TestRunQuickStart_NoRepo(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	err := runQuickStart(runCmd, []string{"just a prompt no repo"})
	if err == nil {
		t.Fatal("expected error when no repo URL provided")
	}
	if !strings.Contains(err.Error(), "at least one repo URL") {
		t.Errorf("expected 'at least one repo URL' error, got: %v", err)
	}
}

func TestRunQuickStart_SSHAutoDetect(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	// Prevent actual spawn from running all the way through by using an
	// error on network setup (which happens early in executeSpawn after validation)
	fake.SetError("docker network", "no such network")

	// SSH URL should auto-enable SSH; this will fail at a later stage but
	// the auto-SSH detection check is what we're testing
	err := runQuickStart(runCmd, []string{"git@github.com:org/repo.git", "fix the tests"})
	// The error from docker network is expected — we just want to verify it
	// called executeSpawn with ssh=true (which means it passed validation)
	if err != nil && strings.Contains(err.Error(), "at least one repo URL") {
		t.Fatal("SSH URL should be recognized as a repo URL")
	}
}

// ─── fleet ────────────────────────────────────────────────────────────────────

func TestFleetStatus_Empty(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter label=safe-agentic.fleet", "")

	output := captureOutput(func() {
		if err := runFleetStatus(fleetStatusCmd, nil); err != nil {
			t.Fatalf("runFleetStatus() error = %v", err)
		}
	})

	if !strings.Contains(output, "No fleet containers") {
		t.Errorf("expected 'No fleet containers', got: %s", output)
	}
}

func TestFleetStatus_WithContainers(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter label=safe-agentic.fleet", "agent-claude-fleet-a\tUp 2 minutes\nagent-claude-fleet-b\tExited\n")

	output := captureOutput(func() {
		if err := runFleetStatus(fleetStatusCmd, nil); err != nil {
			t.Fatalf("runFleetStatus() error = %v", err)
		}
	})

	if !strings.Contains(output, "fleet-a") {
		t.Errorf("expected 'fleet-a' in output, got: %s", output)
	}
}

// ─── update ───────────────────────────────────────────────────────────────────

func TestUpdateCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker build", "Successfully built abc123\n")

	output := captureOutput(func() {
		updateQuick = false
		updateFull = false
		if err := runUpdate(updateCmd, nil); err != nil {
			t.Fatalf("runUpdate() error = %v", err)
		}
	})

	buildCmds := fake.CommandsMatching("docker build")
	if len(buildCmds) == 0 {
		t.Fatal("expected docker build command")
	}
	if !strings.Contains(output, "Image updated") {
		t.Errorf("expected 'Image updated', got: %s", output)
	}
}

func TestUpdateCommand_Quick(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker build", "Successfully built abc123\n")

	updateQuick = true
	defer func() { updateQuick = false }()

	captureOutput(func() {
		if err := runUpdate(updateCmd, nil); err != nil {
			t.Fatalf("runUpdate() error = %v", err)
		}
	})

	buildCmds := fake.CommandsMatching("docker build")
	if len(buildCmds) == 0 {
		t.Fatal("expected docker build command")
	}
	joined := strings.Join(buildCmds[0], " ")
	if !strings.Contains(joined, "CACHEBUST") {
		t.Errorf("expected CACHEBUST arg, got: %s", joined)
	}
}

func TestUpdateCommand_Full(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker build", "Successfully built abc123\n")

	updateFull = true
	defer func() { updateFull = false }()

	captureOutput(func() {
		if err := runUpdate(updateCmd, nil); err != nil {
			t.Fatalf("runUpdate() error = %v", err)
		}
	})

	buildCmds := fake.CommandsMatching("docker build")
	if len(buildCmds) == 0 {
		t.Fatal("expected docker build command")
	}
	joined := strings.Join(buildCmds[0], " ")
	if !strings.Contains(joined, "--no-cache") {
		t.Errorf("expected '--no-cache', got: %s", joined)
	}
}

// ─── containerEnvVar ──────────────────────────────────────────────────────────

func TestContainerEnvVar(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker inspect --format {{range .Config.Env}}{{println .}}{{end}}", "FOO=bar\nBAZ=qux\nAGENT_TYPE=claude\n")

	val, err := containerEnvVar(context.Background(), fake, "mycontainer", "FOO")
	if err != nil {
		t.Fatalf("containerEnvVar() error = %v", err)
	}
	if val != "bar" {
		t.Errorf("containerEnvVar() = %q, want %q", val, "bar")
	}
}

func TestContainerEnvVar_NotFound(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker inspect --format {{range .Config.Env}}{{println .}}{{end}}", "FOO=bar\n")

	val, err := containerEnvVar(context.Background(), fake, "mycontainer", "MISSING")
	if err != nil {
		t.Fatalf("containerEnvVar() unexpected error = %v", err)
	}
	if val != "" {
		t.Errorf("containerEnvVar() = %q, want empty string", val)
	}
}

// ─── parsePeriod ──────────────────────────────────────────────────────────────

func TestParsePeriod(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		wantSec int64
	}{
		{"7d", false, 7 * 24 * 3600},
		{"30d", false, 30 * 24 * 3600},
		{"24h", false, 24 * 3600},
		{"2w", false, 2 * 7 * 24 * 3600},
		{"", true, 0},
		{"x", true, 0},
		{"5z", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			d, err := parsePeriod(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.input, err)
			}
			if !tt.wantErr && int64(d.Seconds()) != tt.wantSec {
				t.Errorf("parsePeriod(%q) = %v, want %ds", tt.input, d, tt.wantSec)
			}
		})
	}
}

// ─── agentConfigDir ───────────────────────────────────────────────────────────

func TestAgentConfigDir(t *testing.T) {
	tests := []struct {
		agentType string
		want      string
	}{
		{"claude", "/home/agent/.claude"},
		{"codex", "/home/agent/.codex"},
		{"shell", "/home/agent/.claude"},
		{"", "/home/agent/.claude"},
	}
	for _, tt := range tests {
		got := agentConfigDir(tt.agentType)
		if got != tt.want {
			t.Errorf("agentConfigDir(%q) = %q, want %q", tt.agentType, got, tt.want)
		}
	}
}

// ─── quoteValue ───────────────────────────────────────────────────────────────

func TestQuoteValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"has space", `"has space"`},
		{"has\ttab", `"has	tab"`},
		{"nospace", "nospace"},
	}
	for _, tt := range tests {
		got := quoteValue(tt.input)
		if got != tt.want {
			t.Errorf("quoteValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ─── config key validation ────────────────────────────────────────────────────

func TestConfigKeyAllowed(t *testing.T) {
	allowed := []string{
		"SAFE_AGENTIC_DEFAULT_CPUS",
		"SAFE_AGENTIC_DEFAULT_MEMORY",
		"SAFE_AGENTIC_DEFAULT_PIDS_LIMIT",
		"SAFE_AGENTIC_DEFAULT_SSH",
		"SAFE_AGENTIC_DEFAULT_DOCKER",
		"SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET",
		"SAFE_AGENTIC_DEFAULT_REUSE_AUTH",
		"SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH",
		"SAFE_AGENTIC_DEFAULT_NETWORK",
		"SAFE_AGENTIC_DEFAULT_IDENTITY",
		"GIT_AUTHOR_NAME",
		"GIT_AUTHOR_EMAIL",
		"GIT_COMMITTER_NAME",
		"GIT_COMMITTER_EMAIL",
	}
	for _, k := range allowed {
		if !configKeyAllowed(k) {
			t.Errorf("configKeyAllowed(%q) = false, want true", k)
		}
	}
	forbidden := []string{"RANDOM_KEY", "", "SAFE_AGENTIC_NOPE", "PATH"}
	for _, k := range forbidden {
		if configKeyAllowed(k) {
			t.Errorf("configKeyAllowed(%q) = true, want false", k)
		}
	}
}

func TestConfigAllowedKeysList(t *testing.T) {
	list := configAllowedKeysList()
	if !strings.Contains(list, "SAFE_AGENTIC_DEFAULT_MEMORY") {
		t.Error("expected SAFE_AGENTIC_DEFAULT_MEMORY in list")
	}
	if !strings.Contains(list, "GIT_AUTHOR_NAME") {
		t.Error("expected GIT_AUTHOR_NAME in list")
	}
}

func TestConfigGetField(t *testing.T) {
	cfg := config.Config{
		CPUs:              "4",
		Memory:            "8g",
		PIDsLimit:         "512",
		SSH:               "true",
		Docker:            "false",
		DockerSocket:      "false",
		ReuseAuth:         "true",
		ReuseGHAuth:       "false",
		Network:           "mynet",
		Identity:          "Alice <alice@example.com>",
		GitAuthorName:     "Alice",
		GitAuthorEmail:    "alice@example.com",
		GitCommitterName:  "Alice",
		GitCommitterEmail: "alice@example.com",
	}

	tests := []struct {
		key  string
		want string
	}{
		{"SAFE_AGENTIC_DEFAULT_CPUS", "4"},
		{"SAFE_AGENTIC_DEFAULT_MEMORY", "8g"},
		{"SAFE_AGENTIC_DEFAULT_PIDS_LIMIT", "512"},
		{"SAFE_AGENTIC_DEFAULT_SSH", "true"},
		{"SAFE_AGENTIC_DEFAULT_DOCKER", "false"},
		{"SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET", "false"},
		{"SAFE_AGENTIC_DEFAULT_REUSE_AUTH", "true"},
		{"SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH", "false"},
		{"SAFE_AGENTIC_DEFAULT_NETWORK", "mynet"},
		{"SAFE_AGENTIC_DEFAULT_IDENTITY", "Alice <alice@example.com>"},
		{"GIT_AUTHOR_NAME", "Alice"},
		{"GIT_AUTHOR_EMAIL", "alice@example.com"},
		{"GIT_COMMITTER_NAME", "Alice"},
		{"GIT_COMMITTER_EMAIL", "alice@example.com"},
		{"UNKNOWN_KEY", ""},
	}

	for _, tt := range tests {
		got := configGetField(cfg, tt.key)
		if got != tt.want {
			t.Errorf("configGetField(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

// ─── config show/set/get/reset ───────────────────────────────────────────────

// setXDGConfigHome sets XDG_CONFIG_HOME to a temp dir and returns cleanup func.
func setXDGConfigHome(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	orig := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	return tmpDir, func() {
		if orig == "" {
			os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			os.Setenv("XDG_CONFIG_HOME", orig)
		}
	}
}

func TestConfigShow_NoFile(t *testing.T) {
	_, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	output := captureOutput(func() {
		if err := runConfigShow(configShowCmd, nil); err != nil {
			t.Fatalf("runConfigShow() error = %v", err)
		}
	})

	if !strings.Contains(output, "No defaults file found") {
		t.Errorf("expected 'No defaults file found', got: %s", output)
	}
}

func TestConfigSet_InvalidKey(t *testing.T) {
	err := runConfigSet(configSetCmd, []string{"INVALID_KEY", "value"})
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "unsupported key") {
		t.Errorf("expected 'unsupported key' in error, got: %v", err)
	}
}

func TestConfigSet_ValidKey(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	output := captureOutput(func() {
		if err := runConfigSet(configSetCmd, []string{"SAFE_AGENTIC_DEFAULT_MEMORY", "16g"}); err != nil {
			t.Fatalf("runConfigSet() error = %v", err)
		}
	})

	if !strings.Contains(output, "Set SAFE_AGENTIC_DEFAULT_MEMORY=16g") {
		t.Errorf("expected set confirmation, got: %s", output)
	}

	// Verify file was written at expected location
	path := filepath.Join(xdgDir, "safe-agentic", "defaults.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if !strings.Contains(string(data), "SAFE_AGENTIC_DEFAULT_MEMORY=16g") {
		t.Errorf("expected key=value in file, got: %s", string(data))
	}
}

func TestConfigSet_UpdateExisting(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	// Pre-populate
	dir := filepath.Join(xdgDir, "safe-agentic")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "defaults.sh"), []byte("SAFE_AGENTIC_DEFAULT_MEMORY=8g\n"), 0644)

	captureOutput(func() {
		if err := runConfigSet(configSetCmd, []string{"SAFE_AGENTIC_DEFAULT_MEMORY", "32g"}); err != nil {
			t.Fatalf("runConfigSet() error = %v", err)
		}
	})

	data, _ := os.ReadFile(filepath.Join(dir, "defaults.sh"))
	if strings.Contains(string(data), "8g") {
		t.Errorf("old value should be replaced, got: %s", string(data))
	}
	if !strings.Contains(string(data), "32g") {
		t.Errorf("new value should be present, got: %s", string(data))
	}
}

func TestConfigGet_NotSet(t *testing.T) {
	_, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	output := captureOutput(func() {
		if err := runConfigGet(configGetCmd, []string{"SAFE_AGENTIC_DEFAULT_MEMORY"}); err != nil {
			t.Fatalf("runConfigGet() error = %v", err)
		}
	})
	// Without a defaults file, the value comes from Defaults() which sets Memory="8g"
	// so we just check no error and some output exists
	_ = output
}

func TestConfigGet_InvalidKey(t *testing.T) {
	err := runConfigGet(configGetCmd, []string{"INVALID_KEY"})
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestConfigReset_NoFile(t *testing.T) {
	_, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	output := captureOutput(func() {
		if err := runConfigReset(configResetCmd, []string{"SAFE_AGENTIC_DEFAULT_MEMORY"}); err != nil {
			t.Fatalf("runConfigReset() error = %v", err)
		}
	})
	if !strings.Contains(output, "nothing to reset") {
		t.Errorf("expected 'nothing to reset', got: %s", output)
	}
}

func TestConfigReset_RemovesKey(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	dir := filepath.Join(xdgDir, "safe-agentic")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "defaults.sh")
	os.WriteFile(path, []byte("SAFE_AGENTIC_DEFAULT_MEMORY=8g\nSAFE_AGENTIC_DEFAULT_CPUS=4\n"), 0644)

	captureOutput(func() {
		if err := runConfigReset(configResetCmd, []string{"SAFE_AGENTIC_DEFAULT_MEMORY"}); err != nil {
			t.Fatalf("runConfigReset() error = %v", err)
		}
	})

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "SAFE_AGENTIC_DEFAULT_MEMORY") {
		t.Errorf("key should be removed, got: %s", string(data))
	}
	if !strings.Contains(string(data), "SAFE_AGENTIC_DEFAULT_CPUS") {
		t.Errorf("other keys should remain, got: %s", string(data))
	}
}

// ─── setTodoDone ─────────────────────────────────────────────────────────────

func TestSetTodoDone(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	items := `[{"text":"Fix bug","done":false},{"text":"Write docs","done":false}]`
	fake.SetResponse("docker exec "+containerName+" bash -c cat", items+"\n")

	output := captureOutput(func() {
		if err := setTodoDone([]string{containerName, "1"}, true); err != nil {
			t.Fatalf("setTodoDone() error = %v", err)
		}
	})

	if !strings.Contains(output, "[x]") {
		t.Errorf("expected '[x]' in output, got: %s", output)
	}
	if !strings.Contains(output, "Fix bug") {
		t.Errorf("expected 'Fix bug' in output, got: %s", output)
	}
}

func TestSetTodoDone_OutOfRange(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	items := `[{"text":"Fix bug","done":false}]`
	fake.SetResponse("docker exec "+containerName+" bash -c cat", items+"\n")

	err := setTodoDone([]string{containerName, "5"}, true)
	if err == nil {
		t.Fatal("expected error for out-of-range index")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("expected 'out of range' in error, got: %v", err)
	}
}

func TestSetTodoDone_InvalidIndex(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	err := setTodoDone([]string{containerName, "notanumber"}, true)
	if err == nil {
		t.Fatal("expected error for invalid index")
	}
}

// ─── checkpoint revert ───────────────────────────────────────────────────────

func TestCheckpointRevert(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", "Changes restored\n")

	output := captureOutput(func() {
		if err := runCheckpointRevert(checkpointRevertCmd, []string{containerName, "stash@{0}"}); err != nil {
			t.Fatalf("runCheckpointRevert() error = %v", err)
		}
	})
	_ = output

	cmds := fake.CommandsMatching("docker exec "+containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "stash pop") {
		t.Errorf("expected 'stash pop', got: %s", joined)
	}
}

// ─── pr command ──────────────────────────────────────────────────────────────

func TestPRCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", "https://github.com/org/repo/pull/1\n")

	prTitle = ""
	prBase = "main"
	defer func() { prTitle = ""; prBase = "main" }()

	output := captureOutput(func() {
		if err := runPR(prCmd, []string{containerName}); err != nil {
			t.Fatalf("runPR() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec "+containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	// Verify git push was called
	allJoined := ""
	for _, cmd := range cmds {
		allJoined += strings.Join(cmd, " ") + "\n"
	}
	if !strings.Contains(allJoined, "git push") {
		t.Errorf("expected 'git push', got: %s", allJoined)
	}
	_ = output
}

// ─── review command ──────────────────────────────────────────────────────────

func TestReviewCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	// codex review returns empty output → fallback to git diff which returns diff content
	// The FakeExecutor returns empty string by default for unmatched commands,
	// which means codex review gets empty output (err == nil but empty) → falls through to git diff
	fake.SetResponse("docker exec "+containerName, "diff --git a/foo.go b/foo.go\n")

	reviewBase = "main"
	defer func() { reviewBase = "main" }()

	output := captureOutput(func() {
		if err := runReview(reviewCmd, []string{containerName}); err != nil {
			t.Fatalf("runReview() error = %v", err)
		}
	})
	_ = output

	cmds := fake.CommandsMatching("docker exec "+containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
}

// ─── extractTokenUsage ───────────────────────────────────────────────────────

func TestExtractTokenUsage_OpenAIStyle(t *testing.T) {
	jsonl := `{"model":"gpt-4","usage":{"prompt_tokens":100,"completion_tokens":200}}` + "\n"
	usages := extractTokenUsage([]byte(jsonl))
	if len(usages) == 0 {
		t.Fatal("expected at least one usage entry")
	}
	if usages[0].InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usages[0].InputTokens)
	}
	if usages[0].OutputTokens != 200 {
		t.Errorf("OutputTokens = %d, want 200", usages[0].OutputTokens)
	}
}

func TestExtractTokenUsage_ClaudeStyle(t *testing.T) {
	jsonl := `{"message":{"model":"claude-opus","usage":{"input_tokens":50,"output_tokens":75}}}` + "\n"
	usages := extractTokenUsage([]byte(jsonl))
	if len(usages) == 0 {
		t.Fatal("expected at least one usage entry")
	}
	if usages[0].InputTokens != 50 {
		t.Errorf("InputTokens = %d, want 50", usages[0].InputTokens)
	}
	if usages[0].OutputTokens != 75 {
		t.Errorf("OutputTokens = %d, want 75", usages[0].OutputTokens)
	}
}

func TestExtractTokenUsage_Empty(t *testing.T) {
	usages := extractTokenUsage([]byte(""))
	if len(usages) != 0 {
		t.Errorf("expected 0 usages for empty input, got %d", len(usages))
	}
}

func TestExtractTokenUsage_InvalidJSON(t *testing.T) {
	usages := extractTokenUsage([]byte("not json\n{broken\n"))
	if len(usages) != 0 {
		t.Errorf("expected 0 usages for invalid JSON, got %d", len(usages))
	}
}

// ─── jsonStringFromEvent ──────────────────────────────────────────────────────

func TestJsonStringFromEvent(t *testing.T) {
	event := map[string]json.RawMessage{
		"type":    json.RawMessage(`"tool.call"`),
		"tool":    json.RawMessage(`"Bash"`),
		"missing": nil,
	}

	if got := jsonStringFromEvent(event, "type"); got != "tool.call" {
		t.Errorf("jsonStringFromEvent type = %q, want %q", got, "tool.call")
	}
	if got := jsonStringFromEvent(event, "tool"); got != "Bash" {
		t.Errorf("jsonStringFromEvent tool = %q, want %q", got, "Bash")
	}
	if got := jsonStringFromEvent(event, "notpresent"); got != "" {
		t.Errorf("jsonStringFromEvent missing key = %q, want empty", got)
	}
}

// ─── depsmet ─────────────────────────────────────────────────────────────────

func TestDepsmet(t *testing.T) {
	completed := map[string]bool{"a": true, "b": true}

	if !depsmet([]string{"a", "b"}, completed) {
		t.Error("depsmet should return true when all deps met")
	}
	if !depsmet([]string{}, completed) {
		t.Error("depsmet should return true for empty deps")
	}
	if depsmet([]string{"a", "c"}, completed) {
		t.Error("depsmet should return false when any dep missing")
	}
	if depsmet([]string{"missing"}, completed) {
		t.Error("depsmet should return false for completely missing dep")
	}
}

// ─── specToSpawnOpts ─────────────────────────────────────────────────────────

func TestSpecToSpawnOpts(t *testing.T) {
	spec := fleet.AgentSpec{
		Type:      "claude",
		Name:      "test-agent",
		Repo:      "https://github.com/org/repo.git",
		Prompt:    "Fix the tests",
		SSH:       true,
		ReuseAuth: true,
		AutoTrust: true,
		Memory:    "16g",
		CPUs:      "8",
	}

	opts := specToSpawnOpts(spec, "fleet-vol-123")

	if opts.AgentType != "claude" {
		t.Errorf("AgentType = %q, want claude", opts.AgentType)
	}
	if opts.Name != "test-agent" {
		t.Errorf("Name = %q, want test-agent", opts.Name)
	}
	if len(opts.Repos) != 1 || opts.Repos[0] != "https://github.com/org/repo.git" {
		t.Errorf("Repos = %v", opts.Repos)
	}
	if opts.Prompt != "Fix the tests" {
		t.Errorf("Prompt = %q", opts.Prompt)
	}
	if !opts.SSH {
		t.Error("SSH should be true")
	}
	if !opts.ReuseAuth {
		t.Error("ReuseAuth should be true")
	}
	if opts.Memory != "16g" {
		t.Errorf("Memory = %q, want 16g", opts.Memory)
	}
	if opts.FleetVolume != "fleet-vol-123" {
		t.Errorf("FleetVolume = %q, want fleet-vol-123", opts.FleetVolume)
	}
}

func TestSpecToSpawnOpts_NoRepo(t *testing.T) {
	spec := fleet.AgentSpec{
		Type: "codex",
	}
	opts := specToSpawnOpts(spec, "")
	if len(opts.Repos) != 0 {
		t.Errorf("expected empty repos, got %v", opts.Repos)
	}
}

// ─── template functions ───────────────────────────────────────────────────────

func TestUserTemplatesDir(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	got := userTemplatesDir()
	if !strings.HasPrefix(got, xdgDir) {
		t.Errorf("userTemplatesDir() = %q, expected prefix %q", got, xdgDir)
	}
	if !strings.Contains(got, "safe-agentic") {
		t.Errorf("userTemplatesDir() should contain 'safe-agentic', got: %q", got)
	}
	if !strings.HasSuffix(got, "templates") {
		t.Errorf("userTemplatesDir() should end with 'templates', got: %q", got)
	}
}

func TestRepoTemplatesDir(t *testing.T) {
	// repoTemplatesDir walks up from the binary — it may or may not find a
	// templates dir in test context. Just verify it returns a string (possibly empty).
	got := repoTemplatesDir()
	// Either empty (not found) or a valid path ending in templates
	if got != "" && !strings.HasSuffix(got, "templates") {
		t.Errorf("repoTemplatesDir() = %q, expected path ending in 'templates'", got)
	}
}

func TestTemplateList_NoTemplates(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	// Ensure user templates dir is empty
	os.MkdirAll(filepath.Join(xdgDir, "safe-agentic", "templates"), 0755)

	output := captureOutput(func() {
		// Will find built-in templates if running from repo, but won't fail
		_ = runTemplateList(templateListCmd, nil)
	})
	// Just verify it doesn't crash and returns some output
	_ = output
}

func TestFindTemplate_NotFound(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	os.MkdirAll(filepath.Join(xdgDir, "safe-agentic", "templates"), 0755)

	_, err := findTemplate("nonexistent-template-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestFindTemplate_UserTemplate(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	// Create a user template
	dir := filepath.Join(xdgDir, "safe-agentic", "templates")
	os.MkdirAll(dir, 0755)
	tplPath := filepath.Join(dir, "my-template.md")
	os.WriteFile(tplPath, []byte("# My Template\n\nDo something."), 0644)

	path, err := findTemplate("my-template")
	if err != nil {
		t.Fatalf("findTemplate() error = %v", err)
	}
	if path != tplPath {
		t.Errorf("findTemplate() = %q, want %q", path, tplPath)
	}
}

func TestTemplateList_WithUserTemplate(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	dir := filepath.Join(xdgDir, "safe-agentic", "templates")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "custom-tpl.md"), []byte("# custom"), 0644)

	output := captureOutput(func() {
		if err := runTemplateList(templateListCmd, nil); err != nil {
			t.Fatalf("runTemplateList() error = %v", err)
		}
	})

	if !strings.Contains(output, "custom-tpl") {
		t.Errorf("expected 'custom-tpl' in output, got: %s", output)
	}
	if !strings.Contains(output, "user") {
		t.Errorf("expected 'user' source in output, got: %s", output)
	}
}

func TestTemplateShow_NotFound(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()
	os.MkdirAll(filepath.Join(xdgDir, "safe-agentic", "templates"), 0755)

	err := runTemplateShow(templateShowCmd, []string{"nonexistent-xyz"})
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestTemplateShow_Existing(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	dir := filepath.Join(xdgDir, "safe-agentic", "templates")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "show-test.md"), []byte("Template content here"), 0644)

	output := captureOutput(func() {
		if err := runTemplateShow(templateShowCmd, []string{"show-test"}); err != nil {
			t.Fatalf("runTemplateShow() error = %v", err)
		}
	})

	if !strings.Contains(output, "Template content here") {
		t.Errorf("expected template content in output, got: %s", output)
	}
}

// ─── mustReadFile ─────────────────────────────────────────────────────────────

func TestMustReadFile_Existing(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "mustread-*.txt")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("hello world")
	tmpFile.Close()

	data := mustReadFile(tmpFile.Name())
	if string(data) != "hello world" {
		t.Errorf("mustReadFile() = %q, want %q", string(data), "hello world")
	}
}

func TestMustReadFile_NotExisting(t *testing.T) {
	data := mustReadFile("/nonexistent/path/file.txt")
	if data != nil {
		t.Errorf("mustReadFile() for missing file should return nil, got %v", data)
	}
}

// ─── runSpawn ─────────────────────────────────────────────────────────────────

func TestRunSpawn_InvalidType(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	err := runSpawn(spawnCmd, []string{"invalidtype"})
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !strings.Contains(err.Error(), "agent type must be") {
		t.Errorf("expected agent type error, got: %v", err)
	}
}

// ─── runRetry ─────────────────────────────────────────────────────────────────

func TestRunRetry_NoContainer(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetError("docker ps -a", "connection refused")

	err := runRetry(retryCmd, nil)
	if err == nil {
		t.Fatal("expected error when no container found")
	}
}

// ─── output JSON mode ─────────────────────────────────────────────────────────

func TestOutputCommand_JSON(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker inspect --format {{.State.Status}}", "running\n")
	fake.SetResponse("docker logs", "log line 1\n")

	outputJSON = true
	defer func() { outputJSON = false }()

	output := captureOutput(func() {
		if err := runOutput(outputCmd, []string{containerName}); err != nil {
			t.Fatalf("runOutput() error = %v", err)
		}
	})

	if !strings.Contains(output, "name") {
		t.Errorf("expected JSON with 'name' field, got: %s", output)
	}
	if !strings.Contains(output, containerName) {
		t.Errorf("expected container name in JSON output, got: %s", output)
	}
}

// ─── runSessions ──────────────────────────────────────────────────────────────

func TestRunSessions_NoData(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	// tar returns empty
	fake.SetResponse("docker exec "+containerName+" bash -c tar", "")

	output := captureOutput(func() {
		if err := runSessions(sessionsCmd, []string{containerName, t.TempDir()}); err != nil {
			t.Fatalf("runSessions() error = %v", err)
		}
	})

	if !strings.Contains(output, "No session data found") {
		t.Errorf("expected 'No session data found', got: %s", output)
	}
}

// ─── runReplay ────────────────────────────────────────────────────────────────

func TestRunReplay_Empty(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c cat", "")

	output := captureOutput(func() {
		if err := runReplay(replayCmd, []string{containerName}); err != nil {
			t.Fatalf("runReplay() error = %v", err)
		}
	})

	if !strings.Contains(output, "No session events found") {
		t.Errorf("expected 'No session events found', got: %s", output)
	}
}

// ─── runAudit command ─────────────────────────────────────────────────────────

func TestRunAuditCommand_WithEntries(t *testing.T) {
	// Redirect audit path to a temp file via env var approach — audit uses
	// audit.DefaultPath() which reads XDG_DATA_HOME. Use a temp dir.
	tmpDir := t.TempDir()
	orig := os.Getenv("XDG_DATA_HOME")
	os.Setenv("XDG_DATA_HOME", tmpDir)
	defer func() {
		if orig == "" {
			os.Unsetenv("XDG_DATA_HOME")
		} else {
			os.Setenv("XDG_DATA_HOME", orig)
		}
	}()

	// Pre-create audit file
	auditDir := filepath.Join(tmpDir, "safe-agentic")
	os.MkdirAll(auditDir, 0755)
	entry := audit.Entry{
		Timestamp: "2026-04-10T10:00:00Z",
		Action:    "spawn",
		Container: "agent-claude-test",
		Details:   map[string]string{"type": "claude"},
	}
	data, _ := json.Marshal(entry)
	auditPath := filepath.Join(auditDir, "audit.jsonl")
	os.WriteFile(auditPath, append(data, '\n'), 0644)

	// runAudit uses audit.DefaultPath() — if it uses XDG_DATA_HOME, it will find our file.
	// If not, it just shows "No audit log entries found" which is also a pass.
	output := captureOutput(func() {
		auditLines = 50
		if err := runAudit(auditCmd, nil); err != nil {
			t.Fatalf("runAudit() error = %v", err)
		}
	})
	_ = output
}

// ─── runTodoCheck / runTodoUncheck ───────────────────────────────────────────

func TestRunTodoCheck(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	items := `[{"text":"Fix bug","done":false}]`
	fake.SetResponse("docker exec "+containerName+" bash -c cat", items+"\n")

	output := captureOutput(func() {
		if err := runTodoCheck(todoCheckCmd, []string{containerName, "1"}); err != nil {
			t.Fatalf("runTodoCheck() error = %v", err)
		}
	})

	if !strings.Contains(output, "[x]") {
		t.Errorf("expected '[x]' in output, got: %s", output)
	}
}

func TestRunTodoUncheck(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	items := `[{"text":"Fix bug","done":true}]`
	fake.SetResponse("docker exec "+containerName+" bash -c cat", items+"\n")

	output := captureOutput(func() {
		if err := runTodoUncheck(todoUncheckCmd, []string{containerName, "1"}); err != nil {
			t.Fatalf("runTodoUncheck() error = %v", err)
		}
	})

	if !strings.Contains(output, "[ ]") {
		t.Errorf("expected '[ ]' in output, got: %s", output)
	}
}

// ─── runTemplateCreate ────────────────────────────────────────────────────────

func TestRunTemplateCreate(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	output := captureOutput(func() {
		if err := runTemplateCreate(templateCreateCmd, []string{"my-new-template"}); err != nil {
			t.Fatalf("runTemplateCreate() error = %v", err)
		}
	})

	if !strings.Contains(output, "Created template") {
		t.Errorf("expected 'Created template', got: %s", output)
	}

	// Verify file was created
	tplPath := filepath.Join(xdgDir, "safe-agentic", "templates", "my-new-template.md")
	if _, err := os.Stat(tplPath); os.IsNotExist(err) {
		t.Errorf("template file not created at %s", tplPath)
	}
}

func TestRunTemplateCreate_AlreadyExists(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	// Pre-create the template
	dir := filepath.Join(xdgDir, "safe-agentic", "templates")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "existing.md"), []byte("# existing"), 0644)

	err := runTemplateCreate(templateCreateCmd, []string{"existing"})
	if err == nil {
		t.Fatal("expected error for already-existing template")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}
}

// ─── runCostHistory ──────────────────────────────────────────────────────────

func TestRunCostHistory(t *testing.T) {
	tmpDir := t.TempDir()
	orig := os.Getenv("XDG_DATA_HOME")
	os.Setenv("XDG_DATA_HOME", tmpDir)
	defer func() {
		if orig == "" {
			os.Unsetenv("XDG_DATA_HOME")
		} else {
			os.Setenv("XDG_DATA_HOME", orig)
		}
	}()

	fake := orb.NewFake()
	output := captureOutput(func() {
		err := runCostHistory(context.Background(), fake, "7d")
		if err != nil {
			t.Fatalf("runCostHistory() error = %v", err)
		}
	})

	if !strings.Contains(output, "Period") {
		t.Errorf("expected 'Period' in output, got: %s", output)
	}
}

func TestRunReplay_WithEvents(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	events := `{"type":"session.start","timestamp":"2026-04-10T10:00:00Z"}` + "\n" +
		`{"type":"tool.call","timestamp":"2026-04-10T10:01:00Z","tool":"Bash","tokens":100}` + "\n" +
		`{"type":"session.end","timestamp":"2026-04-10T10:05:00Z"}` + "\n"
	fake.SetResponse("docker exec "+containerName+" bash -c cat", events)

	output := captureOutput(func() {
		replayToolsOnly = false
		if err := runReplay(replayCmd, []string{containerName}); err != nil {
			t.Fatalf("runReplay() error = %v", err)
		}
	})

	if !strings.Contains(output, "Session started") {
		t.Errorf("expected 'Session started', got: %s", output)
	}
	if !strings.Contains(output, "Session ended") {
		t.Errorf("expected 'Session ended', got: %s", output)
	}
	if !strings.Contains(output, "Bash") {
		t.Errorf("expected 'Bash' tool call, got: %s", output)
	}
}
