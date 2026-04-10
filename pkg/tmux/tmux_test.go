package tmux

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"safe-agentic/pkg/orb"
)

func TestSessionName_Default(t *testing.T) {
	t.Setenv("SAFE_AGENTIC_TMUX_SESSION_NAME", "")
	name := SessionName()
	if name != "safe-agentic" {
		t.Errorf("expected default session name %q, got %q", "safe-agentic", name)
	}
}

func TestSessionName_EnvOverride(t *testing.T) {
	t.Setenv("SAFE_AGENTIC_TMUX_SESSION_NAME", "my-session")
	name := SessionName()
	if name != "my-session" {
		t.Errorf("expected session name %q from env, got %q", "my-session", name)
	}
}

func TestHasSession_ReturnsTrueWhenDockerExecSucceeds(t *testing.T) {
	f := orb.NewFake()
	// Default fake executor returns no error — simulates success
	has, err := HasSession(context.Background(), f, "mycontainer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected HasSession to return true when docker exec succeeds")
	}
}

func TestHasSession_ReturnsFalseWhenDockerExecFails(t *testing.T) {
	f := orb.NewFake()
	f.SetError("docker exec mycontainer tmux has-session", "exit status 1")

	has, err := HasSession(context.Background(), f, "mycontainer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected HasSession to return false when docker exec fails")
	}
}

func TestHasSession_UsesCorrectArgs(t *testing.T) {
	f := orb.NewFake()
	_, _ = HasSession(context.Background(), f, "mycontainer")

	if len(f.Log) != 1 {
		t.Fatalf("expected 1 command logged, got %d", len(f.Log))
	}
	cmd := f.Log[0]
	joined := strings.Join(cmd, " ")
	expected := fmt.Sprintf("docker exec mycontainer tmux has-session -t %s", defaultSessionName)
	if joined != expected {
		t.Errorf("unexpected command: %q, want %q", joined, expected)
	}
}

func TestBuildAttachArgs_ReturnsCorrectArgs(t *testing.T) {
	args := BuildAttachArgs("mycontainer")
	expected := []string{
		"docker", "exec", "-it", "mycontainer",
		"tmux", "attach", "-t", defaultSessionName,
	}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d]: expected %q, got %q", i, expected[i], a)
		}
	}
}

func TestBuildCapturePaneArgs_IncludesSFlag(t *testing.T) {
	args := BuildCapturePaneArgs("mycontainer", 50)

	// Must include -S flag
	hasSFlag := false
	for i, a := range args {
		if a == "-S" {
			hasSFlag = true
			if i+1 >= len(args) {
				t.Fatal("-S flag has no value following it")
			}
			if args[i+1] != "-50" {
				t.Errorf("expected -S value %q, got %q", "-50", args[i+1])
			}
			break
		}
	}
	if !hasSFlag {
		t.Errorf("expected -S flag in capture-pane args, got: %v", args)
	}
}

func TestBuildCapturePaneArgs_ReturnsCorrectArgs(t *testing.T) {
	args := BuildCapturePaneArgs("mycontainer", 30)
	expected := []string{
		"docker", "exec", "mycontainer",
		"tmux", "capture-pane", "-t", defaultSessionName, "-p", "-S", "-30",
	}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d]: expected %q, got %q", i, expected[i], a)
		}
	}
}

func TestWaitForSession_SucceedsImmediately(t *testing.T) {
	f := orb.NewFake()
	// Default fake returns success, so session is immediately available
	err := WaitForSession(context.Background(), f, "mycontainer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWaitForSession_ReturnsErrorOnContextCancel(t *testing.T) {
	f := orb.NewFake()
	// Make has-session always fail so WaitForSession has to wait
	f.SetError("docker exec mycontainer tmux has-session", "exit status 1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := WaitForSession(ctx, f, "mycontainer")
	if err == nil {
		t.Fatal("expected error on cancelled context, got nil")
	}
}
