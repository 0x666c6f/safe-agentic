package docker

import (
	"context"
	"safe-agentic/pkg/orb"
	"strings"
	"testing"
)

func TestAppendSSHMount_Success(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("bash -c echo $SSH_AUTH_SOCK", "/run/ssh-relay.sock")
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmdStr := strings.Join(cmd.Build(), " ")
	if !strings.Contains(cmdStr, "SSH_AUTH_SOCK="+sshSocketPath) {
		t.Errorf("missing SSH_AUTH_SOCK env in: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "/run/ssh-relay.sock:"+sshSocketPath) {
		t.Errorf("missing socket volume mount in: %s", cmdStr)
	}
}

func TestAppendSSHMount_EmptySocket(t *testing.T) {
	fake := orb.NewFake()
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
	fake := orb.NewFake()
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
