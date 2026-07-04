package poll

import (
	"testing"
	"time"

	"github.com/0x666c6f/safe-agentic/app/internal/emit"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
)

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestPollerEmitsOnChangeOnly(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker ps -a", "agent-x\tclaude\trepo\ton\t\t\t\t\t\t\ttmux\tUp 1 minute\n")
	rec := &emit.Recorder{}
	p := NewPoller(fake, rec, 20*time.Millisecond)
	p.Start()
	defer p.Stop()

	waitFor(t, func() bool { return len(rec.Named("agents.changed")) == 1 })
	// Same output → no second emit even after several intervals.
	time.Sleep(100 * time.Millisecond)
	if n := len(rec.Named("agents.changed")); n != 1 {
		t.Fatalf("want 1 emit on identical snapshots, got %d", n)
	}
	// Change output → one more emit.
	fake.SetResponse("docker ps -a", "agent-x\tclaude\trepo\ton\t\t\t\t\t\t\ttmux\tExited (0) now\n")
	waitFor(t, func() bool { return len(rec.Named("agents.changed")) == 2 })
}

func TestPollerEmitsVMStatusOnError(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("docker ps -a", "vm unreachable")
	rec := &emit.Recorder{}
	p := NewPoller(fake, rec, 20*time.Millisecond)
	p.Start()
	defer p.Stop()
	waitFor(t, func() bool { return len(rec.Named("vm.status")) >= 1 })
}
