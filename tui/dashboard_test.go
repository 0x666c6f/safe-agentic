package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if !strings.Contains(rec.Body.String(), "agent-beta") {
		t.Fatalf("interactive body = %q", rec.Body.String())
	}
}

func TestDashboardAgentTransferActions(t *testing.T) {
	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()

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
	if strings.Join(gotArgs, " ") != "docker cp agent-beta:/workspace/out.txt ./out.txt" {
		t.Fatalf("copy args = %q", strings.Join(gotArgs, " "))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/agents/agent-beta/action/push", strings.NewReader(`{"source":"./in.txt","destination":"/workspace/in.txt"}`))
	rec = httptest.NewRecorder()
	d.handleAPIAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("push status = %d", rec.Code)
	}
	if strings.Join(gotArgs, " ") != "docker cp ./in.txt agent-beta:/workspace/in.txt" {
		t.Fatalf("push args = %q", strings.Join(gotArgs, " "))
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
