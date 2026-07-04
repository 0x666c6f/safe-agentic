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

// TestWatcherHandlesPartialLineAcrossAppends covers Finding 1: appending a
// line WITHOUT a trailing newline must not be consumed (and must not corrupt
// the offset) until a later append supplies the completing newline. Both
// lines must eventually be emitted intact.
func TestWatcherHandlesPartialLineAcrossAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &emit.Recorder{}
	w := NewWatcher(path, rec, 10*time.Millisecond)
	w.Notify = func(events.SystemNotification) error { return nil }
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	partial := `{"timestamp":"2026-07-04T10:00:00Z","type":"agent.exit","payload":{"container":"agent-partial"}}`
	if _, err := f.WriteString(partial); err != nil { // deliberately no trailing newline
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Give the watcher several ticks to observe the file while the last line
	// is still incomplete: it must emit nothing yet.
	time.Sleep(80 * time.Millisecond)
	if n := len(rec.Named("event.new")); n != 0 {
		t.Fatalf("want 0 events before the completing newline arrives, got %d", n)
	}

	f, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	second := `{"timestamp":"2026-07-04T10:00:01Z","type":"agent.exit","payload":{"container":"agent-second"}}`
	if _, err := f.WriteString("\n" + second + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(rec.Named("event.new")) < 2 {
		time.Sleep(10 * time.Millisecond)
	}
	got := rec.Named("event.new")
	if len(got) != 2 {
		t.Fatalf("want 2 events, got %d: %+v", len(got), got)
	}
	firstEvent := eventFromRecorded(t, got[0])
	if firstEvent.Payload["container"] != "agent-partial" {
		t.Fatalf("first event corrupted: %+v", firstEvent)
	}
	secondEvent := eventFromRecorded(t, got[1])
	if secondEvent.Payload["container"] != "agent-second" {
		t.Fatalf("second event corrupted: %+v", secondEvent)
	}
}

// TestWatcherPicksUpEventsAfterTruncation covers the truncation/rotation
// reset path: if the file shrinks below the last-known offset, the watcher
// must reset to the start and pick up newly appended events rather than
// getting stuck waiting for bytes that will never arrive.
func TestWatcherPicksUpEventsAfterTruncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	initial := `{"timestamp":"2026-07-04T10:00:00Z","type":"agent.exit","payload":{"container":"agent-before","note":"padding-padding-padding-padding"}}` + "\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &emit.Recorder{}
	w := NewWatcher(path, rec, 10*time.Millisecond)
	w.Notify = func(events.SystemNotification) error { return nil }
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Rewrite the file smaller than the offset Start() captured (simulates
	// truncation/rotation), with a fresh event appended.
	truncated := `{"timestamp":"2026-07-04T10:00:02Z","type":"agent.exit","payload":{"container":"agent-after"}}` + "\n"
	if len(truncated) >= len(initial) {
		t.Fatalf("test setup invalid: truncated content must be shorter than initial")
	}
	if err := os.WriteFile(path, []byte(truncated), 0o600); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(rec.Named("event.new")) < 1 {
		time.Sleep(10 * time.Millisecond)
	}
	got := rec.Named("event.new")
	if len(got) != 1 {
		t.Fatalf("want 1 event after truncation, got %d: %+v", len(got), got)
	}
	ev := eventFromRecorded(t, got[0])
	if ev.Payload["container"] != "agent-after" {
		t.Fatalf("want agent-after, got %+v", ev)
	}
}

// TestWatcherStopWaitsForGoroutine covers Finding 2: Stop() must block until
// the watch loop goroutine has actually returned, so callers can rely on no
// further drain()/Notify activity happening after Stop() returns.
func TestWatcherStopWaitsForGoroutine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rec := &emit.Recorder{}
	w := NewWatcher(path, rec, 5*time.Millisecond)
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}

	w.Stop()
	select {
	case <-w.done:
	default:
		t.Fatal("Stop() returned before the watch goroutine's done channel was closed")
	}

	// Calling Stop() again must not hang or panic.
	w.Stop()
}

// TestWatcherStopBeforeStart covers the guard for Stop() called without a
// prior Start(): done is never closed, so Stop() must not block forever.
func TestWatcherStopBeforeStart(t *testing.T) {
	w := NewWatcher(filepath.Join(t.TempDir(), "events.jsonl"), &emit.Recorder{}, time.Second)
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() before Start() blocked forever")
	}
}

// eventFromRecorded unwraps the events.Event that drain() packs into the
// "event.new" emit payload.
func eventFromRecorded(t *testing.T, r emit.Recorded) events.Event {
	t.Helper()
	data, ok := r.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected recorded data type %T", r.Data)
	}
	ev, ok := data["event"].(events.Event)
	if !ok {
		t.Fatalf("unexpected event field type %T", data["event"])
	}
	return ev
}
