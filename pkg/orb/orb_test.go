package orb

import (
	"context"
	"testing"
)

func TestFakeExecutor_CapturesCommandsInLog(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	_, _ = f.Run(ctx, "docker", "ps")
	_, _ = f.Run(ctx, "docker", "run", "ubuntu")
	_ = f.RunInteractive("docker", "exec", "-it", "mycontainer", "bash")

	if len(f.Log) != 3 {
		t.Fatalf("expected 3 log entries, got %d", len(f.Log))
	}
	if f.Log[0][0] != "docker" || f.Log[0][1] != "ps" {
		t.Errorf("unexpected first command: %v", f.Log[0])
	}
	if f.Log[1][0] != "docker" || f.Log[1][1] != "run" {
		t.Errorf("unexpected second command: %v", f.Log[1])
	}
	if f.Log[2][0] != "docker" || f.Log[2][2] != "-it" {
		t.Errorf("unexpected third command: %v", f.Log[2])
	}
}

func TestFakeExecutor_ReturnsConfiguredResponseByPrefix(t *testing.T) {
	f := NewFake()
	f.SetResponse("docker ps", "container1\ncontainer2")

	out, err := f.Run(context.Background(), "docker", "ps")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "container1\ncontainer2" {
		t.Errorf("unexpected output: %q", string(out))
	}
}

func TestFakeExecutor_ReturnsEmptyByDefault(t *testing.T) {
	f := NewFake()

	out, err := f.Run(context.Background(), "docker", "inspect", "somecontainer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "" {
		t.Errorf("expected empty output, got %q", string(out))
	}
}

func TestFakeExecutor_ReturnsErrorForConfiguredPrefix(t *testing.T) {
	f := NewFake()
	f.SetError("docker run", "container start failed")

	_, err := f.Run(context.Background(), "docker", "run", "ubuntu")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "container start failed" {
		t.Errorf("unexpected error message: %q", err.Error())
	}
}

func TestFakeExecutor_LastCommand(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	if f.LastCommand() != nil {
		t.Error("expected nil LastCommand on fresh fake")
	}

	_, _ = f.Run(ctx, "docker", "ps")
	_, _ = f.Run(ctx, "docker", "images")

	last := f.LastCommand()
	if len(last) != 2 || last[0] != "docker" || last[1] != "images" {
		t.Errorf("unexpected last command: %v", last)
	}
}

func TestFakeExecutor_CommandsMatching(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	_, _ = f.Run(ctx, "docker", "ps")
	_, _ = f.Run(ctx, "docker", "run", "ubuntu")
	_, _ = f.Run(ctx, "docker", "run", "alpine")
	_, _ = f.Run(ctx, "docker", "inspect", "abc")

	matching := f.CommandsMatching("docker run")
	if len(matching) != 2 {
		t.Errorf("expected 2 matching commands, got %d", len(matching))
	}
}

func TestFakeExecutor_Reset(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	f.SetResponse("docker ps", "output")
	f.SetError("docker run", "fail")
	_, _ = f.Run(ctx, "docker", "ps")
	_, _ = f.Run(ctx, "docker", "run", "ubuntu")

	f.Reset()

	if len(f.Log) != 0 {
		t.Errorf("expected empty log after reset, got %d entries", len(f.Log))
	}
	// responses and errors should be cleared — default empty output, no error
	out, err := f.Run(ctx, "docker", "ps")
	if err != nil {
		t.Fatalf("unexpected error after reset: %v", err)
	}
	if string(out) != "" {
		t.Errorf("expected empty output after reset, got %q", string(out))
	}
}

func TestOrbExecutor_BuildArgs(t *testing.T) {
	e := &OrbExecutor{VMName: "safe-agentic"}
	args := e.buildArgs("docker", "ps")

	expected := []string{"run", "-m", "safe-agentic", "docker", "ps"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d]: expected %q, got %q", i, expected[i], a)
		}
	}
}

func TestOrbExecutor_BuildArgs_EmptyPayload(t *testing.T) {
	e := &OrbExecutor{VMName: "my-vm"}
	args := e.buildArgs()

	expected := []string{"run", "-m", "my-vm"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d]: expected %q, got %q", i, expected[i], a)
		}
	}
}
