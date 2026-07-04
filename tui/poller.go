package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/agentstate"
)

// statePaneLines is how much of the live tmux pane the poller inspects to infer
// agent state (mirrors cmd/safe-ag/status.go).
const statePaneLines = 40

// dockerPSEntry maps the JSON output of `docker ps --format '{{json .}}'`.
type dockerPSEntry struct {
	Names  string `json:"Names"`
	Status string `json:"Status"`
	Labels string `json:"Labels"`
}

// dockerStatsEntry maps the JSON output of `docker stats --no-stream --format '{{json .}}'`.
type dockerStatsEntry struct {
	Name     string `json:"Name"`
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
	NetIO    string `json:"NetIO"`
	PIDs     string `json:"PIDs"`
}

// Poller periodically fetches agent data from the VM.
type Poller struct {
	mu       sync.Mutex
	agents   []Agent
	stale    bool
	stopCh   chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once
	onUpdate func([]Agent, bool) // callback: agents, stale
}

// NewPoller creates a new Poller.
func NewPoller(onUpdate func([]Agent, bool)) *Poller {
	return &Poller{
		onUpdate: onUpdate,
		stopCh:   make(chan struct{}),
		stopped:  make(chan struct{}),
	}
}

// Start begins the polling loop.
func (p *Poller) Start() {
	go p.loop()
}

// Stop stops the polling loop and waits briefly for it to finish.
// Safe to call multiple times. Uses a short timeout so quit is never blocked
// by slow docker commands or a deadlocked QueueUpdateDraw.
func (p *Poller) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopCh)
	})
	select {
	case <-p.stopped:
	case <-time.After(500 * time.Millisecond):
	}
}

// GetAgents returns a copy of the current agent list (thread-safe).
func (p *Poller) GetAgents() []Agent {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]Agent, len(p.agents))
	copy(result, p.agents)
	return result
}

// ForceRefresh triggers an immediate poll outside the regular interval.
func (p *Poller) ForceRefresh() {
	go p.poll()
}

// Restart stops the current loop, creates new channels, and restarts.
func (p *Poller) Restart() {
	p.Stop()
	p.mu.Lock()
	p.stopCh = make(chan struct{})
	p.stopped = make(chan struct{})
	p.stopOnce = sync.Once{}
	p.mu.Unlock()
	go p.loop()
}

func (p *Poller) loop() {
	defer close(p.stopped)
	// Delay the initial poll slightly so TUI callers have time to start the
	// tview event loop before onUpdate triggers QueueUpdateDraw.
	select {
	case <-p.stopCh:
		return
	case <-time.After(100 * time.Millisecond):
	}
	p.poll()
	ticker := time.NewTicker(time.Duration(pollInterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

func (p *Poller) poll() {
	agents, err := fetchAgents()
	p.mu.Lock()
	if err != nil {
		p.stale = true
	} else {
		probeActivities(agents)
		p.agents = agents
		p.stale = false
	}
	stale := p.stale
	current := make([]Agent, len(p.agents))
	copy(current, p.agents)
	p.mu.Unlock()

	// Don't call onUpdate if we're stopping — tview may have exited and
	// QueueUpdateDraw would deadlock on the closed event loop.
	select {
	case <-p.stopCh:
		return
	default:
	}

	if p.onUpdate != nil {
		p.onUpdate(current, stale)
	}
}

// probeActivities checks each running container's agent process CPU usage in parallel
// and infers its semantic state (blocked/working/done/idle) from the tmux pane.
// Reads /proc/<pid>/stat twice with a short gap to detect CPU tick delta.
// This is per-container and works even when containers share auth volumes.
func probeActivities(agents []Agent) {
	var wg sync.WaitGroup
	for i := range agents {
		if !agents[i].Running {
			agents[i].Activity = "Stopped"
			setStoppedState(&agents[i])
			continue
		}
		wg.Add(1)
		go func(a *Agent) {
			defer wg.Done()
			a.Activity = probeProcessActivity(a.Name, a.Type)
			probeAgentState(a)
		}(&agents[i])
	}
	wg.Wait()
}

// setStoppedState classifies a stopped container: a clean "Exited (0)" is done,
// anything else is a non-zero exit. Mirrors status.go's terminalState without an
// extra inspect (the exit status is already in the poller's ps output).
func setStoppedState(a *Agent) {
	if a.Finished { // parsed from "Exited (0)"
		a.State = string(agentstate.StateDone)
		a.StateReason = "exited cleanly"
		return
	}
	a.State = string(agentstate.StateExited)
	a.StateReason = a.Status
}

// probeAgentState captures the running container's tmux pane and classifies it.
// Non-tmux terminal modes have no pane to read, so they are reported as working
// (mirrors status.go's resolveState).
func probeAgentState(a *Agent) {
	if a.Terminal != "" && a.Terminal != "tmux" {
		a.State = string(agentstate.StateWorking)
		a.StateReason = "running (" + a.Terminal + " mode, no tmux pane)"
		return
	}
	out, err := execVM("docker", "exec", a.Name, "tmux", "capture-pane",
		"-t", tmuxSessionName, "-p", "-S", fmt.Sprintf("-%d", statePaneLines))
	if err != nil {
		// No readable pane on a running container — headless/background mode, a
		// shell running bash directly, or a session still starting. It is up,
		// so report working rather than unknown. (The Terminal label is
		// unreliable: spawn stamps every container "tmux".)
		a.State = string(agentstate.StateWorking)
		a.StateReason = "running (no tmux pane)"
		return
	}
	res := agentstate.Detect(a.Type, strings.Split(string(out), "\n"))
	a.State = res.State.String()
	a.StateReason = res.Reason
}

// probeProcessActivity checks if the agent process (codex or claude) consumed
// any CPU ticks in a 1-second window. Returns "Working" or "Idle".
func probeProcessActivity(name, agentType string) string {
	// Both Claude and Codex are native binaries: pgrep -x matches the process name.
	// We sum CPU ticks across ALL matching pids (agent + child processes).
	var pgrepCmd string
	switch agentType {
	case "claude":
		pgrepCmd = `pgrep -x claude 2>/dev/null`
	default:
		pgrepCmd = `pgrep -x codex 2>/dev/null`
	}

	// Read utime+stime from /proc/<pid>/stat for all matching PIDs at two points 1s apart.
	// Fields 14 (utime) and 15 (stime) are cumulative CPU ticks.
	script := `pids=$(` + pgrepCmd + `); ` +
		`[ -n "$pids" ] || exit 1; ` +
		`sum1=0; for p in $pids; do t=$(awk '{print $14+$15}' /proc/$p/stat 2>/dev/null) && sum1=$((sum1+t)); done; ` +
		`sleep 1; ` +
		`sum2=0; for p in $pids; do t=$(awk '{print $14+$15}' /proc/$p/stat 2>/dev/null) && sum2=$((sum2+t)); done; ` +
		`[ "$sum2" -gt "$sum1" ] && echo working`
	out, err := execVM("docker", "exec", name, "bash", "-c", script)
	if err == nil && strings.TrimSpace(string(out)) == "working" {
		return "Working"
	}
	return "Idle"
}

func execVM(args ...string) ([]byte, error) {
	return execVMTimeout(5*time.Second, args...)
}

func execVMLong(args ...string) ([]byte, error) {
	return execVMTimeout(30*time.Second, args...)
}

func execVMTimeout(timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "container", machineRunArgs(args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	err := cmd.Run()
	return out.Bytes(), err
}

func fetchAgents() ([]Agent, error) {
	// Use a custom format with explicit label extraction to avoid comma-in-value
	// corruption that occurs with Docker's {{json .}} Labels field.
	format := strings.Join([]string{
		"{{.Names}}",
		`{{.Label "safe-agentic.agent-type"}}`,
		`{{.Label "safe-agentic.repo-display"}}`,
		`{{.Label "safe-agentic.ssh"}}`,
		`{{.Label "safe-agentic.auth"}}`,
		`{{.Label "safe-agentic.gh-auth"}}`,
		`{{.Label "safe-agentic.docker"}}`,
		`{{.Label "safe-agentic.network-mode"}}`,
		`{{.Label "safe-agentic.fleet"}}`,
		`{{.Label "safe-agentic.hierarchy"}}`,
		`{{.Label "safe-agentic.terminal"}}`,
		"{{.Status}}",
	}, "\t")
	psData, psErr := execVM("docker", "ps", "-a",
		"--filter", "name=^"+containerPrefix+"-",
		"--format", format)
	if psErr != nil {
		return nil, psErr
	}

	agents := parsePSOutput(psData)

	// docker stats doesn't support --filter; pass running container names explicitly
	var running []string
	for _, a := range agents {
		if a.Running {
			running = append(running, a.Name)
		}
	}
	if len(running) > 0 {
		args := append([]string{"docker", "stats", "--no-stream", "--format", "{{json .}}"}, running...)
		statsData, _ := execVM(args...)
		mergeStats(agents, statsData)
	}

	return agents, nil
}

func parsePSOutput(data []byte) []Agent {
	var agents []Agent
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		parts := strings.Split(string(line), "\t")
		if len(parts) < 12 {
			continue
		}
		status, running, finished := normalizeContainerStatus(parts[11])
		agents = append(agents, Agent{
			Name:        parts[0],
			Type:        parts[1],
			Repo:        parts[2],
			SSH:         parts[3],
			Auth:        parts[4],
			GHAuth:      parts[5],
			Docker:      parts[6],
			NetworkMode: parts[7],
			Fleet:       parts[8],
			Hierarchy:   parts[9],
			Terminal:    parts[10],
			Status:      status,
			Running:     running,
			Finished:    finished,
		})
	}
	return agents
}

func normalizeContainerStatus(raw string) (status string, running, finished bool) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "Up"):
		return raw, true, false
	case strings.HasPrefix(raw, "Exited (0)"):
		suffix := strings.TrimSpace(strings.TrimPrefix(raw, "Exited (0)"))
		if suffix == "" {
			return "Finished", false, true
		}
		return "Finished " + suffix, false, true
	default:
		return raw, false, false
	}
}

func mergeStats(agents []Agent, data []byte) {
	statsMap := make(map[string]dockerStatsEntry)
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var entry dockerStatsEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		statsMap[entry.Name] = entry
	}
	for i := range agents {
		if s, ok := statsMap[agents[i].Name]; ok {
			agents[i].CPU = s.CPUPerc
			agents[i].Memory = s.MemUsage
			agents[i].NetIO = s.NetIO
			agents[i].PIDs = s.PIDs
		}
	}
}

func parseLabels(raw string) map[string]string {
	labels := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			labels[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return labels
}

func splitLines(data []byte) [][]byte {
	return bytes.Split(bytes.TrimSpace(data), []byte("\n"))
}
