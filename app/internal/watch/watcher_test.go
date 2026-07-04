package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0x666c6f/safe-agentic/app/internal/emit"
	"github.com/0x666c6f/safe-agentic/pkg/events"
)

func TestWatcherEmitsAndNotifiesOnAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &emit.Recorder{}
	w := NewWatcher(path, rec, 20*time.Millisecond)
	var notified []events.SystemNotification
	w.Notify = func(n events.SystemNotification) error { notified = append(notified, n); return nil }
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(`{"timestamp":"2026-07-04T10:00:00Z","type":"agent.exit","payload":{"container":"agent-x","status":"needs-auth","message":"auth expired"}}` + "\n")
	f.WriteString(`{"timestamp":"2026-07-04T10:00:01Z","type":"agent.exit","payload":{"container":"agent-y","status":"info"}}` + "\n")
	f.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(rec.Named("event.new")) == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(rec.Named("event.new")); n != 2 {
		t.Fatalf("want 2 event.new, got %d", n)
	}
	if len(notified) != 1 || notified[0].Container != "agent-x" || notified[0].Message != "auth expired" {
		t.Fatalf("want 1 notification for agent-x, got %+v", notified)
	}
}
