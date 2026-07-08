package docker

import (
	"context"
	"github.com/0x666c6f/berth/pkg/vmexec"
	"strings"
	"testing"
)

func TestManagedNetworkName(t *testing.T) {
	name := ManagedNetworkName("agent-claude-abc")
	if name != "agent-claude-abc-net" {
		t.Errorf("expected agent-claude-abc-net, got %s", name)
	}
}

func TestManagedBridgeName(t *testing.T) {
	name := ManagedBridgeName("agent-claude-abc")
	if !strings.HasPrefix(name, "bt") {
		t.Fatalf("managed bridge name %q should match VM egress guardrail prefix bt+", name)
	}
	if len(name) > 15 {
		t.Fatalf("managed bridge name %q exceeds Linux interface length", name)
	}
	if name != ManagedBridgeName("agent-claude-abc") {
		t.Fatalf("managed bridge name should be deterministic")
	}
}

func TestCreateManagedNetwork(t *testing.T) {
	fake := vmexec.NewFake()
	name, err := CreateManagedNetwork(context.Background(), fake, "agent-claude-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "agent-claude-abc-net" {
		t.Errorf("expected agent-claude-abc-net, got %s", name)
	}
	cmds := fake.CommandsMatching("docker network create")
	if len(cmds) == 0 {
		t.Fatal("expected docker network create command")
	}
	cmdStr := strings.Join(cmds[0], " ")
	if !strings.Contains(cmdStr, "--driver bridge") {
		t.Errorf("missing --driver bridge in: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "com.docker.network.bridge.name="+ManagedBridgeName("agent-claude-abc")) {
		t.Errorf("missing managed bridge name in: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "app=berth") {
		t.Errorf("missing app label in: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "agent-claude-abc-net") {
		t.Errorf("missing network name in: %s", cmdStr)
	}
}

func TestCreateManagedNetwork_Error(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("docker network create", "network already exists")
	_, err := CreateManagedNetwork(context.Background(), fake, "agent-claude-abc")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "create network") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRemoveManagedNetwork(t *testing.T) {
	fake := vmexec.NewFake()
	err := RemoveManagedNetwork(context.Background(), fake, "agent-claude-abc-net")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmds := fake.CommandsMatching("docker network rm")
	if len(cmds) == 0 {
		t.Fatal("expected docker network rm command")
	}
}

func TestRemoveManagedNetwork_Error(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("docker network rm", "network not found")
	err := RemoveManagedNetwork(context.Background(), fake, "agent-claude-abc-net")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "remove network") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrepareNetwork_Managed(t *testing.T) {
	fake := vmexec.NewFake()
	name, mode, err := PrepareNetwork(context.Background(), fake, "agent-claude-abc", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "agent-claude-abc-net" {
		t.Errorf("expected agent-claude-abc-net, got %s", name)
	}
	if mode != "managed" {
		t.Errorf("expected managed mode, got %s", mode)
	}
}

func TestPrepareNetwork_ManagedDryRun(t *testing.T) {
	fake := vmexec.NewFake()
	name, mode, err := PrepareNetwork(context.Background(), fake, "agent-claude-abc", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "agent-claude-abc-net" {
		t.Errorf("expected agent-claude-abc-net, got %s", name)
	}
	if mode != "managed" {
		t.Errorf("expected managed mode, got %s", mode)
	}
	// Should not have called network create in dry run
	cmds := fake.CommandsMatching("docker network create")
	if len(cmds) != 0 {
		t.Error("should not call docker network create in dry run")
	}
}

func TestPrepareNetwork_None(t *testing.T) {
	fake := vmexec.NewFake()
	name, mode, err := PrepareNetwork(context.Background(), fake, "agent-claude-abc", "none", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "none" {
		t.Errorf("expected none, got %s", name)
	}
	if mode != "none" {
		t.Errorf("expected none mode, got %s", mode)
	}
}

func TestPrepareNetwork_Custom(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker network inspect my-custom-net", "exists")
	name, mode, err := PrepareNetwork(context.Background(), fake, "agent-claude-abc", "my-custom-net", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "my-custom-net" {
		t.Errorf("expected my-custom-net, got %s", name)
	}
	if mode != "custom" {
		t.Errorf("expected custom mode, got %s", mode)
	}
}

func TestPrepareNetwork_CustomNotExists(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("docker network inspect my-missing-net", "network not found")
	_, _, err := PrepareNetwork(context.Background(), fake, "agent-claude-abc", "my-missing-net", false)
	if err == nil {
		t.Fatal("expected error for missing network")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrepareNetwork_InvalidCustom(t *testing.T) {
	fake := vmexec.NewFake()
	_, _, err := PrepareNetwork(context.Background(), fake, "agent-claude-abc", "host", false)
	if err == nil {
		t.Fatal("expected error for unsafe network mode")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAPIOnlyBridgeNameHasBtiPrefix(t *testing.T) {
	name := APIOnlyBridgeName("my-forensic-agent")
	if !strings.HasPrefix(name, "bti") {
		t.Fatalf("api-only bridge %q must start with bti", name)
	}
	if len(name) > 15 {
		t.Fatalf("bridge name %q exceeds Linux IFNAMSIZ (15)", name)
	}
	// Deterministic for a given container name.
	if name != APIOnlyBridgeName("my-forensic-agent") {
		t.Fatal("bridge name must be deterministic")
	}
	// Managed bridge for the same name must NOT collide with the bti prefix.
	if strings.HasPrefix(ManagedBridgeName("my-forensic-agent"), "bti") {
		t.Fatal("managed bridge unexpectedly starts with bti")
	}
}

func TestPrepareNetworkAPIOnly(t *testing.T) {
	fake := vmexec.NewFake()
	name, mode, err := PrepareNetwork(context.Background(), fake, "agent1", "api-only", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != "api-only" {
		t.Fatalf("mode = %q, want api-only", mode)
	}
	if name != ManagedNetworkName("agent1") {
		t.Fatalf("network name = %q, want %q", name, ManagedNetworkName("agent1"))
	}
	created := fake.CommandsMatching("docker network create")
	if len(created) != 1 {
		t.Fatalf("expected one network create, got %d", len(created))
	}
	if !strings.Contains(strings.Join(created[0], " "), "com.docker.network.bridge.name="+APIOnlyBridgeName("agent1")) {
		t.Fatalf("create did not pin bti bridge name: %s", created[0])
	}
}

func TestPrepareNetworkAPIOnlyDryRun(t *testing.T) {
	fake := vmexec.NewFake()
	name, mode, err := PrepareNetwork(context.Background(), fake, "agent1", "api-only", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != "api-only" || name != ManagedNetworkName("agent1") {
		t.Fatalf("dry-run returned name=%q mode=%q", name, mode)
	}
	if len(fake.CommandsMatching("docker network create")) != 0 {
		t.Fatal("dry-run must not create a network")
	}
}
