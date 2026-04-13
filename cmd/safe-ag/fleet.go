package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/fleet"
	"github.com/0x666c6f/safe-agentic/pkg/orb"

	"github.com/spf13/cobra"
)

// ─── fleet ──────────────────────────────────────────────────────────────────

var fleetDryRun bool

var fleetCmd = &cobra.Command{
	Use:   "fleet <manifest.yaml>",
	Short: "Spawn agents from a fleet manifest",
	Args:  cobra.ExactArgs(1),
	RunE:  runFleet,
}

func init() {
	fleetCmd.Flags().BoolVar(&fleetDryRun, "dry-run", false, "Print what would run without executing")
	fleetCmd.AddCommand(fleetStatusCmd)
	rootCmd.AddCommand(fleetCmd)
}

func runFleet(cmd *cobra.Command, args []string) error {
	m, err := fleet.ParseFleet(args[0])
	if err != nil {
		return err
	}

	if len(m.Agents) == 0 {
		fmt.Println("No agents defined in manifest.")
		return nil
	}

	// Create shared fleet volume (unless dry-run)
	timestamp := time.Now().Format("20060102-150405")
	fleetVolume := "fleet-" + timestamp

	if fleetDryRun {
		printFleetDryRun(args[0], m)
		return nil
	}

	ctx := context.Background()
	exec := newExecutor()

	if err := createFleetVolume(ctx, exec, fleetVolume); err != nil {
		return err
	}
	fmt.Printf("Fleet volume: %s\n", fleetVolume)
	spawned, err := spawnFleetAgents(cmd, m.Agents, fleetVolume)
	if err != nil {
		return err
	}
	fmt.Printf("Fleet spawned %d agent(s).\n", spawned)
	return nil
}

// ─── fleet status ────────────────────────────────────────────────────────────

var fleetStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show running fleet progress",
	RunE:  runFleetStatus,
}

func runFleetStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "label=safe-agentic.fleet",
		"--format", "{{.Names}}\t{{.Status}}")
	if err != nil {
		return fmt.Errorf("list fleet containers: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		fmt.Println("No fleet containers found.")
		return nil
	}
	fmt.Print(string(out))
	return nil
}

// ─── pipeline ────────────────────────────────────────────────────────────────

var pipelineDryRun bool
var pipelineBackground bool

const pipelineDetachedEnv = "SAFE_AGENTIC_PIPELINE_DETACHED"

var launchDetachedPipeline = launchDetachedPipelineImpl

var pipelineCmd = &cobra.Command{
	Use:   "pipeline <pipeline.yaml>",
	Short: "Run sequential pipeline with dependency ordering",
	Long: `Run a multi-step pipeline defined in a YAML manifest.

Steps can declare dependencies via depends_on, on_failure, retry, when,
and outputs fields. Stages with no unmet dependencies are spawned first;
subsequent stages run once all their dependencies have completed.

Example manifest fields per step:
  depends_on: <step-name>   # wait for this step to succeed
  on_failure:  <step-name>  # run this step on failure
  retry:       N            # retry up to N times
  when:        <condition>  # skip step if condition not met
  outputs:     <command>    # command to extract outputs for downstream steps`,
	Args: cobra.ExactArgs(1),
	RunE: runPipeline,
}

func init() {
	pipelineCmd.Flags().BoolVar(&pipelineDryRun, "dry-run", false, "Print execution plan without running")
	pipelineCmd.Flags().BoolVar(&pipelineBackground, "background", false, "Run the pipeline in the background and return immediately")
	rootCmd.AddCommand(pipelineCmd)
}

func runPipeline(cmd *cobra.Command, args []string) error {
	m, err := fleet.ParsePipeline(args[0])
	if err != nil {
		return err
	}

	if len(m.Stages) == 0 {
		fmt.Println("No stages defined in pipeline manifest.")
		return nil
	}

	name := m.Name
	if name == "" {
		name = args[0]
	}

	if pipelineDryRun {
		printPipelineTree(name, m.Stages)
		return nil
	}

	if pipelineBackground && os.Getenv(pipelineDetachedEnv) != "1" {
		return launchDetachedPipeline(args[0])
	}

	ctx := context.Background()
	exec := newExecutor()
	timestamp := time.Now().Format("20060102-150405")
	return runPipelineManifest(ctx, exec, m, pipelineDryRun, timestamp, "", nil)
}

func launchDetachedPipelineImpl(manifestPath string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return fmt.Errorf("resolve home dir: %w", homeErr)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}

	logDir := filepath.Join(stateHome, "safe-agentic", "pipelines")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("create pipeline log dir: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", filepath.Base(manifestPath), ts))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open pipeline log: %w", err)
	}

	childArgs := []string{"pipeline", manifestPath, "--background"}
	cmd := exec.Command(self, childArgs...)
	cmd.Env = append(os.Environ(), pipelineDetachedEnv+"=1")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start detached pipeline: %w", err)
	}
	_ = logFile.Close()

	fmt.Printf("Pipeline detached: pid=%d log=%s\n", cmd.Process.Pid, logPath)
	return nil
}

// runPipelineManifest executes a pipeline manifest. Extracted for recursive sub-pipeline support.
func runPipelineManifest(ctx context.Context, exec orb.Executor, m *fleet.PipelineManifest, dryRun bool, timestamp string, rootLabel string, parentPath []string) error {
	name, rootLabel, currentPath := pipelineContext(m, timestamp, rootLabel, parentPath)
	completed := make(map[string]bool)
	remaining := append([]fleet.PipelineStage{}, m.Stages...)

	for len(remaining) > 0 {
		ready, notReady, err := partitionReadyStages(remaining, completed)
		if err != nil {
			return err
		}
		containerNames, err := runReadyStages(ctx, exec, ready, dryRun, timestamp, rootLabel, currentPath)
		if err != nil {
			return err
		}
		if err := waitForContainers(ctx, exec, containerNames); err != nil {
			return err
		}
		markStagesCompleted(completed, ready)
		remaining = notReady
	}

	fmt.Printf("Pipeline %q complete.\n", name)
	return nil
}

// depsmet reports whether all dependencies in deps are present in completed.
func depsmet(deps []string, completed map[string]bool) bool {
	for _, d := range deps {
		if !completed[d] {
			return false
		}
	}
	return true
}

func printFleetDryRun(path string, m *fleet.FleetManifest) {
	fmt.Printf("Fleet manifest: %s\n", path)
	if m.Name != "" {
		fmt.Printf("Fleet name: %s\n", m.Name)
	}
	fmt.Printf("Agents: %d\n\n", len(m.Agents))
	for _, spec := range m.Agents {
		printFleetDryRunSpec(spec)
	}
}

func printFleetDryRunSpec(spec fleet.AgentSpec) {
	if spec.Type == "" {
		fmt.Printf("  [skip] %q — missing type\n", spec.Name)
		return
	}
	opts := specToSpawnOpts(spec, "fleet-dry-run")
	fmt.Printf("  Would spawn: safe-ag spawn %s", opts.AgentType)
	appendFleetDryRunFlags(opts)
	fmt.Println()
}

func appendFleetDryRunFlags(opts SpawnOpts) {
	if opts.Name != "" {
		fmt.Printf(" --name %s", opts.Name)
	}
	for _, r := range opts.Repos {
		fmt.Printf(" --repo %s", r)
	}
	appendBoolFlag(opts.SSH, " --ssh")
	appendBoolFlag(opts.ReuseAuth, " --reuse-auth")
	appendBoolFlag(opts.ReuseGHAuth, " --reuse-gh-auth")
	appendBoolFlag(opts.AutoTrust, " --auto-trust")
	appendBoolFlag(opts.Background, " --background")
	appendBoolFlag(opts.DockerAccess, " --docker")
	appendStringFlag(opts.Network, " --network %s")
	appendStringFlag(opts.Memory, " --memory %s")
	appendStringFlag(opts.CPUs, " --cpus %s")
	appendStringFlag(opts.AWSProfile, " --aws %s")
	if opts.Prompt != "" {
		fmt.Printf(" --prompt %q", opts.Prompt)
	}
}

func appendBoolFlag(enabled bool, flag string) {
	if enabled {
		fmt.Print(flag)
	}
}

func appendStringFlag(value, format string) {
	if value != "" {
		fmt.Printf(format, value)
	}
}

func createFleetVolume(ctx context.Context, exec orb.Executor, fleetVolume string) error {
	if _, err := exec.Run(ctx, "docker", "volume", "create",
		"--label", "app=safe-agentic",
		"--label", "safe-agentic.type=fleet",
		fleetVolume,
	); err != nil {
		return fmt.Errorf("create fleet volume: %w", err)
	}
	return nil
}

func spawnFleetAgents(cmd *cobra.Command, agents []fleet.AgentSpec, fleetVolume string) (int, error) {
	var spawned int
	for _, spec := range agents {
		if spec.Type == "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "[fleet] skipping entry %q — missing type\n", spec.Name)
			continue
		}
		if err := executeSpawn(specToSpawnOpts(spec, fleetVolume)); err != nil {
			return 0, fmt.Errorf("spawn %q: %w", spec.Name, err)
		}
		spawned++
	}
	return spawned, nil
}

func pipelineContext(m *fleet.PipelineManifest, timestamp, rootLabel string, parentPath []string) (string, string, []string) {
	name := m.Name
	if name == "" {
		name = "(inline)"
	}
	if rootLabel == "" {
		rootLabel = name
		if rootLabel == "(inline)" {
			rootLabel = "pipeline-" + timestamp
		}
	}
	currentPath := append(append([]string{}, parentPath...), name)
	return name, rootLabel, currentPath
}

func partitionReadyStages(remaining []fleet.PipelineStage, completed map[string]bool) ([]fleet.PipelineStage, []fleet.PipelineStage, error) {
	var ready []fleet.PipelineStage
	var notReady []fleet.PipelineStage
	for _, stage := range remaining {
		if depsmet(stage.DependsOn, completed) {
			ready = append(ready, stage)
		} else {
			notReady = append(notReady, stage)
		}
	}
	if len(ready) > 0 {
		return ready, notReady, nil
	}
	return nil, nil, fmt.Errorf("pipeline stuck: stages with unmet dependencies: %s", strings.Join(stageNames(notReady), ", "))
}

func stageNames(stages []fleet.PipelineStage) []string {
	names := make([]string, 0, len(stages))
	for _, s := range stages {
		names = append(names, s.Name)
	}
	return names
}

func runReadyStages(ctx context.Context, exec orb.Executor, ready []fleet.PipelineStage, dryRun bool, timestamp, rootLabel string, currentPath []string) ([]string, error) {
	var containerNames []string
	for _, stage := range ready {
		names, err := runPipelineStage(ctx, exec, stage, dryRun, timestamp, rootLabel, currentPath)
		if err != nil {
			return nil, err
		}
		containerNames = append(containerNames, names...)
	}
	return containerNames, nil
}

func runPipelineStage(ctx context.Context, exec orb.Executor, stage fleet.PipelineStage, dryRun bool, timestamp, rootLabel string, currentPath []string) ([]string, error) {
	fmt.Printf("Running stage: %s\n", stage.Name)
	if stage.Pipeline != "" {
		return nil, runSubPipelineStage(ctx, exec, stage, dryRun, timestamp, rootLabel, currentPath)
	}
	return spawnPipelineStageAgents(stage, rootLabel, currentPath)
}

func runSubPipelineStage(ctx context.Context, exec orb.Executor, stage fleet.PipelineStage, dryRun bool, timestamp, rootLabel string, currentPath []string) error {
	fmt.Printf("  Sub-pipeline: %s\n", stage.Pipeline)
	subManifest, err := fleet.ParsePipeline(stage.Pipeline)
	if err != nil {
		return fmt.Errorf("stage %q: parse sub-pipeline %q: %w", stage.Name, stage.Pipeline, err)
	}
	if err := runPipelineManifest(ctx, exec, subManifest, dryRun, timestamp, rootLabel, currentPath); err != nil {
		return fmt.Errorf("stage %q: sub-pipeline %q: %w", stage.Name, stage.Pipeline, err)
	}
	return nil
}

func spawnPipelineStageAgents(stage fleet.PipelineStage, rootLabel string, currentPath []string) ([]string, error) {
	var containerNames []string
	for _, spec := range stage.Agents {
		name, err := spawnPipelineAgent(stage, spec, rootLabel, currentPath)
		if err != nil {
			return nil, err
		}
		if name != "" {
			containerNames = append(containerNames, name)
		}
	}
	return containerNames, nil
}

func spawnPipelineAgent(stage fleet.PipelineStage, spec fleet.AgentSpec, rootLabel string, currentPath []string) (string, error) {
	if spec.Type == "" {
		return "", nil
	}
	opts := specToSpawnOpts(spec, rootLabel)
	opts.Hierarchy = pipelineStageHierarchy(currentPath, stage.Name)
	opts.Background = true
	if err := executeSpawn(opts); err != nil {
		return "", fmt.Errorf("stage %q: spawn %q: %w", stage.Name, spec.Name, err)
	}
	return resolveContainerName(opts.AgentType, opts.Name, time.Now().Format("20060102-150405"), opts.Repos), nil
}

func pipelineStageHierarchy(currentPath []string, stageName string) string {
	return strings.Join(append(append([]string{}, currentPath...), stageName), "/")
}

func markStagesCompleted(completed map[string]bool, ready []fleet.PipelineStage) {
	for _, stage := range ready {
		completed[stage.Name] = true
	}
}

// waitForContainers polls docker inspect until all containers exit successfully.
func waitForContainers(ctx context.Context, exec orb.Executor, names []string) error {
	if len(names) == 0 {
		return nil
	}
	fmt.Printf("Waiting for %d agent(s) to complete...\n", len(names))
	done := make(map[string]bool)
	var failed []string
	for {
		allDone := true
		for _, name := range names {
			if done[name] {
				continue
			}
			state, err := containerState(ctx, exec, name)
			if err != nil {
				allDone = false
				continue
			}
			if state == "exited" || state == "dead" {
				exitCode, err := containerExitCode(ctx, exec, name)
				if err != nil {
					return fmt.Errorf("inspect exit code for %s: %w", name, err)
				}
				if exitCode != 0 {
					return fmt.Errorf("container %s exited with status %d", name, exitCode)
				}
				done[name] = true
				exitCode, codeErr := containerExitCode(ctx, exec, name)
				if codeErr != nil {
					fmt.Printf("  ✓ %s exited\n", name)
					allDone = false
					continue
				}
				if exitCode == 0 {
					fmt.Printf("  ✓ %s exited\n", name)
				} else {
					fmt.Printf("  ✗ %s exited (%d)\n", name, exitCode)
					failed = append(failed, fmt.Sprintf("%s (%d)", name, exitCode))
				}
			} else {
				allDone = false
			}
		}
		if allDone {
			if len(failed) > 0 {
				return fmt.Errorf("containers failed: %s", strings.Join(failed, ", "))
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// ─── helper ──────────────────────────────────────────────────────────────────

// specToSpawnOpts converts an AgentSpec from a fleet or pipeline manifest into
// SpawnOpts suitable for executeSpawn.
func specToSpawnOpts(spec fleet.AgentSpec, fleetVolume string) SpawnOpts {
	repos := spec.Repos
	if len(repos) == 0 && spec.Repo != "" {
		repos = []string{spec.Repo}
	}
	return SpawnOpts{
		AgentType:    spec.Type,
		Repos:        repos,
		Name:         spec.Name,
		Prompt:       spec.Prompt,
		SSH:          spec.SSH,
		ReuseAuth:    spec.ReuseAuth,
		ReuseGHAuth:  spec.ReuseGHAuth,
		AutoTrust:    spec.AutoTrust,
		Background:   spec.Background,
		DockerAccess: spec.Docker,
		Network:      spec.Network,
		Memory:       spec.Memory,
		CPUs:         spec.CPUs,
		AWSProfile:   spec.AWS,
		FleetVolume:  fleetVolume,
	}
}

// printPipelineTree renders pipeline stages as a tree diagram under a root node.
// Stages with the same Parent are grouped under a sub-pipeline node.
func printPipelineTree(name string, stages []fleet.PipelineStage) {
	fmt.Printf("🔄 %s\n", name)

	// Group consecutive stages by parent
	type group struct {
		parent string
		stages []fleet.PipelineStage
	}
	var groups []group
	for _, stage := range stages {
		p := stage.Parent
		if p == "" {
			p = stage.Name // standalone
		}
		if len(groups) > 0 && groups[len(groups)-1].parent == p {
			groups[len(groups)-1].stages = append(groups[len(groups)-1].stages, stage)
		} else {
			groups = append(groups, group{parent: p, stages: []fleet.PipelineStage{stage}})
		}
	}

	for gi, g := range groups {
		gLast := gi == len(groups)-1
		gBranch := "├──"
		gPrefix := "│   "
		if gLast {
			gBranch = "└──"
			gPrefix = "    "
		}

		// If group has multiple stages (model expansion), show parent
		if len(g.stages) > 1 {
			deps := ""
			if len(g.stages[0].DependsOn) > 0 {
				deps = fmt.Sprintf(" (after: %s)", strings.Join(g.stages[0].DependsOn, ", "))
			}
			fmt.Printf("%s 📦 %s%s\n", gBranch, g.parent, deps)
			for si, stage := range g.stages {
				sLast := si == len(g.stages)-1
				sBranch := gPrefix + "├──"
				sPrefix := gPrefix + "│   "
				if sLast {
					sBranch = gPrefix + "└──"
					sPrefix = gPrefix + "    "
				}
				fmt.Printf("%s 📦 %s\n", sBranch, stage.Name)
				printStageAgents(stage, sPrefix)
			}
		} else {
			stage := g.stages[0]
			deps := ""
			if len(stage.DependsOn) > 0 {
				deps = fmt.Sprintf(" (after: %s)", strings.Join(stage.DependsOn, ", "))
			}
			if stage.Pipeline != "" {
				fmt.Printf("%s 📋 %s%s → %s\n", gBranch, stage.Name, deps, stage.Pipeline)
			} else {
				fmt.Printf("%s 📦 %s%s\n", gBranch, stage.Name, deps)
				printStageAgents(stage, gPrefix)
			}
		}
	}
}

func printStageAgents(stage fleet.PipelineStage, prefix string) {
	for j, spec := range stage.Agents {
		agentLast := j == len(stage.Agents)-1
		agentBranch := prefix + "├── "
		if agentLast {
			agentBranch = prefix + "└── "
		}
		icon := "🤖"
		switch spec.Type {
		case "claude":
			icon = "🟠"
		case "codex":
			icon = "🔵"
		}
		label := spec.Name
		if label == "" {
			label = spec.Type
		}
		fmt.Printf("%s%s %s", agentBranch, icon, label)
		if spec.Memory != "" {
			fmt.Printf(" [%s", spec.Memory)
			if spec.CPUs != "" {
				fmt.Printf(", %s cpu", spec.CPUs)
			}
			fmt.Print("]")
		}
		fmt.Println()
	}
}
