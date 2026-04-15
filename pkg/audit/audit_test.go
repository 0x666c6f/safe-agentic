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

func TestDefaultPath_UsesSafeAgStateHome(t *testing.T) {
	t.Setenv("HOME", "/custom/home")
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	path := DefaultPath()
	expected := "/custom/home/.safe-ag/state/audit.jsonl"
	if path != expected {
		t.Errorf("DefaultPath = %q, want %q", path, expected)
	}
}

func TestDefaultPath_WithoutXDGConfigHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	path := DefaultPath()
	if path == "" {
		t.Error("DefaultPath should not be empty")
	}
	if filepath.Base(path) != "audit.jsonl" {
		t.Errorf("expected basename audit.jsonl, got %q", filepath.Base(path))
	}
	if filepath.Base(filepath.Dir(path)) != "state" {
		t.Errorf("expected parent dir state, got %q", filepath.Base(filepath.Dir(path)))
	}
}

func TestRead_SkipsMalformedJSONLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	// Write a mix of valid and invalid JSON lines
	validEntry := `{"timestamp":"2024-01-01T00:00:00Z","action":"spawn","container":"agent-1"}` + "\n"
	invalidLine := "this is not json at all\n"
	validEntry2 := `{"timestamp":"2024-01-01T00:00:01Z","action":"stop","container":"agent-2"}` + "\n"
	content := validEntry + invalidLine + validEntry2

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	l := &Logger{Path: path}
	entries, err := l.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	// Malformed line should be skipped; only 2 valid entries returned
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (malformed line skipped), got %d", len(entries))
	}
	if entries[0].Action != "spawn" {
		t.Errorf("entries[0].Action: expected %q, got %q", "spawn", entries[0].Action)
	}
	if entries[1].Action != "stop" {
		t.Errorf("entries[1].Action: expected %q, got %q", "stop", entries[1].Action)
	}
}

func TestRead_ReturnsAllWhenNIsZero(t *testing.T) {
	dir := t.TempDir()
	l := &Logger{Path: filepath.Join(dir, "audit.jsonl")}

	for i := 0; i < 10; i++ {
		if err := l.Log("spawn", "agent", nil); err != nil {
			t.Fatalf("Log entry %d: %v", i, err)
		}
	}

	entries, err := l.Read(0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(entries) != 10 {
		t.Errorf("expected all 10 entries with n=0, got %d", len(entries))
	}
}

func TestLog_FailsOnUncreatableDir(t *testing.T) {
	// Use a path under an existing file (not a dir) to force MkdirAll failure
	dir := t.TempDir()
	blockingFile := filepath.Join(dir, "blocking")
	if err := os.WriteFile(blockingFile, []byte("x"), 0644); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	l := &Logger{Path: filepath.Join(blockingFile, "subdir", "audit.jsonl")}
	err := l.Log("spawn", "agent", nil)
	if err == nil {
		t.Error("expected error when MkdirAll fails, got nil")
	}
}

func TestLog_FailsWhenLogFileIsDirectory(t *testing.T) {
	// Make the audit log path itself a directory so os.OpenFile fails
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.jsonl")
	if err := os.Mkdir(logPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	l := &Logger{Path: logPath}
	err := l.Log("spawn", "agent", nil)
	if err == nil {
		t.Error("expected error when log path is a directory, got nil")
	}
}

func TestRead_FailsOnPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denial as root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	if err := os.WriteFile(path, []byte(`{"timestamp":"2024-01-01T00:00:00Z","action":"spawn","container":"a"}`+"\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Remove read permission
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(path, 0644) //nolint:errcheck

	l := &Logger{Path: path}
	_, err := l.Read(0)
	if err == nil {
		t.Error("expected error reading permission-denied file, got nil")
	}
}

func TestRead_FailsOnScannerError(t *testing.T) {
	// bufio.Scanner returns an error when a single line exceeds its buffer capacity.
	// Default max token size is bufio.MaxScanTokenSize = 64*1024 bytes.
	// Write a line that is longer than that to trigger scanner.Err().
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	// Create a valid JSON line, then an oversized line
	validLine := `{"timestamp":"2024-01-01T00:00:00Z","action":"spawn","container":"a"}` + "\n"
	// A line with a "field" value > 64KB triggers bufio.ErrTooLong
	oversizedValue := string(make([]byte, 70*1024))
	oversizedLine := `{"timestamp":"2024-01-01T00:00:00Z","action":"spawn","container":"` + oversizedValue + `"}` + "\n"
	content := validLine + oversizedLine

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	l := &Logger{Path: path}
	_, err := l.Read(0)
	if err == nil {
		t.Error("expected scanner error for oversized line, got nil")
	}
}
