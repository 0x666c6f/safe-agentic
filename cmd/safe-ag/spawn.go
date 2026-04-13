package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/audit"
	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/events"
	"github.com/0x666c6f/safe-agentic/pkg/inject"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/orb"
	"github.com/0x666c6f/safe-agentic/pkg/repourl"
	"github.com/0x666c6f/safe-agentic/pkg/tmux"
	"github.com/0x666c6f/safe-agentic/pkg/validate"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type SpawnOpts struct {
	AgentType         string
	Repos             []string
	Name              string
	Prompt            string
	Template          string
	Instructions      string
	InstructionsFile  string
	SSH               bool
	ReuseAuth         bool
	EphemeralAuth     bool
	ReuseGHAuth       bool
	DockerAccess      bool
	DockerSocket      bool
	Network           string
	Memory            string
	CPUs              string
	PIDsLimit         int
	Identity          string
	AWSProfile        string
	AutoTrust         bool
	Background        bool
	OnExit            string
	OnComplete        string
	OnFail            string
	MaxCost           string
	Notify            string
	FleetVolume       string
	Hierarchy         string
	AllowSetupScripts bool
	DryRun            bool
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
	f.BoolVar(&spawnOpts.AllowSetupScripts, "allow-setup-scripts", false, "Allow repo-provided safe-agentic.json setup hooks to run")
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
	rf.BoolVar(&spawnOpts.AllowSetupScripts, "allow-setup-scripts", false, "Allow repo-provided safe-agentic.json setup hooks to run")
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
	opts.Identity = identity
	return executeSpawn(opts)
}

func executeSpawn(opts SpawnOpts) error {
	ctx := context.Background()
	exec := newExecutor()

	if err := validateSpawnOpts(opts); err != nil {
		return err
	}

	resolved, err := prepareSpawnResolved(opts)
	if err != nil {
		return err
	}
	if err := validateResolvedSpawn(resolved); err != nil {
		return err
	}
	if err := prepareSpawnNetwork(ctx, exec, opts, &resolved); err != nil {
		return err
	}

	cmd := buildSpawnRunCmd(opts, resolved)
	if err := appendSpawnSSH(ctx, exec, cmd, opts); err != nil {
		return err
	}
	appendSpawnLabels(cmd, opts, resolved)
	appendAuthVolumes(ctx, exec, cmd, opts, resolved)
	appendGHAuth(cmd, opts)
	appendHostConfig(cmd, opts)
	if err := appendAWSConfig(cmd, opts); err != nil {
		return err
	}
	if err := appendPromptAndTemplate(cmd, opts); err != nil {
		return err
	}
	appendCallbacksAndMetadata(cmd, opts)
	if err := appendDockerMode(ctx, exec, cmd, opts, resolved); err != nil {
		return err
	}
	if opts.DryRun {
		fmt.Println("Would execute:")
		fmt.Printf("  orb run -m safe-agentic %s\n", cmd.Render())
		return nil
	}

	if err := startSpawnContainer(ctx, exec, cmd, opts, resolved); err != nil {
		return err
	}
	logSpawnEvent(opts, resolved)
	return maybeAttachSpawn(ctx, exec, opts, resolved)
}

type spawnResolved struct {
	Config        config.Config
	Memory        string
	CPUs          string
	PIDsLimit     int
	GitName       string
	GitEmail      string
	ContainerName string
	NetworkName   string
	NetworkMode   string
	ImageName     string
}

func validateSpawnOpts(opts SpawnOpts) error {
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
	if opts.ReuseAuth && opts.EphemeralAuth {
		return fmt.Errorf("--reuse-auth and --ephemeral-auth are mutually exclusive")
	}
	if opts.DockerAccess && opts.DockerSocket {
		return fmt.Errorf("--docker and --docker-socket are mutually exclusive")
	}
	if err := validate.MemoryLimit(opts.Memory); err != nil {
		return err
	}
	if err := validate.CPUs(opts.CPUs); err != nil {
		return err
	}
	if opts.PIDsLimit > 0 {
		if err := validate.PIDsLimit(opts.PIDsLimit); err != nil {
			return err
		}
	}
	return docker.EnsureSSHForRepos(opts.SSH, opts.Repos)
}

func validateResolvedSpawn(resolved spawnResolved) error {
	if err := validate.MemoryLimit(resolved.Memory); err != nil {
		return err
	}
	if err := validate.CPUs(resolved.CPUs); err != nil {
		return err
	}
	if resolved.PIDsLimit > 0 {
		if err := validate.PIDsLimit(resolved.PIDsLimit); err != nil {
			return err
		}
	}
	return nil
}

func prepareSpawnResolved(opts SpawnOpts) (spawnResolved, error) {
	cfg, err := config.LoadDefaults(config.DefaultsPath())
	if err != nil {
		return spawnResolved{}, fmt.Errorf("load config: %w", err)
	}

	resolved := spawnResolved{
		Config:        cfg,
		Memory:        coalesce(opts.Memory, cfg.Memory),
		CPUs:          coalesce(opts.CPUs, cfg.CPUs),
		PIDsLimit:     opts.PIDsLimit,
		ContainerName: resolveContainerName(opts.AgentType, opts.Name, time.Now().Format("20060102-150405"), opts.Repos),
		ImageName:     "safe-agentic:latest",
	}
	if resolved.PIDsLimit == 0 && cfg.PIDsLimit != "" {
		v, err := strconv.Atoi(cfg.PIDsLimit)
		if err != nil {
			return spawnResolved{}, fmt.Errorf("parse configured PIDs limit %q: %w", cfg.PIDsLimit, err)
		}
		resolved.PIDsLimit = v
	}

	identity := opts.Identity
	if identity == "" {
		identity = cfg.Identity
	}
	if identity == "" {
		identity = config.DetectGitIdentity()
	}
	if identity == "" {
		return resolved, nil
	}

	resolved.GitName, resolved.GitEmail, err = config.ParseIdentity(identity)
	if err != nil {
		return spawnResolved{}, fmt.Errorf("parse identity: %w", err)
	}
	return resolved, nil
}

func prepareSpawnNetwork(ctx context.Context, exec orb.Executor, opts SpawnOpts, resolved *spawnResolved) error {
	customNetwork := opts.Network
	if customNetwork == "" {
		customNetwork = resolved.Config.Network
	}
	networkName, networkMode, err := docker.PrepareNetwork(ctx, exec, resolved.ContainerName, customNetwork, opts.DryRun)
	if err != nil {
		return fmt.Errorf("prepare network: %w", err)
	}
	resolved.NetworkName = networkName
	resolved.NetworkMode = networkMode
	return nil
}

func buildSpawnRunCmd(opts SpawnOpts, resolved spawnResolved) *docker.DockerRunCmd {
	cmd := docker.NewRunCmd(resolved.ContainerName, resolved.ImageName)
	cmd.AddEnv("AGENT_TYPE", opts.AgentType)
	if len(opts.Repos) > 0 {
		cmd.AddEnv("REPOS", strings.Join(opts.Repos, ","))
	}
	if resolved.GitName != "" {
		cmd.AddEnv("GIT_AUTHOR_NAME", resolved.GitName)
		cmd.AddEnv("GIT_AUTHOR_EMAIL", resolved.GitEmail)
		cmd.AddEnv("GIT_COMMITTER_NAME", resolved.GitName)
		cmd.AddEnv("GIT_COMMITTER_EMAIL", resolved.GitEmail)
	}
	docker.AppendRuntimeHardening(cmd, docker.HardeningOpts{
		Network:   resolved.NetworkName,
		Memory:    resolved.Memory,
		CPUs:      resolved.CPUs,
		PIDsLimit: resolved.PIDsLimit,
	})
	docker.AppendCacheMounts(cmd)
	if opts.AutoTrust {
		cmd.AddEnv("SAFE_AGENTIC_AUTO_TRUST", "1")
	}
	if opts.AllowSetupScripts {
		cmd.AddEnv("SAFE_AGENTIC_ALLOW_SETUP_SCRIPTS", "1")
	}
	return cmd
}

func appendSpawnSSH(ctx context.Context, exec orb.Executor, cmd *docker.DockerRunCmd, opts SpawnOpts) error {
	if !opts.SSH {
		return nil
	}
	if opts.DryRun {
		docker.AppendSSHMountDryRun(cmd)
		return nil
	}
	return docker.AppendSSHMount(ctx, exec, cmd)
}

func appendSpawnLabels(cmd *docker.DockerRunCmd, opts SpawnOpts, resolved spawnResolved) {
	cmd.AddLabel(labels.AgentType, opts.AgentType)
	cmd.AddLabel(labels.SSH, fmt.Sprintf("%v", opts.SSH))
	cmd.AddLabel(labels.RepoDisplay, repourl.DisplayLabel(opts.Repos))
	cmd.AddLabel(labels.NetworkMode, resolved.NetworkMode)
	cmd.AddLabel(labels.Resources, fmt.Sprintf("cpu=%s,mem=%s,pids=%d", resolved.CPUs, resolved.Memory, resolved.PIDsLimit))
	cmd.AddLabel(labels.Terminal, "tmux")
}

func appendAuthVolumes(ctx context.Context, exec orb.Executor, cmd *docker.DockerRunCmd, opts SpawnOpts, resolved spawnResolved) {
	if opts.FleetVolume != "" && opts.ReuseAuth {
		sharedVol := docker.AuthVolumeName(opts.AgentType, false, "")
		perContainerVol := resolved.ContainerName + "-auth"
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
		return
	}
	if opts.ReuseAuth {
		cmd.AddLabel(labels.AuthType, "shared")
		cmd.AddNamedVolume(docker.AuthVolumeName(opts.AgentType, false, ""), authDestination(opts.AgentType))
		return
	}
	cmd.AddLabel(labels.AuthType, "ephemeral")
	cmd.AddTmpfsOwned(authDestination(opts.AgentType), "8m", true, false, 1000, 1000)
}

func appendGHAuth(cmd *docker.DockerRunCmd, opts SpawnOpts) {
	if !opts.ReuseGHAuth {
		return
	}
	cmd.AddLabel(labels.GHAuth, "shared")
	cmd.AddNamedVolume(docker.GHAuthVolumeName(opts.AgentType), "/home/agent/.config/gh")
}

func appendHostConfig(cmd *docker.DockerRunCmd, opts SpawnOpts) {
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
		envs, err := inject.ReadClaudeConfig(claudeDir)
		appendEnvMap(cmd, envs, err)
		envs, err = inject.ReadClaudeOAuthToken()
		appendEnvMap(cmd, envs, err)
		envs, err = inject.ReadClaudeAuth(os.Getenv("HOME"))
		appendEnvMap(cmd, envs, err)
		envs, err = inject.ReadClaudeSupportFiles(claudeDir)
		appendEnvMap(cmd, envs, err)
	}
	if opts.AgentType == "codex" || opts.AgentType == "shell" {
		envs, err := inject.ReadCodexConfig(codexHome)
		appendEnvMap(cmd, envs, err)
		envs, err = inject.ReadCodexAuth(codexHome)
		appendEnvMap(cmd, envs, err)
	}
}

func appendEnvMap(cmd *docker.DockerRunCmd, envs map[string]string, err error) {
	if err != nil {
		return
	}
	for k, v := range envs {
		cmd.AddEnv(k, v)
	}
}

func appendAWSConfig(cmd *docker.DockerRunCmd, opts SpawnOpts) error {
	if opts.AWSProfile == "" {
		return nil
	}
	home, _ := os.UserHomeDir()
	envs, err := inject.ReadAWSCredentials(home+"/.aws/credentials", opts.AWSProfile)
	if err != nil {
		return fmt.Errorf("AWS credentials: %w", err)
	}
	for k, v := range envs {
		cmd.AddEnv(k, v)
	}
	cmd.AddTmpfs("/home/agent/.aws", "1m", true, false)
	cmd.AddLabel(labels.AWS, opts.AWSProfile)
	return nil
}

func appendPromptAndTemplate(cmd *docker.DockerRunCmd, opts SpawnOpts) error {
	templateContent := ""
	if opts.Template != "" {
		path, err := findTemplate(opts.Template)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read template: %w", err)
		}
		templateContent = strings.TrimSpace(string(data))
	}

	instructions := opts.Instructions
	if templateContent != "" {
		if instructions == "" && opts.InstructionsFile == "" && opts.Prompt == "" {
			opts.Prompt = templateContent
		} else if instructions == "" {
			instructions = templateContent
		} else {
			instructions = templateContent + "\n\n" + instructions
		}
	}

	if opts.Prompt != "" {
		if opts.AgentType == "codex" {
			cmd.AddCmdArgs(opts.Prompt)
		} else {
			cmd.AddCmdArgs("-p", opts.Prompt)
		}
		cmd.AddLabel(labels.Prompt, truncate(opts.Prompt, 100))
	}
	if instructions != "" {
		cmd.AddEnv("SAFE_AGENTIC_INSTRUCTIONS_B64", inject.EncodeB64(instructions))
		cmd.AddLabel(labels.Instructions, "1")
	}
	if opts.InstructionsFile != "" {
		data, err := os.ReadFile(opts.InstructionsFile)
		if err != nil {
			return fmt.Errorf("read instructions file: %w", err)
		}
		fileInstructions := string(data)
		if templateContent != "" {
			fileInstructions = templateContent + "\n\n" + fileInstructions
		}
		cmd.AddEnv("SAFE_AGENTIC_INSTRUCTIONS_B64", inject.EncodeB64(fileInstructions))
		cmd.AddLabel(labels.Instructions, "1")
	}
	if opts.Template != "" {
		cmd.AddEnv("SAFE_AGENTIC_TEMPLATE_B64", inject.EncodeB64(opts.Template))
	}
	return nil
}

func appendCallbacksAndMetadata(cmd *docker.DockerRunCmd, opts SpawnOpts) {
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
		cmd.AddEnv("SAFE_AGENTIC_FLEET", "1")
	}
	if opts.Hierarchy != "" {
		cmd.AddLabel(labels.Hierarchy, opts.Hierarchy)
	}
}

func appendDockerMode(ctx context.Context, exec orb.Executor, cmd *docker.DockerRunCmd, opts SpawnOpts, resolved spawnResolved) error {
	if opts.DockerSocket {
		if !opts.DryRun {
			if err := docker.AppendHostDockerSocket(ctx, exec, cmd); err != nil {
				return fmt.Errorf("mount docker socket: %w", err)
			}
		}
		cmd.AddLabel(labels.DockerMode, "host-socket")
		return nil
	}
	if opts.DockerAccess {
		docker.AppendDinDAccess(cmd, resolved.ContainerName)
		cmd.AddLabel(labels.DockerMode, "dind")
		return nil
	}
	cmd.AddLabel(labels.DockerMode, "off")
	return nil
}

func startSpawnContainer(ctx context.Context, exec orb.Executor, cmd *docker.DockerRunCmd, opts SpawnOpts, resolved spawnResolved) error {
	cmd.Detached = true
	if _, err := exec.Run(ctx, cmd.Build()...); err != nil {
		return fmt.Errorf("start container: %w", err)
	}
	fmt.Printf("Agent %s started: %s\n", opts.AgentType, resolved.ContainerName)
	if !opts.DockerAccess {
		return nil
	}
	if err := docker.StartDinDRuntime(ctx, exec, resolved.ContainerName, resolved.NetworkName, resolved.ImageName); err != nil {
		return fmt.Errorf("start Docker runtime: %w", err)
	}
	return nil
}

func logSpawnEvent(opts SpawnOpts, resolved spawnResolved) {
	auditLogger := &audit.Logger{Path: audit.DefaultPath()}
	auditLogger.Log("spawn", resolved.ContainerName, map[string]string{
		"type":    opts.AgentType,
		"repos":   strings.Join(opts.Repos, ","),
		"ssh":     fmt.Sprintf("%v", opts.SSH),
		"network": resolved.NetworkMode,
	})
	events.Emit(events.DefaultEventsPath(), "agent.spawned", map[string]string{
		"container": resolved.ContainerName,
		"type":      opts.AgentType,
	})
}

func maybeAttachSpawn(ctx context.Context, exec orb.Executor, opts SpawnOpts, resolved spawnResolved) error {
	if opts.Background || opts.AgentType == "shell" {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Printf("Not attaching to %s: stdin is not a terminal. Use --background to silence this message.\n", resolved.ContainerName)
		return nil
	}
	if err := tmux.WaitForSession(ctx, exec, resolved.ContainerName); err != nil {
		return err
	}
	return tmux.Attach(exec, resolved.ContainerName)
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
