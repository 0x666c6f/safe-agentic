package poll

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/0x666c6f/safe-agentic/app/internal/emit"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
)

type Poller struct {
	exec       vmexec.Executor
	em         emit.Emitter
	interval   time.Duration
	OnSnapshot func([]Agent)

	mu      sync.Mutex
	last    []Agent
	lastOK  *bool
	stop    chan struct{}
	stopped sync.Once
	done    chan struct{}
	started bool
	pollMu  sync.Mutex // serializes pollOnce across ticker and ForceRefresh
}

func NewPoller(exec vmexec.Executor, em emit.Emitter, interval time.Duration) *Poller {
	return &Poller{exec: exec, em: em, interval: interval,
		stop: make(chan struct{}), done: make(chan struct{})}
}

func (p *Poller) Start() {
	p.started = true
	go func() {
		defer close(p.done)
		p.pollOnce()
		t := time.NewTicker(p.interval)
		defer t.Stop()
		for {
			select {
			case <-p.stop:
				return
			case <-t.C:
				p.pollOnce()
			}
		}
	}()
}

// Stop blocks until the poll loop has exited and any in-flight poll
// (including ForceRefresh) has drained — no emits after Stop returns.
func (p *Poller) Stop() {
	p.stopped.Do(func() { close(p.stop) })
	if p.started {
		<-p.done
	}
	p.pollMu.Lock()
	p.pollMu.Unlock() //nolint:staticcheck // drain in-flight pollOnce
}

func (p *Poller) ForceRefresh() { go p.pollOnce() }

func (p *Poller) Snapshot() []Agent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Agent(nil), p.last...)
}

func (p *Poller) setVMStatus(ok bool, errMsg string) {
	p.mu.Lock()
	changed := p.lastOK == nil || *p.lastOK != ok
	p.lastOK = &ok
	p.mu.Unlock()
	if changed {
		p.em.Emit("vm.status", map[string]any{"ok": ok, "error": errMsg})
	}
}

// semanticEqual compares snapshots ignoring volatile docker-stats fields
// (CPU/Memory/NetIO/PIDs churn every poll on any busy container and would
// defeat change-diffing).
func semanticEqual(a, b []Agent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		x.CPU, x.Memory, x.NetIO, x.PIDs = "", "", "", ""
		y.CPU, y.Memory, y.NetIO, y.PIDs = "", "", "", ""
		if x != y {
			return false
		}
	}
	return true
}

func (p *Poller) pollOnce() {
	p.pollMu.Lock()
	defer p.pollMu.Unlock()
	select {
	case <-p.stop:
		return
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := p.exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^agent-", "--format", PSFormat())
	if err != nil {
		p.setVMStatus(false, err.Error())
		return
	}
	p.setVMStatus(true, "")
	agents := ParsePS(out)
	p.mergeStats(ctx, agents)
	probeActivities(ctx, p.exec, agents)

	p.mu.Lock()
	changed := !semanticEqual(agents, p.last)
	if changed {
		p.last = agents
	}
	p.mu.Unlock()
	if changed {
		p.em.Emit("agents.changed", agents)
		if p.OnSnapshot != nil {
			p.OnSnapshot(agents)
		}
	}
}

type dockerStatsEntry struct {
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
	NetIO    string `json:"NetIO"`
	PIDs     string `json:"PIDs"`
}

// docker stats has no --filter: pass running container names explicitly.
func (p *Poller) mergeStats(ctx context.Context, agents []Agent) {
	var running []string
	for _, a := range agents {
		if a.Running {
			running = append(running, a.Name)
		}
	}
	if len(running) == 0 {
		return
	}
	args := append([]string{"docker", "stats", "--no-stream", "--format", "{{json .}}"}, running...)
	out, err := p.exec.Run(ctx, args...)
	if err != nil {
		return
	}
	byName := map[string]dockerStatsEntry{}
	for _, line := range strings.Split(string(out), "\n") {
		var e dockerStatsEntry
		if json.Unmarshal([]byte(line), &e) == nil && e.Name != "" {
			byName[e.Name] = e
		}
	}
	for i := range agents {
		if e, ok := byName[agents[i].Name]; ok {
			agents[i].CPU, agents[i].Memory = e.CPUPerc, e.MemUsage
			agents[i].NetIO, agents[i].PIDs = e.NetIO, e.PIDs
		}
	}
}
