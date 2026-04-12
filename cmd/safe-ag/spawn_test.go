package main

import (
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"strings"
	"testing"
)

func TestResolveContainerName(t *testing.T) {
	tests := []struct {
		agentType string
		name      string
		timestamp string
		repos     []string
		want      string
	}{
		{"claude", "my-agent", "20260410-120000", nil, "agent-claude-my-agent"},
		{"codex", "", "20260410-120000", []string{"https://github.com/org/repo.git"}, "agent-codex-repo"},
		{"claude", "", "20260410-120000", nil, "agent-claude-20260410-120000"},
		{"shell", "debug", "20260410-120000", nil, "agent-shell-debug"},
		{"claude", "", "20260410-120000", []string{"https://github.com/org/very-long-repository-name-here.git"}, "agent-claude-very-long-repository"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := resolveContainerName(tt.agentType, tt.name, tt.timestamp, tt.repos)
			if got != tt.want {
				t.Fatalf("resolveContainerName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpawnDryRunContainsSecurityFlags(t *testing.T) {
	containerName := "agent-claude-test"
	cmd := docker.NewRunCmd(containerName, "safe-agentic:latest")
	cmd.AddEnv("AGENT_TYPE", "claude")
	cmd.AddEnv("REPOS", "https://github.com/org/repo.git")

	docker.AppendRuntimeHardening(cmd, docker.HardeningOpts{
		Network:   "agent-claude-test-net",
		Memory:    "8g",
		CPUs:      "4",
		PIDsLimit: 512,
	})
	docker.AppendCacheMounts(cmd)

	args := cmd.Build()
	joined := strings.Join(args, " ")

	required := []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--security-opt=seccomp=/etc/safe-agentic/seccomp.json",
		"--read-only",
		"--network agent-claude-test-net",
		"--memory 8g",
		"--cpus 4",
		"--pids-limit 512",
		"--ulimit nofile=65536:65536",
		"-e AGENT_TYPE=claude",
		"-e REPOS=https://github.com/org/repo.git",
		"--tmpfs /tmp:rw,noexec,nosuid,size=512m",
		"--tmpfs /var/tmp:rw,noexec,nosuid,size=256m",
		"--tmpfs /run:rw,noexec,nosuid,size=16m",
		"--tmpfs /dev/shm:rw,noexec,nosuid,size=64m",
		"--mount type=volume,dst=/workspace",
		"--mount type=volume,dst=/home/agent/.npm",
		"--mount type=volume,dst=/home/agent/.cache/pip",
		"--mount type=volume,dst=/home/agent/go",
		"safe-agentic:latest",
	}
	for _, r := range required {
		if !strings.Contains(joined, r) {
			t.Errorf("missing %q in docker command:\n%s", r, joined)
		}
	}

	forbidden := []string{
		"--privileged",
		"--cap-add",
		"--network host",
	}
	for _, f := range forbidden {
		if strings.Contains(joined, f) {
			t.Errorf("forbidden flag %q found in docker command", f)
		}
	}
}

func TestAuthDestination(t *testing.T) {
	tests := []struct {
		agentType string
		want      string
	}{
		{"claude", "/home/agent/.claude"},
		{"codex", "/home/agent/.codex"},
		{"shell", "/home/agent/.claude"},
	}
	for _, tt := range tests {
		got := authDestination(tt.agentType)
		if got != tt.want {
			t.Fatalf("authDestination(%q) = %q, want %q", tt.agentType, got, tt.want)
		}
	}
}

func TestCoalesce(t *testing.T) {
	if coalesce("a", "b") != "a" {
		t.Error("coalesce should prefer first non-empty")
	}
	if coalesce("", "b") != "b" {
		t.Error("coalesce should fall back to second")
	}
	if coalesce("", "") != "" {
		t.Error("coalesce should return empty if both empty")
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 100) != "short" {
		t.Error("truncate should pass through short strings")
	}
	got := truncate("this is a long string", 10)
	if got != "this is a ..." {
		t.Fatalf("truncate() = %q", got)
	}
}
