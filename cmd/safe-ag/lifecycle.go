package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/audit"
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/inject"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/orb"
	"github.com/0x666c6f/safe-agentic/pkg/tmux"

	"github.com/spf13/cobra"
)

// ─── list ──────────────────────────────────────────────────────────────────

var listJSON bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agent containers",
	RunE:  runList,
}

func init() {
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
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
		fmt.Print(string(out))
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
		fmt.Printf("%s%s %s %s  %s  %s\n", indent, statusIcon, typeIcon, e.name, e.repo, e.status)
	}
	fmt.Println()
	return nil
}

// ─── attach ────────────────────────────────────────────────────────────────

var attachCmd = &cobra.Command{
	Use:   "attach <name|--latest>",
	Short: "Attach to an agent's tmux session",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runAttach,
}

func init() {
	addLatestFlag(attachCmd)
	rootCmd.AddCommand(attachCmd)
}

func runAttach(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := targetFromArgs(cmd, args)
	name, err := docker.ResolveTarget(ctx, exec, target)
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

	switch state {
	case "running":
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

// containerState returns the .State.Status of a container.
func containerState(ctx context.Context, exec orb.Executor, name string) (string, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", "{{.State.Status}}", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func containerExitCode(ctx context.Context, exec orb.Executor, name string) (int, error) {
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

var stopCmd = &cobra.Command{
	Use:   "stop <name|--latest|--all>",
	Short: "Stop agent containers",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStop,
}

func init() {
	stopCmd.Flags().BoolVar(&stopAll, "all", false, "Stop and remove all agent containers")
	addLatestFlag(stopCmd)
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	if stopAll {
		return stopAllContainers(ctx, exec)
	}

	target := targetFromArgs(cmd, args)
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}
	return stopOneContainer(ctx, exec, name)
}

func stopOneContainer(ctx context.Context, exec orb.Executor, name string) error {
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
	return nil
}

func stopAllContainers(ctx context.Context, exec orb.Executor) error {
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

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove all containers, networks, and optionally auth volumes",
	RunE:  runCleanup,
}

func init() {
	cleanupCmd.Flags().BoolVar(&cleanupAuth, "auth", false, "Also remove shared auth volumes")
	rootCmd.AddCommand(cleanupCmd)
}

func runCleanup(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

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
	}

	auditLogger := &audit.Logger{Path: audit.DefaultPath()}
	auditLogger.Log("cleanup", "", map[string]string{
		"auth": fmt.Sprintf("%v", cleanupAuth),
	})

	fmt.Println("Cleanup complete.")
	return nil
}

// ─── retry ─────────────────────────────────────────────────────────────────

var retryFeedback string

var retryCmd = &cobra.Command{
	Use:   "retry <name|--latest>",
	Short: "Retry a failed agent with optional feedback",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runRetry,
}

func init() {
	retryCmd.Flags().StringVar(&retryFeedback, "feedback", "", "Feedback for the next attempt")
	rootCmd.AddCommand(retryCmd)
}

func runRetry(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := ""
	if len(args) > 0 {
		target = args[0]
	}
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
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

// reconstructSpawnOpts reads labels and env vars from an existing container
// and builds a SpawnOpts that reproduces it.
func reconstructSpawnOpts(ctx context.Context, exec orb.Executor, name string) (SpawnOpts, error) {
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
}

func applyReconstructedDockerMode(opts *SpawnOpts, dockerMode string) {
	switch dockerMode {
	case "dind":
		opts.DockerAccess = true
	case "host-socket":
		opts.DockerSocket = true
	}
}

func applyReconstructedEnvs(opts *SpawnOpts, getEnv func(string) string) {
	if reposEnv := getEnv("REPOS"); reposEnv != "" {
		opts.Repos = strings.Fields(reposEnv)
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
func containerEnvVar(ctx context.Context, exec orb.Executor, name, envName string) (string, error) {
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
