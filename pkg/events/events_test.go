package events

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEmitWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	if err := Emit(path, "agent.started", map[string]string{"container": "agent-1"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if err := Emit(path, "agent.stopped", map[string]string{"container": "agent-1", "exit": "0"}); err != nil {
		t.Fatalf("Emit second event: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal line: %v", err)
		}
		events = append(events, e)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestEmitTypeField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	if err := Emit(path, "agent.started", map[string]string{"container": "agent-1"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("expected at least one line")
	}
	var e Event
	if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if e.Type != "agent.started" {
		t.Errorf("Type: expected %q, got %q", "agent.started", e.Type)
	}
	if e.Payload["container"] != "agent-1" {
		t.Errorf("Payload[container]: expected %q, got %q", "agent-1", e.Payload["container"])
	}
	if e.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
}

func TestEmitCreatesParentDirectory(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "deep", "nested", "events.jsonl")

	if err := Emit(path, "test.event", nil); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("events file not created: %v", err)
	}
}

func TestEmitCreatesPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "events.jsonl")

	if err := Emit(path, "test.event", nil); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat events file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("events file mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat events dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("events dir mode = %o, want 700", got)
	}
}

func TestEmitAppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	for i := 0; i < 5; i++ {
		if err := Emit(path, "test.event", map[string]string{"i": "x"}); err != nil {
			t.Fatalf("Emit %d: %v", i, err)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		count++
	}
	if count != 5 {
		t.Errorf("expected 5 lines, got %d", count)
	}
}

func TestReadReturnsRecentEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	for _, typ := range []string{"one", "two", "three"} {
		if err := Emit(path, typ, map[string]string{"container": "agent"}); err != nil {
			t.Fatalf("Emit %s: %v", typ, err)
		}
	}
	events, err := Read(path, 2)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(events) != 2 || events[0].Type != "two" || events[1].Type != "three" {
		t.Fatalf("events = %#v", events)
	}
}

func TestReadMissingFile(t *testing.T) {
	events, err := Read(filepath.Join(t.TempDir(), "missing.jsonl"), 0)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events len = %d, want 0", len(events))
	}
}

func TestClassifyFields(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		payload   map[string]string
		want      string
	}{
		{
			name:      "explicit",
			eventType: "agent.note",
			payload:   map[string]string{"status": StatusReadyForReview},
			want:      StatusReadyForReview,
		},
		{
			name:      "auth",
			eventType: "agent.failed",
			payload:   map[string]string{"error": "401 unauthorized from GitHub"},
			want:      StatusNeedsAuth,
		},
		{
			name:      "tests",
			eventType: "action.failed",
			payload:   map[string]string{"command": "go test ./...", "error": "exit 1"},
			want:      StatusFailedTests,
		},
		{
			name:      "blocked-explicit",
			eventType: "agent.state",
			payload:   map[string]string{"status": StatusBlocked},
			want:      StatusBlocked,
		},
		{
			name:      "blocked-heuristic",
			eventType: "agent.state",
			payload:   map[string]string{"reason": "permission prompt", "detail": "waiting for input"},
			want:      StatusBlocked,
		},
		{
			name:      "auth-still-wins-over-blocked",
			eventType: "agent.failed",
			payload:   map[string]string{"error": "permission denied (publickey)"},
			want:      StatusNeedsAuth,
		},
		{
			name:      "stuck",
			eventType: "pipeline.failed",
			payload:   map[string]string{"error": "unmet dependencies"},
			want:      StatusStuck,
		},
		{
			name:      "failed",
			eventType: "cron.failed",
			payload:   map[string]string{"error": "exit 1"},
			want:      StatusFailed,
		},
		{
			name:      "info",
			eventType: "agent.spawned",
			payload:   map[string]string{"container": "agent"},
			want:      StatusInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyFields(tt.eventType, tt.payload); got != tt.want {
				t.Fatalf("ClassifyFields() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaultEventsPath(t *testing.T) {
	path := DefaultEventsPath()
	if path == "" {
		t.Error("DefaultEventsPath should not be empty")
	}
	if filepath.Base(path) != "events.jsonl" {
		t.Errorf("expected basename events.jsonl, got %q", filepath.Base(path))
	}
}

func TestDefaultEventsPath_UsesBerthStateHome(t *testing.T) {
	t.Setenv("HOME", "/custom/home")
	t.Setenv("XDG_CONFIG_HOME", "/custom/xdg/config")
	path := DefaultEventsPath()
	expected := "/custom/home/.berth/state/events.jsonl"
	if path != expected {
		t.Errorf("DefaultEventsPath = %q, want %q", path, expected)
	}
}

func TestDefaultEventsPath_WithoutXDGConfigHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	path := DefaultEventsPath()
	if filepath.Base(path) != "events.jsonl" {
		t.Errorf("expected basename events.jsonl, got %q", filepath.Base(path))
	}
	if filepath.Base(filepath.Dir(path)) != "state" {
		t.Errorf("expected parent dir state, got %q", filepath.Base(filepath.Dir(path)))
	}
}

func TestEmit_FailsWhenPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create a directory at the path where we'd normally create the events file
	targetPath := filepath.Join(dir, "events.jsonl")
	if err := os.Mkdir(targetPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Emit should fail because the "file" path is actually a directory
	err := Emit(targetPath, "test.event", nil)
	if err == nil {
		t.Error("expected error when event file path is a directory, got nil")
	}
}

func TestEmit_FailsWhenMkdirAllFails(t *testing.T) {
	dir := t.TempDir()
	// Create a file where MkdirAll would need to create a directory
	blockingFile := filepath.Join(dir, "blocking")
	if err := os.WriteFile(blockingFile, []byte("x"), 0644); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	// Path under the blocking file — MkdirAll will fail because blocking is a file not a dir
	path := filepath.Join(blockingFile, "subdir", "events.jsonl")
	err := Emit(path, "test.event", nil)
	if err == nil {
		t.Error("expected error when MkdirAll fails, got nil")
	}
}
