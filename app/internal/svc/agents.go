package svc

import (
	"context"
	"time"

	"github.com/0x666c6f/safe-agentic/app/internal/cli"
	"github.com/0x666c6f/safe-agentic/app/internal/poll"
)

type AgentService struct {
	Runner *cli.Runner
	Poller *poll.Poller // nil in unit tests
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
func (s *AgentService) VMStart() (string, error)      { return s.run("vm", "start") }

func (s *AgentService) PipelineRun(file string) (string, error) {
	return s.run("pipeline", file, "--background")
}

type SpawnRequest struct {
	Agent, Name, Repo, Prompt, Template, Network, Memory, CPUs string
	SSH, ReuseAuth, Worktree, DryRun                           bool
}

func spawnArgs(req SpawnRequest) []string {
	args := []string{"spawn", req.Agent}
	if req.Name != "" {
		args = append(args, "--name", req.Name)
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
	args = append(args, "--background")
	if req.DryRun {
		args = append(args, "--dry-run")
	}
	return args
}

func (s *AgentService) Spawn(req SpawnRequest) (string, error) {
	return s.run(spawnArgs(req)...)
}
