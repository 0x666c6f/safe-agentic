package poll

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/0x666c6f/berth/app/internal/emit"
	"github.com/0x666c6f/berth/pkg/labels"
	"github.com/0x666c6f/berth/pkg/vmexec"
)

// Probe cadence: docker ps runs every tick (add/remove stays snappy), but the
// expensive per-agent state/activity exec probe and docker stats run less
// often. probeEvery/statsEvery count ticks; the first tick always does both.
const (
	probeEvery = 3 // per-agent state/activity probe (2 execs + 1s sleep each)
	statsEvery = 5 // docker stats + agents.stats emit
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

	// tick/labelCache are touched only inside pollOnce (serialized by pollMu).
	tick       int
	labelCache map[string]labelMeta
}

// labelMeta holds the immutable-per-container labels surfaced on Agent.
type labelMeta struct{ Prompt, MaxCost string }

// AgentStats is the per-container payload of the agents.stats event.
type AgentStats struct {
	CPU    string `json:"CPU"`
	Memory string `json:"Memory"`
	NetIO  string `json:"NetIO"`
	PIDs   string `json:"PIDs"`
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
	p.enrichLabels(ctx, agents)

	p.tick++
	probeTick := (p.tick-1)%probeEvery == 0
	statsTick := (p.tick-1)%statsEvery == 0

	p.mu.Lock()
	prev := p.last
	p.mu.Unlock()

	// Fresh probe/stats on their cadence; otherwise carry the last-known
	// values so throttling never blanks live agents or the InfoTab.
	if probeTick {
		probeActivities(ctx, p.exec, agents)
	} else {
		carryState(agents, prev)
	}
	if statsTick {
		p.mergeStats(ctx, agents)
	} else {
		carryStats(agents, prev)
	}

	p.mu.Lock()
	changed := !semanticEqual(agents, p.last)
	p.last = agents // always keep latest (fresh stats + carried/probed state)
	p.mu.Unlock()

	if changed {
		p.em.Emit("agents.changed", agents)
		if p.OnSnapshot != nil {
			p.OnSnapshot(agents)
		}
	}
	if statsTick {
		p.emitStats(agents)
	}
}

// enrichLabels fills Prompt/MaxCost from docker labels, inspecting each
// container at most once (labels are immutable per container).
func (p *Poller) enrichLabels(ctx context.Context, agents []Agent) {
	if p.labelCache == nil {
		p.labelCache = map[string]labelMeta{}
	}
	for i := range agents {
		name := agents[i].Name
		meta, ok := p.labelCache[name]
		if !ok {
			var got bool
			if meta, got = p.fetchLabels(ctx, name); got {
				p.labelCache[name] = meta
			}
		}
		agents[i].Prompt = meta.Prompt
		agents[i].MaxCost = meta.MaxCost
	}
}

// fetchLabels reads the container's label map. The bool is false only on a
// transient inspect error (retry next tick); a container with no labels caches
// an empty meta so it isn't re-inspected every tick.
func (p *Poller) fetchLabels(ctx context.Context, name string) (labelMeta, bool) {
	out, err := p.exec.Run(ctx, "docker", "inspect", "--format", "{{json .Config.Labels}}", name)
	if err != nil {
		return labelMeta{}, false
	}
	var m map[string]string
	if json.Unmarshal(out, &m) != nil {
		return labelMeta{}, true // empty/non-JSON output → nothing to surface
	}
	return labelMeta{Prompt: m[labels.Prompt], MaxCost: m[labels.MaxCost]}, true
}

// carryStats reuses the previous snapshot's docker-stats values on ticks where
// we skip the docker stats call, keeping Snapshot() coherent between samples.
func carryStats(agents []Agent, prev []Agent) {
	byName := make(map[string]Agent, len(prev))
	for _, a := range prev {
		byName[a.Name] = a
	}
	for i := range agents {
		if p, ok := byName[agents[i].Name]; ok {
			agents[i].CPU, agents[i].Memory = p.CPU, p.Memory
			agents[i].NetIO, agents[i].PIDs = p.NetIO, p.PIDs
		}
	}
}

// emitStats publishes name→stats for running agents that have samples. Kept
// separate from agents.changed (which is semantic-only) so the InfoTab's
// live CPU/Memory/NetIO/PIDs refresh without churning change-diffing.
func (p *Poller) emitStats(agents []Agent) {
	stats := map[string]AgentStats{}
	for _, a := range agents {
		if !a.Running {
			continue
		}
		if a.CPU == "" && a.Memory == "" && a.NetIO == "" && a.PIDs == "" {
			continue
		}
		stats[a.Name] = AgentStats{CPU: a.CPU, Memory: a.Memory, NetIO: a.NetIO, PIDs: a.PIDs}
	}
	if len(stats) > 0 {
		p.em.Emit("agents.stats", stats)
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
