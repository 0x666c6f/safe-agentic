package term

import (
	"encoding/base64"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/0x666c6f/safe-agentic/app/internal/emit"
)

func TestOpenWriteEchoClose(t *testing.T) {
	rec := &emit.Recorder{}
	m := NewManager(rec, func(container string) *exec.Cmd {
		return exec.Command("/bin/cat")
	})
	id, err := m.Open("agent-x")
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
		var got strings.Builder
		for _, e := range rec.Named("term:data:" + id) {
			raw, _ := base64.StdEncoding.DecodeString(e.Data.(string))
			got.Write(raw)
		}
		if strings.Contains(got.String(), "hello") {
			m.Close(id)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("echo not received over PTY")
}

func TestWriteUnknownID(t *testing.T) {
	m := NewManager(&emit.Recorder{}, func(string) *exec.Cmd { return exec.Command("/bin/cat") })
	if err := m.Write("nope", "x"); err == nil {
		t.Fatal("want error for unknown session id")
	}
}
