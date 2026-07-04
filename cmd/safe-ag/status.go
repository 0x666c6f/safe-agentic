package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/agentstate"
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/events"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/tmux"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"

	"github.com/spf13/cobra"
)

// statePaneLines is how much of the live tmux pane to inspect for state.
const statePaneLines = 40

var (
	statusAll  bool
	statusJSON bool
)

var statusCmd = &cobra.Command{
	Use:   "status [name|--latest]",
	Short: "Show live agent state (blocked/working/done/idle/exited)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusAll, "all", false, "Show all safe-agentic containers")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
	addLatestFlag(statusCmd)
	rootCmd.AddCommand(statusCmd)
}

// statusInfo is one agent's status row.
type statusInfo struct {
	Name      string `json:"name"`
	AgentType string `json:"agent_type"`
	Running   bool   `json:"running"`
	Status    string `json:"status"` // raw docker state (running / exited / …)
	State     string `json:"state"`  // agentstate classification
	Reason    string `json:"reason"`
	Matched   string `json:"matched,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	if statusAll {
		return runStatusAll(ctx, exec)
	}

	target := targetFromArgs(cmd, args)
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}
	info := gatherStatus(ctx, exec, name)
	if statusJSON {
		return encodeStatusJSON(info)
	}
	fmt.Println(formatStatusLine(info))
	return nil
}

func runStatusAll(ctx context.Context, exec vmexec.Executor) error {
	names, err := listAgentContainerNames(ctx, exec)
	if err != nil {
		return err
	}
	infos := make([]statusInfo, 0, len(names))
	for _, name := range names {
		infos = append(infos, gatherStatus(ctx, exec, name))
	}
	if statusJSON {
		return encodeStatusJSON(infos)
	}
	if len(infos) == 0 {
		fmt.Println("No agent containers found.")
		return nil
	}
	for _, info := range infos {
		fmt.Println(formatStatusLine(info))
	}
	return nil
}

// gatherStatus collects a full status row for one container.
func gatherStatus(ctx context.Context, exec vmexec.Executor, name string) statusInfo {
	running, _ := docker.IsRunning(ctx, exec, name)
	statusStr, _ := inspectField(ctx, exec, name, "{{.State.Status}}")
	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	terminal, _ := docker.InspectLabel(ctx, exec, name, labels.Terminal)

	info := statusInfo{Name: name, AgentType: agentType, Running: running, Status: statusStr}
	res := resolveState(ctx, exec, name, agentType, terminal, running)
	info.State = res.State.String()
	info.Reason = res.Reason
	info.Matched = res.Matched
	if !running {
		info.ExitCode, _ = docker.ExitCode(ctx, exec, name)
	}
	return info
}

// resolveState determines the agent state: pane detection for a running tmux
// agent, or a terminal (done/exited) state for a stopped container.
func resolveState(ctx context.Context, exec vmexec.Executor, name, agentType, terminal string, running bool) agentstate.Result {
	if !running {
		code, _ := docker.ExitCode(ctx, exec, name)
		return terminalState(code)
	}
	if terminal != "" && terminal != "tmux" {
		return agentstate.Result{
			State:  agentstate.StateWorking,
			Reason: "running (" + terminal + " mode, no tmux pane)",
		}
	}
	out, err := exec.Run(ctx, tmux.BuildCapturePaneArgs(name, statePaneLines)...)
	if err != nil {
		// A running container with no readable tmux pane — headless/background
		// mode, a shell that runs bash directly (no tmux session), or a session
		// still starting up. It is up and running, so report working rather
		// than unknown. (The Terminal label is unreliable here: spawn currently
		// stamps it "tmux" for every container regardless of mode.)
		return agentstate.Result{State: agentstate.StateWorking, Reason: "running (no tmux pane)"}
	}
	return agentstate.Detect(agentType, strings.Split(string(out), "\n"))
}

// terminalState maps a stopped container's exit code to a state.
func terminalState(code int) agentstate.Result {
	if code == 0 {
		return agentstate.Result{State: agentstate.StateDone, Reason: "exited cleanly (code 0)"}
	}
	return agentstate.Result{State: agentstate.StateExited, Reason: fmt.Sprintf("exited with code %d", code)}
}

// formatStateReason renders "state — reason" for compact display.
func formatStateReason(res agentstate.Result) string {
	if res.Reason == "" {
		return res.State.String()
	}
	return fmt.Sprintf("%s — %s", res.State, res.Reason)
}

func formatStatusLine(info statusInfo) string {
	icon := stateIcon(info.State)
	status := info.Status
	if status == "" {
		status = "-"
	}
	return fmt.Sprintf("%s %-8s %-30s %-26s %s", icon, info.State, info.Name, info.Reason, status)
}

func stateIcon(state string) string {
	switch state {
	case agentstate.StateBlocked.String():
		return "🔴"
	case agentstate.StateWorking.String():
		return "🟢"
	case agentstate.StateIdle.String():
		return "🟡"
	case agentstate.StateDone.String():
		return "✅"
	case agentstate.StateExited.String():
		return "⏹"
	default:
		return "❔"
	}
}

func encodeStatusJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// listAgentContainerNames returns all safe-agentic container names.
func listAgentContainerNames(ctx context.Context, exec vmexec.Executor) ([]string, error) {
	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^"+docker.ContainerPrefix+"-",
		"--format", "{{.Names}}")
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	return splitLines(string(out)), nil
}

// liveBlockedEntries sweeps running agents and returns a synthetic timeline
// entry for each one currently blocked, so `safe-ag inbox` surfaces agents that
// need human input. It is best-effort: any error yields no entries.
func liveBlockedEntries(ctx context.Context, exec vmexec.Executor) []timelineEntry {
	names, err := listAgentContainerNames(ctx, exec)
	if err != nil {
		return nil
	}
	var out []timelineEntry
	for _, name := range names {
		info := gatherStatus(ctx, exec, name)
		if info.State != agentstate.StateBlocked.String() {
			continue
		}
		summary := "state=blocked"
		if info.Reason != "" {
			summary += " reason=" + info.Reason
		}
		out = append(out, timelineEntry{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Type:      "agent.state",
			Status:    events.StatusBlocked,
			Container: name,
			Summary:   summary,
		})
	}
	return out
}
