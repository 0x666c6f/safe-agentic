package term

import (
	"encoding/base64"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0x666c6f/berth/app/internal/emit"
)

func TestNewManagerNilFactoryFallsBack(t *testing.T) {
	m := NewManager(&emit.Recorder{}, nil)
	if m.factory == nil {
		t.Fatal("nil factory must fall back to DefaultFactory")
	}
}

func TestDefaultFactorySingleTERM(t *testing.T) {
	t.Setenv("TERM", "dumb")
	cmd := DefaultFactory("berth")("agent-x")
	var terms []string
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "TERM=") {
			terms = append(terms, kv)
		}
	}
	if len(terms) != 1 || terms[0] != "TERM=xterm-256color" {
		t.Fatalf("want exactly TERM=xterm-256color, got %v", terms)
	}
}

func TestOpenWriteEchoClose(t *testing.T) {
	rec := &emit.Recorder{}
	m := NewManager(rec, func(container string) *exec.Cmd {
		return exec.Command("/bin/cat")
	})
	id, err := m.Open("agent-x", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Resize(id, 80, 24); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(id, "hello\r"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(decodeChunks(rec, id), "hello") {
			m.Close(id)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("echo not received over PTY")
}

// decodeChunks joins a subscriber's term:data payloads ("seq|base64" — seq
// must be present and monotonically increasing).
func decodeChunks(rec *emit.Recorder, id string) string {
	var got strings.Builder
	last := 0
	for _, e := range rec.Named("term:data:" + id) {
		payload := e.Data.(string)
		seqStr, b64, ok := strings.Cut(payload, "|")
		if !ok {
			panic("term:data payload missing seq prefix: " + payload)
		}
		seq, err := strconv.Atoi(seqStr)
		if err != nil || seq <= last {
			panic("term:data seq not increasing: " + payload)
		}
		last = seq
		raw, _ := base64.StdEncoding.DecodeString(b64)
		got.Write(raw)
	}
	return got.String()
}

func TestWriteUnknownID(t *testing.T) {
	m := NewManager(&emit.Recorder{}, func(string) *exec.Cmd { return exec.Command("/bin/cat") })
	if err := m.Write("nope", "x"); err == nil {
		t.Fatal("want error for unknown session id")
	}
}

func TestSecondOpenSameContainerSharesPTY(t *testing.T) {
	rec := &emit.Recorder{}
	spawns := 0
	m := NewManager(rec, func(container string) *exec.Cmd {
		spawns++
		return exec.Command("/bin/cat")
	})
	id1, err := m.Open("agent-x", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := m.Open("agent-x", 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	if spawns != 1 {
		t.Fatalf("same container must share one PTY, spawned %d", spawns)
	}
	if id1 == id2 {
		t.Fatal("subscribers need distinct ids")
	}

	// Output fans out to both subscribers.
	if err := m.Write(id2, "fanout\r"); err != nil {
		t.Fatal(err)
	}
	seen := func(id string) bool {
		return strings.Contains(decodeChunks(rec, id), "fanout")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !(seen(id1) && seen(id2)) {
		time.Sleep(20 * time.Millisecond)
	}
	if !seen(id1) || !seen(id2) {
		t.Fatalf("fan-out missing: id1=%v id2=%v", seen(id1), seen(id2))
	}

	// PTY tracks the smallest subscriber; closing it frees the bigger grid.
	m.mu.Lock()
	s := m.byID[id1]
	m.mu.Unlock()
	if c, r := s.minSize(); c != 100 || r != 30 {
		t.Fatalf("min size = %dx%d, want 100x30", c, r)
	}
	if err := m.Close(id2); err != nil {
		t.Fatal(err)
	}
	if c, r := s.minSize(); c != 120 || r != 40 {
		t.Fatalf("after close min size = %dx%d, want 120x40", c, r)
	}
	// First subscriber still lives on the shared PTY; last close tears down.
	if err := m.Write(id1, "x"); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(id1); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(id1, "x"); err == nil {
		t.Fatal("closed session must not accept writes")
	}
}
