package poll

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/0x666c6f/safe-agentic/pkg/agentstate"
	"github.com/0x666c6f/safe-agentic/pkg/tmux"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
)

// statePaneLines is how much of the live tmux pane the state probe inspects
// (mirrors tui/poller.go).
const statePaneLines = 40

const probeScript = `pids=$(pgrep -x %s 2>/dev/null); [ -z "$pids" ] && echo idle && exit 0
total() { t=0; for p in $pids; do s=$(awk '{print $14+$15}' /proc/$p/stat 2>/dev/null || echo 0); t=$((t+s)); done; echo $t; }
a=$(total); sleep 1; b=$(total)
[ "$b" -gt "$a" ] && echo working || echo idle`

func binaryFor(agentType string) string {
	if agentType == "codex" {
		return "codex"
	}
	return "claude"
}

func probeActivities(ctx context.Context, exec vmexec.Executor, agents []Agent) {
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
			script := strings.Replace(probeScript, "%s", binaryFor(a.Type), 1)
			out, err := exec.Run(ctx, "docker", "exec", a.Name, "bash", "-c", script)
			if err == nil && strings.Contains(string(out), "working") {
				a.Activity = "Working"
			}
			probeAgentState(ctx, exec, a)
		}(&agents[i])
	}
	wg.Wait()
}

// setStoppedState mirrors tui/poller.go: clean "Exited (0)" is done, anything
// else a non-zero exit.
func setStoppedState(a *Agent) {
	if a.Finished {
		a.State = string(agentstate.StateDone)
		a.StateReason = "exited cleanly"
		return
	}
	a.State = string(agentstate.StateExited)
	a.StateReason = a.Status
}

// probeAgentState captures the running container's tmux pane and classifies it
// with pkg/agentstate. Non-tmux terminal modes and unreadable panes report
// working (mirrors tui/poller.go).
func probeAgentState(ctx context.Context, exec vmexec.Executor, a *Agent) {
	if a.Terminal != "" && a.Terminal != "tmux" {
		a.State = string(agentstate.StateWorking)
		a.StateReason = "running (" + a.Terminal + " mode, no tmux pane)"
		return
	}
	out, err := exec.Run(ctx, "docker", "exec", a.Name, "tmux", "capture-pane",
		"-t", tmux.SessionName(), "-p", "-S", fmt.Sprintf("-%d", statePaneLines))
	if err != nil {
		a.State = string(agentstate.StateWorking)
		a.StateReason = "running (no tmux pane)"
		return
	}
	res := agentstate.Detect(a.Type, strings.Split(string(out), "\n"))
	a.State = res.State.String()
	a.StateReason = res.Reason
}
