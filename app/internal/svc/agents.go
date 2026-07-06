package svc

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/0x666c6f/safe-agentic/app/internal/cli"
	"github.com/0x666c6f/safe-agentic/app/internal/poll"
	"github.com/0x666c6f/safe-agentic/app/internal/state"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
)

type AgentService struct {
	Runner  *cli.Runner
	Poller  *poll.Poller    // nil in unit tests
	Exec    vmexec.Executor // VM/docker reads (clone config reconstruction)
	State   *state.Service  // projects store (nil in unit tests)
	VMName  string          // for raw stdin-streaming commands
	PickDir func() (string, error)
}

func (s *AgentService) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 120*time.Second)
}

func (s *AgentService) run(args ...string) (string, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	out, err := s.Runner.Run(ctx, args...)
	if s.Poller != nil {
		s.Poller.ForceRefresh()
	}
	return string(out), err
}

func (s *AgentService) Agents() []poll.Agent {
	if s.Poller == nil {
		return nil
	}
	return s.Poller.Snapshot()
}

func (s *AgentService) Refresh() {
	if s.Poller != nil {
		s.Poller.ForceRefresh()
	}
}

func (s *AgentService) Stop(name string) error   { _, err := s.run("stop", name); return err }
func (s *AgentService) PR(name string) error     { _, err := s.run("pr", name); return err }
func (s *AgentService) Review(name string) error { _, err := s.run("review", name); return err }

func (s *AgentService) Steer(name, message string) error {
	_, err := s.run("steer", name, message)
	return err
}

func (s *AgentService) Retry(name, feedback string) error {
	args := []string{"retry", name}
	if feedback != "" {
		args = append(args, "--feedback", feedback)
	}
	_, err := s.run(args...)
	return err
}

func (s *AgentService) Output(name string) (cli.OutputInfo, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	return s.Runner.Output(ctx, name)
}

func (s *AgentService) Diff(name string) (string, error) { return s.run("diff", name) }

// Stage/Revert operate on the whole workspace: the CLI requires explicit
// paths ("." = everything).
func (s *AgentService) WorkspaceStage(name string) error {
	_, err := s.run("workspace", "stage", name, ".")
	return err
}

func (s *AgentService) WorkspaceRevert(name string) error {
	_, err := s.run("workspace", "revert", name, ".", "--yes")
	return err
}

func (s *AgentService) WorkspaceStagePath(name, path string) error {
	_, err := s.run("workspace", "stage", name, path)
	return err
}

func (s *AgentService) WorkspaceRevertPath(name, path string) error {
	_, err := s.run("workspace", "revert", name, path, "--yes")
	return err
}

func (s *AgentService) CheckpointList(name string) (string, error) {
	return s.run("checkpoint", "list", name)
}

func (s *AgentService) CheckpointCreate(name, label string) error {
	args := []string{"checkpoint", "create", name}
	if label != "" {
		args = append(args, label)
	}
	_, err := s.run(args...)
	return err
}

func (s *AgentService) CheckpointRestore(name, ref string) error {
	_, err := s.run("checkpoint", "restore", name, ref)
	return err
}

func (s *AgentService) Cost(name string) (string, error) { return s.run("cost", name) }

func (s *AgentService) CostHistory(window string) (string, error) {
	return s.run("cost", "--history", window)
}

func (s *AgentService) TemplateList() (string, error) { return s.run("template", "list") }

// Clone spawns a fresh session with the same agent type, repos and SSH mode
// as an existing container (config reconstructed from its env + labels).
func (s *AgentService) Clone(name string) (string, error) {
	if s.Exec == nil {
		return "", fmt.Errorf("clone unavailable: no VM executor")
	}
	ctx, cancel := s.ctx()
	defer cancel()
	out, err := s.Exec.Run(ctx, "docker", "inspect", "--format",
		`{{index .Config.Labels "safe-agentic.agent-type"}}|{{index .Config.Labels "safe-agentic.ssh"}}|{{range .Config.Env}}{{println .}}{{end}}`,
		name)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", name, err)
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), "|", 3)
	if len(parts) < 3 {
		return "", fmt.Errorf("unexpected inspect output for %s", name)
	}
	agentType, sshLabel := parts[0], parts[1]
	var repos []string
	for _, line := range strings.Split(parts[2], "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "REPOS="); ok && v != "" {
			repos = strings.Fields(v)
		}
	}
	args := []string{"spawn", agentType}
	for _, r := range repos {
		args = append(args, "--repo", r)
	}
	if sshLabel == "true" || sshLabel == "on" {
		args = append(args, "--ssh")
	} else {
		args = append(args, "--no-ssh")
	}
	args = append(args, "--seed-auth", "--background")
	return s.run(args...)
}

// ConfigSync pushes current host Claude settings into the container;
// restart applies them immediately (the session resumes).
func (s *AgentService) ConfigSync(name string, restart bool) (string, error) {
	args := []string{"config-sync", name}
	if restart {
		args = append(args, "--restart")
	}
	return s.run(args...)
}
func (s *AgentService) VMStart() (string, error) { return s.run("vm", "start") }

// PipelineRun runs (or --dry-run validates) a saved pipeline by name, passing
// any ${vars} the manifest declares as --var key=value.
func (s *AgentService) PipelineRun(name string, vars map[string]string, dryRun bool) (string, error) {
	args := []string{"pipeline", name}
	for k, v := range vars {
		args = append(args, "--var", k+"="+v)
	}
	if dryRun {
		args = append(args, "--dry-run")
	} else {
		args = append(args, "--background")
	}
	return s.run(args...)
}

type SpawnRequest struct {
	Agent, Name, Repo, Prompt, Template, Network, Memory, CPUs string
	MaxCost                                                    string // USD; engine kills the agent past this budget
	SSH, ReuseAuth, Worktree, DryRun, NoSeedAuth               bool
}

// nameSanitize maps user-typed names onto the engine's allowed charset
// (letters, numbers, ., _, -): whitespace/invalid runs become dashes.
var nameSanitize = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func spawnArgs(req SpawnRequest) []string {
	args := []string{"spawn", req.Agent}
	if n := strings.Trim(nameSanitize.ReplaceAllString(req.Name, "-"), "-."); n != "" {
		args = append(args, "--name", n)
	}
	if req.Repo != "" {
		args = append(args, "--repo", req.Repo)
	}
	if req.Prompt != "" {
		args = append(args, "--prompt", req.Prompt)
	}
	if req.Template != "" {
		args = append(args, "--template", req.Template)
	}
	if req.SSH {
		args = append(args, "--ssh")
	}
	if req.ReuseAuth {
		args = append(args, "--reuse-auth")
	}
	// Always seed the host's current Claude/Codex login so GUI-spawned
	// agents are logged in without an interactive login inside the
	// container (personal tool on a trusted host). --no-seed-auth opts out.
	if req.NoSeedAuth {
		args = append(args, "--no-seed-auth")
	} else {
		args = append(args, "--seed-auth")
	}
	if req.Worktree {
		args = append(args, "--worktree")
	}
	if req.Network != "" {
		args = append(args, "--network", req.Network)
	}
	if req.Memory != "" {
		args = append(args, "--memory", req.Memory)
	}
	if req.CPUs != "" {
		args = append(args, "--cpus", req.CPUs)
	}
	if req.MaxCost != "" {
		args = append(args, "--max-cost", req.MaxCost)
	}
	args = append(args, "--background")
	if req.DryRun {
		args = append(args, "--dry-run")
	}
	return args
}

func (s *AgentService) Spawn(req SpawnRequest) (string, error) {
	return s.run(spawnArgs(req)...)
}
