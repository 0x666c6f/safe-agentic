package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/audit"
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/events"
	"github.com/0x666c6f/safe-agentic/pkg/inject"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/tmux"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// agentSupportsResume reports whether an agent type has a continue mode.
func agentSupportsResume(agentType string) bool {
	return agentType == "claude" || agentType == "codex"
}

// authVolumePersists reports whether the auth volume (which holds the agent's
// conversation transcript under ~/.claude / ~/.codex) survives a container
// stop. Ephemeral auth is tmpfs-backed and lost on stop; shared and
// fleet-isolated auth are named volumes that persist.
func authVolumePersists(authType string) bool {
	switch authType {
	case "shared", "fleet-isolated":
		return true
	default: // "ephemeral" or unknown
		return false
	}
}

// ─── list ──────────────────────────────────────────────────────────────────

var listJSON bool

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List agent containers (running and stopped)",
	GroupID: groupManage,
	RunE:    runList,
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Emit the container list as JSON for scripting")
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	if listJSON {
		out, err := exec.Run(ctx, "docker", "ps", "-a",
			"--filter", "name=^agent-",
			"--format", "{{json .}}")
		if err != nil {
			return fmt.Errorf("list containers: %w", err)
		}
		// Emit the raw docker ps JSON per line, augmented with a "state" field
		// (agentstate classification). Existing fields are preserved verbatim.
		for _, line := range splitLines(string(out)) {
			fmt.Println(augmentListJSON(ctx, exec, line))
		}
		return nil
	}

	// Fetch containers with fleet label for grouping
	format := "{{.Names}}\t{{.Label \"" + labels.AgentType + "\"}}\t" +
		"{{.Label \"" + labels.RepoDisplay + "\"}}\t" +
		"{{.Label \"" + labels.Fleet + "\"}}\t" +
		"{{.Status}}"
	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^agent-",
		"--format", format)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	lines := splitLines(string(out))
	if len(lines) == 0 {
		fmt.Println("No agent containers found.")
		return nil
	}

	type entry struct {
		name, agentType, repo, fleet, status string
	}
	var entries []entry
	for _, line := range lines {
		parts := strings.Split(string(line), "\t")
		if len(parts) < 5 {
			continue
		}
		entries = append(entries, entry{parts[0], parts[1], parts[2], parts[3], parts[4]})
	}

	// Group by fleet
	lastFleet := ""
	for _, e := range entries {
		if e.fleet != "" && e.fleet != lastFleet {
			icon := "🔄"
			fmt.Printf("\n%s %s\n", icon, e.fleet)
			lastFleet = e.fleet
		}

		statusIcon := "⏹"
		if strings.HasPrefix(e.status, "Up") {
			statusIcon = "▶"
		}

		typeIcon := "🤖"
		switch e.agentType {
		case "claude":
			typeIcon = "🟠"
		case "codex":
			typeIcon = "🔵"
		}

		indent := ""
		if e.fleet != "" {
			indent = "  "
		}
		state := gatherStatus(ctx, exec, e.name).State
		fmt.Printf("%s%s %s %s  %s  %s %-8s  %s\n",
			indent, statusIcon, typeIcon, e.name, e.repo, stateIcon(state), state, e.status)
	}
	fmt.Println()
	return nil
}

// augmentListJSON adds a "state" field to a single docker ps JSON line without
// touching the existing fields (backward compatible). Lines that are not JSON
// objects are returned unchanged.
func augmentListJSON(ctx context.Context, exec vmexec.Executor, line string) string {
	var meta struct {
		Names string `json:"Names"`
	}
	if err := json.Unmarshal([]byte(line), &meta); err != nil || meta.Names == "" {
		return line
	}
	return injectStateField(line, gatherStatus(ctx, exec, meta.Names).State)
}

// injectStateField appends "state":<state> to a JSON object line, preserving
// every existing field. Non-object lines are returned unchanged.
func injectStateField(line, state string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasSuffix(trimmed, "}") {
		return trimmed
	}
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return trimmed
	}
	inner := trimmed[:len(trimmed)-1]
	sep := ","
	if strings.HasSuffix(strings.TrimSpace(inner), "{") { // empty object
		sep = ""
	}
	return inner + sep + `"state":` + string(stateJSON) + "}"
}

// ─── attach ────────────────────────────────────────────────────────────────

var attachResume bool

var attachCmd = &cobra.Command{
	Use:     "attach <name|--latest>",
	Short:   "Attach to an agent's tmux session (restarts it if stopped)",
	Args:    cobra.MaximumNArgs(1),
	GroupID: groupManage,
	RunE:    runAttach,
}

func init() {
	addLatestFlag(attachCmd)
	attachCmd.Flags().BoolVar(&attachResume, "resume", false, "On a stopped agent, continue its previous conversation instead of a fresh session")
	rootCmd.AddCommand(attachCmd)
}

// hostClaudeSettings reads the host's CURRENT settings.json.
func hostClaudeSettings() ([]byte, bool) {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, false
		}
		dir = filepath.Join(home, ".claude")
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		return nil, false
	}
	return data, true
}

// pushClaudeConfig copies the host's CURRENT settings.json into a container,
// best-effort. Spawn-time env injection freezes config at creation; this
// keeps preferences dynamic for containers attached/resumed later. Applies
// on the agent's next process start inside the container. When staged is
// true it also writes settings.host.json — a one-shot file the entrypoint
// consumes in preference to the spawn-time env on the next restart.
func pushClaudeConfig(ctx context.Context, exec vmexec.Executor, name string, staged bool) {
	data, ok := hostClaudeSettings()
	if !ok {
		return
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	script := "mkdir -p ~/.claude && echo '" + b64 + "' | base64 -d > ~/.claude/settings.json"
	if staged {
		script += " && cp ~/.claude/settings.json ~/.claude/settings.host.json"
	}
	if _, err := exec.Run(ctx, "docker", "exec", name, "bash", "-c", script); err != nil {
		fmt.Fprintf(os.Stderr, "note: could not sync Claude settings into %s: %v\n", name, err)
	}
}

// ─── config-sync ───────────────────────────────────────────────────────────

var configSyncRestart bool

var configSyncCmd = &cobra.Command{
	Use:   "config-sync <name|--latest>",
	Short: "Push current host Claude settings into an agent (--restart to apply now)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigSync,
}

func init() {
	addLatestFlag(configSyncCmd)
	configSyncCmd.Flags().BoolVar(&configSyncRestart, "restart", false,
		"Restart the container so the agent relaunches with the synced settings (resumes its session)")
	rootCmd.AddCommand(configSyncCmd)
}

func runConfigSync(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()
	name, err := docker.ResolveTarget(ctx, exec, targetFromArgs(cmd, args))
	if err != nil {
		return err
	}
	state, err := containerState(ctx, exec, name)
	if err != nil {
		return fmt.Errorf("inspect container %s: %w", name, err)
	}
	if state != "running" {
		return fmt.Errorf("container %s is %s; settings sync automatically when it next starts via attach/resume", name, state)
	}
	if _, ok := hostClaudeSettings(); !ok {
		return fmt.Errorf("no host settings.json found to sync")
	}
	pushClaudeConfig(ctx, exec, name, configSyncRestart)
	if !configSyncRestart {
		fmt.Printf("Synced current Claude settings into %s (applies when the agent next restarts; use --restart to apply now)\n", name)
		return nil
	}
	fmt.Printf("Synced settings; restarting %s to apply (session resumes from its state file)…\n", name)
	if _, err := exec.Run(ctx, "docker", "restart", name); err != nil {
		return fmt.Errorf("restart container %s: %w", name, err)
	}
	if err := tmux.WaitForSession(ctx, exec, name); err != nil {
		return err
	}
	fmt.Printf("%s restarted with current settings\n", name)
	return nil
}

func runAttach(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := targetFromArgs(cmd, args)
	name, err := resolveTargetCoded(ctx, exec, target)
	if err != nil {
		return err
	}

	// Get current container state
	state, err := containerState(ctx, exec, name)
	if err != nil {
		return fmt.Errorf("inspect container %s: %w", name, err)
	}

	// Check terminal mode
	termMode, _ := docker.InspectLabel(ctx, exec, name, labels.Terminal)
	usesTmux := termMode == "tmux" || termMode == ""

	if attachResume {
		return resumeAttach(ctx, exec, name, state, usesTmux)
	}

	switch state {
	case "running":
		pushClaudeConfig(ctx, exec, name, false)
		if usesTmux {
			if has, _ := tmux.HasSession(ctx, exec, name); has {
				return tmux.Attach(exec, name)
			}
			// tmux label but no session yet — fall through to docker attach
		}
		return exec.RunInteractive("docker", "attach", "--sig-proxy=false", name)

	case "exited", "created":
		// Start the stopped container
		if _, err := exec.Run(ctx, "docker", "start", name); err != nil {
			return fmt.Errorf("start container %s: %w", name, err)
		}
		pushClaudeConfig(ctx, exec, name, false)
		if usesTmux {
			if err := tmux.WaitForSession(ctx, exec, name); err != nil {
				return err
			}
			return tmux.Attach(exec, name)
		}
		return exec.RunInteractive("docker", "attach", "--sig-proxy=false", name)

	default:
		return fmt.Errorf("container %s is in state %q, cannot attach", name, state)
	}
}

// resumeAttach reattaches to an agent while continuing its previous
// conversation instead of starting fresh.
func resumeAttach(ctx context.Context, exec vmexec.Executor, name, state string, usesTmux bool) error {
	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	if !agentSupportsResume(agentType) {
		return fmt.Errorf("--resume only supports claude and codex agents (%s is %q)", name, agentType)
	}
	if !usesTmux {
		return fmt.Errorf("--resume requires a tmux session; container %s does not use tmux", name)
	}

	switch state {
	case "running":
		// A live session means the agent is still active — attaching already
		// drops the user back into the ongoing conversation.
		if has, _ := tmux.HasSession(ctx, exec, name); has {
			return tmux.Attach(exec, name)
		}
		// Running but no tmux session: we cannot tell a session that just
		// exited from a headless agent (e.g. --background) that is still alive.
		// Relaunching here could start a SECOND agent against the same workspace
		// and auth volume, so refuse and point at the safe options.
		return fmt.Errorf(
			"container %s is running but has no attachable tmux session; the agent may still be running headless.\n"+
				"Use `safe-ag steer %s \"...\"` to send it input, or `safe-ag stop %s` then `safe-ag attach %s --resume` to restart and continue.",
			name, name, name, name)

	case "exited", "created":
		authType, _ := docker.InspectLabel(ctx, exec, name, labels.AuthType)
		if !authVolumePersists(authType) {
			// Ephemeral auth was tmpfs-backed and is gone now the container is
			// stopped. Restarting cannot start fresh either: the entrypoint's
			// session-state marker (persisted in the workspace volume) makes the
			// agent auto-continue against an empty auth dir and error out. There
			// is no clean host-side way to clear that marker on a stopped
			// container, so refuse and point at a fresh run.
			return fmt.Errorf(
				"container %s used ephemeral auth; its conversation transcript was tmpfs-backed and did not survive the stop, so --resume cannot recover it.\n"+
					"Use `safe-ag attach %s` (without --resume) or `safe-ag retry %s` for a fresh run, or re-run the task with --reuse-auth so future sessions persist.",
				name, name, name)
		}
		// Restarting re-runs the entrypoint, which auto-resumes from its
		// session-state file (the auth volume persisted).
		if _, err := exec.Run(ctx, "docker", "start", name); err != nil {
			return fmt.Errorf("start container %s: %w", name, err)
		}
		pushClaudeConfig(ctx, exec, name, false)
		if err := tmux.WaitForSession(ctx, exec, name); err != nil {
			return err
		}
		return tmux.Attach(exec, name)

	default:
		return fmt.Errorf("container %s is in state %q, cannot resume", name, state)
	}
}

// containerState returns the .State.Status of a container.
func containerState(ctx context.Context, exec vmexec.Executor, name string) (string, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", "{{.State.Status}}", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func containerExitCode(ctx context.Context, exec vmexec.Executor, name string) (int, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", "{{.State.ExitCode}}", name)
	if err != nil {
		return 0, err
	}
	code, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
	if convErr != nil {
		return 0, convErr
	}
	return code, nil
}

// ─── stop ──────────────────────────────────────────────────────────────────

var stopAll bool
var stopYes bool

var stopCmd = &cobra.Command{
	Use:     "stop <name|--latest|--all>",
	Short:   "Stop and remove agent containers (auth volumes are kept)",
	Args:    cobra.MaximumNArgs(1),
	GroupID: groupManage,
	RunE:    runStop,
}

func init() {
	stopCmd.Flags().BoolVar(&stopAll, "all", false, "Stop and remove every agent container instead of a named one")
	stopCmd.Flags().BoolVar(&stopYes, "yes", false, "Skip the confirmation prompt (for scripts/automation)")
	addLatestFlag(stopCmd)
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	if stopAll {
		if ok, err := confirmDestructive("Stop and remove ALL agent containers", stopYes); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("aborted")
		}
		return stopAllContainers(ctx, exec)
	}

	target := targetFromArgs(cmd, args)
	name, err := resolveTargetCoded(ctx, exec, target)
	if err != nil {
		return err
	}
	return stopOneContainer(ctx, exec, name)
}

func stopOneContainer(ctx context.Context, exec vmexec.Executor, name string) error {
	fmt.Printf("Stopping %s...\n", name)
	if _, err := exec.Run(ctx, "docker", "stop", "-t", "30", name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: stop %s: %v\n", name, err)
	}
	if _, err := exec.Run(ctx, "docker", "rm", name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: rm %s: %v\n", name, err)
	}

	// Clean up DinD sidecar and managed network (best-effort)
	docker.RemoveDinDRuntime(ctx, exec, name)
	netName := docker.ManagedNetworkName(name)
	docker.RemoveManagedNetwork(ctx, exec, netName)

	auditLogger := &audit.Logger{Path: audit.DefaultPath()}
	auditLogger.Log("stop", name, nil)
	events.Emit(events.DefaultEventsPath(), "agent.stopped", map[string]string{"container": name})
	return nil
}

func stopAllContainers(ctx context.Context, exec vmexec.Executor) error {
	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^agent-",
		"--format", "{{.Names}}")
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	names := splitLines(string(out))
	if len(names) == 0 {
		fmt.Println("No agent containers found.")
		return nil
	}

	total := len(names)
	for i, name := range names {
		fmt.Printf("  [%d/%d] Stopping %s...\n", i+1, total, name)
		exec.Run(ctx, "docker", "stop", "-t", "10", name)
		exec.Run(ctx, "docker", "rm", name)
		docker.RemoveDinDRuntime(ctx, exec, name)
		docker.RemoveManagedNetwork(ctx, exec, docker.ManagedNetworkName(name))
	}

	fmt.Printf("Done. Stopped %d container(s).\n", total)
	return nil
}

// ─── cleanup ───────────────────────────────────────────────────────────────

var cleanupAuth bool
var cleanupYes bool

var cleanupCmd = &cobra.Command{
	Use:     "cleanup",
	Short:   "Remove all agent containers and managed networks (keeps auth by default)",
	GroupID: groupManage,
	RunE:    runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVar(&cleanupAuth, "auth", false, "Also delete shared auth volumes (you'll have to sign agents in again)")
	cleanupCmd.Flags().BoolVar(&cleanupYes, "yes", false, "Skip the confirmation prompt (for scripts/automation)")
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	action := "Remove all agent containers and managed networks"
	if cleanupAuth {
		action = "Remove all agent containers, managed networks, AND shared auth volumes (deletes stored agent credentials)"
	}
	if ok, err := confirmDestructive(action, cleanupYes); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("aborted")
	}

	// 1. Stop all running agent containers
	runningOut, _ := exec.Run(ctx, "docker", "ps",
		"--filter", "name=^agent-",
		"--format", "{{.Names}}")
	running := splitLines(string(runningOut))
	if len(running) > 0 {
		stopArgs := append([]string{"docker", "stop", "-t", "30"}, running...)
		exec.Run(ctx, stopArgs...)
	}

	// 2. Remove all stopped agent containers
	allOut, _ := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^agent-",
		"--format", "{{.Names}}")
	all := splitLines(string(allOut))
	if len(all) > 0 {
		rmArgs := append([]string{"docker", "rm"}, all...)
		exec.Run(ctx, rmArgs...)
	}

	// 3. Cleanup DinD sidecars & volumes
	docker.CleanupAllDinD(ctx, exec)

	// 4. Prune managed networks
	exec.Run(ctx, "docker", "network", "prune", "-f",
		"--filter", "label=app=safe-agentic")

	// 5. Prune dangling images
	exec.Run(ctx, "docker", "image", "prune", "-f")

	// 6. Auth volumes (optional)
	if cleanupAuth {
		volOut, _ := exec.Run(ctx, "docker", "volume", "ls",
			"--filter", "name=safe-agentic-",
			"--format", "{{.Name}}")
		for _, vol := range splitLines(string(volOut)) {
			if strings.HasSuffix(vol, "-auth") || strings.HasSuffix(vol, "-gh-auth") {
				exec.Run(ctx, "docker", "volume", "rm", vol)
			}
		}
		isolatedOut, _ := exec.Run(ctx, "docker", "volume", "ls",
			"--filter", "label=safe-agentic.type=auth",
			"--format", "{{.Name}}")
		for _, vol := range splitLines(string(isolatedOut)) {
			exec.Run(ctx, "docker", "volume", "rm", vol)
		}
	}

	auditLogger := &audit.Logger{Path: audit.DefaultPath()}
	auditLogger.Log("cleanup", "", map[string]string{
		"auth": fmt.Sprintf("%v", cleanupAuth),
	})
	events.Emit(events.DefaultEventsPath(), "cleanup.completed", map[string]string{"auth": fmt.Sprintf("%v", cleanupAuth)})

	fmt.Println("Cleanup complete.")
	return nil
}

// ─── retry ─────────────────────────────────────────────────────────────────

var retryFeedback string
var retryResume bool

var retryCmd = &cobra.Command{
	Use:   "retry <name|--latest>",
	Short: "Re-run a finished agent with the same config",
	Long: `Spawn a fresh agent reusing a finished one's repo, flags, and prompt — handy
after a failure. Add --feedback to steer the new attempt.`,
	Args:    cobra.MaximumNArgs(1),
	GroupID: groupManage,
	RunE:    runRetry,
}

func init() {
	retryCmd.Flags().StringVar(&retryFeedback, "feedback", "", "Extra guidance for the new attempt, appended to the original prompt")
	retryCmd.Flags().BoolVar(&retryResume, "resume", false, "Reuse the source auth volume and continue its conversation instead of re-running the prompt")
	addLatestFlag(retryCmd)
	rootCmd.AddCommand(retryCmd)
}

func runRetry(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	name, err := resolveTargetCoded(ctx, exec, targetFromArgs(cmd, args))
	if err != nil {
		return err
	}

	if retryResume {
		return resumeRetry(ctx, exec, name, retryFeedback)
	}

	opts, err := reconstructSpawnOpts(ctx, exec, name)
	if err != nil {
		return fmt.Errorf("reconstruct opts from %s: %w", name, err)
	}

	if retryFeedback != "" {
		if opts.Prompt != "" {
			opts.Prompt = opts.Prompt + "\n\nPrevious attempt failed. Feedback: " + retryFeedback + ". Try a different approach."
		} else {
			opts.Prompt = "Previous attempt failed. Feedback: " + retryFeedback + ". Try a different approach."
		}
	}

	// Clear the name so a new container is spawned
	opts.Name = ""
	return executeSpawn(opts)
}

// resumeRetry continues the source container's conversation instead of spawning
// a fresh attempt. Reusing the container itself is the surest way to reuse the
// exact same session/auth volume (shared or fleet-isolated). A stopped container
// is restarted so the entrypoint auto-resumes from its session-state file; a
// running container must already hold a live session (otherwise we refuse rather
// than risk a second headless agent). Optional feedback is delivered as a
// follow-up message through the tmux input path, exactly like `steer`.
func resumeRetry(ctx context.Context, exec vmexec.Executor, name, feedback string) error {
	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	if !agentSupportsResume(agentType) {
		return fmt.Errorf("--resume only supports claude and codex agents (%s is %q)", name, agentType)
	}
	termMode, _ := docker.InspectLabel(ctx, exec, name, labels.Terminal)
	if termMode != "" && termMode != "tmux" {
		return fmt.Errorf("--resume requires a tmux session; container %s does not use tmux", name)
	}

	running, _ := docker.IsRunning(ctx, exec, name)
	authType, _ := docker.InspectLabel(ctx, exec, name, labels.AuthType)

	// Ephemeral auth transcripts live on tmpfs; once the container stops they
	// are gone, so --resume cannot recover them. Fail with an actionable hint
	// instead of silently starting a fresh conversation.
	if !running && !authVolumePersists(authType) {
		return fmt.Errorf(
			"container %s used ephemeral auth; its conversation transcript did not survive the stop, so --resume cannot recover it.\n"+
				"Retry without --resume to start a fresh attempt (add --feedback to steer it), or re-run the task with --reuse-auth so future sessions persist.",
			name)
	}

	if running {
		// A running container with no tmux session may be a headless agent
		// (e.g. --background) that is still alive. Relaunching would risk a
		// second agent against the same workspace/auth volume, so refuse rather
		// than guess. A live session is reused (feedback is steered in below).
		if has, _ := tmux.HasSession(ctx, exec, name); !has {
			return fmt.Errorf(
				"container %s is still running but has no attachable tmux session; the agent may be running headless.\n"+
					"Use `safe-ag steer %s \"...\"` to send input, or `safe-ag stop %s` first, then `safe-ag retry %s --resume`.",
				name, name, name, name)
		}
	} else if _, err := exec.Run(ctx, "docker", "start", name); err != nil {
		// Restarting re-runs the entrypoint, which auto-resumes from its
		// session-state file.
		return fmt.Errorf("start container %s: %w", name, err)
	} else {
		pushClaudeConfig(ctx, exec, name, false)
	}

	if err := tmux.WaitForSession(ctx, exec, name); err != nil {
		return err
	}

	if feedback != "" {
		if _, err := exec.Run(ctx, "docker", "exec", name,
			"tmux", "send-keys", "-t", tmux.SessionName(), "--", feedback, "Enter"); err != nil {
			return fmt.Errorf("send feedback to %s: %w", name, err)
		}
	}

	auditLogger := &audit.Logger{Path: audit.DefaultPath()}
	auditLogger.Log("retry-resume", name, map[string]string{"feedback": fmt.Sprintf("%v", feedback != "")})
	events.Emit(events.DefaultEventsPath(), "agent.resumed", map[string]string{"container": name})
	fmt.Printf("Resumed %s\n", name)

	if term.IsTerminal(int(os.Stdin.Fd())) {
		return tmux.Attach(exec, name)
	}
	return nil
}

// reconstructSpawnOpts reads labels and env vars from an existing container
// and builds a SpawnOpts that reproduces it.
func reconstructSpawnOpts(ctx context.Context, exec vmexec.Executor, name string) (SpawnOpts, error) {
	getLabel := func(l string) string {
		v, _ := docker.InspectLabel(ctx, exec, name, l)
		return v
	}

	getEnv := func(envName string) string {
		v, _ := containerEnvVar(ctx, exec, name, envName)
		return v
	}

	opts := SpawnOpts{}

	opts.AgentType = getLabel(labels.AgentType)
	if opts.AgentType == "" {
		return opts, fmt.Errorf("missing label %s", labels.AgentType)
	}
	applyReconstructedLabels(&opts, getLabel)
	applyReconstructedEnvs(&opts, getEnv)
	opts.Identity = reconstructedIdentity(getEnv)

	return opts, nil
}

func applyReconstructedLabels(opts *SpawnOpts, getLabel func(string) string) {
	opts.SSH = getLabel(labels.SSH) == "true"
	opts.NoSSH = !opts.SSH
	opts.SeedAuth = getLabel(labels.SeedAuth) == "true"
	opts.NoSeedAuth = !opts.SeedAuth
	applyReconstructedAuth(opts, getLabel(labels.AuthType), getLabel(labels.GHAuth))
	applyReconstructedDockerMode(opts, getLabel(labels.DockerMode))
	opts.MaxCost = getLabel(labels.MaxCost)
	opts.AWSProfile = getLabel(labels.AWS)
	opts.Notify = decodeB64Value(getLabel(labels.NotifyB64))
	opts.OnComplete = decodeB64Value(getLabel(labels.OnCompleteB64))
	opts.OnFail = decodeB64Value(getLabel(labels.OnFailB64))
}

func applyReconstructedAuth(opts *SpawnOpts, authType, ghAuth string) {
	opts.EphemeralAuth = authType == "ephemeral"
	opts.ReuseAuth = authType == "shared"
	opts.ReuseGHAuth = ghAuth == "shared"
	opts.NoReuseAuth = authType == "ephemeral"
	opts.NoReuseGHAuth = !opts.ReuseGHAuth
}

func applyReconstructedDockerMode(opts *SpawnOpts, dockerMode string) {
	switch dockerMode {
	case "dind":
		opts.DockerAccess = true
		opts.NoDockerSocket = true
	case "host-socket":
		opts.DockerSocket = true
		opts.NoDocker = true
	case "off":
		opts.NoDocker = true
		opts.NoDockerSocket = true
	}
}

func applyReconstructedEnvs(opts *SpawnOpts, getEnv func(string) string) {
	if reposEnv := getEnv("REPOS"); reposEnv != "" {
		parts := strings.Split(reposEnv, ",")
		opts.Repos = opts.Repos[:0]
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				opts.Repos = append(opts.Repos, part)
			}
		}
	}
	opts.Prompt = decodeB64Value(getEnv("SAFE_AGENTIC_PROMPT_B64"))
	opts.Instructions = decodeB64Value(getEnv("SAFE_AGENTIC_INSTRUCTIONS_B64"))
	opts.Template = decodeB64Value(getEnv("SAFE_AGENTIC_TEMPLATE_B64"))
	opts.OnExit = decodeB64Value(getEnv("SAFE_AGENTIC_ON_EXIT_B64"))
}

func decodeB64Value(raw string) string {
	if raw == "" {
		return ""
	}
	decoded, err := inject.DecodeB64(raw)
	if err != nil {
		return ""
	}
	return decoded
}

func reconstructedIdentity(getEnv func(string) string) string {
	gitName := getEnv("GIT_AUTHOR_NAME")
	gitEmail := getEnv("GIT_AUTHOR_EMAIL")
	if gitName == "" || gitEmail == "" {
		return ""
	}
	return gitName + " <" + gitEmail + ">"
}

// containerEnvVar reads a specific env var from a running or stopped container
// by inspecting the container config.
func containerEnvVar(ctx context.Context, exec vmexec.Executor, name, envName string) (string, error) {
	// Use docker inspect to list all env vars then search for the one we want
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", "{{range .Config.Env}}{{println .}}{{end}}", name)
	if err != nil {
		return "", err
	}
	prefix := envName + "="
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix), nil
		}
	}
	return "", nil
}

// confirmDestructive gates a destructive action behind an explicit typed
// confirmation, mirroring the workspace-revert pattern. With yes=true it
// approves silently. When stdin is not a terminal it refuses with a clear
// message so non-interactive callers must pass --yes rather than hang on a
// prompt that can never be answered.
func confirmDestructive(action string, yes bool) (bool, error) {
	if yes {
		return true, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, fmt.Errorf("%s requires --yes when stdin is not a terminal", action)
	}
	fmt.Printf("%s. Type yes to continue: ", action)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(answer) == "yes", nil
}

// confirmProceed gates a risky (but not destructive) action behind a short
// [y/N] prompt. Same non-interactive contract as confirmDestructive.
func confirmProceed(action string, yes bool) (bool, error) {
	if yes {
		return true, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, fmt.Errorf("%s requires --yes when stdin is not a terminal", action)
	}
	fmt.Printf("%s. Continue? [y/N]: ", action)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// splitLines splits a newline-delimited string, filtering empty lines.
func splitLines(s string) []string {
	var result []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
