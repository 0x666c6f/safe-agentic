package docker

import (
	"context"
	"safe-agentic/pkg/orb"
	"strings"
	"testing"
)

func TestContainerExists_Found(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker inspect mycontainer", "some output")
	found, err := ContainerExists(context.Background(), fake, "mycontainer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected container to be found")
	}
}

func TestContainerExists_NotFound(t *testing.T) {
	fake := orb.NewFake()
	fake.SetError("docker inspect nocontainer", "no such container")
	found, err := ContainerExists(context.Background(), fake, "nocontainer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected container to not be found")
	}
}

func TestResolveLatest_Found(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}} --latest", "agent-claude-abc")
	name, err := ResolveLatest(context.Background(), fake)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "agent-claude-abc" {
		t.Errorf("expected agent-claude-abc, got %s", name)
	}
}

func TestResolveLatest_Empty(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}} --latest", "")
	_, err := ResolveLatest(context.Background(), fake)
	if err == nil {
		t.Fatal("expected error for empty output")
	}
	if !strings.Contains(err.Error(), "no safe-agentic containers found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveTarget_ExactMatch(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-abc\nagent-claude-xyz")
	name, err := ResolveTarget(context.Background(), fake, "agent-claude-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "agent-claude-abc" {
		t.Errorf("expected agent-claude-abc, got %s", name)
	}
}

func TestResolveTarget_PartialMatch(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-abc\nagent-claude-xyz")
	name, err := ResolveTarget(context.Background(), fake, "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "agent-claude-abc" {
		t.Errorf("expected agent-claude-abc, got %s", name)
	}
}

func TestResolveTarget_Latest(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}} --latest", "agent-claude-latest")
	name, err := ResolveTarget(context.Background(), fake, "--latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "agent-claude-latest" {
		t.Errorf("expected agent-claude-latest, got %s", name)
	}
}

func TestResolveTarget_EmptyNameUsesLatest(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}} --latest", "agent-claude-latest")
	name, err := ResolveTarget(context.Background(), fake, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "agent-claude-latest" {
		t.Errorf("expected agent-claude-latest, got %s", name)
	}
}

func TestResolveTarget_NotFound(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-abc")
	_, err := ResolveTarget(context.Background(), fake, "notexist")
	if err == nil {
		t.Fatal("expected error when container not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInspectLabel(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker inspect --format", "claude")
	val, err := InspectLabel(context.Background(), fake, "mycontainer", "safe-agentic.agent-type")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "claude" {
		t.Errorf("expected claude, got %s", val)
	}
}

func TestIsRunning_True(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker inspect --format {{.State.Running}}", "true")
	running, err := IsRunning(context.Background(), fake, "mycontainer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !running {
		t.Error("expected container to be running")
	}
}

func TestIsRunning_False(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker inspect --format {{.State.Running}}", "false")
	running, err := IsRunning(context.Background(), fake, "mycontainer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if running {
		t.Error("expected container to not be running")
	}
}

func TestInspectLabel_Error(t *testing.T) {
	fake := orb.NewFake()
	fake.SetError("docker inspect", "not found")
	_, err := InspectLabel(context.Background(), fake, "missing", "some-label")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsRunning_Error(t *testing.T) {
	fake := orb.NewFake()
	fake.SetError("docker inspect --format {{.State.Running}}", "not found")
	running, err := IsRunning(context.Background(), fake, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if running {
		t.Fatal("should not be running on error")
	}
}
