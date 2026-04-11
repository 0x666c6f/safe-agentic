package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"safe-agentic/pkg/audit"
	"safe-agentic/pkg/docker"
	"safe-agentic/pkg/inject"
	"safe-agentic/pkg/labels"
	"safe-agentic/pkg/orb"
	"safe-agentic/pkg/tmux"

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

	var format string
	if listJSON {
		format = "{{json .}}"
	} else {
		format = "table {{.Names}}\t" +
			"{{.Label \"" + labels.AgentType + "\"}}\t" +
			"{{.Label \"" + labels.RepoDisplay + "\"}}\t" +
			"{{.Label \"" + labels.SSH + "\"}}\t" +
			"{{.Label \"" + labels.AuthType + "\"}}\t" +
			"{{.Label \"" + labels.GHAuth + "\"}}\t" +
			"{{.Label \"" + labels.DockerMode + "\"}}\t" +
			"{{.Label \"" + labels.NetworkMode + "\"}}\t" +
			"{{.Status}}"
	}

	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^agent-",
		"--format", format)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	fmt.Print(string(out))
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
	// Bulk stop + rm
	stopArgs := append([]string{"docker", "stop", "-t", "30"}, names...)
	exec.Run(ctx, stopArgs...)
	rmArgs := append([]string{"docker", "rm"}, names...)
	exec.Run(ctx, rmArgs...)

	// Per-container cleanup
	for _, name := range names {
		docker.RemoveDinDRuntime(ctx, exec, name)
		netName := docker.ManagedNetworkName(name)
		docker.RemoveManagedNetwork(ctx, exec, netName)
	}
	fmt.Printf("Stopped %d container(s).\n", len(names))
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

	// Agent type
	opts.AgentType = getLabel(labels.AgentType)
	if opts.AgentType == "" {
		return opts, fmt.Errorf("missing label %s", labels.AgentType)
	}

	// SSH
	opts.SSH = getLabel(labels.SSH) == "true"

	// Auth
	authType := getLabel(labels.AuthType)
	opts.EphemeralAuth = authType == "ephemeral"
	opts.ReuseAuth = authType == "shared"

	// GH auth
	opts.ReuseGHAuth = getLabel(labels.GHAuth) == "shared"

	// Docker mode
	switch getLabel(labels.DockerMode) {
	case "dind":
		opts.DockerAccess = true
	case "host-socket":
		opts.DockerSocket = true
	}

	// Max cost
	opts.MaxCost = getLabel(labels.MaxCost)

	// AWS profile
	opts.AWSProfile = getLabel(labels.AWS)

	// Repos
	if reposEnv := getEnv("REPOS"); reposEnv != "" {
		opts.Repos = strings.Fields(reposEnv)
	}

	// Prompt
	if promptB64 := getEnv("SAFE_AGENTIC_PROMPT_B64"); promptB64 != "" {
		if decoded, err := inject.DecodeB64(promptB64); err == nil {
			opts.Prompt = decoded
		}
	}

	// Instructions
	if instrB64 := getEnv("SAFE_AGENTIC_INSTRUCTIONS_B64"); instrB64 != "" {
		if decoded, err := inject.DecodeB64(instrB64); err == nil {
			opts.Instructions = decoded
		}
	}

	// Template
	if tplB64 := getEnv("SAFE_AGENTIC_TEMPLATE_B64"); tplB64 != "" {
		if decoded, err := inject.DecodeB64(tplB64); err == nil {
			opts.Template = decoded
		}
	}

	// OnExit
	if onExitB64 := getEnv("SAFE_AGENTIC_ON_EXIT_B64"); onExitB64 != "" {
		if decoded, err := inject.DecodeB64(onExitB64); err == nil {
			opts.OnExit = decoded
		}
	}

	// Notify
	if notifyB64 := getLabel(labels.NotifyB64); notifyB64 != "" {
		if decoded, err := inject.DecodeB64(notifyB64); err == nil {
			opts.Notify = decoded
		}
	}

	// OnComplete
	if onCompleteB64 := getLabel(labels.OnCompleteB64); onCompleteB64 != "" {
		if decoded, err := inject.DecodeB64(onCompleteB64); err == nil {
			opts.OnComplete = decoded
		}
	}

	// OnFail
	if onFailB64 := getLabel(labels.OnFailB64); onFailB64 != "" {
		if decoded, err := inject.DecodeB64(onFailB64); err == nil {
			opts.OnFail = decoded
		}
	}

	// Git identity from env
	gitName := getEnv("GIT_AUTHOR_NAME")
	gitEmail := getEnv("GIT_AUTHOR_EMAIL")
	if gitName != "" && gitEmail != "" {
		opts.Identity = gitName + " <" + gitEmail + ">"
	}

	return opts, nil
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

