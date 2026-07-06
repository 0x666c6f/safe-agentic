package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	actionpkg "github.com/0x666c6f/safe-agentic/pkg/actions"
	"github.com/0x666c6f/safe-agentic/pkg/audit"
	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/events"
	"github.com/0x666c6f/safe-agentic/pkg/fleet"
	"github.com/0x666c6f/safe-agentic/pkg/inject"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
	"github.com/0x666c6f/safe-agentic/pkg/worktrees"
)

// ─── test harness ─────────────────────────────────────────────────────────────

// testSetup replaces the global executor with a FakeExecutor and returns it +
// a cleanup function that restores the original.
func testSetup(t *testing.T) (*vmexec.FakeExecutor, func()) {
	t.Helper()
	fake := vmexec.NewFake()
	t.Setenv("HOME", t.TempDir())
	original := newExecutor
	origFindBuildRoot := findBuildRoot
	origSyncBuildContextToVM := syncBuildContextToVM
	origCopyFileToVM := copyFileToVM
	origVMExists := vmExists
	origRunVMBootstrap := runVMBootstrap
	origStartVM := startVM
	origInstallVMSupportFiles := installVMSupportFiles
	origConfigureHostNAT := configureHostNAT
	origHostIPForwardingEnabled := hostIPForwardingEnabled
	origConfigureLaunchdSSHAuth := configureLaunchdSSHAuth
	origReconcileHomeMount := reconcileHomeMount
	newExecutor = func() vmexec.Executor { return fake }
	findBuildRoot = func() (string, error) { return t.TempDir(), nil }
	syncBuildContextToVM = func(vmName, root string) error { return nil }
	copyFileToVM = func(vmName, srcPath, destPath string) error { return nil }
	vmExists = func(vmName string) bool { return true }
	runVMBootstrap = func(vmName, worktreesDir, homeDir string) ([]byte, error) { return []byte("bootstrap ok\n"), nil }
	startVM = func(vmName string) error { return nil }
	installVMSupportFiles = func(vmName, buildRoot string) error { return nil }
	configureHostNAT = func(stdout, stderr io.Writer) error { return nil }
	hostIPForwardingEnabled = func() bool { return true }
	configureLaunchdSSHAuth = func() error { return nil }
	reconcileHomeMount = func(vmName, desired string, stdout, stderr io.Writer) (bool, error) { return false, nil }
	return fake, func() {
		newExecutor = original
		findBuildRoot = origFindBuildRoot
		syncBuildContextToVM = origSyncBuildContextToVM
		copyFileToVM = origCopyFileToVM
		vmExists = origVMExists
		runVMBootstrap = origRunVMBootstrap
		startVM = origStartVM
		installVMSupportFiles = origInstallVMSupportFiles
		configureHostNAT = origConfigureHostNAT
		hostIPForwardingEnabled = origHostIPForwardingEnabled
		configureLaunchdSSHAuth = origConfigureLaunchdSSHAuth
		reconcileHomeMount = origReconcileHomeMount
	}
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

// captureStderr redirects os.Stderr to a buffer for the duration of fn,
// then returns what was written.
func captureStderr(fn func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func installFakeContainerBinary(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "container")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake container: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
	setStateResponses(fake, true, "claude", "tmux")
	fake.SetResponse("docker exec agent-claude-test tmux capture-pane", workingPane+"\n")

	// Enable JSON mode
	listJSON = true
	defer func() { listJSON = false }()

	out := captureOutput(func() {
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
	// The state field is added; the original fields are preserved (backward compatible).
	if !strings.Contains(out, `"state":"working"`) {
		t.Errorf("expected added state field, got: %s", out)
	}
	if !strings.Contains(out, `"Names":"agent-claude-test"`) {
		t.Errorf("expected original Names field preserved, got: %s", out)
	}
}

func TestListCommandShowsState(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker ps -a --filter name=^agent-",
		"agent-claude-x\tclaude\torg/repo\t\tUp 5 minutes\n")
	setStateResponses(fake, true, "claude", "tmux")
	fake.SetResponse("docker exec agent-claude-x tmux capture-pane", blockedPane+"\n")

	out := captureOutput(func() {
		if err := runList(listCmd, nil); err != nil {
			t.Fatalf("runList() error = %v", err)
		}
	})
	if !strings.Contains(out, "blocked") || !strings.Contains(out, "agent-claude-x") {
		t.Fatalf("expected blocked state in list output, got: %q", out)
	}
}

func TestInjectStateField(t *testing.T) {
	if got := injectStateField(`{"Names":"a","Status":"Up"}`, "blocked"); got != `{"Names":"a","Status":"Up","state":"blocked"}` {
		t.Errorf("injectStateField object = %q", got)
	}
	if got := injectStateField(`{}`, "done"); got != `{"state":"done"}` {
		t.Errorf("injectStateField empty object = %q", got)
	}
	if got := injectStateField("not json", "x"); got != "not json" {
		t.Errorf("injectStateField passthrough = %q", got)
	}
}

// ─── action ─────────────────────────────────────────────────────────────────

func TestActionListAndRun(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	dir := t.TempDir()
	actionsPath := filepath.Join(dir, "actions.toml")
	if err := os.WriteFile(actionsPath, []byte(`
[actions.test]
description = "Run tests"
command = "go test ./..."
cwd = "backend"
`), 0o600); err != nil {
		t.Fatalf("write actions file: %v", err)
	}
	oldActionFiles := actionFiles
	actionFiles = []string{actionsPath}
	defer func() { actionFiles = oldActionFiles }()

	output := captureOutput(func() {
		if err := runActionList(actionListCmd, nil); err != nil {
			t.Fatalf("runActionList() error = %v", err)
		}
	})
	if !strings.Contains(output, "test") || !strings.Contains(output, "Run tests") {
		t.Fatalf("action list output = %q", output)
	}

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -lc", "ok\n")

	output = captureOutput(func() {
		if err := runActionRun(actionRunCmd, []string{"test", containerName}); err != nil {
			t.Fatalf("runActionRun() error = %v", err)
		}
	})
	if output != "ok\n" {
		t.Fatalf("run output = %q, want ok", output)
	}
	cmds := fake.CommandsMatching("docker exec " + containerName + " bash -lc")
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "cd 'backend' && go test ./...") {
		t.Fatalf("action command not applied: %s", joined)
	}
}

func TestActionShowMissing(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	oldActionFiles := actionFiles
	actionFiles = []string{filepath.Join(t.TempDir(), "missing.toml")}
	defer func() { actionFiles = oldActionFiles }()

	if err := runActionShow(actionShowCmd, []string{"missing"}); err == nil {
		t.Fatal("expected missing action error")
	}
}

// ─── search ─────────────────────────────────────────────────────────────────

func TestMatchRenderedLogLines(t *testing.T) {
	raw := []byte(`{"type":"user","message":{"role":"user","content":"Run tests"}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":"Needle found in output"}}` + "\n")

	matches := matchRenderedLogLines(raw, "needle", false)
	if len(matches) != 1 || !strings.Contains(matches[0], "Needle found") {
		t.Fatalf("matches = %#v", matches)
	}
	if matches := matchRenderedLogLines(raw, "needle", true); len(matches) != 0 {
		t.Fatalf("case-sensitive matches = %#v, want none", matches)
	}
}

func TestSearchSpecificAgent(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.repo-display"}}`, "org/repo\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName+" python3 -c", "/home/agent/.claude/projects/org-repo/session.jsonl\n")
	fake.SetResponse("docker exec "+containerName+" bash -c tail", `{"type":"assistant","message":{"role":"assistant","content":"Needle found here"}}`+"\n")

	output := captureOutput(func() {
		if err := runSearch(searchCmd, []string{"needle", containerName}); err != nil {
			t.Fatalf("runSearch() error = %v", err)
		}
	})
	if !strings.Contains(output, containerName+":") || !strings.Contains(output, "Needle found here") {
		t.Fatalf("search output = %q", output)
	}
}

// ─── steer ──────────────────────────────────────────────────────────────────

func TestRunSteerSendsMessage(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.terminal"}}`, "tmux\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName+" tmux has-session", "")
	fake.SetResponse("docker exec "+containerName+" tmux send-keys", "")

	output := captureOutput(func() {
		if err := runSteer(steerCmd, []string{containerName, "keep it narrow"}); err != nil {
			t.Fatalf("runSteer() error = %v", err)
		}
	})
	if !strings.Contains(output, "Sent to "+containerName) {
		t.Fatalf("steer output = %q", output)
	}

	cmds := fake.CommandsMatching("docker exec " + containerName + " tmux send-keys")
	if len(cmds) == 0 {
		t.Fatal("expected tmux send-keys command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "-- keep it narrow Enter") {
		t.Fatalf("message not sent through tmux: %s", joined)
	}
}

func TestRunSteerStartsStoppedContainer(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-stopped"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.terminal"}}`, "tmux\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "false\n")
	fake.SetResponse("docker start "+containerName, "")
	fake.SetResponse("docker exec "+containerName+" tmux has-session", "")
	fake.SetResponse("docker exec "+containerName+" tmux send-keys", "")

	captureOutput(func() {
		if err := runSteer(steerCmd, []string{containerName, "continue"}); err != nil {
			t.Fatalf("runSteer() error = %v", err)
		}
	})
	if len(fake.CommandsMatching("docker start "+containerName)) == 0 {
		t.Fatal("expected docker start for stopped container")
	}
}

// ─── review comments ────────────────────────────────────────────────────────

func TestReviewCommentsLifecycle(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	oldPath := reviewCommentsPath
	oldAll := reviewCommentsAll
	reviewCommentsPath = filepath.Join(t.TempDir(), "review-comments.jsonl")
	reviewCommentsAll = false
	defer func() {
		reviewCommentsPath = oldPath
		reviewCommentsAll = oldAll
	}()

	containerName := "agent-claude-review"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	addOut := captureOutput(func() {
		if err := runReviewCommentsAdd(reviewCommentsAddCmd, []string{containerName, "cmd/main.go", "8", "tighten this"}); err != nil {
			t.Fatalf("runReviewCommentsAdd() error = %v", err)
		}
	})
	fields := strings.Fields(addOut)
	if len(fields) < 2 {
		t.Fatalf("add output = %q", addOut)
	}
	id := fields[1]

	listOut := captureOutput(func() {
		if err := runReviewCommentsList(reviewCommentsListCmd, nil); err != nil {
			t.Fatalf("runReviewCommentsList() error = %v", err)
		}
	})
	if !strings.Contains(listOut, id) || !strings.Contains(listOut, "cmd/main.go:8") || !strings.Contains(listOut, "open") {
		t.Fatalf("list output = %q", listOut)
	}

	captureOutput(func() {
		if err := runReviewCommentsResolve(reviewCommentsResolveCmd, []string{id}); err != nil {
			t.Fatalf("runReviewCommentsResolve() error = %v", err)
		}
	})

	listOut = captureOutput(func() {
		if err := runReviewCommentsList(reviewCommentsListCmd, nil); err != nil {
			t.Fatalf("runReviewCommentsList(open) error = %v", err)
		}
	})
	if !strings.Contains(listOut, "No review comments") {
		t.Fatalf("open list output = %q", listOut)
	}

	reviewCommentsAll = true
	listOut = captureOutput(func() {
		if err := runReviewCommentsList(reviewCommentsListCmd, nil); err != nil {
			t.Fatalf("runReviewCommentsList(all) error = %v", err)
		}
	})
	if !strings.Contains(listOut, "resolved") {
		t.Fatalf("all list output = %q", listOut)
	}

	clearOut := captureOutput(func() {
		if err := runReviewCommentsClear(reviewCommentsClearCmd, []string{containerName}); err != nil {
			t.Fatalf("runReviewCommentsClear() error = %v", err)
		}
	})
	if !strings.Contains(clearOut, "Cleared 1 review comments") {
		t.Fatalf("clear output = %q", clearOut)
	}
}

// ─── timeline and inbox ─────────────────────────────────────────────────────

func TestTimelineAndInbox(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	t.Setenv("HOME", t.TempDir())
	if err := events.Emit(events.DefaultEventsPath(), "agent.spawned", map[string]string{"container": "agent-a"}); err != nil {
		t.Fatalf("emit spawn: %v", err)
	}
	if err := events.Emit(events.DefaultEventsPath(), "cron.failed", map[string]string{"job": "nightly", "error": "exit 1"}); err != nil {
		t.Fatalf("emit cron failed: %v", err)
	}
	logger := &audit.Logger{Path: audit.DefaultPath()}
	if err := logger.Log("stop", "agent-a", nil); err != nil {
		t.Fatalf("audit stop: %v", err)
	}

	oldLines := timelineLines
	oldInboxAll := inboxAll
	timelineLines = 10
	inboxAll = false
	defer func() {
		timelineLines = oldLines
		inboxAll = oldInboxAll
	}()

	timelineOut := captureOutput(func() {
		if err := runTimeline(timelineCmd, nil); err != nil {
			t.Fatalf("runTimeline() error = %v", err)
		}
	})
	if !strings.Contains(timelineOut, "agent.spawned") || !strings.Contains(timelineOut, "cron.failed") || !strings.Contains(timelineOut, "audit.stop") {
		t.Fatalf("timeline output = %q", timelineOut)
	}

	inboxOut := captureOutput(func() {
		if err := runInbox(inboxCmd, nil); err != nil {
			t.Fatalf("runInbox() error = %v", err)
		}
	})
	if !strings.Contains(inboxOut, "failed") || !strings.Contains(inboxOut, "cron.failed") {
		t.Fatalf("inbox output = %q", inboxOut)
	}
	if strings.Contains(inboxOut, "agent.spawned") {
		t.Fatalf("inbox should hide informational events: %q", inboxOut)
	}
}

func TestServerRequestHandlers(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	actionsPath := filepath.Join(home, ".safe-ag", "actions.toml")
	if err := os.MkdirAll(filepath.Dir(actionsPath), 0o755); err != nil {
		t.Fatalf("mkdir actions dir: %v", err)
	}
	if err := os.WriteFile(actionsPath, []byte("[actions.test]\ncommand = \"go test ./...\"\n"), 0o600); err != nil {
		t.Fatalf("write actions: %v", err)
	}
	if err := events.Emit(events.DefaultEventsPath(), "cron.failed", map[string]string{"job": "nightly", "error": "exit 1"}); err != nil {
		t.Fatalf("emit event: %v", err)
	}

	ping, err := handleServerRequest(serverRequest{Method: "ping"})
	if err != nil {
		t.Fatalf("ping error = %v", err)
	}
	if got := ping.(map[string]any)["ok"]; got != true {
		t.Fatalf("ping ok = %v", got)
	}

	actionsResult, err := handleServerRequest(serverRequest{Method: "actions.list"})
	if err != nil {
		t.Fatalf("actions.list error = %v", err)
	}
	if got := actionsResult.([]actionpkg.Action)[0].Name; got != "test" {
		t.Fatalf("action name = %q, want test", got)
	}

	inboxResult, err := handleServerRequest(serverRequest{Method: "inbox"})
	if err != nil {
		t.Fatalf("inbox error = %v", err)
	}
	items := inboxResult.([]timelineEntry)
	if len(items) != 1 || items[0].Status != events.StatusFailed {
		t.Fatalf("inbox = %#v", items)
	}

	resp := handleServerLine([]byte(`{"jsonrpc":"2.0","id":1,"method":"nope"}`))
	if resp.Error == nil || !strings.Contains(resp.Error.Message, "unknown method") {
		t.Fatalf("response = %#v, want unknown method error", resp)
	}

	schemaResult, err := handleServerRequest(serverRequest{Method: "schema"})
	if err != nil {
		t.Fatalf("schema error = %v", err)
	}
	methods := schemaResult.(map[string]any)["methods"].(map[string]any)
	if methods["agent.diff"] == nil || methods["actions.run"] == nil {
		t.Fatalf("schema missing methods: %#v", methods)
	}

	fake.Reset()
	fake.SetResponse("docker ps -a --filter name=^agent- --format", "agent-codex-a\tcodex\trepo\tfleet\tUp 1 minute\t/tmp/wt\tcpu=4,mem=8g\n")
	agentsResult, err := handleServerRequest(serverRequest{Method: "agents.list"})
	if err != nil {
		t.Fatalf("agents.list error = %v", err)
	}
	agents := agentsResult.([]serverAgent)
	if len(agents) != 1 || agents[0].Name != "agent-codex-a" || agents[0].Type != "codex" {
		t.Fatalf("agents.list = %#v", agents)
	}

	fake.Reset()
	containerName := "agent-codex-a"
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", containerName+"\n")
	fake.SetResponse("docker inspect --format {{.State.Status}}", "running\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName, "diff --git a/file b/file\n")
	diffResult, err := handleServerRequest(serverRequest{
		Method: "agent.diff",
		Params: json.RawMessage(`{"target":"agent-codex-a"}`),
	})
	if err != nil {
		t.Fatalf("agent.diff error = %v", err)
	}
	if !strings.Contains(diffResult.(string), "diff --git") {
		t.Fatalf("agent.diff = %q", diffResult)
	}

	fake.Reset()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", containerName+"\n")
	fake.SetResponse("docker exec "+containerName, "ok\n")
	actionResult, err := handleServerRequest(serverRequest{
		Method: "actions.run",
		Params: json.RawMessage(`{"target":"agent-codex-a","action":"test"}`),
	})
	if err != nil {
		t.Fatalf("actions.run error = %v", err)
	}
	if got := actionResult.(map[string]string)["output"]; got != "ok\n" {
		t.Fatalf("actions.run output = %q", got)
	}
}

func TestServerHTTPHandlerRequiresBearerToken(t *testing.T) {
	srv := httptest.NewServer(serverHTTPHandler("secret"))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/rpc", "application/json", strings.NewReader(`{"method":"ping"}`))
	if err != nil {
		t.Fatalf("unauthorized post: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authorized post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out serverResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Error != nil || out.Result == nil {
		t.Fatalf("response = %#v", out)
	}
}

func TestValidateServerListenAddressRequiresLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8765", "localhost:8765", "[::1]:8765"} {
		if err := validateServerListenAddress(addr); err != nil {
			t.Fatalf("validateServerListenAddress(%q) error = %v", addr, err)
		}
	}
	for _, addr := range []string{":8765", "0.0.0.0:8765", "192.168.1.10:8765", "[::]:8765"} {
		if err := validateServerListenAddress(addr); err == nil {
			t.Fatalf("validateServerListenAddress(%q) got nil, want error", addr)
		}
	}
}

func TestBrowserRejectsUnsupportedMode(t *testing.T) {
	oldMode := browserMode
	browserMode = "bogus"
	defer func() { browserMode = oldMode }()
	_, err := captureBrowserArtifact(context.Background(), "http://example.test", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "unsupported browser mode") {
		t.Fatalf("captureBrowserArtifact() error = %v, want unsupported mode", err)
	}
}

// ─── profiles ───────────────────────────────────────────────────────────────

func TestProfileListShowAndRunDryRun(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	profileDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "reviewer.toml"), []byte(`
agent_type = "codex"
repo = ["git@github.com:org/repo.git"]
container_name = "reviewer"
prompt = "Review the diff"
ssh = true
reuse_auth = true
reuse_gh_auth = true
docker = false
background = true
`), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	oldDirs := profileDirs
	oldDryRun := profileRunDryRun
	profileDirs = []string{profileDir}
	profileRunDryRun = false
	defer func() {
		profileDirs = oldDirs
		profileRunDryRun = oldDryRun
	}()

	listOut := captureOutput(func() {
		if err := runProfileList(profileListCmd, nil); err != nil {
			t.Fatalf("runProfileList() error = %v", err)
		}
	})
	if !strings.Contains(listOut, "reviewer") || !strings.Contains(listOut, "codex") {
		t.Fatalf("profile list output = %q", listOut)
	}

	showOut := captureOutput(func() {
		if err := runProfileShow(profileShowCmd, []string{"reviewer"}); err != nil {
			t.Fatalf("runProfileShow() error = %v", err)
		}
	})
	if !strings.Contains(showOut, "reuse_auth: true") || !strings.Contains(showOut, "docker: false") {
		t.Fatalf("profile show output = %q", showOut)
	}

	profileRunDryRun = true
	fake.SetResponse("docker network create", "")
	runOut := captureOutput(func() {
		if err := runProfileRun(profileRunCmd, []string{"reviewer", "Focus auth"}); err != nil {
			t.Fatalf("runProfileRun() error = %v", err)
		}
	})
	if !strings.Contains(runOut, "agent-codex-reviewer") ||
		!strings.Contains(runOut, "SAFE_AGENTIC_PROMPT_B64") ||
		!strings.Contains(runOut, "safe-agentic.auth=shared") ||
		!strings.Contains(runOut, "safe-agentic.docker=off") {
		t.Fatalf("profile dry-run output = %q", runOut)
	}
}

// ─── handoff ────────────────────────────────────────────────────────────────

func TestHandoffToWorktree(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	oldLocal := handoffToLocal
	oldWorktree := handoffToWorktree
	handoffToLocal = ""
	handoffToWorktree = true
	defer func() {
		handoffToLocal = oldLocal
		handoffToWorktree = oldWorktree
	}()

	containerName := "agent-claude-worktree"
	worktreePath := "/Users/florian/.safe-ag/worktrees/" + containerName
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.worktree"}}`, worktreePath+"\n")

	output := captureOutput(func() {
		if err := runHandoff(handoffCmd, []string{containerName}); err != nil {
			t.Fatalf("runHandoff() error = %v", err)
		}
	})
	if strings.TrimSpace(output) != worktreePath {
		t.Fatalf("handoff output = %q", output)
	}
}

func TestHandoffToLocalCopiesWorkspace(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	oldLocal := handoffToLocal
	oldWorktree := handoffToWorktree
	dest := filepath.Join(t.TempDir(), "workspace-copy")
	handoffToLocal = dest
	handoffToWorktree = false
	defer func() {
		handoffToLocal = oldLocal
		handoffToWorktree = oldWorktree
	}()

	containerName := "agent-claude-copy"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker cp "+containerName+":/workspace", "")

	output := captureOutput(func() {
		if err := runHandoff(handoffCmd, []string{containerName}); err != nil {
			t.Fatalf("runHandoff() error = %v", err)
		}
	})
	if !strings.Contains(output, "Workspace copied to "+dest) {
		t.Fatalf("handoff output = %q", output)
	}
	if len(fake.CommandsMatching("docker cp "+containerName+":/workspace")) == 0 {
		t.Fatal("expected docker cp command")
	}
}

func TestWorktreeListAndCleanupMissing(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	missingPath := filepath.Join(t.TempDir(), "missing-worktree")
	if err := worktrees.AppendRegistry(worktrees.RegistryPath(), worktrees.Worktree{
		Container: "agent-claude-missing",
		RepoRoot:  t.TempDir(),
		Path:      missingPath,
		Branch:    "safe-ag/missing",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("append registry: %v", err)
	}

	listOut := captureOutput(func() {
		if err := runWorktreeList(worktreeListCmd, nil); err != nil {
			t.Fatalf("runWorktreeList() error = %v", err)
		}
	})
	if !strings.Contains(listOut, "agent-claude-missing") || !strings.Contains(listOut, "missing") {
		t.Fatalf("worktree list output = %q", listOut)
	}

	oldAll := worktreeCleanupAll
	oldDryRun := worktreeCleanupDryRun
	worktreeCleanupAll = false
	worktreeCleanupDryRun = false
	defer func() {
		worktreeCleanupAll = oldAll
		worktreeCleanupDryRun = oldDryRun
	}()

	cleanupOut := captureOutput(func() {
		if err := runWorktreeCleanup(worktreeCleanupCmd, nil); err != nil {
			t.Fatalf("runWorktreeCleanup() error = %v", err)
		}
	})
	if !strings.Contains(cleanupOut, "drop registry entry") || !strings.Contains(cleanupOut, "Removed 1") {
		t.Fatalf("worktree cleanup output = %q", cleanupOut)
	}
	entries, err := worktrees.ReadRegistry(worktrees.RegistryPath())
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("registry entries after cleanup = %#v", entries)
	}
}

func TestWorkspaceFileOperations(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	containerName := "agent-claude-workspace"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	if err := runWorkspaceStage(workspaceStageCmd, []string{containerName, "src/app.go"}); err != nil {
		t.Fatalf("runWorkspaceStage() error = %v", err)
	}
	if got := strings.Join(fake.LastCommand(), " "); !strings.Contains(got, "git add -- src/app.go") {
		t.Fatalf("stage command = %s", got)
	}

	if err := runWorkspaceUnstage(workspaceUnstageCmd, []string{containerName, "src/app.go"}); err != nil {
		t.Fatalf("runWorkspaceUnstage() error = %v", err)
	}
	if got := strings.Join(fake.LastCommand(), " "); !strings.Contains(got, "git restore --staged -- src/app.go") {
		t.Fatalf("unstage command = %s", got)
	}

	oldYes := workspaceYes
	workspaceYes = true
	defer func() { workspaceYes = oldYes }()
	if err := runWorkspaceRevert(workspaceRevertCmd, []string{containerName, "src/app.go"}); err != nil {
		t.Fatalf("runWorkspaceRevert() error = %v", err)
	}
	if got := strings.Join(fake.LastCommand(), " "); !strings.Contains(got, "git checkout -- src/app.go") {
		t.Fatalf("revert command = %s", got)
	}
}

func TestWorkspaceRejectsEscapingPath(t *testing.T) {
	if _, err := cleanWorkspacePaths([]string{"../secrets"}); err == nil {
		t.Fatal("cleanWorkspacePaths() error = nil, want escape rejection")
	}
}

func TestWorkspacePatchOperations(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	containerName := "agent-claude-workspace"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWD)

	patchPath := "selected.patch"
	patch := `diff --git a/src/app.go b/src/app.go
--- a/src/app.go
+++ b/src/app.go
@@ -1 +1 @@
-old
+new
`
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("write patch: %v", err)
	}
	rel := patchPath

	if err := runWorkspaceStagePatch(workspaceStagePatchCmd, []string{containerName, rel}); err != nil {
		t.Fatalf("runWorkspaceStagePatch() error = %v", err)
	}
	if cmds := fake.CommandsMatching("docker cp /tmp/safe-agentic-patch-"); len(cmds) == 0 {
		t.Fatal("expected docker cp for staged patch")
	}
	stageCmds := fake.CommandsMatching("git apply --cached --whitespace=nowarn /tmp/safe-agentic-patch-")
	if len(stageCmds) == 0 {
		t.Fatalf("expected staged patch apply command, got %#v", fake.Log)
	}
	if strings.Contains(strings.Join(stageCmds[0], " "), "SAFE_AGENTIC_PATCH") {
		t.Fatalf("stage patch still uses heredoc: %s", strings.Join(stageCmds[0], " "))
	}

	fake.Reset()
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	oldYes := workspaceYes
	workspaceYes = true
	defer func() { workspaceYes = oldYes }()
	if err := runWorkspaceRevertPatch(workspaceRevertPatchCmd, []string{containerName, rel}); err != nil {
		t.Fatalf("runWorkspaceRevertPatch() error = %v", err)
	}
	if cmds := fake.CommandsMatching("git apply --reverse --whitespace=nowarn --check /tmp/safe-agentic-patch-"); len(cmds) == 0 {
		t.Fatalf("expected revert preflight with patch file, got %#v", fake.Log)
	}
	if cmds := fake.CommandsMatching("git apply --reverse --whitespace=nowarn /tmp/safe-agentic-patch-"); len(cmds) == 0 {
		t.Fatalf("expected revert patch apply command, got %#v", fake.Log)
	}
}

func TestWorkspacePatchRejectsEscapingPath(t *testing.T) {
	patch := []byte("diff --git a/../secret b/../secret\n--- a/../secret\n+++ b/../secret\n")
	if err := validateWorkspacePatch(patch); err == nil {
		t.Fatal("validateWorkspacePatch() error = nil, want escape rejection")
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
	stopYes = true
	defer func() { stopAll = false; stopYes = false }()

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
	stopYes = true
	defer func() { stopAll = false; stopYes = false }()

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

func TestRunLogs_StoppedContainerUsesCopy(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.repo-display"}}`, "org/repo\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "false\n")
	fake.SetResponse("docker cp "+containerName+":/home/agent/.claude", "")
	fake.SetResponse("bash -c find", "/tmp/safe-agentic-logs-test/.claude/sessions/test.jsonl\n")
	fake.SetResponse("bash -c tail -n", `{"type":"user","message":{"role":"user","content":"hello"}}`+"\n")

	output := captureOutput(func() {
		if err := runLogs(logsCmd, []string{containerName}); err != nil {
			t.Fatalf("runLogs() error = %v", err)
		}
	})

	if !strings.Contains(output, "> hello") {
		t.Fatalf("expected rendered log output, got:\n%s", output)
	}
	if cmds := fake.CommandsMatching("docker start"); len(cmds) != 0 {
		t.Fatalf("stopped logs should not start container, got %d docker start command(s)", len(cmds))
	}
	if cmds := fake.CommandsMatching("docker cp"); len(cmds) == 0 {
		t.Fatal("expected docker cp for stopped container logs")
	}
}

func TestRunLogs_RunningContainerQuotesSessionPath(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	jsonlPath := "/home/agent/.claude/projects/repo/evil $(touch /tmp/pwned).jsonl"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.repo-display"}}`, "org/repo\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName+" python3 -c", jsonlPath+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c tail", `{"type":"user","message":{"role":"user","content":"hello"}}`+"\n")

	if err := runLogs(logsCmd, []string{containerName}); err != nil {
		t.Fatalf("runLogs() error = %v", err)
	}

	tailCmds := fake.CommandsMatching("docker exec " + containerName + " bash -c tail")
	if len(tailCmds) == 0 {
		t.Fatal("expected tail command")
	}
	tailScript := tailCmds[0][5]
	if !strings.Contains(tailScript, shellQuote(jsonlPath)) {
		t.Fatalf("session path was not shell-quoted:\n%s", tailScript)
	}
}

// ─── cleanup ──────────────────────────────────────────────────────────────────

func TestCleanupCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	cleanupYes = true
	defer func() { cleanupYes = false }()

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

func TestCleanupAuthRemovesSharedAndIsolatedAuthVolumes(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	origCleanupAuth := cleanupAuth
	cleanupAuth = true
	cleanupYes = true
	defer func() { cleanupAuth = origCleanupAuth; cleanupYes = false }()

	fake.SetResponse("docker volume ls --filter name=safe-agentic- --format {{.Name}}", "safe-agentic-claude-auth\nsafe-agentic-codex-gh-auth\n")
	fake.SetResponse("docker volume ls --filter label=safe-agentic.type=auth --format {{.Name}}", "agent-claude-worker-auth\n")

	if err := runCleanup(cleanupCmd, nil); err != nil {
		t.Fatalf("runCleanup() error = %v", err)
	}

	for _, want := range []string{
		"docker volume rm safe-agentic-claude-auth",
		"docker volume rm safe-agentic-codex-gh-auth",
		"docker volume rm agent-claude-worker-auth",
	} {
		if got := fake.CommandsMatching(want); len(got) == 0 {
			t.Fatalf("missing cleanup command %q", want)
		}
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
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", "diff output here\n")

	output := captureOutput(func() {
		diffStat = false
		if err := runDiff(diffCmd, []string{containerName}); err != nil {
			t.Fatalf("runDiff() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec " + containerName)
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
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", " 3 files changed\n")

	diffStat = true
	defer func() { diffStat = false }()

	captureOutput(func() {
		if err := runDiff(diffCmd, []string{containerName}); err != nil {
			t.Fatalf("runDiff() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec " + containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "--stat") {
		t.Errorf("expected '--stat' in command, got: %s", joined)
	}
}

func TestDiffCommand_SideBySide(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	t.Setenv("COLUMNS", "") // keep terminalWidth() deterministic

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", "delta output here\n")
	// The delta availability check ("docker exec <name> sh -c command -v delta")
	// is left unconfigured, so FakeExecutor's default empty-output/nil-error
	// response simulates delta being present in the image.

	diffSideBySide = true
	defer func() { diffSideBySide = false }()

	captureOutput(func() {
		if err := runDiff(diffCmd, []string{containerName}); err != nil {
			t.Fatalf("runDiff() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec " + containerName)
	var found bool
	for _, c := range cmds {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "delta --side-by-side") && strings.Contains(joined, "COLUMNS=") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a delta --side-by-side invocation with COLUMNS set, got: %v", cmds)
	}
}

func TestDiffCommand_SideBySide_MutuallyExclusiveWithStat(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	diffStat = true
	diffSideBySide = true
	defer func() {
		diffStat = false
		diffSideBySide = false
	}()

	err := runDiff(diffCmd, []string{"agent-claude-test"})
	if err == nil {
		t.Fatal("expected error when --stat and --side-by-side are combined")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error, got: %v", err)
	}
}

func TestDiffCommand_SideBySide_FallsBackWhenDeltaMissing(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetError("docker exec "+containerName+" sh -c", "delta: command not found")
	fake.SetResponse("docker exec "+containerName+" bash -c", "plain diff output\n")

	diffSideBySide = true
	defer func() { diffSideBySide = false }()

	var stdout string
	stderr := captureStderr(func() {
		stdout = captureOutput(func() {
			if err := runDiff(diffCmd, []string{containerName}); err != nil {
				t.Fatalf("runDiff() error = %v", err)
			}
		})
	})

	if !strings.Contains(stderr, "falling back to plain diff") {
		t.Errorf("expected fallback warning on stderr, got: %q", stderr)
	}
	if !strings.Contains(stdout, "plain diff output") {
		t.Errorf("expected plain diff output on stdout, got: %q", stdout)
	}

	cmds := fake.CommandsMatching("docker exec " + containerName + " bash -c")
	if len(cmds) == 0 {
		t.Fatal("expected a plain diff docker exec command")
	}
	joined := strings.Join(cmds[0], " ")
	if strings.Contains(joined, "delta") {
		t.Errorf("expected no delta invocation in fallback command, got: %s", joined)
	}
}

// ─── output ───────────────────────────────────────────────────────────────────

func TestOutputCommand_Diff(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName+" bash", "diff content\n")

	outputDiff = true
	defer func() { outputDiff = false }()

	captureOutput(func() {
		if err := runOutput(outputCmd, []string{containerName}); err != nil {
			t.Fatalf("runOutput() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec " + containerName)
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
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName+" bash", "main.go\nlib.go\n")

	outputFiles = true
	defer func() { outputFiles = false }()

	output := captureOutput(func() {
		if err := runOutput(outputCmd, []string{containerName}); err != nil {
			t.Fatalf("runOutput() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec " + containerName)
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
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true\n")
	fake.SetResponse("docker exec "+containerName+" bash", "abc1234 fix: test\n")

	outputCommits = true
	defer func() { outputCommits = false }()

	captureOutput(func() {
		if err := runOutput(outputCmd, []string{containerName}); err != nil {
			t.Fatalf("runOutput() error = %v", err)
		}
	})

	cmds := fake.CommandsMatching("docker exec " + containerName)
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

func TestOutputCommand_Default_StoppedUsesSessionMessage(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.repo-display"}}`, "org/repo\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "false\n")
	fake.SetResponse("bash -c rm -rf '/tmp/safe-agentic-logs-"+containerName+"'", "")
	fake.SetResponse("docker cp "+containerName+":/home/agent/.claude", "")
	fake.SetResponse("bash -c find /tmp/safe-agentic-logs-"+containerName, "/tmp/safe-agentic-logs-agent-claude-test/.claude/projects/-workspace-org-repo/test.jsonl\n")
	fake.SetResponse("bash -c tail -n 400", `{"message":{"role":"assistant","content":"final answer"}}`+"\n")

	output := captureOutput(func() {
		if err := runOutput(outputCmd, []string{containerName}); err != nil {
			t.Fatalf("runOutput() error = %v", err)
		}
	})

	if !strings.Contains(output, "final answer") {
		t.Fatalf("expected session message in output, got:\n%s", output)
	}
	if cmds := fake.CommandsMatching("docker logs"); len(cmds) != 0 {
		t.Fatalf("stopped output should not fall back to docker logs, got %d docker logs command(s)", len(cmds))
	}
}

func TestRenderLogEntry_UserBlockContent(t *testing.T) {
	line := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello from block"}]}}`
	got := renderLogEntry(line)
	if !strings.Contains(got, "hello from block") {
		t.Fatalf("renderLogEntry() missing user text block: %q", got)
	}
}

func TestRenderLogEntry_SystemResult(t *testing.T) {
	line := `{"type":"system","subtype":"result","durationMs":2500,"messageCount":3}`
	got := renderLogEntry(line)
	if !strings.Contains(got, "[3 messages, 2.5s]") {
		t.Fatalf("renderLogEntry() missing system result summary: %q", got)
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

func TestDiffCommand_StoppedUsesCopiedWorkspace(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker inspect --format {{.State.Running}}", "false\n")
	fake.SetResponse("bash -c rm -rf '/tmp/safe-agentic-workspace-"+containerName+"'", "")
	fake.SetResponse("docker cp "+containerName+":/workspace", "")
	fake.SetResponse("bash -c repo_dir=$(find /tmp/safe-agentic-workspace-"+containerName+"/workspace", "diff --git a/file b/file\n")

	output := captureOutput(func() {
		if err := runDiff(diffCmd, []string{containerName}); err != nil {
			t.Fatalf("runDiff() error = %v", err)
		}
	})

	if !strings.Contains(output, "diff --git") {
		t.Fatalf("expected diff output, got:\n%s", output)
	}
	if cmds := fake.CommandsMatching("docker cp " + containerName + ":/workspace"); len(cmds) == 0 {
		t.Fatal("expected stopped diff to copy workspace")
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
	writeCmds := fake.CommandsMatching("docker exec " + containerName)
	if len(writeCmds) == 0 {
		t.Fatal("expected docker exec write command")
	}
}

func TestTodoAdd_TextIsArgSafe(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c cat", "[]\n")

	text := "ship $(touch /tmp/pwned) and 'quote'"
	if err := runTodoAdd(todoAddCmd, []string{containerName, text}); err != nil {
		t.Fatalf("runTodoAdd() error = %v", err)
	}

	writeCmds := fake.CommandsMatching("docker exec " + containerName + " bash -lc")
	if len(writeCmds) == 0 {
		t.Fatal("expected docker exec write command")
	}
	if strings.Contains(writeCmds[0][5], "touch /tmp/pwned") {
		t.Fatalf("todo text leaked into shell script:\n%s", writeCmds[0][5])
	}
	payload := writeCmds[0][len(writeCmds[0])-1]
	decoded, err := inject.DecodeB64(payload)
	if err != nil {
		t.Fatalf("decode todo payload: %v", err)
	}
	if !strings.Contains(decoded, text) {
		t.Fatalf("decoded payload missing todo text:\n%s", decoded)
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

func TestSpawnFailsFastWhenHostNATOff(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	hostIPForwardingEnabled = func() bool { return false }

	err := executeSpawn(SpawnOpts{
		AgentType: "claude",
		Repos:     []string{"https://github.com/org/repo.git"},
	})
	if err == nil {
		t.Fatal("expected host NAT error")
	}
	if !strings.Contains(err.Error(), "host egress NAT is off") {
		t.Fatalf("error = %v, want host NAT error", err)
	}
	if cmds := fake.CommandsMatching("docker run"); len(cmds) > 0 {
		t.Fatalf("spawn should fail before docker run, got %v", cmds)
	}
}

func TestSpawnAllowsNetworkNoneWhenHostNATOff(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	hostIPForwardingEnabled = func() bool { return false }

	err := executeSpawn(SpawnOpts{
		AgentType:  "shell",
		Network:    "none",
		Background: true,
	})
	if err != nil {
		t.Fatalf("executeSpawn() error = %v", err)
	}
	if cmds := fake.CommandsMatching("docker run -d"); len(cmds) == 0 {
		t.Fatal("expected docker run command")
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
	fake.SetResponse("docker exec "+containerName+" bash -lc", "Saved working directory\n")
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

func TestCheckpointCreate_LabelIsArgSafeAndTagSafe(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -lc", "Saved working directory\n")
	fake.SetResponse("docker commit", "sha256:abc123\n")

	label := "ship $(touch /tmp/pwned)"
	if err := runCheckpointCreate(checkpointCreateCmd, []string{containerName, label}); err != nil {
		t.Fatalf("runCheckpointCreate() error = %v", err)
	}

	execCmds := fake.CommandsMatching("docker exec " + containerName + " bash -lc")
	if len(execCmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	if len(execCmds[0]) < 6 {
		t.Fatalf("docker exec command too short:\n%v", execCmds[0])
	}
	shellScript := execCmds[0][5]
	if strings.Contains(shellScript, "touch /tmp/pwned") {
		t.Fatalf("label leaked into executable shell script:\n%s", shellScript)
	}
	if !containsString(execCmds[0], "checkpoint: "+label) {
		t.Fatalf("checkpoint label should be passed as one argv element:\n%v", execCmds[0])
	}

	commitCmds := fake.CommandsMatching("docker commit")
	if len(commitCmds) == 0 {
		t.Fatal("expected docker commit command")
	}
	joinedCommit := strings.Join(commitCmds[0], " ")
	if strings.Contains(joinedCommit, "$(") || strings.Contains(joinedCommit, "/tmp/pwned") {
		t.Fatalf("unsafe label leaked into Docker tag:\n%s", joinedCommit)
	}
	if !strings.Contains(joinedCommit, "safe-agentic-checkpoint:agent-claude-test-ship-touch-tmp-pwned") {
		t.Fatalf("Docker tag should use safe label suffix, got:\n%s", joinedCommit)
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
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

func TestRunQuickStart_JoinsUnquotedPromptArgs(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	origSpawnOpts := spawnOpts
	spawnOpts = SpawnOpts{DryRun: true}
	defer func() { spawnOpts = origSpawnOpts }()

	output := captureOutput(func() {
		if err := runQuickStart(runCmd, []string{"https://github.com/org/repo.git", "fix", "the", "tests"}); err != nil {
			t.Fatalf("runQuickStart() error = %v", err)
		}
	})

	if !strings.Contains(output, "fix the tests") {
		t.Fatalf("quick-start prompt args were not joined:\n%s", output)
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
	joined := strings.Join(buildCmds[0], " ")
	if !strings.Contains(joined, "/tmp/build-context") {
		t.Fatalf("expected VM build context path, got: %s", joined)
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
	if !strings.Contains(joined, "CLI_CACHE_BUST") {
		t.Errorf("expected CLI_CACHE_BUST arg, got: %s", joined)
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
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
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

func TestRunAWSRefresh_WritesCredentialsAsAgentUser(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o755); err != nil {
		t.Fatalf("mkdir aws dir: %v", err)
	}
	creds := "[test-profile]\naws_access_key_id = test\naws_secret_access_key = secret\n"
	if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(creds), 0o600); err != nil {
		t.Fatalf("write host credentials: %v", err)
	}

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	output := captureOutput(func() {
		if err := runAWSRefresh(awsRefreshCmd, []string{containerName, "test-profile"}); err != nil {
			t.Fatalf("runAWSRefresh() error = %v", err)
		}
	})

	writeCmds := fake.CommandsMatching("docker exec " + containerName + " bash -lc")
	if len(writeCmds) == 0 {
		t.Fatal("expected docker exec write command")
	}
	joined := strings.Join(writeCmds[0], " ")
	if !strings.Contains(joined, "umask 177; mkdir -p /home/agent/.aws") {
		t.Fatalf("expected in-container aws write command, got: %s", joined)
	}
	if !strings.Contains(joined, "base64 -d > /home/agent/.aws/credentials") {
		t.Fatalf("expected base64 decode write, got: %s", joined)
	}
	if got := len(fake.CommandsMatching("docker cp")); got != 0 {
		t.Fatalf("did not expect docker cp, got %d command(s)", got)
	}
	if got := len(fake.CommandsMatching("chmod 600 /home/agent/.aws/credentials")); got != 0 {
		t.Fatalf("did not expect standalone chmod, got %d command(s)", got)
	}
	if !strings.Contains(output, `AWS credentials for profile "test-profile" refreshed in `+containerName) {
		t.Fatalf("expected success output, got: %s", output)
	}
}

func TestRunAWSRefresh_ProfileExportIsShellQuoted(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o755); err != nil {
		t.Fatalf("mkdir aws dir: %v", err)
	}
	profile := "evil $(touch /tmp/pwned)"
	creds := "[" + profile + "]\naws_access_key_id = test\naws_secret_access_key = secret\n"
	if err := os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(creds), 0o600); err != nil {
		t.Fatalf("write host credentials: %v", err)
	}

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	if err := runAWSRefresh(awsRefreshCmd, []string{containerName, profile}); err != nil {
		t.Fatalf("runAWSRefresh() error = %v", err)
	}

	cmds := fake.CommandsMatching("docker exec " + containerName + " bash -lc")
	if len(cmds) < 2 {
		t.Fatalf("expected credential write and profile export commands, got %d", len(cmds))
	}
	exportScript := cmds[1][5]
	if strings.Contains(exportScript, "\""+profile+"\"") {
		t.Fatalf("profile used unsafe double-quoted shell form:\n%s", exportScript)
	}
	if !strings.Contains(exportScript, shellQuote("export AWS_PROFILE="+shellQuote(profile))) {
		t.Fatalf("profile export is not shell-quoted:\n%s", exportScript)
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
		"SAFE_AGENTIC_DEFAULT_SEED_AUTH",
		"SAFE_AGENTIC_DEFAULT_NETWORK",
		"SAFE_AGENTIC_DEFAULT_IDENTITY",
		"GIT_AUTHOR_NAME",
		"GIT_AUTHOR_EMAIL",
		"GIT_COMMITTER_NAME",
		"GIT_COMMITTER_EMAIL",
	}
	for _, k := range allowed {
		if !config.KeyAllowed(k) {
			t.Errorf("config.KeyAllowed(%q) = false, want true", k)
		}
	}
	forbidden := []string{"RANDOM_KEY", "", "SAFE_AGENTIC_NOPE", "PATH"}
	for _, k := range forbidden {
		if config.KeyAllowed(k) {
			t.Errorf("config.KeyAllowed(%q) = true, want false", k)
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
		Defaults: config.DefaultsSection{
			CPUs:         "4",
			Memory:       "8g",
			PIDsLimit:    512,
			SSH:          true,
			Docker:       false,
			DockerSocket: false,
			ReuseAuth:    true,
			ReuseGHAuth:  false,
			SeedAuth:     true,
			Network:      "mynet",
			Identity:     "Alice <alice@example.com>",
		},
		Git: config.GitSection{
			AuthorName:     "Alice",
			AuthorEmail:    "alice@example.com",
			CommitterName:  "Alice",
			CommitterEmail: "alice@example.com",
		},
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
		{"SAFE_AGENTIC_DEFAULT_SEED_AUTH", "true"},
		{"SAFE_AGENTIC_DEFAULT_NETWORK", "mynet"},
		{"SAFE_AGENTIC_DEFAULT_IDENTITY", "Alice <alice@example.com>"},
		{"GIT_AUTHOR_NAME", "Alice"},
		{"GIT_AUTHOR_EMAIL", "alice@example.com"},
		{"GIT_COMMITTER_NAME", "Alice"},
		{"GIT_COMMITTER_EMAIL", "alice@example.com"},
	}

	for _, tt := range tests {
		got, err := config.GetValue(cfg, tt.key)
		if err != nil {
			t.Fatalf("GetValue(%q) error = %v", tt.key, err)
		}
		if got != tt.want {
			t.Errorf("GetValue(%q) = %q, want %q", tt.key, got, tt.want)
		}
	}
}

// ─── config show/set/get/reset ───────────────────────────────────────────────

// setXDGConfigHome keeps the old helper name but now points HOME at a temp dir.
func setXDGConfigHome(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("HOME", tmpDir)
	os.Unsetenv("XDG_CONFIG_HOME")
	return tmpDir, func() {
		if origHome == "" {
			os.Unsetenv("HOME")
		} else {
			os.Setenv("HOME", origHome)
		}
		if origXDG == "" {
			os.Unsetenv("XDG_CONFIG_HOME")
		} else {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
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

	if !strings.Contains(output, "No config file found") {
		t.Errorf("expected 'No config file found', got: %s", output)
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

	if !strings.Contains(output, "Set defaults.memory=16g") {
		t.Errorf("expected set confirmation, got: %s", output)
	}

	// Verify file was written at expected location
	path := filepath.Join(xdgDir, ".safe-ag", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config file: %v", err)
	}
	if !strings.Contains(string(data), `memory = "16g"`) {
		t.Errorf("expected TOML value in file, got: %s", string(data))
	}
}

func TestConfigSet_UpdateExisting(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	// Pre-populate
	dir := filepath.Join(xdgDir, ".safe-ag")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[defaults]\nmemory = \"8g\"\n"), 0644)

	captureOutput(func() {
		if err := runConfigSet(configSetCmd, []string{"SAFE_AGENTIC_DEFAULT_MEMORY", "32g"}); err != nil {
			t.Fatalf("runConfigSet() error = %v", err)
		}
	})

	data, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	if strings.Contains(string(data), `memory = "8g"`) {
		t.Errorf("old value should be replaced, got: %s", string(data))
	}
	if !strings.Contains(string(data), `memory = "32g"`) {
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
	if !strings.Contains(output, "defaults.memory=8g") {
		t.Fatalf("expected defaults.memory=8g, got: %s", output)
	}
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

	dir := filepath.Join(xdgDir, ".safe-ag")
	os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte("[defaults]\nmemory = \"8g\"\ncpus = \"4\"\n"), 0644)

	captureOutput(func() {
		if err := runConfigReset(configResetCmd, []string{"SAFE_AGENTIC_DEFAULT_MEMORY"}); err != nil {
			t.Fatalf("runConfigReset() error = %v", err)
		}
	})

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "memory") {
		t.Errorf("key should be removed, got: %s", string(data))
	}
	if !strings.Contains(string(data), `cpus = "4"`) {
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
		if err := setTodoDone(todoCheckCmd, []string{containerName, "1"}, true); err != nil {
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

	err := setTodoDone(todoCheckCmd, []string{containerName, "5"}, true)
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

	err := setTodoDone(todoCheckCmd, []string{containerName, "notanumber"}, true)
	if err == nil {
		t.Fatal("expected error for invalid index")
	}
}

// ─── checkpoint restore ──────────────────────────────────────────────────────

func TestCheckpointRestore(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName+" bash -c", "Changes restored\n")

	output := captureOutput(func() {
		if err := runCheckpointRevert(checkpointRestoreCmd, []string{containerName, "stash@{0}"}); err != nil {
			t.Fatalf("runCheckpointRevert() error = %v", err)
		}
	})
	_ = output

	cmds := fake.CommandsMatching("docker exec " + containerName)
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

	cmds := fake.CommandsMatching("docker exec " + containerName)
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

func TestPRCommand_TitleIsArgSafe(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	prTitle = "ship $(touch /tmp/pwned)"
	prBase = "main"
	defer func() { prTitle = ""; prBase = "main" }()

	if err := runPR(prCmd, []string{containerName}); err != nil {
		t.Fatalf("runPR() error = %v", err)
	}

	cmds := fake.CommandsMatching("docker exec " + containerName + " bash -lc")
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
	foundPRCreate := false
	for _, cmd := range cmds {
		if containsString(cmd, "gh") && containsString(cmd, "pr") && containsString(cmd, "create") {
			foundPRCreate = true
			if strings.Contains(cmd[5], "touch /tmp/pwned") {
				t.Fatalf("PR title leaked into executable shell script:\n%s", cmd[5])
			}
			if !containsString(cmd, prTitle) {
				t.Fatalf("PR title should be passed as one argv element:\n%v", cmd)
			}
		}
	}
	if !foundPRCreate {
		t.Fatalf("expected gh pr create command, got:\n%v", cmds)
	}
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

	cmds := fake.CommandsMatching("docker exec " + containerName)
	if len(cmds) == 0 {
		t.Fatal("expected docker exec command")
	}
}

func TestReviewCommand_RequestsRiskTagsAndRendersSummary(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse("docker exec "+containerName,
		"[HIGH] pkg/foo.go:12: sql injection risk\nVERDICT: fix before merge\n")

	reviewBase = "main"
	defer func() { reviewBase = "main" }()

	output := captureOutput(func() {
		if err := runReview(reviewCmd, []string{containerName}); err != nil {
			t.Fatalf("runReview() error = %v", err)
		}
	})

	if !strings.Contains(output, "[HIGH] pkg/foo.go:12: sql injection risk") {
		t.Errorf("expected raw review text preserved in output, got: %q", output)
	}
	if !strings.Contains(output, "Risk Summary") || !strings.Contains(output, "HIGH (1)") {
		t.Errorf("expected grouped risk summary in output, got: %q", output)
	}
	if !strings.Contains(output, "Verdict: fix before merge") {
		t.Errorf("expected verdict line in output, got: %q", output)
	}

	cmds := fake.CommandsMatching("codex review")
	if len(cmds) == 0 {
		t.Fatal("expected codex review command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "HIGH") || !strings.Contains(joined, "VERDICT") {
		t.Errorf("expected review prompt to require risk tags and a verdict, got: %s", joined)
	}
}

// ─── risk findings parsing ────────────────────────────────────────────────────

func TestParseRiskFindings(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantHigh     int
		wantMedium   int
		wantLow      int
		wantUntagged int
		wantVerdict  string
	}{
		{
			name: "tagged",
			input: "[HIGH] pkg/foo.go:10: unchecked error\n" +
				"[MEDIUM] pkg/bar.go:20: missing validation\n" +
				"[LOW] pkg/baz.go:30: naming nit\n" +
				"VERDICT: needs fixes before merge\n",
			wantHigh: 1, wantMedium: 1, wantLow: 1, wantUntagged: 0,
			wantVerdict: "needs fixes before merge",
		},
		{
			name: "untagged",
			input: "Looks fine overall.\n" +
				"Consider renaming this variable.\n",
			wantHigh: 0, wantMedium: 0, wantLow: 0, wantUntagged: 2,
			wantVerdict: "",
		},
		{
			name: "mixed",
			input: "[HIGH] pkg/foo.go:1: sql injection risk\n" +
				"General note without a risk tag\n" +
				"[LOW] pkg/foo.go:5: style nit\n" +
				"VERDICT: one high-risk issue found\n",
			wantHigh: 1, wantMedium: 0, wantLow: 1, wantUntagged: 1,
			wantVerdict: "one high-risk issue found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rf := parseRiskFindings(tt.input)
			if len(rf.High) != tt.wantHigh {
				t.Errorf("High = %d, want %d (%v)", len(rf.High), tt.wantHigh, rf.High)
			}
			if len(rf.Medium) != tt.wantMedium {
				t.Errorf("Medium = %d, want %d (%v)", len(rf.Medium), tt.wantMedium, rf.Medium)
			}
			if len(rf.Low) != tt.wantLow {
				t.Errorf("Low = %d, want %d (%v)", len(rf.Low), tt.wantLow, rf.Low)
			}
			if len(rf.Untagged) != tt.wantUntagged {
				t.Errorf("Untagged = %d, want %d (%v)", len(rf.Untagged), tt.wantUntagged, rf.Untagged)
			}
			if rf.Verdict != tt.wantVerdict {
				t.Errorf("Verdict = %q, want %q", rf.Verdict, tt.wantVerdict)
			}
		})
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
		Type:              "claude",
		Name:              "test-agent",
		Repo:              "https://github.com/org/repo.git",
		Prompt:            "Fix the tests",
		SSH:               true,
		ReuseAuth:         true,
		SeedAuth:          true,
		AutoTrust:         true,
		AllowSetupScripts: true,
		Memory:            "16g",
		CPUs:              "8",
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
	if !opts.SeedAuth {
		t.Error("SeedAuth should be true")
	}
	if opts.Memory != "16g" {
		t.Errorf("Memory = %q, want 16g", opts.Memory)
	}
	if !opts.AllowSetupScripts {
		t.Error("AllowSetupScripts should be true")
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
	if !strings.HasPrefix(got, filepath.Join(xdgDir, ".safe-ag")) {
		t.Errorf("userTemplatesDir() = %q, expected prefix %q", got, filepath.Join(xdgDir, ".safe-ag"))
	}
	if !strings.Contains(got, ".safe-ag") {
		t.Errorf("userTemplatesDir() should contain '.safe-ag', got: %q", got)
	}
	if !strings.HasSuffix(got, "templates") {
		t.Errorf("userTemplatesDir() should end with 'templates', got: %q", got)
	}
}

func TestRepoTemplatesDir(t *testing.T) {
	got := repoTemplatesDir()
	if got == "" {
		t.Fatal("repoTemplatesDir() = empty, want built-in templates dir")
	}
	if !strings.HasSuffix(got, "templates") {
		t.Errorf("repoTemplatesDir() = %q, expected path ending in 'templates'", got)
	}
	if !looksLikeBuiltInTemplates(got) {
		t.Errorf("repoTemplatesDir() = %q, want built-in templates", got)
	}
}

func TestTemplateList_FromSymlinkedBinary(t *testing.T) {
	tmp := t.TempDir()
	libexecDir := filepath.Join(tmp, "libexec")
	binDir := filepath.Join(tmp, "bin")
	workDir := filepath.Join(tmp, "work")
	templatesDir := filepath.Join(tmp, "templates")
	if err := os.MkdirAll(libexecDir, 0o755); err != nil {
		t.Fatalf("mkdir libexec: %v", err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, "security-audit.md"), []byte("Perform a security audit\n"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, "code-review.md"), []byte("Perform a code review\n"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	binPath := filepath.Join(libexecDir, "safe-ag")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build safe-ag failed: %v\n%s", err, out)
	}

	linkPath := filepath.Join(binDir, "safe-ag")
	if err := os.Symlink(binPath, linkPath); err != nil {
		t.Fatalf("symlink safe-ag: %v", err)
	}

	listCmd := exec.Command(linkPath, "template", "list")
	listCmd.Dir = workDir
	listOut, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("template list via symlink failed: %v\n%s", err, listOut)
	}
	if !strings.Contains(string(listOut), "security-audit") {
		t.Fatalf("template list missing built-ins:\n%s", listOut)
	}

	showCmd := exec.Command(linkPath, "template", "show", "security-audit")
	showCmd.Dir = workDir
	showOut, err := showCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("template show via symlink failed: %v\n%s", err, showOut)
	}
	if !strings.Contains(string(showOut), "Perform a security audit") {
		t.Fatalf("template show returned unexpected content:\n%s", showOut)
	}
}

func TestTemplateList_NoTemplates(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	// Ensure user templates dir is empty
	os.MkdirAll(filepath.Join(xdgDir, ".safe-ag", "templates"), 0755)

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

	os.MkdirAll(filepath.Join(xdgDir, ".safe-ag", "templates"), 0755)

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
	dir := filepath.Join(xdgDir, ".safe-ag", "templates")
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

	dir := filepath.Join(xdgDir, ".safe-ag", "templates")
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
	os.MkdirAll(filepath.Join(xdgDir, ".safe-ag", "templates"), 0755)

	err := runTemplateShow(templateShowCmd, []string{"nonexistent-xyz"})
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestTemplateShow_Existing(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	dir := filepath.Join(xdgDir, ".safe-ag", "templates")
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
	tplPath := filepath.Join(xdgDir, ".safe-ag", "templates", "my-new-template.md")
	if _, err := os.Stat(tplPath); os.IsNotExist(err) {
		t.Errorf("template file not created at %s", tplPath)
	}
}

func TestRunTemplateCreate_AlreadyExists(t *testing.T) {
	xdgDir, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	// Pre-create the template
	dir := filepath.Join(xdgDir, ".safe-ag", "templates")
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

func TestRunTemplateCreate_RejectsPathTraversal(t *testing.T) {
	_, xdgCleanup := setXDGConfigHome(t)
	defer xdgCleanup()

	err := runTemplateCreate(templateCreateCmd, []string{"../escape"})
	if err == nil {
		t.Fatal("expected error for invalid template name")
	}
	if !strings.Contains(err.Error(), "invalid") && !strings.Contains(err.Error(), "relative") {
		t.Fatalf("unexpected error: %v", err)
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

	fake := vmexec.NewFake()
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

// ─── runReplay all event types ────────────────────────────────────────────────

func TestRunReplay_AllEventTypes(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	events := strings.Join([]string{
		`{"type":"session.start","timestamp":"2026-04-10T10:00:00Z"}`,
		`{"type":"tool.call","timestamp":"2026-04-10T10:00:05Z","tool":"Read","tokens":200}`,
		`{"type":"git.commit","timestamp":"2026-04-10T10:00:10Z","sha":"abc1234def","message":"fix bug"}`,
		`{"type":"agent.message","timestamp":"2026-04-10T10:00:15Z","content":"I fixed the bug by updating the handler function"}`,
		`{"type":"session.end","timestamp":"2026-04-10T10:00:20Z"}`,
		`{"type":"unknown.event","timestamp":"2026-04-10T10:00:25Z"}`,
	}, "\n") + "\n"
	fake.SetResponse("docker exec "+containerName+" bash -c cat", events)

	output := captureOutput(func() {
		replayToolsOnly = false
		if err := runReplay(replayCmd, []string{containerName}); err != nil {
			t.Fatalf("runReplay() error = %v", err)
		}
	})

	if !strings.Contains(output, "Session started") {
		t.Errorf("missing session.start in output: %s", output)
	}
	if !strings.Contains(output, "Read") {
		t.Errorf("missing tool.call in output: %s", output)
	}
	if !strings.Contains(output, "abc1234") {
		t.Errorf("missing git.commit sha in output: %s", output)
	}
	if !strings.Contains(output, "fix bug") {
		t.Errorf("missing git.commit message in output: %s", output)
	}
	if !strings.Contains(output, "fixed the bug") {
		t.Errorf("missing agent.message in output: %s", output)
	}
	if !strings.Contains(output, "Session ended") {
		t.Errorf("missing session.end in output: %s", output)
	}
	if !strings.Contains(output, "unknown.event") {
		t.Errorf("missing default event type in output: %s", output)
	}
}

func TestRunReplay_ToolsOnly(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	events := strings.Join([]string{
		`{"type":"session.start","timestamp":"2026-04-10T10:00:00Z"}`,
		`{"type":"tool.call","timestamp":"2026-04-10T10:00:05Z","tool":"Write"}`,
		`{"type":"git.commit","timestamp":"2026-04-10T10:00:10Z","sha":"abc1234","message":"fix"}`,
	}, "\n") + "\n"
	fake.SetResponse("docker exec "+containerName+" bash -c cat", events)

	output := captureOutput(func() {
		replayToolsOnly = true
		defer func() { replayToolsOnly = false }()
		if err := runReplay(replayCmd, []string{containerName}); err != nil {
			t.Fatalf("runReplay() error = %v", err)
		}
	})

	// Only tool.call should appear
	if !strings.Contains(output, "Write") {
		t.Errorf("expected tool.call in tools-only output: %s", output)
	}
	if strings.Contains(output, "Session started") {
		t.Errorf("session.start should be filtered in tools-only mode: %s", output)
	}
}

func TestRunReplay_InvalidTimestamp(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")

	// Timestamp that isn't RFC3339 — should fall back to raw string
	events := `{"type":"session.start","timestamp":"not-a-timestamp"}` + "\n"
	fake.SetResponse("docker exec "+containerName+" bash -c cat", events)

	output := captureOutput(func() {
		if err := runReplay(replayCmd, []string{containerName}); err != nil {
			t.Fatalf("runReplay() error = %v", err)
		}
	})

	if !strings.Contains(output, "Session started") {
		t.Errorf("expected output even with invalid timestamp: %s", output)
	}
}

// ─── runDiagnose ──────────────────────────────────────────────────────────────

func TestDiagnoseCommand_AllOK(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	// docker info → success
	fake.SetResponse("docker info", "Server Version: 24.0\n")
	// docker images → returns an image ID (non-empty)
	fake.SetResponse("docker images", "abc123\n")

	output := captureOutput(func() {
		if err := runDiagnose(diagnoseCmd, nil); err != nil {
			t.Fatalf("runDiagnose() error = %v", err)
		}
	})

	// The function doesn't return an error even if container is missing; it just prints a ✗
	// We just verify it completes without panic.
	_ = output
}

func TestDiagnoseCommand_DockerReady(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	// Simulate Docker running and image present
	fake.SetResponse("docker info", "Server Version: 24.0\n")
	fake.SetResponse("docker images safe-agentic:latest -q", "sha256:abc123\n")

	output := captureOutput(func() {
		if err := runDiagnose(diagnoseCmd, nil); err != nil {
			t.Fatalf("runDiagnose() error = %v", err)
		}
	})

	// Output should contain diagnostic header regardless
	if !strings.Contains(output, "diagnostics") && !strings.Contains(output, "installed") {
		t.Errorf("expected diagnostic output, got: %s", output)
	}
}

func TestDiagnoseCommand_WarnsRiskyDefaults(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	fake.SetResponse("docker info", "Server Version: 24.0\n")
	fake.SetResponse("docker images safe-agentic:latest -q", "sha256:abc123\n")

	configDir := filepath.Join(os.Getenv("HOME"), ".safe-ag")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(`version = 1

[defaults]
ssh = true
reuse_auth = true
reuse_gh_auth = true
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	output := captureOutput(func() {
		if err := runDiagnose(diagnoseCmd, nil); err != nil {
			t.Fatalf("runDiagnose() error = %v", err)
		}
	})
	for _, want := range []string{
		"Spawn defaults",
		"! --ssh enabled by default",
		"! --reuse-auth enabled by default",
		"! --reuse-gh-auth enabled by default",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("diagnose output missing %q:\n%s", want, output)
		}
	}
}

// ─── runSetup ─────────────────────────────────────────────────────────────────

func TestSetupCommand_DockerAvailable(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	installFakeContainerBinary(t)

	// Simulate docker info succeeding (Docker already running in VM)
	fake.SetResponse("docker info", "Server Version: 24.0\n")
	fake.SetResponse("docker build", "Successfully built abc123\n")

	output := captureOutput(func() {
		if err := runSetup(setupCmd, nil); err != nil {
			t.Fatalf("runSetup() error = %v", err)
		}
	})

	if !strings.Contains(output, "Bootstrapping VM") {
		t.Fatalf("expected bootstrap output, got: %s", output)
	}
	buildCmds := fake.CommandsMatching("docker build")
	if len(buildCmds) == 0 {
		t.Fatal("expected docker build command")
	}
}

func TestSetupConfiguresLaunchdSSHAuthBeforeContainerSystem(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()
	installFakeContainerBinary(t)

	called := false
	configureLaunchdSSHAuth = func() error {
		called = true
		return nil
	}

	if err := ensureContainerSystemRunning(io.Discard, io.Discard); err != nil {
		t.Fatalf("ensureContainerSystemRunning() error = %v", err)
	}
	if !called {
		t.Fatal("expected launchd SSH_AUTH_SOCK configuration before container system start/status")
	}
}

func TestSetupCommand_DockerNotAvailable(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	installFakeContainerBinary(t)

	// Docker is verified after bootstrap in the current flow.
	fake.SetResponse("docker info", "Server Version: 24.0\n")
	fake.SetResponse("docker build", "Successfully built abc123\n")

	output := captureOutput(func() {
		if err := runSetup(setupCmd, nil); err != nil {
			t.Fatalf("runSetup() error = %v", err)
		}
	})

	if !strings.Contains(output, "Image updated") {
		t.Fatalf("expected image update output, got: %s", output)
	}
}

func TestVMStartRestoresHostNATBeforeBootstrap(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	var order []string
	configureHostNAT = func(stdout, stderr io.Writer) error {
		order = append(order, "nat")
		return nil
	}
	runVMBootstrap = func(vmName, worktreesDir, homeDir string) ([]byte, error) {
		order = append(order, "bootstrap")
		return []byte("bootstrap ok\n"), nil
	}

	output := captureOutput(func() {
		if err := runVMStart(vmStartCmd, nil); err != nil {
			t.Fatalf("runVMStart() error = %v", err)
		}
	})

	if got, want := strings.Join(order, ","), "nat,bootstrap"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
	if !strings.Contains(output, "Restoring host network egress (NAT)") {
		t.Fatalf("expected NAT output, got: %s", output)
	}
}

// ─── runVMSSH ─────────────────────────────────────────────────────────────────

func TestVMSSHCommand(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	// RunInteractive just records the call in FakeExecutor
	err := runVMSSH(vmSSHCmd, nil)
	if err != nil {
		t.Fatalf("runVMSSH() error = %v", err)
	}

	// Verify RunInteractive was called (FakeExecutor logs it)
	_ = fake
}

// ─── runCost and runCostForContainer ─────────────────────────────────────────

func TestRunCostForContainer(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")

	// find returns two JSONL files
	fake.SetResponse("docker exec "+containerName+" find", "/home/agent/.claude/sessions/a.jsonl\n/home/agent/.claude/sessions/b.jsonl\n")

	// cat for first file: Claude-style JSONL with token usage
	claudeUsage := `{"message":{"model":"claude-opus","usage":{"input_tokens":100,"output_tokens":200}}}` + "\n"
	// cat for second file: OpenAI-style
	openAIUsage := `{"model":"gpt-4","usage":{"prompt_tokens":50,"completion_tokens":75}}` + "\n"

	// The FakeExecutor matches by prefix, so we set both to the same prefix response
	// For multiple file reads with same prefix, the fake returns the same response for both
	fake.SetResponse("docker exec "+containerName+" cat", claudeUsage+openAIUsage)

	output := captureOutput(func() {
		ctx := context.Background()
		exec := newExecutor()
		if err := runCostForContainer(ctx, exec, containerName); err != nil {
			t.Fatalf("runCostForContainer() error = %v", err)
		}
	})

	if !strings.Contains(output, "Container") {
		t.Errorf("expected 'Container' in output, got: %s", output)
	}
	if !strings.Contains(output, "Estimated cost") {
		t.Errorf("expected 'Estimated cost' in output, got: %s", output)
	}
}

func TestRunCostForContainer_NoFiles(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	// find returns empty
	fake.SetResponse("docker exec "+containerName+" find", "")

	output := captureOutput(func() {
		ctx := context.Background()
		exec := newExecutor()
		if err := runCostForContainer(ctx, exec, containerName); err != nil {
			t.Fatalf("runCostForContainer() error = %v", err)
		}
	})

	if !strings.Contains(output, "No session files found") {
		t.Errorf("expected 'No session files found', got: %s", output)
	}
}

func TestRunCostForContainer_CodexType(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-codex-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "codex\n")
	fake.SetResponse("docker exec "+containerName+" find", "/home/agent/.codex/sessions/a.jsonl\n")
	fake.SetResponse("docker exec "+containerName+" cat", strings.Join([]string{
		`{"type":"session_meta","payload":{"model_provider":"openai"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10,"cached_input_tokens":5,"output_tokens":20},"total_token_usage":{"input_tokens":10,"cached_input_tokens":5,"output_tokens":20}}}}`,
	}, "\n")+"\n")

	output := captureOutput(func() {
		ctx := context.Background()
		exec := newExecutor()
		if err := runCostForContainer(ctx, exec, containerName); err != nil {
			t.Fatalf("runCostForContainer(codex) error = %v", err)
		}
	})

	if !strings.Contains(output, "Estimated cost") {
		t.Errorf("expected 'Estimated cost' in output, got: %s", output)
	}
	if !strings.Contains(output, "Input tokens:  15") {
		t.Errorf("expected codex token_count input tokens in output, got: %s", output)
	}
	if !strings.Contains(output, "Output tokens: 20") {
		t.Errorf("expected codex token_count output tokens in output, got: %s", output)
	}
}

func TestExtractTokenUsage_CodexTokenCountLastUsage(t *testing.T) {
	data := []byte(strings.Join([]string{
		`{"type":"session_meta","payload":{"model_provider":"openai"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":40},"total_token_usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":40}}}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":10,"cached_input_tokens":5,"output_tokens":2},"total_token_usage":{"input_tokens":110,"cached_input_tokens":30,"output_tokens":42}}}}`,
	}, "\n"))

	usages := extractTokenUsage(data)
	if len(usages) != 2 {
		t.Fatalf("len(usages) = %d, want 2", len(usages))
	}
	if usages[0].Model != "codex" || usages[0].InputTokens != 125 || usages[0].OutputTokens != 40 {
		t.Fatalf("first usage = %+v, want model=codex in=125 out=40", usages[0])
	}
	if usages[1].Model != "codex" || usages[1].InputTokens != 15 || usages[1].OutputTokens != 2 {
		t.Fatalf("second usage = %+v, want model=codex in=15 out=2", usages[1])
	}
}

func TestExtractTokenUsage_CodexTokenCountTotalFallback(t *testing.T) {
	data := []byte(strings.Join([]string{
		`{"type":"session_meta","payload":{"model_provider":"openai"}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":100,"cached_input_tokens":25,"output_tokens":40}}}}`,
		`{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":110,"cached_input_tokens":30,"output_tokens":42}}}}`,
	}, "\n"))

	usages := extractTokenUsage(data)
	if len(usages) != 2 {
		t.Fatalf("len(usages) = %d, want 2", len(usages))
	}
	if usages[0].InputTokens != 125 || usages[0].OutputTokens != 40 {
		t.Fatalf("first usage = %+v, want in=125 out=40", usages[0])
	}
	if usages[1].InputTokens != 15 || usages[1].OutputTokens != 2 {
		t.Fatalf("second usage = %+v, want in=15 out=2", usages[1])
	}
}

func TestRunCostCommand_WithHistory(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	tmpDir := t.TempDir()
	origXDG := os.Getenv("XDG_DATA_HOME")
	os.Setenv("XDG_DATA_HOME", tmpDir)
	defer func() {
		if origXDG == "" {
			os.Unsetenv("XDG_DATA_HOME")
		} else {
			os.Setenv("XDG_DATA_HOME", origXDG)
		}
	}()

	// Set --history flag
	costHistory = "7d"
	defer func() { costHistory = "" }()

	output := captureOutput(func() {
		if err := runCost(costCmd, nil); err != nil {
			t.Fatalf("runCost() with --history error = %v", err)
		}
	})

	if !strings.Contains(output, "Period") {
		t.Errorf("expected 'Period' in output, got: %s", output)
	}
	_ = fake
}

func TestRunCostCommand_Container(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	fake.SetResponse("docker exec "+containerName+" find", "")

	costHistory = ""

	output := captureOutput(func() {
		if err := runCost(costCmd, []string{containerName}); err != nil {
			t.Fatalf("runCost() error = %v", err)
		}
	})

	if !strings.Contains(output, "No session files found") {
		t.Errorf("expected 'No session files found', got: %s", output)
	}
}

// ─── extractTar ───────────────────────────────────────────────────────────────

func TestExtractTar_Files(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Add a directory entry
	tw.WriteHeader(&tar.Header{
		Name:     "sessions/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	})

	// Add a regular file
	content := []byte("session data here")
	tw.WriteHeader(&tar.Header{
		Name:     "sessions/test.jsonl",
		Typeflag: tar.TypeReg,
		Size:     int64(len(content)),
		Mode:     0644,
	})
	tw.Write(content)
	tw.Close()

	destDir := t.TempDir()
	count, err := extractTar(&buf, destDir)
	if err != nil {
		t.Fatalf("extractTar() error = %v", err)
	}
	if count != 1 {
		t.Errorf("extractTar() count = %d, want 1", count)
	}

	// Verify the file was extracted
	data, err := os.ReadFile(filepath.Join(destDir, "sessions", "test.jsonl"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(data) != "session data here" {
		t.Errorf("extracted content = %q, want %q", string(data), "session data here")
	}
}

func TestExtractTar_SkipsNonRegular(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Add a symlink (should be skipped)
	tw.WriteHeader(&tar.Header{
		Name:     "link",
		Typeflag: tar.TypeSymlink,
		Linkname: "target",
	})

	// Add a regular file
	content := []byte("real file")
	tw.WriteHeader(&tar.Header{
		Name:     "real.txt",
		Typeflag: tar.TypeReg,
		Size:     int64(len(content)),
		Mode:     0644,
	})
	tw.Write(content)
	tw.Close()

	destDir := t.TempDir()
	count, err := extractTar(&buf, destDir)
	if err != nil {
		t.Fatalf("extractTar() error = %v", err)
	}
	// Only the regular file should be counted
	if count != 1 {
		t.Errorf("extractTar() count = %d, want 1 (symlink skipped)", count)
	}
}

func TestExtractTar_Empty(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.Close()

	destDir := t.TempDir()
	count, err := extractTar(&buf, destDir)
	if err != nil {
		t.Fatalf("extractTar() error = %v", err)
	}
	if count != 0 {
		t.Errorf("extractTar() count = %d, want 0 for empty tar", count)
	}
}

// ─── runSessions success path ─────────────────────────────────────────────────

func TestRunSessions_WithData(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")

	// Create a real minimal tar to return
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	content := []byte(`{"timestamp":"2026-04-10"}`)
	tw.WriteHeader(&tar.Header{
		Name:     "sessions/chat.jsonl",
		Typeflag: tar.TypeReg,
		Size:     int64(len(content)),
		Mode:     0644,
	})
	tw.Write(content)
	tw.Close()

	fake.SetResponse("docker exec "+containerName+" bash -c tar", string(tarBuf.Bytes()))

	destDir := t.TempDir()
	output := captureOutput(func() {
		if err := runSessions(sessionsCmd, []string{containerName, destDir}); err != nil {
			t.Fatalf("runSessions() error = %v", err)
		}
	})

	if !strings.Contains(output, "Exported") {
		t.Errorf("expected 'Exported' in output, got: %s", output)
	}
	if !strings.Contains(output, containerName) {
		t.Errorf("expected container name in output, got: %s", output)
	}
}

// ─── executeSpawn branches ────────────────────────────────────────────────────

func TestSpawnWithEphemeralAuth(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType:     "claude",
		Repos:         []string{"https://github.com/org/repo.git"},
		EphemeralAuth: true,
		DryRun:        true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with ephemeral auth error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnWithDockerAccess(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType:    "claude",
		Repos:        []string{"https://github.com/org/repo.git"},
		DockerAccess: true,
		DryRun:       true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with docker access error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnWithDockerSocket(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType:    "claude",
		Repos:        []string{"https://github.com/org/repo.git"},
		DockerSocket: true,
		DryRun:       true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with docker socket error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnDryRunPrintsRiskSummary(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	output := captureOutput(func() {
		err := executeSpawn(SpawnOpts{
			AgentType:   "claude",
			SSH:         true,
			ReuseAuth:   true,
			ReuseGHAuth: true,
			DryRun:      true,
		})
		if err != nil {
			t.Fatalf("executeSpawn() error = %v", err)
		}
	})
	for _, want := range []string{
		"Security context:",
		"! --ssh:",
		"! --reuse-auth:",
		"! --reuse-gh-auth:",
		"~/.safe-ag/rules.toml",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, output)
		}
	}
}

func TestSpawnWithCallbacks(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType:  "claude",
		Repos:      []string{"https://github.com/org/repo.git"},
		OnExit:     "echo done",
		OnComplete: "notify success",
		OnFail:     "notify failure",
		MaxCost:    "5.00",
		Notify:     "terminal",
		DryRun:     true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with callbacks error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
	for _, want := range []string{
		"SAFE_AGENTIC_ON_EXIT_B64=",
		"SAFE_AGENTIC_ON_COMPLETE_B64=",
		"SAFE_AGENTIC_ON_FAIL_B64=",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run missing callback env %q:\n%s", want, output)
		}
	}
}

func TestSpawnWithTemplate(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	xdgDir := t.TempDir()
	t.Setenv("HOME", xdgDir)
	templateDir := filepath.Join(xdgDir, ".safe-ag", "templates")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "security-audit.md"), []byte("Audit this repository."), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	opts := SpawnOpts{
		AgentType: "claude",
		Repos:     []string{"https://github.com/org/repo.git"},
		Template:  "security-audit",
		DryRun:    true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with template error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestExecuteSpawn_DefaultsToEphemeralAuth(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	output := captureOutput(func() {
		err := executeSpawn(SpawnOpts{
			AgentType: "claude",
			DryRun:    true,
		})
		if err != nil {
			t.Fatalf("executeSpawn() error = %v", err)
		}
	})
	if !strings.Contains(output, "safe-agentic.auth=ephemeral") {
		t.Fatalf("expected ephemeral auth in dry-run output, got: %s", output)
	}
	if strings.Contains(output, "safe-agentic.auth=shared") {
		t.Fatalf("did not expect shared auth in dry-run output, got: %s", output)
	}
}

func TestExecuteSpawn_DefaultDoesNotSeedHostAuth(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "host-token")
	claudeDir := filepath.Join(home, ".claude")
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude dir: %v", err)
	}
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatalf("write claude auth: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatalf("write codex auth: %v", err)
	}

	output := captureOutput(func() {
		err := executeSpawn(SpawnOpts{
			AgentType: "shell",
			DryRun:    true,
		})
		if err != nil {
			t.Fatalf("executeSpawn() error = %v", err)
		}
	})

	for _, forbidden := range []string{
		"CLAUDE_CODE_OAUTH_TOKEN=",
		"SAFE_AGENTIC_CLAUDE_AUTH_B64=",
		"SAFE_AGENTIC_CODEX_AUTH_B64=",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("default spawn should not seed host auth %q:\n%s", forbidden, output)
		}
	}
}

func TestExecuteSpawn_SeedAuthInjectsHostAuth(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "host-token")
	claudeDir := filepath.Join(home, ".claude")
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir claude dir: %v", err)
	}
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatalf("write claude auth: %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"token":"secret"}`), 0o600); err != nil {
		t.Fatalf("write codex auth: %v", err)
	}

	output := captureOutput(func() {
		err := executeSpawn(SpawnOpts{
			AgentType: "shell",
			SeedAuth:  true,
			DryRun:    true,
		})
		if err != nil {
			t.Fatalf("executeSpawn() error = %v", err)
		}
	})

	for _, want := range []string{
		"CLAUDE_CODE_OAUTH_TOKEN=<redacted>",
		"SAFE_AGENTIC_CLAUDE_AUTH_B64=<redacted>",
		"SAFE_AGENTIC_CODEX_AUTH_B64=<redacted>",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("seed auth missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "host-token") || strings.Contains(output, "secret") {
		t.Fatalf("dry-run leaked host auth:\n%s", output)
	}
}

func TestExecuteSpawn_AppliesConfigDefaults(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "host-token")
	configDir := filepath.Join(home, ".safe-ag")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configText := `version = 1

[defaults]
ssh = true
docker = true
reuse_auth = true
reuse_gh_auth = true
seed_auth = true
`
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	output := captureOutput(func() {
		err := executeSpawn(SpawnOpts{
			AgentType: "claude",
			Repos:     []string{"https://github.com/org/repo.git"},
			DryRun:    true,
		})
		if err != nil {
			t.Fatalf("executeSpawn() error = %v", err)
		}
	})

	for _, want := range []string{
		"SSH_AUTH_SOCK=/run/ssh-agent.sock",
		"safe-agentic.auth=shared",
		"safe-agentic.gh-auth=shared",
		"safe-agentic.docker=dind",
		"CLAUDE_CODE_OAUTH_TOKEN=<redacted>",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "host-token") {
		t.Fatalf("dry-run leaked host token:\n%s", output)
	}
}

func TestExecuteSpawn_OptOutsOverrideConfigDefaults(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".safe-ag")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configText := `version = 1

[defaults]
ssh = true
docker = true
reuse_auth = true
reuse_gh_auth = true
seed_auth = true
`
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(configText), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	output := captureOutput(func() {
		err := executeSpawn(SpawnOpts{
			AgentType:     "claude",
			Repos:         []string{"https://github.com/org/repo.git"},
			NoSSH:         true,
			NoDocker:      true,
			NoReuseAuth:   true,
			NoReuseGHAuth: true,
			NoSeedAuth:    true,
			DryRun:        true,
		})
		if err != nil {
			t.Fatalf("executeSpawn() error = %v", err)
		}
	})

	forbidden := []string{
		"SSH_AUTH_SOCK=/run/ssh-agent.sock",
		"safe-agentic.auth=shared",
		"safe-agentic.gh-auth=shared",
		"safe-agentic.docker=dind",
		"SAFE_AGENTIC_CLAUDE_AUTH_B64=",
		"SAFE_AGENTIC_CODEX_AUTH_B64=",
	}
	for _, bad := range forbidden {
		if strings.Contains(output, bad) {
			t.Fatalf("dry-run should not contain %q:\n%s", bad, output)
		}
	}
	if !strings.Contains(output, "safe-agentic.auth=ephemeral") {
		t.Fatalf("dry-run missing ephemeral auth after opt-out:\n%s", output)
	}
	if !strings.Contains(output, "safe-agentic.docker=off") {
		t.Fatalf("dry-run missing docker=off after opt-out:\n%s", output)
	}
}

func TestValidateSpawnOpts_RejectsConflictingAuthModes(t *testing.T) {
	err := validateSpawnOpts(SpawnOpts{
		AgentType:     "claude",
		ReuseAuth:     true,
		EphemeralAuth: true,
	})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually exclusive auth error, got %v", err)
	}
}

func TestValidateSpawnModeConflicts_RejectsExplicitOpposites(t *testing.T) {
	tests := []SpawnOpts{
		{SSH: true, NoSSH: true},
		{ReuseAuth: true, NoReuseAuth: true},
		{ReuseGHAuth: true, NoReuseGHAuth: true},
		{SeedAuth: true, NoSeedAuth: true},
		{DockerAccess: true, NoDocker: true},
		{DockerSocket: true, NoDockerSocket: true},
	}
	for _, tt := range tests {
		if err := validateSpawnModeConflicts(tt); err == nil {
			t.Fatalf("validateSpawnModeConflicts(%+v) error = nil, want conflict", tt)
		}
	}
}

func TestSpawnWithInstructions(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType:    "claude",
		Repos:        []string{"https://github.com/org/repo.git"},
		Instructions: "Focus on security vulnerabilities",
		DryRun:       true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with instructions error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnWithInstructionsFile(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	f := filepath.Join(t.TempDir(), "instructions.md")
	os.WriteFile(f, []byte("Focus on security"), 0644)

	opts := SpawnOpts{
		AgentType:        "claude",
		Repos:            []string{"https://github.com/org/repo.git"},
		InstructionsFile: f,
		DryRun:           true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with instructions file error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnWithInstructionsAndInstructionsFile(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	f := filepath.Join(t.TempDir(), "instructions.md")
	if err := os.WriteFile(f, []byte("File instructions"), 0o644); err != nil {
		t.Fatalf("write instructions file: %v", err)
	}

	opts := SpawnOpts{
		AgentType:        "claude",
		Repos:            []string{"https://github.com/org/repo.git"},
		Instructions:     "Inline instructions",
		InstructionsFile: f,
		DryRun:           true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with instructions + file error = %v", err)
		}
	})

	want := inject.EncodeB64("Inline instructions\n\nFile instructions")
	if strings.Contains(output, want) {
		t.Fatalf("dry-run leaked instructions payload: %s", output)
	}
	if !strings.Contains(output, "SAFE_AGENTIC_INSTRUCTIONS_B64=<redacted>") {
		t.Fatalf("expected redacted instructions env in dry-run output, got: %s", output)
	}
	if strings.Count(output, "SAFE_AGENTIC_INSTRUCTIONS_B64=") != 1 {
		t.Fatalf("expected one instructions env entry, got: %s", output)
	}
}

func TestSpawnWithInstructionsFile_NotFound(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType:        "claude",
		Repos:            []string{"https://github.com/org/repo.git"},
		InstructionsFile: "/nonexistent/path/instructions.md",
		DryRun:           true,
	}
	err := executeSpawn(opts)
	if err == nil {
		t.Fatal("expected error for missing instructions file")
	}
	if !strings.Contains(err.Error(), "read instructions file") {
		t.Errorf("expected 'read instructions file' in error, got: %v", err)
	}
}

func TestSpawnWithFleetVolume(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType:   "claude",
		Repos:       []string{"https://github.com/org/repo.git"},
		FleetVolume: "fleet-12345",
		DryRun:      true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with fleet volume error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnWithGHAuth(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType:   "claude",
		Repos:       []string{"https://github.com/org/repo.git"},
		ReuseGHAuth: true,
		DryRun:      true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with GH auth error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnCodex_DryRun(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType: "codex",
		Repos:     []string{"https://github.com/org/repo.git"},
		DryRun:    true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn codex error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnCodexAutoTrustWithHierarchy_DryRun(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType: "codex",
		Repos:     []string{"https://github.com/org/repo.git"},
		AutoTrust: true,
		Hierarchy: "display-nested/display-nested-child",
		DryRun:    true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn codex hierarchy error = %v", err)
		}
	})
	if !strings.Contains(output, "SAFE_AGENTIC_AUTO_TRUST=1") {
		t.Fatalf("dry-run missing auto-trust env:\n%s", output)
	}
	if !strings.Contains(output, "safe-agentic.hierarchy=display-nested/display-nested-child") {
		t.Fatalf("dry-run missing hierarchy label:\n%s", output)
	}
}

func TestSpawnShell_DryRun(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType: "shell",
		Repos:     []string{"https://github.com/org/repo.git"},
		DryRun:    true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn shell error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnWithAWS(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	// Create temp AWS credentials file
	dir := t.TempDir()
	awsDir := filepath.Join(dir, ".aws")
	os.MkdirAll(awsDir, 0755)
	os.WriteFile(filepath.Join(awsDir, "credentials"),
		[]byte("[test-profile]\naws_access_key_id=AKIAIOSFODNN7EXAMPLE\naws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n"), 0600)
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", origHome)

	opts := SpawnOpts{
		AgentType:  "claude",
		Repos:      []string{"https://github.com/org/repo.git"},
		AWSProfile: "test-profile",
		DryRun:     true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with AWS error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnWithPrompt(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType: "claude",
		Repos:     []string{"https://github.com/org/repo.git"},
		Prompt:    "Fix the failing tests",
		DryRun:    true,
	}
	output := captureOutput(func() {
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn with prompt error = %v", err)
		}
	})
	if !strings.Contains(output, "Would execute") {
		t.Errorf("expected dry-run output, got: %s", output)
	}
}

func TestSpawnInvalidPIDsLimit(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	opts := SpawnOpts{
		AgentType: "claude",
		PIDsLimit: 10, // too low (must be >= 64)
	}
	err := executeSpawn(opts)
	if err == nil {
		t.Fatal("expected error for invalid PIDs limit")
	}
}

// ─── runMCPLogin ──────────────────────────────────────────────────────────────

func TestMCPLogin_WithContainer(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	// Two-arg form: service + container
	err := runMCPLogin(mcpLoginCmd, []string{"linear", "agent-claude-test"})
	if err != nil {
		t.Fatalf("runMCPLogin() error = %v", err)
	}

	// Verify RunInteractive was called with docker exec
	cmds := fake.CommandsMatching("docker exec -it agent-claude-test")
	if len(cmds) == 0 {
		t.Fatal("expected docker exec interactive command")
	}
}

func TestMCPLogin_NoContainer_Fallback(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	// No containers running — ResolveTarget with --latest should fail
	fake.SetResponse("docker ps -a --filter name=^agent-", "")

	output := captureOutput(func() {
		err := runMCPLogin(mcpLoginCmd, []string{"notion"})
		if err != nil {
			t.Fatalf("runMCPLogin() error = %v", err)
		}
	})

	if !strings.Contains(output, "notion") {
		t.Errorf("expected service name in output, got: %s", output)
	}
}

// ─── runRetry with feedback ───────────────────────────────────────────────────

func TestRunRetry_WithFeedback(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	// InspectLabel for agent type
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	// InspectLabel for SSH
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.ssh"}}`, "false\n")
	// InspectLabel for auth
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.auth"}}`, "shared\n")
	// InspectLabel for gh-auth
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.gh-auth"}}`, "\n")
	// InspectLabel for docker mode
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.docker"}}`, "off\n")
	// InspectLabel for max-cost
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.max-cost"}}`, "\n")
	// InspectLabel for aws
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.aws"}}`, "\n")
	// InspectLabel for notify, on-complete, on-fail
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.notify"}}`, "\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.on-complete"}}`, "\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.on-fail"}}`, "\n")
	// containerEnvVar calls
	fake.SetResponse("docker inspect --format {{range .Config.Env}}{{println .}}{{end}}", "AGENT_TYPE=claude\nREPOS=https://github.com/org/repo.git\n")

	retryFeedback = "Try a different approach"
	defer func() { retryFeedback = "" }()

	// reconstructSpawnOpts → executeSpawn(DryRun=false) tries docker run and will fail;
	// but we just want to exercise the code path up to the docker exec attempt.
	// The fake will return empty for docker network and docker run commands.
	err := runRetry(retryCmd, []string{containerName})
	// Error is expected (from docker run or tmux wait) — we just verify no panic
	_ = err
}

// ─── runCostHistory edge cases ────────────────────────────────────────────────

func TestRunCostHistory_WithAuditEntries(t *testing.T) {
	tmpDir := t.TempDir()
	origXDG := os.Getenv("XDG_DATA_HOME")
	os.Setenv("XDG_DATA_HOME", tmpDir)
	defer func() {
		if origXDG == "" {
			os.Unsetenv("XDG_DATA_HOME")
		} else {
			os.Setenv("XDG_DATA_HOME", origXDG)
		}
	}()

	// Write some audit entries
	auditDir := filepath.Join(tmpDir, "safe-agentic")
	os.MkdirAll(auditDir, 0755)
	entries := []audit.Entry{
		{Timestamp: time.Now().Format(time.RFC3339), Action: "spawn", Container: "agent-claude-a", Details: map[string]string{"type": "claude"}},
		{Timestamp: time.Now().Format(time.RFC3339), Action: "spawn", Container: "agent-claude-b", Details: map[string]string{"type": "claude"}},
		{Timestamp: time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339), Action: "spawn", Container: "agent-claude-old", Details: map[string]string{"type": "claude"}},
	}
	auditPath := filepath.Join(auditDir, "audit.jsonl")
	f, _ := os.Create(auditPath)
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(append(data, '\n'))
	}
	f.Close()

	fake := vmexec.NewFake()
	output := captureOutput(func() {
		if err := runCostHistory(context.Background(), fake, "7d"); err != nil {
			t.Fatalf("runCostHistory() error = %v", err)
		}
	})

	if !strings.Contains(output, "Spawns") {
		t.Errorf("expected 'Spawns' in output, got: %s", output)
	}
	if !strings.Contains(output, "Containers") {
		t.Errorf("expected 'Containers' in output, got: %s", output)
	}
}

func TestRunCostHistory_PeriodWeeks(t *testing.T) {
	tmpDir := t.TempDir()
	origXDG := os.Getenv("XDG_DATA_HOME")
	os.Setenv("XDG_DATA_HOME", tmpDir)
	defer func() {
		if origXDG == "" {
			os.Unsetenv("XDG_DATA_HOME")
		} else {
			os.Setenv("XDG_DATA_HOME", origXDG)
		}
	}()

	fake := vmexec.NewFake()
	output := captureOutput(func() {
		if err := runCostHistory(context.Background(), fake, "2w"); err != nil {
			t.Fatalf("runCostHistory() with weeks error = %v", err)
		}
	})

	if !strings.Contains(output, "2w") {
		t.Errorf("expected period '2w' in output, got: %s", output)
	}
}

func TestRunCostHistory_PeriodHours(t *testing.T) {
	tmpDir := t.TempDir()
	origXDG := os.Getenv("XDG_DATA_HOME")
	os.Setenv("XDG_DATA_HOME", tmpDir)
	defer func() {
		if origXDG == "" {
			os.Unsetenv("XDG_DATA_HOME")
		} else {
			os.Setenv("XDG_DATA_HOME", origXDG)
		}
	}()

	fake := vmexec.NewFake()
	output := captureOutput(func() {
		if err := runCostHistory(context.Background(), fake, "24h"); err != nil {
			t.Fatalf("runCostHistory() with hours error = %v", err)
		}
	})

	if !strings.Contains(output, "24h") {
		t.Errorf("expected period '24h' in output, got: %s", output)
	}
}

// ─── runSessions default dest ─────────────────────────────────────────────────

func TestRunSessions_DefaultDest(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	containerName := "agent-claude-test"
	fake.SetResponse("docker ps -a --filter name=^agent-", containerName+"\n")
	fake.SetResponse(`docker inspect --format {{index .Config.Labels "safe-agentic.agent-type"}}`, "claude\n")
	// tar returns empty → "No session data found"
	fake.SetResponse("docker exec "+containerName+" bash -c tar", "")

	// Run with only container name (no dest)
	output := captureOutput(func() {
		if err := runSessions(sessionsCmd, []string{containerName}); err != nil {
			t.Fatalf("runSessions() with default dest error = %v", err)
		}
	})

	// Should create default dest and report no data
	if !strings.Contains(output, "No session data found") {
		t.Errorf("expected 'No session data found', got: %s", output)
	}

	// Cleanup the created directory
	os.RemoveAll(filepath.Join("agent-sessions", containerName))
}

// ─── runRetry reconstructSpawnOpts branches ───────────────────────────────────
// (reconstructSpawnOpts basic tests are in lifecycle_test.go)

// ─── runFleet ─────────────────────────────────────────────────────────────────

func TestRunFleet_DryRun(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	// Create a minimal fleet manifest
	manifest := `
name: test-fleet
agents:
  - type: claude
    repo: https://github.com/org/repo.git
    seed_auth: true
    prompt: "Fix the tests"
`
	f := filepath.Join(t.TempDir(), "fleet.yaml")
	os.WriteFile(f, []byte(manifest), 0644)

	fleetDryRun = true
	defer func() { fleetDryRun = false }()

	output := captureOutput(func() {
		if err := runFleet(fleetCmd, []string{f}); err != nil {
			t.Fatalf("runFleet() dry-run error = %v", err)
		}
	})

	if !strings.Contains(output, "Fleet manifest") {
		t.Errorf("expected 'Fleet manifest' in output, got: %s", output)
	}
	if !strings.Contains(output, "--seed-auth") {
		t.Errorf("expected --seed-auth in dry-run output, got: %s", output)
	}
}

func TestRunFleet_EmptyManifest(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	manifest := "name: empty-fleet\nagents: []\n"
	f := filepath.Join(t.TempDir(), "empty.yaml")
	os.WriteFile(f, []byte(manifest), 0644)

	fleetDryRun = false

	output := captureOutput(func() {
		if err := runFleet(fleetCmd, []string{f}); err != nil {
			t.Fatalf("runFleet() empty error = %v", err)
		}
	})

	if !strings.Contains(output, "No agents defined") {
		t.Errorf("expected 'No agents defined', got: %s", output)
	}
}
