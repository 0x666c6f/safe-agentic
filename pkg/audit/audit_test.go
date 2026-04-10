package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogAndRead(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	if err := l.Log("spawn", "agent-1", map[string]string{"repo": "github.com/org/repo"}); err != nil {
		t.Fatalf("Log entry 1: %v", err)
	}
	if err := l.Log("stop", "agent-2", nil); err != nil {
		t.Fatalf("Log entry 2: %v", err)
	}

	entries, err := l.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Action != "spawn" {
		t.Errorf("entry[0].Action: expected %q, got %q", "spawn", entries[0].Action)
	}
	if entries[0].Container != "agent-1" {
		t.Errorf("entry[0].Container: expected %q, got %q", "agent-1", entries[0].Container)
	}
	if entries[1].Action != "stop" {
		t.Errorf("entry[1].Action: expected %q, got %q", "stop", entries[1].Action)
	}
	if entries[1].Container != "agent-2" {
		t.Errorf("entry[1].Container: expected %q, got %q", "agent-2", entries[1].Container)
	}
}

func TestLogCreatesParentDirectory(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "deep", "nested", "dir")
	l := &Logger{Path: filepath.Join(nested, "audit.jsonl")}

	if err := l.Log("spawn", "agent-1", nil); err != nil {
		t.Fatalf("Log: %v", err)
	}

	if _, err := os.Stat(l.Path); err != nil {
		t.Errorf("audit file not created: %v", err)
	}
}

func TestReadWithLimit(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	for i := 0; i < 20; i++ {
		action := "spawn"
		if i%2 == 0 {
			action = "stop"
		}
		if err := l.Log(action, "agent", nil); err != nil {
			t.Fatalf("Log entry %d: %v", i, err)
		}
	}

	entries, err := l.Read(5)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries with limit, got %d", len(entries))
	}
	// last 5 of 20: entries 15-19, alternating stop/spawn
	// entry 15: i=15, 15%2!=0 -> "spawn"
	// entry 16: i=16, 16%2==0 -> "stop"
	// last entry (i=19) is "spawn" (19%2!=0)
	if entries[4].Action != "spawn" {
		t.Errorf("last entry action: expected %q, got %q", "spawn", entries[4].Action)
	}
}

func TestReadMissingFile(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{Path: filepath.Join(dir, "nonexistent.jsonl")}

	entries, err := l.Read(0)
	if err != nil {
		t.Fatalf("Read on missing file should return nil error, got: %v", err)
	}
	if entries != nil {
		t.Errorf("Read on missing file should return nil entries, got %v", entries)
	}
}

func TestLogDetailsWithSpecialCharacters(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	details := map[string]string{
		"message": `say "hello" & <world>`,
		"path":    "/tmp/foo\nbar",
		"unicode": "日本語テスト",
	}
	if err := l.Log("spawn", "agent-special", details); err != nil {
		t.Fatalf("Log: %v", err)
	}

	entries, err := l.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Details["message"] != details["message"] {
		t.Errorf("details[message]: expected %q, got %q", details["message"], e.Details["message"])
	}
	if e.Details["path"] != details["path"] {
		t.Errorf("details[path]: expected %q, got %q", details["path"], e.Details["path"])
	}
	if e.Details["unicode"] != details["unicode"] {
		t.Errorf("details[unicode]: expected %q, got %q", details["unicode"], e.Details["unicode"])
	}
}
