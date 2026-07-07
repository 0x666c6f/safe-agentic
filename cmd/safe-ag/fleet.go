package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/catalog"
	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/events"
	"github.com/0x666c6f/safe-agentic/pkg/fleet"
	"github.com/0x666c6f/safe-agentic/pkg/inject"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/profiles"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ─── fleet ──────────────────────────────────────────────────────────────────

var fleetDryRun bool
var fleetManifestRepos []string
var fleetManifestVars []string

var fleetCmd = &cobra.Command{
	Use:   "fleet <manifest.yaml>",
	Short: "Spawn a group of agents in parallel from a YAML manifest",
	Long: `Spawn several agents at once from a fleet manifest. Every agent in the file
launches in parallel, each in its own hardened container.

Use 'fleet status' to watch progress. For staged, dependency-ordered runs use
'safe-ag pipeline' instead.`,
	Example: `  safe-ag fleet manifest.yaml
  safe-ag fleet manifest.yaml --dry-run
  safe-ag fleet manifest.yaml --repo git@github.com:org/repo.git --var env=staging
  safe-ag fleet status`,
	Args:    cobra.ExactArgs(1),
	GroupID: groupFleet,
	RunE:    runFleet,
}

func init() {
	fleetCmd.Flags().BoolVar(&fleetDryRun, "dry-run", false, "Print the agents that would spawn, then exit without launching")
	fleetCmd.Flags().StringSliceVar(&fleetManifestRepos, "repo", nil, "Fallback repo URL for manifest agents that don't set repo/repos; repeatable")
	fleetCmd.Flags().StringSliceVar(&fleetManifestVars, "var", nil, "Manifest variable as key=value, filling ${key} placeholders; repeatable")
	fleetCmd.AddCommand(fleetStatusCmd)
	rootCmd.AddCommand(fleetCmd)
}

func runFleet(cmd *cobra.Command, args []string) error {
	parseOpts, err := fleetParseOptions(args[0])
	if err != nil {
		return err
	}
	m, err := fleet.ParseFleetWithOptions(args[0], parseOpts)
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
var pipelineManifestRepos []string
var pipelineManifestVars []string
var pipelineSeedAuth bool
var pipelineAutoTrust bool

const pipelineDetachedEnv = "SAFE_AGENTIC_PIPELINE_DETACHED"

var launchDetachedPipeline = launchDetachedPipelineImpl

var pipelineCmd = &cobra.Command{
	Use:   "pipeline <pipeline.yaml|name>",
	Short: "Run a multi-step pipeline with dependency ordering",
	Long: `Run a multi-step pipeline defined in a YAML manifest.

The manifest can be passed as a filesystem path or as a saved pipeline name
from ~/.safe-ag/pipelines.

Steps can declare dependencies via depends_on. Stages with no unmet
dependencies are spawned first; subsequent stages run once all their
dependencies have completed successfully.

Unsupported control fields such as on_failure, retry, when, and outputs are
rejected instead of silently ignored.`,
	Example: `  safe-ag pipeline pipeline.yaml
  safe-ag pipeline pipeline.yaml --dry-run
  safe-ag pipeline my-saved-pipeline --var branch=main
  safe-ag pipeline pipeline.yaml --background`,
	Args:    cobra.ExactArgs(1),
	GroupID: groupFleet,
	RunE:    runPipeline,
}

func init() {
	pipelineCmd.Flags().BoolVar(&pipelineDryRun, "dry-run", false, "Print the resolved execution plan (stage order), then exit without running")
	pipelineCmd.Flags().BoolVar(&pipelineBackground, "background", false, "Launch the pipeline detached and return immediately instead of waiting")
	pipelineCmd.Flags().StringSliceVar(&pipelineManifestRepos, "repo", nil, "Fallback repo URL for manifest agents that don't set repo/repos; repeatable")
	pipelineCmd.Flags().StringSliceVar(&pipelineManifestVars, "var", nil, "Manifest variable as key=value, filling ${key} placeholders; repeatable")
	pipelineCmd.Flags().BoolVar(&pipelineSeedAuth, "seed-auth", false, "Seed the host's Claude/Codex login into every agent (like GUI spawns)")
	pipelineCmd.Flags().BoolVar(&pipelineAutoTrust, "auto-trust", false, "Auto-accept each agent's in-container trust prompt")
	pipelineCmd.AddCommand(pipelineListCmd, pipelineShowCmd, pipelineInspectCmd, pipelineRenderCmd, pipelineValidateCmd, pipelineCreateCmd)
	rootCmd.AddCommand(pipelineCmd)
}

func runPipeline(cmd *cobra.Command, args []string) error {
	manifestPath, err := resolvePipelineManifest(args[0])
	if err != nil {
		return err
	}
	parseOpts, err := pipelineParseOptions(manifestPath)
	if err != nil {
		return err
	}
	m, err := fleet.ParsePipelineWithOptions(manifestPath, parseOpts)
	if err != nil {
		return err
	}

	if len(m.Stages) == 0 {
		fmt.Println("No stages defined in pipeline manifest.")
		return nil
	}
	applyPipelineAuthOverrides(m)

	name := m.Name
	if name == "" {
		name = manifestPath
	}

	if pipelineDryRun {
		printPipelineTree(name, m.Stages)
		return nil
	}

	if pipelineBackground && os.Getenv(pipelineDetachedEnv) != "1" {
		return launchDetachedPipeline(manifestPath)
	}

	ctx := context.Background()
	exec := newExecutor()
	timestamp := time.Now().Format("20060102-150405")
	return runPipelineManifest(ctx, exec, m, parseOpts, pipelineDryRun, timestamp, "", nil)
}

func launchDetachedPipelineImpl(manifestPath string) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = config.StateDir()
	}
	logPath, logFile, err := openDetachedPipelineLog(stateHome, manifestPath)
	if err != nil {
		return err
	}

	childArgs := []string{"pipeline", manifestPath, "--background"}
	for _, repo := range pipelineManifestRepos {
		childArgs = append(childArgs, "--repo", repo)
	}
	for _, variable := range pipelineManifestVars {
		childArgs = append(childArgs, "--var", variable)
	}
	if pipelineSeedAuth {
		childArgs = append(childArgs, "--seed-auth")
	}
	if pipelineAutoTrust {
		childArgs = append(childArgs, "--auto-trust")
	}
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

func openDetachedPipelineLog(stateHome, manifestPath string) (string, *os.File, error) {
	logDir := filepath.Join(stateHome, "pipelines")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return "", nil, fmt.Errorf("create pipeline log dir: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%s.log", filepath.Base(manifestPath), ts))
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", nil, fmt.Errorf("open pipeline log: %w", err)
	}
	if err := logFile.Chmod(0o600); err != nil {
		_ = logFile.Close()
		return "", nil, fmt.Errorf("chmod pipeline log: %w", err)
	}
	return logPath, logFile, nil
}

// runPipelineManifest executes a pipeline manifest. Extracted for recursive sub-pipeline support.
func runPipelineManifest(ctx context.Context, exec vmexec.Executor, m *fleet.PipelineManifest, parseOpts fleet.ParseOptions, dryRun bool, timestamp string, rootLabel string, parentPath []string) error {
	name, rootLabel, currentPath := pipelineContext(m, timestamp, rootLabel, parentPath)
	completed := make(map[string]bool)
	stageContainers := make(map[string][]string)
	remaining := append([]fleet.PipelineStage{}, m.Stages...)

	for len(remaining) > 0 {
		ready, notReady, err := partitionReadyStages(remaining, completed)
		if err != nil {
			return err
		}
		containerNames, err := runReadyStages(ctx, exec, ready, parseOpts, dryRun, timestamp, rootLabel, currentPath, stageContainers)
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
	appendBoolFlag(opts.SeedAuth, " --seed-auth")
	appendBoolFlag(opts.AutoTrust, " --auto-trust")
	appendBoolFlag(opts.Background, " --background")
	appendBoolFlag(opts.DockerAccess, " --docker")
	appendBoolFlag(opts.DockerSocket, " --docker-socket")
	appendBoolFlag(opts.AllowSetupScripts, " --allow-setup-scripts")
	appendBoolFlag(opts.EphemeralAuth, " --ephemeral-auth")
	appendStringFlag(opts.Template, " --template %s")
	for _, variable := range opts.TemplateVars {
		fmt.Printf(" --var %s", variable)
	}
	appendStringFlag(opts.Instructions, " --instructions %q")
	appendStringFlag(opts.InstructionsFile, " --instructions-file %s")
	appendStringFlag(opts.Network, " --network %s")
	appendStringFlag(opts.Memory, " --memory %s")
	appendStringFlag(opts.CPUs, " --cpus %s")
	if opts.PIDsLimit > 0 {
		fmt.Printf(" --pids-limit %d", opts.PIDsLimit)
	}
	appendStringFlag(opts.Identity, " --identity %q")
	appendStringFlag(opts.AWSProfile, " --aws %s")
	appendStringFlag(opts.MaxCost, " --max-cost %s")
	appendStringFlag(opts.Notify, " --notify %s")
	appendStringFlag(opts.OnExit, " --on-exit %q")
	appendStringFlag(opts.OnComplete, " --on-complete %q")
	appendStringFlag(opts.OnFail, " --on-fail %q")
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

func createFleetVolume(ctx context.Context, exec vmexec.Executor, fleetVolume string) error {
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
		// Missing/unknown types are rejected at manifest parse time, so every
		// spec here is spawnable.
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

func runReadyStages(ctx context.Context, exec vmexec.Executor, ready []fleet.PipelineStage, parseOpts fleet.ParseOptions, dryRun bool, timestamp, rootLabel string, currentPath []string, stageContainers map[string][]string) ([]string, error) {
	var containerNames []string
	for _, stage := range ready {
		names, err := runPipelineStage(ctx, exec, stage, parseOpts, dryRun, timestamp, rootLabel, currentPath, stageContainers)
		if err != nil {
			return nil, err
		}
		if len(names) > 0 {
			stageContainers[stage.Name] = append(stageContainers[stage.Name], names...)
		}
		containerNames = append(containerNames, names...)
	}
	return containerNames, nil
}

func runPipelineStage(ctx context.Context, exec vmexec.Executor, stage fleet.PipelineStage, parseOpts fleet.ParseOptions, dryRun bool, timestamp, rootLabel string, currentPath []string, stageContainers map[string][]string) ([]string, error) {
	fmt.Printf("Running stage: %s\n", stage.Name)
	if stage.Judge != nil {
		// The judge manages its own agent lifecycle (spawn + wait), so it
		// contributes no container names to the outer wait barrier.
		return nil, runJudgeStage(ctx, exec, stage, stageContainers, rootLabel, timestamp)
	}
	if stage.Pipeline != "" {
		return nil, runSubPipelineStage(ctx, exec, stage, parseOpts, dryRun, timestamp, rootLabel, currentPath)
	}
	return spawnPipelineStageAgents(stage, rootLabel, currentPath, timestamp)
}

func runSubPipelineStage(ctx context.Context, exec vmexec.Executor, stage fleet.PipelineStage, parseOpts fleet.ParseOptions, dryRun bool, timestamp, rootLabel string, currentPath []string) error {
	fmt.Printf("  Sub-pipeline: %s\n", stage.Pipeline)
	subOpts := parseOpts
	for _, dir := range profileDirsForManifest(stage.Pipeline) {
		subOpts.ProfileDirs = appendUniqueString(subOpts.ProfileDirs, dir)
	}
	subManifest, err := fleet.ParsePipelineWithOptions(stage.Pipeline, subOpts)
	if err != nil {
		return fmt.Errorf("stage %q: parse sub-pipeline %q: %w", stage.Name, stage.Pipeline, err)
	}
	if err := runPipelineManifest(ctx, exec, subManifest, subOpts, dryRun, timestamp, rootLabel, currentPath); err != nil {
		return fmt.Errorf("stage %q: sub-pipeline %q: %w", stage.Name, stage.Pipeline, err)
	}
	return nil
}

var pipelineListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved user pipelines",
	RunE:  runPipelineList,
}

var pipelineShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Display a saved pipeline",
	Args:  cobra.ExactArgs(1),
	RunE:  runPipelineShow,
}

var pipelineCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new saved pipeline",
	Args:  cobra.ExactArgs(1),
	RunE:  runPipelineCreate,
}

var pipelineInspectCmd = &cobra.Command{
	Use:   "inspect <name|path>",
	Short: "Show pipeline metadata and resolved inputs",
	Args:  cobra.ExactArgs(1),
	RunE:  runPipelineInspect,
}

var pipelineRenderCmd = &cobra.Command{
	Use:   "render <name|path>",
	Short: "Render the fully resolved pipeline manifest",
	Args:  cobra.ExactArgs(1),
	RunE:  runPipelineRender,
}

var pipelineValidateCmd = &cobra.Command{
	Use:   "validate <name|path>",
	Short: "Validate a pipeline without running it",
	Args:  cobra.ExactArgs(1),
	RunE:  runPipelineValidate,
}

func runPipelineList(cmd *cobra.Command, args []string) error {
	entries, err := catalog.ListPipelines()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No pipelines found.")
		return nil
	}
	fmt.Printf("%-28s  %-10s  %s\n", "NAME", "SOURCE", "DESCRIPTION")
	fmt.Println(strings.Repeat("─", 88))
	for _, asset := range entries {
		fmt.Printf("%-28s  %-10s  %s\n", asset.Manifest.Name, asset.Source, asset.Manifest.Description)
	}
	return nil
}

func runPipelineShow(cmd *cobra.Command, args []string) error {
	asset, err := catalog.ResolvePipeline(args[0])
	if err != nil {
		return err
	}
	data, err := os.ReadFile(asset.Path)
	if err != nil {
		return fmt.Errorf("read pipeline: %w", err)
	}
	fmt.Printf("Name: %s\nSource: %s\nPath: %s\n", asset.Manifest.Name, asset.Source, asset.Path)
	if asset.Manifest.Description != "" {
		fmt.Printf("Description: %s\n", asset.Manifest.Description)
	}
	fmt.Println()
	fmt.Print(string(data))
	return nil
}

func runPipelineCreate(cmd *cobra.Command, args []string) error {
	if err := catalog.ValidateAssetName(args[0]); err != nil {
		return err
	}
	path := filepath.Join(config.PipelinesDir(), filepath.FromSlash(args[0])+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create pipelines dir: %w", err)
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("pipeline %q already exists at %s", args[0], path)
	}
	starter := fmt.Sprintf(`name: %s
description: Describe what this pipeline does.
inputs:
  - name: repo
    description: Repository URL or current checkout repo.
    infer: repo
  - name: pr
    description: Pull request number when relevant.
    infer: pr
examples:
  - safe-ag pipeline %s
tags:
  - custom
steps:
  - name: review
    type: claude
    repo: ${repo}
    prompt: Review this repository and summarize actionable findings.
`, args[0], args[0])
	if err := os.WriteFile(path, []byte(starter), 0o644); err != nil {
		return fmt.Errorf("create pipeline file: %w", err)
	}
	fmt.Printf("Created pipeline: %s\n", path)
	return nil
}

func resolvePipelineManifest(arg string) (string, error) {
	asset, err := catalog.ResolvePipeline(arg)
	if err != nil {
		return "", err
	}
	return asset.Path, nil
}

func resolveNamedPipeline(name string) (string, error) {
	return resolvePipelineManifest(name)
}

func pipelineNameFromFile(name string) string {
	switch {
	case strings.HasSuffix(name, ".yaml"):
		return strings.TrimSuffix(name, ".yaml")
	case strings.HasSuffix(name, ".yml"):
		return strings.TrimSuffix(name, ".yml")
	default:
		return ""
	}
}

func fleetParseOptions(manifestPath string) (fleet.ParseOptions, error) {
	vars, repos, err := parseTemplateVars(fleetManifestVars, fleetManifestRepos, true)
	if err != nil {
		return fleet.ParseOptions{}, err
	}
	return fleet.ParseOptions{
		Vars:         vars,
		DefaultRepos: repos,
		ProfileDirs:  profileDirsForManifest(manifestPath),
	}, nil
}

func pipelineParseOptions(manifestPath string) (fleet.ParseOptions, error) {
	vars, repos, err := parseTemplateVars(pipelineManifestVars, pipelineManifestRepos, true)
	if err != nil {
		return fleet.ParseOptions{}, err
	}
	return fleet.ParseOptions{
		Vars:         vars,
		DefaultRepos: repos,
		ProfileDirs:  profileDirsForManifest(manifestPath),
	}, nil
}

func profileDirsForManifest(manifestPath string) []string {
	var dirs []string
	dirs = appendUniqueString(dirs, profiles.UserDir())
	if wd, err := os.Getwd(); err == nil {
		dirs = appendUniqueString(dirs, profiles.ProjectDir(wd))
	}
	if manifestPath != "" {
		dirs = appendUniqueString(dirs, profiles.ProjectDir(filepath.Dir(manifestPath)))
	}
	return dirs
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func runPipelineInspect(cmd *cobra.Command, args []string) error {
	asset, err := catalog.ResolvePipeline(args[0])
	if err != nil {
		return err
	}
	parseOpts, err := pipelineParseOptions(asset.Path)
	if err != nil {
		return err
	}
	fmt.Printf("Name: %s\nSource: %s\nPath: %s\n", asset.Manifest.Name, asset.Source, asset.Path)
	if asset.Manifest.Description != "" {
		fmt.Printf("Description: %s\n", asset.Manifest.Description)
	}
	if len(asset.Manifest.Inputs) > 0 {
		fmt.Println("Inputs:")
		for _, input := range asset.Manifest.Inputs {
			status := "optional"
			if input.Required {
				status = "required"
			}
			value := input.Default
			if parseOpts.Vars[input.Name] != "" {
				value = parseOpts.Vars[input.Name]
			} else if input.Name == "repo" && len(parseOpts.DefaultRepos) > 0 {
				value = parseOpts.DefaultRepos[0]
			}
			if input.Infer != "" {
				status += ", infer=" + input.Infer
			}
			if value != "" {
				status += ", value=" + value
			}
			fmt.Printf("  - %s (%s)\n", input.Name, status)
		}
	}
	if len(asset.Manifest.Examples) > 0 {
		fmt.Println("Examples:")
		for _, example := range asset.Manifest.Examples {
			fmt.Printf("  - %s\n", example)
		}
	}
	return nil
}

func runPipelineRender(cmd *cobra.Command, args []string) error {
	manifestPath, err := resolvePipelineManifest(args[0])
	if err != nil {
		return err
	}
	parseOpts, err := pipelineParseOptions(manifestPath)
	if err != nil {
		return err
	}
	m, err := fleet.ParsePipelineWithOptions(manifestPath, parseOpts)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal pipeline: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

func runPipelineValidate(cmd *cobra.Command, args []string) error {
	manifestPath, err := resolvePipelineManifest(args[0])
	if err != nil {
		return err
	}
	parseOpts, err := pipelineParseOptions(manifestPath)
	if err != nil {
		return err
	}
	if _, err := fleet.ParsePipelineWithOptions(manifestPath, parseOpts); err != nil {
		return err
	}
	fmt.Printf("Pipeline %s is valid.\n", args[0])
	return nil
}

func spawnPipelineStageAgents(stage fleet.PipelineStage, rootLabel string, currentPath []string, timestamp string) ([]string, error) {
	var containerNames []string
	for _, spec := range stage.Agents {
		name, err := spawnPipelineAgent(stage, spec, rootLabel, currentPath, timestamp)
		if err != nil {
			return nil, err
		}
		if name != "" {
			containerNames = append(containerNames, name)
		}
	}
	return containerNames, nil
}

func spawnPipelineAgent(stage fleet.PipelineStage, spec fleet.AgentSpec, rootLabel string, currentPath []string, timestamp string) (string, error) {
	// Missing/unknown types are rejected at manifest parse time.
	opts := specToSpawnOpts(spec, rootLabel)
	opts.Name = pipelineContainerSuffix(stage, spec, currentPath, timestamp)
	opts.Hierarchy = pipelineStageHierarchy(currentPath, stage.Name)
	opts.Background = true
	if err := executeSpawn(opts); err != nil {
		return "", fmt.Errorf("stage %q: spawn %q: %w", stage.Name, spec.Name, err)
	}
	return resolveContainerName(opts.AgentType, opts.Name, timestamp, opts.Repos), nil
}

func pipelineStageHierarchy(currentPath []string, stageName string) string {
	return strings.Join(append(append([]string{}, currentPath...), stageName), "/")
}

func pipelineContainerSuffix(stage fleet.PipelineStage, spec fleet.AgentSpec, currentPath []string, timestamp string) string {
	parts := append(append([]string{}, currentPath...), stage.Name)
	if spec.Name != "" {
		parts = append(parts, spec.Name)
	} else {
		parts = append(parts, spec.Type)
	}
	parts = append(parts, timestamp)
	return safeNameComponent(strings.Join(parts, "-"), 96)
}

func safeNameComponent(value string, maxLen int) string {
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		allowed := (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if allowed {
			if r == '-' && lastDash {
				continue
			}
			b.WriteRune(r)
			lastDash = r == '-'
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
		if maxLen > 0 && b.Len() >= maxLen {
			break
		}
	}
	safe := strings.Trim(b.String(), ".-_")
	if safe == "" {
		return "pipeline"
	}
	return safe
}

func markStagesCompleted(completed map[string]bool, ready []fleet.PipelineStage) {
	for _, stage := range ready {
		completed[stage.Name] = true
	}
}

// waitForContainers polls docker inspect until all containers exit successfully.
func waitForContainers(ctx context.Context, exec vmexec.Executor, names []string) error {
	if len(names) == 0 {
		return nil
	}
	fmt.Printf("Waiting for %d agent(s) to complete...\n", len(names))
	done := make(map[string]bool)
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
				dispatchContainerNotify(ctx, exec, name, exitCode)
				if exitCode != 0 {
					return withExitCode(exitAgentFail, fmt.Errorf("container %s exited with status %d", name, exitCode))
				}
				done[name] = true
				fmt.Printf("  ✓ %s exited\n", name)
			} else {
				allDone = false
			}
		}
		if allDone {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// dispatchContainerNotify delivers a completed container's result to the
// --notify targets recorded in its label (set at spawn time). This is the
// reliable completion hook for background fleet/pipeline agents, which the
// operator is typically not watching. Failures are swallowed so a broken
// notifier never aborts the pipeline.
func dispatchContainerNotify(ctx context.Context, exec vmexec.Executor, name string, exitCode int) {
	raw, _ := docker.InspectLabel(ctx, exec, name, labels.NotifyB64)
	if raw == "" {
		return
	}
	spec, err := inject.DecodeB64(raw)
	if err != nil || spec == "" {
		return
	}
	targets := events.ParseNotifyTargets(spec)
	if len(targets) == 0 {
		return
	}
	status, msg := "done", "Agent finished"
	if exitCode != 0 {
		status = events.StatusFailed
		msg = fmt.Sprintf("Agent exited with code %d", exitCode)
	}
	_ = events.Dispatch(targets, events.SystemNotification{
		Container: name,
		Message:   msg,
		Sound:     events.SoundForStatus(status),
	}, os.Stdout)
}

// ─── helper ──────────────────────────────────────────────────────────────────

// applyPipelineAuthOverrides forces host-auth seeding and/or trust-prompt skip
// on every agent when the run passed --seed-auth/--auto-trust. Manifests set
// reuse_auth but not seed_auth, so without this a GUI-run pipeline's agents can
// land unauthenticated ("Not logged in") the way GUI spawns never do.
func applyPipelineAuthOverrides(m *fleet.PipelineManifest) {
	if !pipelineSeedAuth && !pipelineAutoTrust {
		return
	}
	for i := range m.Stages {
		for j := range m.Stages[i].Agents {
			if pipelineSeedAuth {
				m.Stages[i].Agents[j].SeedAuth = true
			}
			if pipelineAutoTrust {
				m.Stages[i].Agents[j].AutoTrust = true
			}
		}
	}
}

// specToSpawnOpts converts an AgentSpec from a fleet or pipeline manifest into
// SpawnOpts suitable for executeSpawn.
func specToSpawnOpts(spec fleet.AgentSpec, fleetVolume string) SpawnOpts {
	repos := spec.Repos
	if len(repos) == 0 && spec.Repo != "" {
		repos = []string{spec.Repo}
	}
	return SpawnOpts{
		AgentType:         spec.Type,
		Repos:             repos,
		Name:              spec.Name,
		Prompt:            spec.Prompt,
		Template:          spec.Template,
		TemplateVars:      spec.TemplateVars,
		Instructions:      spec.Instructions,
		InstructionsFile:  spec.InstructionsFile,
		SSH:               spec.SSH,
		ReuseAuth:         spec.ReuseAuth,
		EphemeralAuth:     spec.EphemeralAuth,
		ReuseGHAuth:       spec.ReuseGHAuth,
		SeedAuth:          spec.SeedAuth,
		AutoTrust:         spec.AutoTrust,
		Background:        spec.Background,
		DockerAccess:      spec.Docker,
		DockerSocket:      spec.DockerSocket,
		Network:           spec.Network,
		Memory:            spec.Memory,
		CPUs:              spec.CPUs,
		PIDsLimit:         spec.PIDsLimit,
		Identity:          spec.Identity,
		AWSProfile:        spec.AWS,
		MaxCost:           spec.MaxCost,
		Notify:            spec.Notify,
		OnExit:            spec.OnExit,
		OnComplete:        spec.OnComplete,
		OnFail:            spec.OnFail,
		AllowSetupScripts: spec.AllowSetupScripts,
		FleetVolume:       fleetVolume,
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
	if stage.Judge != nil {
		criteria := stage.Judge.Criteria
		if criteria == "" {
			criteria = "(default quality criteria)"
		}
		fmt.Printf("%s└── 🏆 judge — %s\n", prefix, criteria)
		if stage.Judge.AutoPR {
			base := stage.Judge.Base
			if base == "" {
				base = "main"
			}
			fmt.Printf("%s    ↳ auto PR from winner → base %s\n", prefix, base)
		}
		return
	}
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
