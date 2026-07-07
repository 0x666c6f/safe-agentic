package docker

import (
	"context"
	"github.com/0x666c6f/berth/pkg/vmexec"
	"strings"
	"testing"
)

func TestContainerExists_Found(t *testing.T) {
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}} --latest", "")
	_, err := ResolveLatest(context.Background(), fake)
	if err == nil {
		t.Fatal("expected error for empty output")
	}
	if !strings.Contains(err.Error(), "no berth containers found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveTarget_ExactMatch(t *testing.T) {
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-abc\nagent-claude-xyz")
	name, err := ResolveTarget(context.Background(), fake, "abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "agent-claude-abc" {
		t.Errorf("expected agent-claude-abc, got %s", name)
	}
}

func TestResolveTarget_AmbiguousPartialMatch(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker ps -a --filter name=^agent- --format {{.Names}}", "agent-claude-abc\nagent-codex-abc")
	_, err := ResolveTarget(context.Background(), fake, "abc")
	if err == nil {
		t.Fatal("expected ambiguous partial error")
	}
	if !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "agent-claude-abc") || !strings.Contains(err.Error(), "agent-codex-abc") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveTarget_Latest(t *testing.T) {
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
	fake.SetResponse("docker inspect --format", "claude")
	val, err := InspectLabel(context.Background(), fake, "mycontainer", "berth.agent-type")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "claude" {
		t.Errorf("expected claude, got %s", val)
	}
}

func TestIsRunning_True(t *testing.T) {
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
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
	fake := vmexec.NewFake()
	fake.SetError("docker inspect", "not found")
	_, err := InspectLabel(context.Background(), fake, "missing", "some-label")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsRunning_Error(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("docker inspect --format {{.State.Running}}", "not found")
	running, err := IsRunning(context.Background(), fake, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if running {
		t.Fatal("should not be running on error")
	}
}

func TestExitCode_Parsed(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker inspect --format {{.State.ExitCode}}", "128\n")
	code, err := ExitCode(context.Background(), fake, "mycontainer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 128 {
		t.Errorf("expected exit code 128, got %d", code)
	}
}

func TestExitCode_NonNumeric(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker inspect --format {{.State.ExitCode}}", "oops")
	if _, err := ExitCode(context.Background(), fake, "mycontainer"); err == nil {
		t.Fatal("expected error on non-numeric exit code")
	}
}

func TestTailLogs_ReturnsTrimmedOutput(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -lc docker logs", "ssh: connect to host github.com port 22: Connection timed out\n")
	logs, err := TailLogs(context.Background(), fake, "mycontainer", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logs != "ssh: connect to host github.com port 22: Connection timed out" {
		t.Errorf("unexpected logs: %q", logs)
	}
	// Container name must be passed as a positional arg, not interpolated.
	cmds := fake.CommandsMatching("docker logs")
	if len(cmds) != 1 {
		t.Fatalf("expected one docker logs call, got %d", len(cmds))
	}
	last := cmds[0]
	if last[len(last)-1] != "mycontainer" {
		t.Errorf("expected container name as trailing positional arg, got %v", last)
	}
}
