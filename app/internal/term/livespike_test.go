//go:build livespike

package term

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0x666c6f/safe-agentic/app/internal/emit"
)

func spikeTarget() string {
	if v := os.Getenv("SPIKE_CONTAINER"); v != "" {
		return v
	}
	return "agent-shell-pty-spike"
}

func collect(rec *emit.Recorder, id string) string {
	var b strings.Builder
	for _, e := range rec.Named("term:data:" + id) {
		raw, _ := base64.StdEncoding.DecodeString(e.Data.(string))
		b.Write(raw)
	}
	return b.String()
}

func waitOutput(t *testing.T, rec *emit.Recorder, id string, within time.Duration, minLen int, label string) string {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if out := collect(rec, id); len(out) >= minLen {
			return out
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s: no PTY output within %s (got %d bytes)", label, within, len(collect(rec, id)))
	return ""
}

func TestLiveAttachSpike(t *testing.T) {
	target := spikeTarget()
	rec := &emit.Recorder{}
	m := NewManager(rec, DefaultFactory("safe-agentic"))
	id, err := m.Open(target, 137, 41)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer m.Close(id)

	if err := m.Resize(id, 120, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	first := waitOutput(t, rec, id, 15*time.Second, 1, "attach redraw")
	t.Logf("attach redraw: %d bytes", len(first))

	if err := m.Write(id, "echo spike-$((6*7))\r"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(collect(rec, id), "spike-42") {
			t.Logf("round-trip OK: command echoed and executed (spike-42 seen)")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("round-trip: 'spike-42' not seen in PTY output; tail: %q",
		tail(collect(rec, id), 400))
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
