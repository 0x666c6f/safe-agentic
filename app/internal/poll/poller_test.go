package poll

import (
	"testing"
	"time"

	"github.com/0x666c6f/berth/app/internal/emit"
	"github.com/0x666c6f/berth/pkg/vmexec"
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

func TestPollerIgnoresVolatileStatsChanges(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker ps -a", "agent-x\tclaude\trepo\ton\t\t\t\t\t\t\ttmux\tUp 1 minute\n")
	fake.SetResponse("docker stats", `{"Name":"agent-x","CPUPerc":"5.00%","MemUsage":"1MiB / 8GiB","NetIO":"1kB / 0B","PIDs":"5"}`)
	rec := &emit.Recorder{}
	p := NewPoller(fake, rec, 20*time.Millisecond)
	p.Start()
	defer p.Stop()

	waitFor(t, func() bool { return len(rec.Named("agents.changed")) == 1 })
	// Only volatile stats change → no new emit.
	fake.SetResponse("docker stats", `{"Name":"agent-x","CPUPerc":"87.00%","MemUsage":"2MiB / 8GiB","NetIO":"9kB / 1kB","PIDs":"7"}`)
	time.Sleep(100 * time.Millisecond)
	if n := len(rec.Named("agents.changed")); n != 1 {
		t.Fatalf("volatile stats change must not emit, got %d emits", n)
	}
}

func TestPollerEmitsStats(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker ps -a", "agent-x\tclaude\trepo\ton\t\t\t\t\t\t\ttmux\tUp 1 minute\n")
	fake.SetResponse("docker stats", `{"Name":"agent-x","CPUPerc":"5.00%","MemUsage":"1MiB / 8GiB","NetIO":"1kB / 0B","PIDs":"5"}`)
	rec := &emit.Recorder{}
	p := NewPoller(fake, rec, 20*time.Millisecond)
	p.Start()
	defer p.Stop()

	waitFor(t, func() bool { return len(rec.Named("agents.stats")) >= 1 })
	ev := rec.Named("agents.stats")[0]
	m, ok := ev.Data.(map[string]AgentStats)
	if !ok {
		t.Fatalf("agents.stats payload type %T", ev.Data)
	}
	if s := m["agent-x"]; s.CPU != "5.00%" || s.Memory != "1MiB / 8GiB" || s.PIDs != "5" {
		t.Fatalf("stats payload: %+v", m)
	}
}

func TestPollerEnrichesLabelsCached(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker ps -a", "agent-x\tclaude\trepo\ton\t\t\t\t\t\t\ttmux\tUp 1 minute\n")
	fake.SetResponse("docker inspect", `{"berth.prompt":"fix the bug","berth.max-cost":"2.50"}`)
	rec := &emit.Recorder{}
	p := NewPoller(fake, rec, 20*time.Millisecond)
	p.Start()
	defer p.Stop()

	waitFor(t, func() bool {
		s := p.Snapshot()
		return len(s) == 1 && s[0].Prompt == "fix the bug" && s[0].MaxCost == "2.50"
	})
	// Labels are immutable → inspected once and cached across ticks.
	time.Sleep(100 * time.Millisecond)
	if n := len(fake.CommandsMatching("docker inspect")); n != 1 {
		t.Fatalf("want 1 inspect (cached), got %d", n)
	}
}

func TestPollerStopBlocksFurtherEmits(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker ps -a", "agent-x\tclaude\trepo\ton\t\t\t\t\t\t\ttmux\tUp 1 minute\n")
	rec := &emit.Recorder{}
	p := NewPoller(fake, rec, 20*time.Millisecond)
	p.Start()
	waitFor(t, func() bool { return len(rec.Named("agents.changed")) == 1 })
	p.Stop()
	before := len(rec.Events)
	fake.SetResponse("docker ps -a", "agent-x\tclaude\trepo\ton\t\t\t\t\t\t\ttmux\tExited (0) now\n")
	time.Sleep(80 * time.Millisecond)
	if after := len(rec.Events); after != before {
		t.Fatalf("no emits allowed after Stop: before=%d after=%d", before, after)
	}
}

func TestPollerStopBeforeStart(t *testing.T) {
	p := NewPoller(vmexec.NewFake(), &emit.Recorder{}, time.Second)
	p.Stop() // must not hang
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
