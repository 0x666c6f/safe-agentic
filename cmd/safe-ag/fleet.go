package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"safe-agentic/pkg/fleet"
	"safe-agentic/pkg/orb"

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
		fmt.Printf("Fleet manifest: %s\n", args[0])
		if m.Name != "" {
			fmt.Printf("Fleet name: %s\n", m.Name)
		}
		fmt.Printf("Agents: %d\n\n", len(m.Agents))
		for _, spec := range m.Agents {
			if spec.Type == "" {
				fmt.Printf("  [skip] %q — missing type\n", spec.Name)
				continue
			}
			opts := specToSpawnOpts(spec, "fleet-dry-run")
			fmt.Printf("  Would spawn: agent spawn %s", opts.AgentType)
			if opts.Name != "" {
				fmt.Printf(" --name %s", opts.Name)
			}
			for _, r := range opts.Repos {
				fmt.Printf(" --repo %s", r)
			}
			if opts.SSH {
				fmt.Print(" --ssh")
			}
			if opts.ReuseAuth {
				fmt.Print(" --reuse-auth")
			}
			if opts.ReuseGHAuth {
				fmt.Print(" --reuse-gh-auth")
			}
			if opts.AutoTrust {
				fmt.Print(" --auto-trust")
			}
			if opts.Background {
				fmt.Print(" --background")
			}
			if opts.DockerAccess {
				fmt.Print(" --docker")
			}
			if opts.Network != "" {
				fmt.Printf(" --network %s", opts.Network)
			}
			if opts.Memory != "" {
				fmt.Printf(" --memory %s", opts.Memory)
			}
			if opts.CPUs != "" {
				fmt.Printf(" --cpus %s", opts.CPUs)
			}
			if opts.AWSProfile != "" {
				fmt.Printf(" --aws %s", opts.AWSProfile)
			}
			if opts.Prompt != "" {
				fmt.Printf(" --prompt %q", opts.Prompt)
			}
			fmt.Println()
		}
		return nil
	}

	ctx := context.Background()
	exec := newExecutor()

	// Create shared fleet volume
	if _, err := exec.Run(ctx, "docker", "volume", "create",
		"--label", "app=safe-agentic",
		"--label", "safe-agentic.type=fleet",
		fleetVolume,
	); err != nil {
		return fmt.Errorf("create fleet volume: %w", err)
	}
	fmt.Printf("Fleet volume: %s\n", fleetVolume)

	var spawned int
	for _, spec := range m.Agents {
		if spec.Type == "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "[fleet] skipping entry %q — missing type\n", spec.Name)
			continue
		}
		opts := specToSpawnOpts(spec, fleetVolume)
		if err := executeSpawn(opts); err != nil {
			return fmt.Errorf("spawn %q: %w", spec.Name, err)
		}
		spawned++
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

	ctx := context.Background()
	exec := newExecutor()
	timestamp := time.Now().Format("20060102-150405")
	return runPipelineManifest(ctx, exec, m, pipelineDryRun, timestamp)
}

// runPipelineManifest executes a pipeline manifest. Extracted for recursive sub-pipeline support.
func runPipelineManifest(ctx context.Context, exec orb.Executor, m *fleet.PipelineManifest, dryRun bool, timestamp string) error {
	name := m.Name
	if name == "" {
		name = "(inline)"
	}

	completed := make(map[string]bool)
	remaining := make([]fleet.PipelineStage, len(m.Stages))
	copy(remaining, m.Stages)

	for len(remaining) > 0 {
		var ready []fleet.PipelineStage
		var notReady []fleet.PipelineStage
		for _, stage := range remaining {
			if depsmet(stage.DependsOn, completed) {
				ready = append(ready, stage)
			} else {
				notReady = append(notReady, stage)
			}
		}
		if len(ready) == 0 {
			var names []string
			for _, s := range notReady {
				names = append(names, s.Name)
			}
			return fmt.Errorf("pipeline stuck: stages with unmet dependencies: %s", strings.Join(names, ", "))
		}

		var containerNames []string
		for _, stage := range ready {
			fmt.Printf("Running stage: %s\n", stage.Name)

			// Sub-pipeline: recursively execute another pipeline file
			if stage.Pipeline != "" {
				fmt.Printf("  Sub-pipeline: %s\n", stage.Pipeline)
				subManifest, err := fleet.ParsePipeline(stage.Pipeline)
				if err != nil {
					return fmt.Errorf("stage %q: parse sub-pipeline %q: %w", stage.Name, stage.Pipeline, err)
				}
				if err := runPipelineManifest(ctx, exec, subManifest, dryRun, timestamp); err != nil {
					return fmt.Errorf("stage %q: sub-pipeline %q: %w", stage.Name, stage.Pipeline, err)
				}
				continue
			}

			for _, spec := range stage.Agents {
				if spec.Type == "" {
					continue
				}
				fleetLabel := name
				if fleetLabel == "(inline)" {
					fleetLabel = "pipeline-" + timestamp
				}
				opts := specToSpawnOpts(spec, fleetLabel)
				opts.Background = true
				if err := executeSpawn(opts); err != nil {
					return fmt.Errorf("stage %q: spawn %q: %w", stage.Name, spec.Name, err)
				}
				containerNames = append(containerNames, resolveContainerName(
					opts.AgentType, opts.Name,
					time.Now().Format("20060102-150405"), opts.Repos))
			}
		}

		if len(containerNames) > 0 {
			if err := waitForContainers(ctx, exec, containerNames); err != nil {
				return err
			}
		}

		for _, stage := range ready {
			completed[stage.Name] = true
		}
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

// waitForContainers polls docker inspect until all containers exit.
func waitForContainers(ctx context.Context, exec orb.Executor, names []string) error {
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

// ─── helper ──────────────────────────────────────────────────────────────────

// specToSpawnOpts converts an AgentSpec from a fleet or pipeline manifest into
// SpawnOpts suitable for executeSpawn.
func specToSpawnOpts(spec fleet.AgentSpec, fleetVolume string) SpawnOpts {
	repos := spec.Repos
	if len(repos) == 0 && spec.Repo != "" {
		repos = []string{spec.Repo}
	}
	return SpawnOpts{
		AgentType:   spec.Type,
		Repos:       repos,
		Name:        spec.Name,
		Prompt:      spec.Prompt,
		SSH:         spec.SSH,
		ReuseAuth:   spec.ReuseAuth,
		ReuseGHAuth: spec.ReuseGHAuth,
		AutoTrust:   spec.AutoTrust,
		Background:  spec.Background,
		DockerAccess: spec.Docker,
		Network:     spec.Network,
		Memory:      spec.Memory,
		CPUs:        spec.CPUs,
		AWSProfile:  spec.AWS,
		FleetVolume: fleetVolume,
	}
}

// printPipelineTree renders pipeline stages as a tree diagram under a root node.
func printPipelineTree(name string, stages []fleet.PipelineStage) {
	fmt.Printf("🔄 %s\n", name)

	for i, stage := range stages {
		isLast := i == len(stages)-1
		branch := "├──"
		childPrefix := "│   "
		if isLast {
			branch = "└──"
			childPrefix = "    "
		}

		// Stage header
		deps := ""
		if len(stage.DependsOn) > 0 {
			deps = fmt.Sprintf(" (after: %s)", strings.Join(stage.DependsOn, ", "))
		}
		if stage.Pipeline != "" {
			fmt.Printf("%s 📋 %s%s → %s\n", branch, stage.Name, deps, stage.Pipeline)
			continue
		}

		fmt.Printf("%s 📦 %s%s\n", branch, stage.Name, deps)

		for j, spec := range stage.Agents {
			agentIsLast := j == len(stage.Agents)-1
			agentBranch := childPrefix + "├── "
			if agentIsLast {
				agentBranch = childPrefix + "└── "
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
}
