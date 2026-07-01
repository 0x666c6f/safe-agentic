package docker

import (
	"context"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
	"strings"
	"testing"
)

func TestAppendSSHMount_WithRelay(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -c echo $SSH_AUTH_SOCK", "/var/host-services/ssh-auth.sock")
	fake.SetResponse("test -S", "") // relay socket exists
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmdStr := strings.Join(cmd.Build(), " ")
	if !strings.Contains(cmdStr, "SSH_AUTH_SOCK="+sshSocketPath) {
		t.Errorf("missing SSH_AUTH_SOCK env in: %s", cmdStr)
	}
	// Should use the relay socket, not the original
	if !strings.Contains(cmdStr, sshRelaySocket+":"+sshSocketPath) {
		t.Errorf("should mount relay socket, got: %s", cmdStr)
	}
}

func TestAppendSSHMount_FallbackDirect(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -c echo $SSH_AUTH_SOCK", "/var/host-services/ssh-auth.sock")
	fake.SetError("test -S", "not found") // relay socket doesn't exist
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmdStr := strings.Join(cmd.Build(), " ")
	if !strings.Contains(cmdStr, "SSH_AUTH_SOCK="+sshSocketPath) {
		t.Errorf("missing SSH_AUTH_SOCK env in: %s", cmdStr)
	}
	// Fallback: direct mount with :ro
	if !strings.Contains(cmdStr, "/var/host-services/ssh-auth.sock:"+sshSocketPath+":ro") {
		t.Errorf("should fallback to direct mount, got: %s", cmdStr)
	}
}

func TestAppendSSHMount_StartsRelayWithoutStartStopDaemon(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -c echo $SSH_AUTH_SOCK", "/var/host-services/ssh-auth.sock")
	fake.SetError("test -S", "not found") // relay socket doesn't exist
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	setupCommands := fake.CommandsMatching("bash -lc set -e; install -d")
	if len(setupCommands) != 1 {
		t.Fatalf("expected one relay setup command, got %d: %v", len(setupCommands), setupCommands)
	}
	setup := strings.Join(setupCommands[0], " ")
	if strings.Contains(setup, "start-stop-daemon") {
		t.Fatalf("relay setup should not require start-stop-daemon: %s", setup)
	}
	if !strings.Contains(setup, "nohup") || !strings.Contains(setup, "socat") {
		t.Fatalf("relay setup should launch socat in background: %s", setup)
	}
}

func TestAppendSSHMount_RestartsStaleRelay(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -c echo $SSH_AUTH_SOCK", "/var/host-services/ssh-auth.sock")
	fake.SetError("bash -lc SSH_AUTH_SOCK='/tmp/safe-agentic-ssh-relay/agent.sock' ssh-add -l", "The agent has no identities.")
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := fake.CommandsMatching("rm -f '/tmp/safe-agentic-ssh-relay/agent.sock' '/tmp/safe-agentic-ssh-relay/relay.pid'"); len(got) == 0 {
		t.Fatalf("expected stale relay cleanup, log: %v", fake.Log)
	}
	if got := fake.CommandsMatching("nohup"); len(got) == 0 {
		t.Fatalf("expected relay restart, log: %v", fake.Log)
	}
}

func TestAppendSSHMount_EmptyVMAgent(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -c echo $SSH_AUTH_SOCK", "/var/host-services/ssh-auth.sock")
	fake.SetError("bash -lc SSH_AUTH_SOCK='/var/host-services/ssh-auth.sock' ssh-add -l", "The agent has no identities.")
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err == nil {
		t.Fatal("expected error for empty VM SSH agent")
	}
	if !strings.Contains(err.Error(), "VM SSH agent has no identities") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppendSSHMount_EmptySocket(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -c echo $SSH_AUTH_SOCK", "")
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err == nil {
		t.Fatal("expected error when SSH_AUTH_SOCK is empty")
	}
	if !strings.Contains(err.Error(), "SSH_AUTH_SOCK not set") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAppendSSHMount_ExecError(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("bash -c echo $SSH_AUTH_SOCK", "permission denied")
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err == nil {
		t.Fatal("expected error when exec fails")
	}
	if !strings.Contains(err.Error(), "find SSH socket") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAppendSSHMount_UnsafeSocketValue(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -c echo $SSH_AUTH_SOCK", "/tmp/unsafe'sock")
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err == nil {
		t.Fatal("expected error for unsafe SSH_AUTH_SOCK")
	}
	if !strings.Contains(err.Error(), "unsafe value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppendSSHMountDryRun(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	AppendSSHMountDryRun(cmd)
	cmdStr := strings.Join(cmd.Build(), " ")
	if !strings.Contains(cmdStr, "SSH_AUTH_SOCK="+sshSocketPath) {
		t.Errorf("missing SSH_AUTH_SOCK env in: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "<SSH_SOCKET>:"+sshSocketPath) {
		t.Errorf("missing dry run socket placeholder in: %s", cmdStr)
	}
}

func TestEnsureSSHForRepos_SSHEnabled(t *testing.T) {
	repos := []string{"git@github.com:org/repo.git", "ssh://github.com/org/repo"}
	err := EnsureSSHForRepos(true, repos)
	if err != nil {
		t.Fatalf("unexpected error with SSH enabled: %v", err)
	}
}

func TestEnsureSSHForRepos_NoSSHNeeded(t *testing.T) {
	repos := []string{"https://github.com/org/repo.git"}
	err := EnsureSSHForRepos(false, repos)
	if err != nil {
		t.Fatalf("unexpected error for HTTPS repo without SSH: %v", err)
	}
}

func TestEnsureSSHForRepos_SSHRepoWithoutSSH(t *testing.T) {
	repos := []string{"https://github.com/org/repo.git", "git@github.com:org/repo.git"}
	err := EnsureSSHForRepos(false, repos)
	if err == nil {
		t.Fatal("expected error when SSH repo is given but SSH is disabled")
	}
	if !strings.Contains(err.Error(), "requires SSH") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEnsureSSHForRepos_Empty(t *testing.T) {
	err := EnsureSSHForRepos(false, []string{})
	if err != nil {
		t.Fatalf("unexpected error for empty repos: %v", err)
	}
}
