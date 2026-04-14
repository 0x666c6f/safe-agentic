package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardIndexAndDetail(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	d.handleIndex(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleIndex status = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "agent-beta") || !strings.Contains(body, "dashboard") {
		t.Fatalf("handleIndex body = %q", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec = httptest.NewRecorder()
	d.handleIndex(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("handleIndex missing status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/agents/agent-beta", nil)
	rec = httptest.NewRecorder()
	d.handleAgentDetail(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleAgentDetail status = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "agent-beta") || !strings.Contains(body, "org/private-api") {
		t.Fatalf("handleAgentDetail body = %q", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/agents/unknown", nil)
	rec = httptest.NewRecorder()
	d.handleAgentDetail(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("handleAgentDetail missing status = %d", rec.Code)
	}
}

func TestDashboardAPIAgentsAndStop(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()
	var gotArgs []string
	d.runCLI = func(args ...string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return "stopped", nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	d.handleAPIAgents(rec, req)
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	var agents []Agent
	if err := json.Unmarshal(rec.Body.Bytes(), &agents); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(agents) != 3 {
		t.Fatalf("len(api agents) = %d", len(agents))
	}

	req = httptest.NewRequest(http.MethodGet, "/api/agents/stop/agent-beta", nil)
	rec = httptest.NewRecorder()
	d.handleAPIStop(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("handleAPIStop GET status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/agents/stop/", nil)
	rec = httptest.NewRecorder()
	d.handleAPIStop(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("handleAPIStop missing name status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/agents/stop/agent-beta", nil)
	rec = httptest.NewRecorder()
	d.handleAPIStop(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleAPIStop POST status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("handleAPIStop body = %q", rec.Body.String())
	}
	if strings.Join(gotArgs, " ") != "stop agent-beta" {
		t.Fatalf("stop args = %q", strings.Join(gotArgs, " "))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/agents/stop/--all", nil)
	rec = httptest.NewRecorder()
	d.handleAPIStop(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("handleAPIStop --all status = %d", rec.Code)
	}
}

func TestFindDashboardAssetDirMissingReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	if got := findDashboardAssetDir(filepath.Join(tmp, "bin", "safe-ag"), tmp); got != "" {
		t.Fatalf("findDashboardAssetDir() = %q, want empty", got)
	}
}

func TestDashboardAgentLogsErrorPath(t *testing.T) {
	d := NewDashboard("localhost:8420")
	req := httptest.NewRequest(http.MethodGet, "/agents/agent-beta/logs", nil)
	rec := httptest.NewRecorder()
	d.handleAgentLogs(rec, req, "agent-beta")
	if rec.Code != http.StatusOK {
		t.Fatalf("handleAgentLogs status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "[agent stopped]") {
		t.Fatalf("handleAgentLogs body = %q", rec.Body.String())
	}
}

func TestDashboardSSECanceledContext(t *testing.T) {
	d := NewDashboard("localhost:8420")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	d.handleSSE(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleSSE status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestDashboardAgentContentAndInteractive(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()
	d.runCLI = func(args ...string) (string, error) {
		return strings.Join(args, " "), nil
	}
	d.runOrb = func(args ...string) ([]byte, error) {
		return []byte(`{"name":"agent-beta"}`), nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/agents/agent-beta/content/summary", nil)
	rec := httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("summary status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "summary agent-beta") {
		t.Fatalf("summary body = %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/agents/agent-beta/content/describe", nil)
	rec = httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("describe status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"name": "agent-beta"`) {
		t.Fatalf("describe body = %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/agents/agent-beta/interactive/attach", nil)
	rec = httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("interactive status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "safe-ag attach agent-beta") {
		t.Fatalf("interactive body = %q", rec.Body.String())
	}
}

func TestDashboardAgentInteractiveOpen(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()
	var gotArgs []string
	d.openTerminal = func(args []string) error {
		gotArgs = append([]string(nil), args...)
		return nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/interactive/attach", nil)
	rec := httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("interactive open status = %d", rec.Code)
	}
	if strings.Join(gotArgs, " ") != cliBinary+" attach agent-beta" {
		t.Fatalf("openTerminal args = %q", strings.Join(gotArgs, " "))
	}
	if !strings.Contains(rec.Body.String(), "Opened Terminal for: safe-ag attach agent-beta") {
		t.Fatalf("interactive open body = %q", rec.Body.String())
	}
}

func TestDashboardInteractiveCommandFromArgsAttachLatest(t *testing.T) {
	d := NewDashboard("localhost:8420")

	cmd, ok, err := d.interactiveCommandFromArgs([]string{"attach", "--latest"})
	if err != nil {
		t.Fatalf("interactiveCommandFromArgs attach latest error = %v", err)
	}
	if !ok {
		t.Fatal("interactiveCommandFromArgs attach latest should be interactive")
	}
	if cmd != "safe-ag attach --latest" {
		t.Fatalf("interactiveCommandFromArgs attach latest = %q", cmd)
	}
}

func TestDashboardAgentTransferActions(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}

	var gotArgs []string
	d.runOrb = func(args ...string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		return []byte("ok"), nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/copy", strings.NewReader(`{"source":"/workspace/out.txt","destination":"./out.txt"}`))
	rec := httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("copy status = %d", rec.Code)
	}
	if strings.Join(gotArgs, " ") != "docker cp agent-beta:/workspace/out.txt "+filepath.Join(wd, "out.txt") {
		t.Fatalf("copy args = %q", strings.Join(gotArgs, " "))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/push", strings.NewReader(`{"source":"./in.txt","destination":"/workspace/in.txt"}`))
	rec = httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("push status = %d", rec.Code)
	}
	if strings.Join(gotArgs, " ") != "docker cp "+filepath.Join(wd, "in.txt")+" agent-beta:/workspace/in.txt" {
		t.Fatalf("push args = %q", strings.Join(gotArgs, " "))
	}
}

func TestDashboardAgentTransferActionsRejectOutsideWorkspace(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()

	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/copy", strings.NewReader(`{"source":"/workspace/out.txt","destination":"../escape.txt"}`))
	rec := httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("copy outside workspace status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/push", strings.NewReader(`{"source":"../secret.txt","destination":"/workspace/in.txt"}`))
	rec = httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("push outside workspace status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/copy", strings.NewReader(`{"source":"/home/agent/.claude/.claude.json","destination":"./out.txt"}`))
	rec = httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("copy outside container workspace status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/push", strings.NewReader(`{"source":"./in.txt","destination":"/home/agent/.bashrc"}`))
	rec = httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("push outside container workspace status = %d", rec.Code)
	}
}

func TestDashboardCheckpointActions(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()

	var gotArgs []string
	d.runCLI = func(args ...string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return "ok", nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/checkpoint-list", nil)
	rec := httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("checkpoint-list status = %d", rec.Code)
	}
	if strings.Join(gotArgs, " ") != "checkpoint list agent-beta" {
		t.Fatalf("checkpoint-list args = %q", strings.Join(gotArgs, " "))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/checkpoint-restore", strings.NewReader(`{"ref":"stash@{0}"}`))
	rec = httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("checkpoint-restore status = %d", rec.Code)
	}
	if strings.Join(gotArgs, " ") != "checkpoint restore agent-beta stash@{0}" {
		t.Fatalf("checkpoint-restore args = %q", strings.Join(gotArgs, " "))
	}
}

func TestDashboardCheckpointCreateRejectsBadJSON(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()

	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/checkpoint", strings.NewReader(`{"label":`))
	rec := httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("checkpoint bad json status = %d", rec.Code)
	}
}

func TestDashboardCommandEndpoint(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()
	d.runCLI = func(args ...string) (string, error) {
		return "ran: " + strings.Join(args, " "), nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/command", strings.NewReader(`{"args":["audit"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	d.handleAPICommand(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("command status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ran: audit") {
		t.Fatalf("command body = %q", rec.Body.String())
	}
}

func TestDashboardCommandEndpointRunBackgroundExecutes(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()
	var gotArgs []string
	d.runCLI = func(args ...string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return "ran", nil
	}

	req := httptest.NewRequest(http.MethodPost, "/api/command", strings.NewReader(`{"args":["run","git@github.com:0x666c6f/safe-agentic.git","--background"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	d.handleAPICommand(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("run background status = %d", rec.Code)
	}
	if strings.Join(gotArgs, " ") != "run git@github.com:0x666c6f/safe-agentic.git --background" {
		t.Fatalf("run background args = %q", strings.Join(gotArgs, " "))
	}
}

func TestDashboardCommandEndpointRejectsDisallowedCommand(t *testing.T) {
	d := NewDashboard("localhost:8420")

	req := httptest.NewRequest(http.MethodPost, "/api/command", strings.NewReader(`{"args":["unknown-cmd"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	d.handleAPICommand(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown command status = %d", rec.Code)
	}
}

func TestDashboardCommandEndpointRejectsSetupAndUpdate(t *testing.T) {
	d := NewDashboard("localhost:8420")

	for _, payload := range []string{
		`{"args":["setup"]}`,
		`{"args":["update","--full"]}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/command", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		d.handleAPICommand(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("payload %s status = %d", payload, rec.Code)
		}
	}
}

func TestNormalizeDashboardTailLines(t *testing.T) {
	cases := map[string]string{
		"":          "500",
		"42":        "42",
		"-1":        "500",
		"999999999": "500",
		"not-a-num": "500",
		"10000":     "10000",
	}
	for in, want := range cases {
		if got := normalizeDashboardTailLines(in); got != want {
			t.Fatalf("normalizeDashboardTailLines(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveDashboardContainerWorkspacePath(t *testing.T) {
	cases := map[string]string{
		"/workspace/file.txt": "/workspace/file.txt",
		"dir/file.txt":        "/workspace/dir/file.txt",
	}
	for in, want := range cases {
		got, err := resolveDashboardContainerWorkspacePath(in)
		if err != nil {
			t.Fatalf("resolveDashboardContainerWorkspacePath(%q) error = %v", in, err)
		}
		if got != want {
			t.Fatalf("resolveDashboardContainerWorkspacePath(%q) = %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{"/home/agent/.bashrc", "../escape", "/workspace/../etc/passwd"} {
		if _, err := resolveDashboardContainerWorkspacePath(in); err == nil {
			t.Fatalf("resolveDashboardContainerWorkspacePath(%q) should fail", in)
		}
	}
}

func TestResolveDashboardWorkspacePathManifest(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	if _, err := resolveDashboardWorkspacePath("../escape.yaml", true); err == nil {
		t.Fatal("resolveDashboardWorkspacePath should reject paths outside workspace")
	}
	got, err := resolveDashboardWorkspacePath("examples/fleet.yaml", true)
	if err != nil {
		t.Fatalf("resolveDashboardWorkspacePath() error = %v", err)
	}
	root, err := dashboardWorkspaceRoot()
	if err != nil {
		t.Fatalf("dashboardWorkspaceRoot() error = %v", err)
	}
	rel, err := filepath.Rel(root, got)
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}
	if rel != filepath.Join("examples", "fleet.yaml") {
		t.Fatalf("resolveDashboardWorkspacePath() rel = %q", rel)
	}
	if _, err := resolveDashboardWorkspacePath("examples/fleet.txt", true); err == nil {
		t.Fatal("resolveDashboardWorkspacePath should require yaml extension")
	}
}
