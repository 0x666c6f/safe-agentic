package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"safe-agentic/pkg/audit"
	"safe-agentic/pkg/config"
	"safe-agentic/pkg/docker"
	"safe-agentic/pkg/events"
	"safe-agentic/pkg/inject"
	"safe-agentic/pkg/labels"
	"safe-agentic/pkg/repourl"
	"safe-agentic/pkg/tmux"
	"safe-agentic/pkg/validate"

	"github.com/spf13/cobra"
)

type SpawnOpts struct {
	AgentType        string
	Repos            []string
	Name             string
	Prompt           string
	Template         string
	Instructions     string
	InstructionsFile string
	SSH              bool
	ReuseAuth        bool
	EphemeralAuth    bool
	ReuseGHAuth      bool
	DockerAccess     bool
	DockerSocket     bool
	Network          string
	Memory           string
	CPUs             string
	PIDsLimit        int
	Identity         string
	AWSProfile       string
	AutoTrust        bool
	Background       bool
	OnExit           string
	OnComplete       string
	OnFail           string
	MaxCost          string
	Notify           string
	FleetVolume      string
	DryRun           bool
}

var spawnOpts SpawnOpts

var spawnCmd = &cobra.Command{
	Use:   "spawn <claude|codex|shell>",
	Short: "Spawn a new agent container",
	Args:  cobra.ExactArgs(1),
	RunE:  runSpawn,
}

var runCmd = &cobra.Command{
	Use:   "run <repo-url> [repo-url...] [prompt]",
	Short: "Quick-start an agent with smart defaults",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runQuickStart,
}

func init() {
	f := spawnCmd.Flags()
	f.StringSliceVar(&spawnOpts.Repos, "repo", nil, "Repository URL to clone (repeatable)")
	f.StringVar(&spawnOpts.Name, "name", "", "Container name")
	f.StringVar(&spawnOpts.Prompt, "prompt", "", "Initial prompt")
	f.StringVar(&spawnOpts.Template, "template", "", "Prompt template name")
	f.StringVar(&spawnOpts.Instructions, "instructions", "", "Task instructions")
	f.StringVar(&spawnOpts.InstructionsFile, "instructions-file", "", "Instructions from file")
	f.BoolVar(&spawnOpts.SSH, "ssh", false, "Enable SSH agent forwarding")
	f.BoolVar(&spawnOpts.ReuseAuth, "reuse-auth", false, "Reuse shared auth volume")
	f.BoolVar(&spawnOpts.EphemeralAuth, "ephemeral-auth", false, "Use ephemeral auth volume")
	f.BoolVar(&spawnOpts.ReuseGHAuth, "reuse-gh-auth", false, "Reuse GitHub CLI auth")
	f.BoolVar(&spawnOpts.DockerAccess, "docker", false, "Enable Docker-in-Docker")
	f.BoolVar(&spawnOpts.DockerSocket, "docker-socket", false, "Mount host Docker socket")
	f.StringVar(&spawnOpts.Network, "network", "", "Custom Docker network")
	f.StringVar(&spawnOpts.Memory, "memory", "", "Memory limit (e.g., 8g)")
	f.StringVar(&spawnOpts.CPUs, "cpus", "", "CPU limit")
	f.IntVar(&spawnOpts.PIDsLimit, "pids-limit", 0, "PIDs limit (>= 64)")
	f.StringVar(&spawnOpts.Identity, "identity", "", "Git identity (Name <email>)")
	f.StringVar(&spawnOpts.AWSProfile, "aws", "", "AWS profile for credential injection")
	f.BoolVar(&spawnOpts.AutoTrust, "auto-trust", false, "Skip trust prompt")
	f.BoolVar(&spawnOpts.Background, "background", false, "Run in background (no tmux attach)")
	f.StringVar(&spawnOpts.OnExit, "on-exit", "", "Command to run on exit")
	f.StringVar(&spawnOpts.OnComplete, "on-complete", "", "Command to run on success")
	f.StringVar(&spawnOpts.OnFail, "on-fail", "", "Command to run on failure")
	f.StringVar(&spawnOpts.MaxCost, "max-cost", "", "Kill if estimated cost exceeds budget")
	f.StringVar(&spawnOpts.Notify, "notify", "", "Notification targets")
	f.StringVar(&spawnOpts.FleetVolume, "fleet-volume", "", "Shared fleet volume name")
	f.BoolVar(&spawnOpts.DryRun, "dry-run", false, "Show what would run without executing")

	rf := runCmd.Flags()
	rf.StringVar(&spawnOpts.Name, "name", "", "Container name")
	rf.StringVar(&spawnOpts.Network, "network", "", "Custom Docker network")
	rf.StringVar(&spawnOpts.Memory, "memory", "", "Memory limit")
	rf.StringVar(&spawnOpts.CPUs, "cpus", "", "CPU limit")
	rf.StringVar(&spawnOpts.MaxCost, "max-cost", "", "Cost budget")
	rf.StringVar(&spawnOpts.Template, "template", "", "Prompt template")
	rf.StringVar(&spawnOpts.Instructions, "instructions", "", "Task instructions")
	rf.BoolVar(&spawnOpts.Background, "background", false, "Background mode")
	rf.BoolVar(&spawnOpts.DryRun, "dry-run", false, "Dry run")

	rootCmd.AddCommand(spawnCmd, runCmd)
}

func runSpawn(cmd *cobra.Command, args []string) error {
	spawnOpts.AgentType = args[0]
	return executeSpawn(spawnOpts)
}

func runQuickStart(cmd *cobra.Command, args []string) error {
	var repos []string
	var prompt string
	for _, arg := range args {
		if strings.HasPrefix(arg, "http") || strings.HasPrefix(arg, "git@") || strings.HasPrefix(arg, "ssh://") {
			repos = append(repos, arg)
		} else {
			prompt = arg
		}
	}
	if len(repos) == 0 {
		return fmt.Errorf("at least one repo URL is required")
	}
	ssh := false
	for _, r := range repos {
		if repourl.UsesSSH(r) {
			ssh = true
			break
		}
	}
	identity := config.DetectGitIdentity()
	opts := spawnOpts
	opts.AgentType = "claude"
	opts.Repos = repos
	opts.Prompt = prompt
	opts.SSH = ssh
	opts.ReuseAuth = true
	opts.Identity = identity
	return executeSpawn(opts)
}

func executeSpawn(opts SpawnOpts) error {
	ctx := context.Background()
	exec := newExecutor()

	// Validation
	switch opts.AgentType {
	case "claude", "codex", "shell":
	default:
		return fmt.Errorf("agent type must be claude, codex, or shell (got %q)", opts.AgentType)
	}
	if opts.Name != "" {
		if err := validate.NameComponent(opts.Name, "container name"); err != nil {
			return err
		}
	}
	if opts.PIDsLimit > 0 {
		if err := validate.PIDsLimit(opts.PIDsLimit); err != nil {
			return err
		}
	}
	if err := docker.EnsureSSHForRepos(opts.SSH, opts.Repos); err != nil {
		return err
	}

	// Load defaults
	cfg, err := config.LoadDefaults(config.DefaultsPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	memory := coalesce(opts.Memory, cfg.Memory)
	cpus := coalesce(opts.CPUs, cfg.CPUs)
	pidsLimit := opts.PIDsLimit
	if pidsLimit == 0 && cfg.PIDsLimit != "" {
		if v, err := strconv.Atoi(cfg.PIDsLimit); err == nil {
			pidsLimit = v
		}
	}

	// Identity
	if opts.Identity == "" {
		opts.Identity = cfg.Identity
	}
	if opts.Identity == "" {
		opts.Identity = config.DetectGitIdentity()
	}
	var gitName, gitEmail string
	if opts.Identity != "" {
		gitName, gitEmail, err = config.ParseIdentity(opts.Identity)
		if err != nil {
			return fmt.Errorf("parse identity: %w", err)
		}
	}

	// Container name
	timestamp := time.Now().Format("20060102-150405")
	containerName := resolveContainerName(opts.AgentType, opts.Name, timestamp, opts.Repos)

	// Network
	customNetwork := opts.Network
	if customNetwork == "" {
		customNetwork = cfg.Network
	}
	networkName, networkMode, err := docker.PrepareNetwork(ctx, exec, containerName, customNetwork, opts.DryRun)
	if err != nil {
		return fmt.Errorf("prepare network: %w", err)
	}

	// Build Docker command
	imageName := "safe-agentic:latest"
	cmd := docker.NewRunCmd(containerName, imageName)
	cmd.AddEnv("AGENT_TYPE", opts.AgentType)
	if len(opts.Repos) > 0 {
		cmd.AddEnv("REPOS", strings.Join(opts.Repos, " "))
	}
	if gitName != "" {
		cmd.AddEnv("GIT_AUTHOR_NAME", gitName)
		cmd.AddEnv("GIT_AUTHOR_EMAIL", gitEmail)
		cmd.AddEnv("GIT_COMMITTER_NAME", gitName)
		cmd.AddEnv("GIT_COMMITTER_EMAIL", gitEmail)
	}

	docker.AppendRuntimeHardening(cmd, docker.HardeningOpts{
		Network:   networkName,
		Memory:    memory,
		CPUs:      cpus,
		PIDsLimit: pidsLimit,
	})
	docker.AppendCacheMounts(cmd)

	// Auto-trust: tell entrypoint to use --dangerously-skip-permissions
	if opts.AutoTrust {
		cmd.AddEnv("SAFE_AGENTIC_AUTO_TRUST", "1")
	}

	// SSH
	if opts.SSH {
		if opts.DryRun {
			docker.AppendSSHMountDryRun(cmd)
		} else {
			if err := docker.AppendSSHMount(ctx, exec, cmd); err != nil {
				return err
			}
		}
	}

	// Labels
	cmd.AddLabel(labels.AgentType, opts.AgentType)
	cmd.AddLabel(labels.SSH, fmt.Sprintf("%v", opts.SSH))
	cmd.AddLabel(labels.RepoDisplay, repourl.DisplayLabel(opts.Repos))
	cmd.AddLabel(labels.NetworkMode, networkMode)
	cmd.AddLabel(labels.Resources, fmt.Sprintf("cpu=%s,mem=%s,pids=%d", cpus, memory, pidsLimit))
	cmd.AddLabel(labels.Terminal, "tmux")

	// Auth volumes
	// Fleet/pipeline agents get per-container volumes (seeded from shared) for session isolation.
	if opts.FleetVolume != "" && !opts.EphemeralAuth {
		sharedVol := docker.AuthVolumeName(opts.AgentType, false, "")
		perContainerVol := containerName + "-auth"
		// Create per-container volume and seed ALL contents from shared volume
		if !opts.DryRun {
			exec.Run(ctx, "docker", "volume", "create", perContainerVol)
			exec.Run(ctx, "docker", "run", "--rm",
				"-v", sharedVol+":/src:ro",
				"-v", perContainerVol+":/dst",
				"alpine", "sh", "-c",
				"cp -a /src/. /dst/ 2>/dev/null; true")
		}
		cmd.AddLabel(labels.AuthType, "fleet-isolated")
		cmd.AddNamedVolume(perContainerVol, authDestination(opts.AgentType))
	} else if opts.EphemeralAuth {
		cmd.AddLabel(labels.AuthType, "ephemeral")
		authVol := docker.AuthVolumeName(opts.AgentType, true, containerName)
		cmd.AddNamedVolume(authVol, authDestination(opts.AgentType))
	} else {
		cmd.AddLabel(labels.AuthType, "shared")
		authVol := docker.AuthVolumeName(opts.AgentType, false, "")
		cmd.AddNamedVolume(authVol, authDestination(opts.AgentType))
	}

	// GH auth
	if opts.ReuseGHAuth {
		cmd.AddLabel(labels.GHAuth, "shared")
		ghVol := docker.GHAuthVolumeName(opts.AgentType)
		cmd.AddNamedVolume(ghVol, "/home/agent/.config/gh")
	}

	// Host config injection
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		home, _ := os.UserHomeDir()
		claudeDir = home + "/.claude"
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		home, _ := os.UserHomeDir()
		codexHome = home + "/.codex"
	}
	if opts.AgentType == "claude" || opts.AgentType == "shell" {
		if envs, err := inject.ReadClaudeConfig(claudeDir); err == nil {
			for k, v := range envs {
				cmd.AddEnv(k, v)
			}
		}
		// Inject support files: CLAUDE.md, hooks/, commands/, statusline-command.sh
		if envs, err := inject.ReadClaudeSupportFiles(claudeDir); err == nil {
			for k, v := range envs {
				cmd.AddEnv(k, v)
			}
		}
	}
	if opts.AgentType == "codex" || opts.AgentType == "shell" {
		if envs, err := inject.ReadCodexConfig(codexHome); err == nil {
			for k, v := range envs {
				cmd.AddEnv(k, v)
			}
		}
	}

	// AWS
	if opts.AWSProfile != "" {
		home, _ := os.UserHomeDir()
		credPath := home + "/.aws/credentials"
		envs, err := inject.ReadAWSCredentials(credPath, opts.AWSProfile)
		if err != nil {
			return fmt.Errorf("AWS credentials: %w", err)
		}
		for k, v := range envs {
			cmd.AddEnv(k, v)
		}
		cmd.AddTmpfs("/home/agent/.aws", "1m", true, false)
		cmd.AddLabel(labels.AWS, opts.AWSProfile)
	}

	// Prompt / instructions / template
	// Pass prompt as -p CMD arg so agent-session.sh handles it natively:
	// - background mode: claude runs non-interactively and exits when done
	// - foreground mode: saved to pending-prompt, sent via tmux send-keys
	if opts.Prompt != "" {
		cmd.AddCmdArgs("-p", opts.Prompt)
		cmd.AddLabel(labels.Prompt, truncate(opts.Prompt, 100))
	}
	if opts.Instructions != "" {
		cmd.AddEnv("SAFE_AGENTIC_INSTRUCTIONS_B64", inject.EncodeB64(opts.Instructions))
		cmd.AddLabel(labels.Instructions, "1")
	}
	if opts.InstructionsFile != "" {
		data, err := os.ReadFile(opts.InstructionsFile)
		if err != nil {
			return fmt.Errorf("read instructions file: %w", err)
		}
		cmd.AddEnv("SAFE_AGENTIC_INSTRUCTIONS_B64", inject.EncodeB64(string(data)))
		cmd.AddLabel(labels.Instructions, "1")
	}
	if opts.Template != "" {
		cmd.AddEnv("SAFE_AGENTIC_TEMPLATE_B64", inject.EncodeB64(opts.Template))
	}

	// Callbacks
	if opts.OnExit != "" {
		cmd.AddLabel(labels.OnExit, "1")
		cmd.AddEnv("SAFE_AGENTIC_ON_EXIT_B64", inject.EncodeB64(opts.OnExit))
	}
	if opts.OnComplete != "" {
		cmd.AddLabel(labels.OnCompleteB64, inject.EncodeB64(opts.OnComplete))
	}
	if opts.OnFail != "" {
		cmd.AddLabel(labels.OnFailB64, inject.EncodeB64(opts.OnFail))
	}
	if opts.MaxCost != "" {
		cmd.AddLabel(labels.MaxCost, opts.MaxCost)
	}
	if opts.Notify != "" {
		cmd.AddLabel(labels.NotifyB64, inject.EncodeB64(opts.Notify))
	}
	if opts.FleetVolume != "" {
		cmd.AddNamedVolume(opts.FleetVolume, "/fleet")
		cmd.AddLabel(labels.Fleet, opts.FleetVolume)
		// Tell agent-session.sh to pass -p directly to Claude (non-interactive exit)
		cmd.AddEnv("SAFE_AGENTIC_FLEET", "1")
	}

	// Docker access
	if opts.DockerSocket {
		if !opts.DryRun {
			if err := docker.AppendHostDockerSocket(ctx, exec, cmd); err != nil {
				return fmt.Errorf("mount docker socket: %w", err)
			}
		}
		cmd.AddLabel(labels.DockerMode, "host-socket")
	} else if opts.DockerAccess {
		docker.AppendDinDAccess(cmd, containerName)
		cmd.AddLabel(labels.DockerMode, "dind")
	} else {
		cmd.AddLabel(labels.DockerMode, "off")
	}

	// Dry run
	if opts.DryRun {
		fmt.Println("Would execute:")
		fmt.Printf("  orb run -m safe-agentic %s\n", cmd.Render())
		return nil
	}

	// Execute
	cmd.Detached = true
	fullArgs := cmd.Build()
	_, err = exec.Run(ctx, fullArgs...)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	fmt.Printf("Agent %s started: %s\n", opts.AgentType, containerName)

	// DinD sidecar
	if opts.DockerAccess {
		if err := docker.StartDinDRuntime(ctx, exec, containerName, networkName, imageName); err != nil {
			return fmt.Errorf("start Docker runtime: %w", err)
		}
	}

	// Audit log
	auditLogger := &audit.Logger{Path: audit.DefaultPath()}
	auditLogger.Log("spawn", containerName, map[string]string{
		"type":    opts.AgentType,
		"repos":   strings.Join(opts.Repos, ","),
		"ssh":     fmt.Sprintf("%v", opts.SSH),
		"network": networkMode,
	})

	// Event
	events.Emit(events.DefaultEventsPath(), "agent.spawned", map[string]string{
		"container": containerName,
		"type":      opts.AgentType,
	})

	// Auto-attach
	if !opts.Background && opts.AgentType != "shell" {
		if err := tmux.WaitForSession(ctx, exec, containerName); err != nil {
			return err
		}
		return tmux.Attach(exec, containerName)
	}
	return nil
}

func resolveContainerName(agentType, name, timestamp string, repos []string) string {
	prefix := docker.ContainerPrefix + "-" + agentType
	if name != "" {
		return prefix + "-" + name
	}
	if len(repos) > 0 {
		slug, err := repourl.ClonePath(repos[0])
		if err == nil {
			parts := strings.Split(slug, "/")
			if len(parts) == 2 {
				short := parts[1]
				if len(short) > 20 {
					short = short[:20]
				}
				return prefix + "-" + short
			}
		}
	}
	return prefix + "-" + timestamp
}

func authDestination(agentType string) string {
	switch agentType {
	case "claude":
		return "/home/agent/.claude"
	case "codex":
		return "/home/agent/.codex"
	default:
		return "/home/agent/.claude"
	}
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
