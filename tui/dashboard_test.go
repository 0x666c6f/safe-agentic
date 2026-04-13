package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	installFakeOrb(t)

	d := NewDashboard("localhost:8420")
	d.poller.agents = testAgents()

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
	if !strings.Contains(rec.Body.String(), `"agent-beta"`) {
		t.Fatalf("handleAPIStop body = %q", rec.Body.String())
	}
	logData, err := os.ReadFile(os.Getenv("SAFE_AGENTIC_TEST_ORB_LOG"))
	if err != nil {
		t.Fatalf("read orb log: %v", err)
	}
	if !strings.Contains(string(logData), "docker stop agent-beta") {
		t.Fatalf("orb log = %q", string(logData))
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
