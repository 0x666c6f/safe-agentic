package docker

import (
	"context"
	"safe-agentic/pkg/orb"
	"strings"
	"testing"
)

func TestDinDContainerName(t *testing.T) {
	name := DinDContainerName("agent-claude-abc")
	if name != "safe-agentic-docker-agent-claude-abc" {
		t.Errorf("expected safe-agentic-docker-agent-claude-abc, got %s", name)
	}
}

func TestDinDSocketVolume(t *testing.T) {
	name := DinDSocketVolume("agent-claude-abc")
	if name != "agent-claude-abc-docker-sock" {
		t.Errorf("expected agent-claude-abc-docker-sock, got %s", name)
	}
}

func TestDinDDataVolume(t *testing.T) {
	name := DinDDataVolume("agent-claude-abc")
	if name != "agent-claude-abc-docker-data" {
		t.Errorf("expected agent-claude-abc-docker-data, got %s", name)
	}
}

func TestAppendDinDAccess(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	AppendDinDAccess(cmd, "agent-claude-abc")
	cmdStr := strings.Join(cmd.Build(), " ")
	if !strings.Contains(cmdStr, "DOCKER_HOST=unix://"+dockerInternalSocketPath) {
		t.Errorf("missing DOCKER_HOST env in: %s", cmdStr)
	}
	socketVol := DinDSocketVolume("agent-claude-abc")
	if !strings.Contains(cmdStr, "src="+socketVol) {
		t.Errorf("missing socket volume mount in: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "dst="+dockerInternalSocketDir) {
		t.Errorf("missing socket dir in: %s", cmdStr)
	}
}

func TestAppendHostDockerSocket(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("bash -c stat -c %g /var/run/docker.sock", "999")
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendHostDockerSocket(context.Background(), fake, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmdStr := strings.Join(cmd.Build(), " ")
	if !strings.Contains(cmdStr, "--group-add 999") {
		t.Errorf("missing --group-add: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "/var/run/docker.sock:/run/docker-host.sock") {
		t.Errorf("missing socket volume: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "DOCKER_HOST=unix:///run/docker-host.sock") {
		t.Errorf("missing DOCKER_HOST env: %s", cmdStr)
	}
}

func TestAppendHostDockerSocket_Error(t *testing.T) {
	fake := orb.NewFake()
	fake.SetError("bash -c stat -c %g /var/run/docker.sock", "stat failed")
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendHostDockerSocket(context.Background(), fake, cmd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "docker socket GID") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStartDinDRuntime(t *testing.T) {
	fake := orb.NewFake()
	// Simulate docker exec succeeding on first try (waitForDinD)
	fake.SetResponse("docker exec safe-agentic-docker-agent-claude-abc docker info", "ok")
	err := StartDinDRuntime(context.Background(), fake, "agent-claude-abc", "agent-claude-abc-net", "docker:dind")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify docker run was called with --privileged
	cmds := fake.CommandsMatching("docker run -d")
	if len(cmds) == 0 {
		t.Fatal("expected docker run command for DinD")
	}
	cmdStr := strings.Join(cmds[0], " ")
	if !strings.Contains(cmdStr, "--privileged") {
		t.Errorf("missing --privileged in DinD run: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "safe-agentic-docker-agent-claude-abc") {
		t.Errorf("missing DinD container name: %s", cmdStr)
	}
}

func TestRemoveDinDRuntime(t *testing.T) {
	fake := orb.NewFake()
	err := RemoveDinDRuntime(context.Background(), fake, "agent-claude-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify docker rm was called
	rmCmds := fake.CommandsMatching("docker rm -f")
	if len(rmCmds) == 0 {
		t.Fatal("expected docker rm command")
	}
	dindName := DinDContainerName("agent-claude-abc")
	if !strings.Contains(strings.Join(rmCmds[0], " "), dindName) {
		t.Errorf("expected docker rm to target DinD container name %s, got: %v", dindName, rmCmds[0])
	}
}

func TestRemoveDinDRuntime_VolumesRemoved(t *testing.T) {
	fake := orb.NewFake()
	err := RemoveDinDRuntime(context.Background(), fake, "agent-claude-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	volCmds := fake.CommandsMatching("docker volume rm")
	if len(volCmds) < 2 {
		t.Fatalf("expected at least 2 docker volume rm calls, got %d", len(volCmds))
	}
}

func TestCleanupAllDinD(t *testing.T) {
	fake := orb.NewFake()
	err := CleanupAllDinD(context.Background(), fake)
	if err != nil {
		t.Fatalf("CleanupAllDinD() error = %v", err)
	}
	// Should have run docker rm and docker volume rm
	rmCmds := fake.CommandsMatching("docker rm")
	if len(rmCmds) == 0 {
		t.Fatal("expected at least one docker rm command")
	}
	volCmds := fake.CommandsMatching("docker volume rm")
	if len(volCmds) == 0 {
		t.Fatal("expected at least one docker volume rm command")
	}
}

func TestWaitForDinD_ContextCancel(t *testing.T) {
	fake := orb.NewFake()
	fake.SetError("docker exec", "not ready")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := waitForDinD(ctx, fake, "test-dind")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
