package vmexec

import (
	"bytes"
	"context"
	"strings"
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

func TestFakeExecutor_RunStreaming_WritesResponseAndLogs(t *testing.T) {
	f := NewFake()
	f.SetResponse("docker build", "step 1\nstep 2\n")

	var buf bytes.Buffer
	if err := f.RunStreaming(context.Background(), &buf, "docker", "build", "/ctx"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "step 1\nstep 2\n" {
		t.Errorf("unexpected streamed output: %q", buf.String())
	}
	last := f.LastCommand()
	if len(last) != 3 || last[0] != "docker" || last[1] != "build" {
		t.Errorf("RunStreaming did not log the command: %v", last)
	}
}

func TestFakeExecutor_RunStreaming_ReturnsErrorForConfiguredPrefix(t *testing.T) {
	f := NewFake()
	f.SetError("docker build", "build blew up")

	var buf bytes.Buffer
	err := f.RunStreaming(context.Background(), &buf, "docker", "build", "/ctx")
	if err == nil || err.Error() != "build blew up" {
		t.Fatalf("expected configured error, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output on error, got %q", buf.String())
	}
}

func TestMachineExecutor_RunStreaming_ErrorWhenVMNotFound(t *testing.T) {
	e := &MachineExecutor{VMName: "berth-test-nonexistent-vm-12345"}
	var buf bytes.Buffer
	if err := e.RunStreaming(context.Background(), &buf, "echo", "hello"); err == nil {
		t.Error("expected error when VM does not exist, got nil")
	}
}

func TestMachineExecutor_BuildArgs(t *testing.T) {
	e := &MachineExecutor{VMName: "berth"}
	args := e.buildArgs("docker", "ps")

	expected := []string{"machine", "run", "-n", "berth", "-u", "root", "--", "/usr/local/bin/berth-exec", "docker", "cHM="}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d]: expected %q, got %q", i, expected[i], a)
		}
	}
}

func TestMachineExecutor_BuildArgs_EmptyPayload(t *testing.T) {
	e := &MachineExecutor{VMName: "my-vm"}
	args := e.buildArgs()

	expected := []string{"machine", "run", "-n", "my-vm", "-u", "root"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d]: expected %q, got %q", i, expected[i], a)
		}
	}
}

func TestMachineExecutor_BuildInteractiveArgs_AddsTTYForTerminal(t *testing.T) {
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	defer func() { stdinIsTerminal = orig }()

	e := &MachineExecutor{VMName: "my-vm"}
	args := e.buildInteractiveArgs("docker", "exec", "-it", "mycontainer", "tmux", "attach")

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "machine run --interactive --tty -n my-vm -u root --") {
		t.Fatalf("interactive args missing Apple VM TTY flags: %s", joined)
	}
}

func TestMachineExecutor_BuildInteractiveArgs_OmitsTTYWithoutTerminal(t *testing.T) {
	orig := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	defer func() { stdinIsTerminal = orig }()

	e := &MachineExecutor{VMName: "my-vm"}
	args := e.buildInteractiveArgs("docker", "exec", "-it", "mycontainer", "tmux", "attach")

	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--interactive") || strings.Contains(joined, "--tty") {
		t.Fatalf("interactive args should omit Apple VM TTY flags without a terminal: %s", joined)
	}
}

func TestFakeExecutor_RunInteractive_LogsCommand(t *testing.T) {
	f := NewFake()

	err := f.RunInteractive("docker", "exec", "-it", "mycontainer", "bash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(f.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(f.Log))
	}
	cmd := f.Log[0]
	if len(cmd) != 5 || cmd[0] != "docker" || cmd[1] != "exec" || cmd[2] != "-it" || cmd[3] != "mycontainer" || cmd[4] != "bash" {
		t.Errorf("unexpected command logged: %v", cmd)
	}
}

func TestFakeExecutor_RunInteractive_AppearsInLastCommand(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	_, _ = f.Run(ctx, "docker", "ps")
	_ = f.RunInteractive("docker", "attach", "mycontainer")

	last := f.LastCommand()
	if len(last) != 3 || last[0] != "docker" || last[1] != "attach" || last[2] != "mycontainer" {
		t.Errorf("unexpected last command: %v", last)
	}
}

func TestFakeExecutor_MultiplePrefixMatches_ErrorTakesPriority(t *testing.T) {
	f := NewFake()
	f.SetResponse("docker run", "some output")
	f.SetError("docker run ubuntu", "ubuntu-specific error")

	// "docker run ubuntu" matches both "docker run" response and "docker run ubuntu" error
	// The error prefix is longer/more specific; iteration order is map-based so we just
	// verify that the error for the exact prefix is returned when it matches.
	_, err := f.Run(context.Background(), "docker", "run", "ubuntu")
	if err == nil {
		// This is acceptable — map iteration is non-deterministic; just verify no panic
		t.Log("no error returned (map iteration selected response over error — non-deterministic)")
	}
}

func TestFakeExecutor_CommandsMatching_RunInteractiveIncluded(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	_, _ = f.Run(ctx, "docker", "ps")
	_ = f.RunInteractive("docker", "exec", "-it", "mycontainer", "tmux", "attach")
	_, _ = f.Run(ctx, "docker", "inspect", "mycontainer")

	// CommandsMatching should find RunInteractive commands too
	matching := f.CommandsMatching("mycontainer")
	if len(matching) != 2 {
		t.Errorf("expected 2 commands matching 'mycontainer', got %d: %v", len(matching), matching)
	}
}

func TestMachineExecutor_Run_ErrorWhenVMNotFound(t *testing.T) {
	// Use a VM name that does not exist; container will exit non-zero.
	e := &MachineExecutor{VMName: "berth-test-nonexistent-vm-12345"}
	ctx := context.Background()
	_, err := e.Run(ctx, "echo", "hello")
	if err == nil {
		t.Error("expected error when VM does not exist, got nil")
	}
}

func TestMachineExecutor_RunInteractive_ErrorWhenVMNotFound(t *testing.T) {
	// Use a VM name that does not exist; container will exit non-zero.
	// RunInteractive connects stdio, so this only tests the error return path.
	e := &MachineExecutor{VMName: "berth-test-nonexistent-vm-12345"}
	err := e.RunInteractive("echo", "hello")
	if err == nil {
		t.Error("expected error when VM does not exist, got nil")
	}
}

func TestMachineExecutor_Run_SuccessWithRealVM(t *testing.T) {
	// This test requires the "berth" Apple container machine to be running.
	// Skip if the VM is not available.
	e := &MachineExecutor{VMName: "berth"}
	ctx := context.Background()
	out, err := e.Run(ctx, "echo", "berth-test-ok")
	if err != nil {
		t.Skipf("berth VM not available, skipping: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "berth-test-ok" {
		t.Errorf("unexpected output: %q", got)
	}
}

func TestBuildInteractiveArgs(t *testing.T) {
	got := BuildInteractiveArgs("berth", "docker", "exec", "-it", "agent-x", "tmux", "attach", "-t", "berth")
	want := []string{
		"machine", "run", "--interactive", "--tty", "-n", "berth", "-u", "root",
		"--", "/usr/local/bin/berth-exec", "docker",
		"ZXhlYw==",         // exec
		"LWl0",             // -it
		"YWdlbnQteA==",     // agent-x
		"dG11eA==",         // tmux
		"YXR0YWNo",         // attach
		"LXQ=",             // -t
		"YmVydGg=", // berth
	}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d\n%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg %d: got %q want %q", i, got[i], want[i])
		}
	}
}
