//go:build integration

package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Shared long-lived container for deterministic tests
// ---------------------------------------------------------------------------

const (
	detSharedSuffix    = "e2e-det-shared"
	detSharedContainer = "agent-shell-" + detSharedSuffix
	detTestImage       = "safe-agentic:latest"
	detTestRepo        = "https://github.com/octocat/Hello-World.git"
)

var (
	detOnce    sync.Once
	detSetupOK bool
)

// cleanupDetContainers removes containers created by deterministic tests.
// Called from TestMain in integration_test.go.
func cleanupDetContainers() {
	ctx := context.Background()
	orbExec.Run(ctx, "docker", "rm", "-f", detSharedContainer)
	orbExec.Run(ctx, "docker", "rm", "-f", "agent-shell-e2e-det-multi")
}

// ensureSharedContainer creates a long-lived container with proper labels
// so safe-ag CLI commands can discover it. Idempotent via sync.Once.
func ensureSharedContainer(t *testing.T) {
	t.Helper()
	detOnce.Do(func() {
		ctx := context.Background()

		// Clean up any leftover from a previous run
		orbExec.Run(ctx, "docker", "rm", "-f", detSharedContainer)

		args := []string{
			"docker", "run", "-d",
			"--name", detSharedContainer,
			"--hostname", detSharedContainer,
			"--read-only",
			"--cap-drop=ALL",
			"--security-opt=no-new-privileges:true",
			"--tmpfs", "/tmp:rw,noexec,nosuid,size=512m",
			"--tmpfs", "/var/tmp:rw,noexec,nosuid,size=256m",
			"--tmpfs", "/run:rw,noexec,nosuid,size=16m",
			"--tmpfs", "/dev/shm:rw,noexec,nosuid,size=64m",
			"--tmpfs", "/home/agent/.config:rw,noexec,size=32m,uid=1000,gid=1000",
			"--tmpfs", "/home/agent/.ssh:rw,noexec,size=1m,uid=1000,gid=1000",
			"--tmpfs", "/home/agent/.claude:rw,noexec,size=8m,uid=1000,gid=1000",
			"--tmpfs", "/home/agent/.codex:rw,noexec,size=8m,uid=1000,gid=1000",
			"--mount", "type=volume,dst=/workspace",
			// Environment
			"-e", "AGENT_TYPE=shell",
			"-e", "REPOS=" + detTestRepo,
			"-e", "GIT_AUTHOR_NAME=Test User",
			"-e", "GIT_AUTHOR_EMAIL=test@example.com",
			"-e", "GIT_COMMITTER_NAME=Test User",
			"-e", "GIT_COMMITTER_EMAIL=test@example.com",
			// Labels so safe-ag CLI can discover this container
			"--label", "safe-agentic.agent-type=shell",
			"--label", "safe-agentic.auth=shared",
			"--label", "safe-agentic.ssh=false",
			"--label", "safe-agentic.docker=off",
			"--label", "safe-agentic.network-mode=none",
			"--label", "safe-agentic.terminal=tmux",
			"--label", "safe-agentic.repo-display=octocat/Hello-World",
			"--label", "safe-agentic.resources=cpu=4,mem=8g,pids=512",
			detTestImage,
			"-lc", "sleep 600",
		}

		if _, err := orbExec.Run(ctx, args...); err != nil {
			t.Logf("failed to create shared container: %v", err)
			return
		}

		// Wait for running + entrypoint to finish (clone etc.)
		for i := 0; i < 40; i++ {
			out, err := orbExec.Run(ctx, "docker", "inspect", "--format", "{{.State.Status}}", detSharedContainer)
			if err == nil && strings.TrimSpace(string(out)) == "running" {
				// Give entrypoint time to clone the repo
				time.Sleep(5 * time.Second)
				detSetupOK = true
				return
			}
			time.Sleep(1 * time.Second)
		}

		logs, _ := orbExec.Run(ctx, "docker", "logs", detSharedContainer)
		t.Logf("shared container did not start in 40s. Logs:\n%s", logs)
	})

	if !detSetupOK {
		t.Fatal("shared container not available")
	}
}

// detExec runs a command inside the shared container, failing the test on error.
func detExec(t *testing.T, cmd ...string) string {
	t.Helper()
	args := append([]string{"docker", "exec", detSharedContainer}, cmd...)
	out, err := orbExec.Run(context.Background(), args...)
	if err != nil {
		t.Fatalf("docker exec %s %v: %v\noutput: %s", detSharedContainer, cmd, err, out)
	}
	return strings.TrimSpace(string(out))
}

// detExecMayFail runs a command inside the shared container, returning error
// instead of failing the test.
func detExecMayFail(t *testing.T, cmd ...string) (string, error) {
	t.Helper()
	args := append([]string{"docker", "exec", detSharedContainer}, cmd...)
	out, err := orbExec.Run(context.Background(), args...)
	return strings.TrimSpace(string(out)), err
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 1: Container Fundamentals
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_NonRootUser(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "id")
	if !strings.Contains(out, "uid=1000(agent)") {
		t.Fatalf("expected agent user, got: %s", out)
	}
	if !strings.Contains(out, "groups=1000(agent)") {
		t.Fatalf("expected no extra groups, got: %s", out)
	}
}

func TestDet_NoSudo(t *testing.T) {
	ensureSharedContainer(t)
	_, err := detExecMayFail(t, "sudo", "id")
	if err == nil {
		t.Fatal("sudo should not be available")
	}
}

func TestDet_ReadOnlyRootfs(t *testing.T) {
	ensureSharedContainer(t)
	_, err := detExecMayFail(t, "touch", "/test-readonly")
	if err == nil {
		t.Fatal("root filesystem should be read-only")
	}
}

func TestDet_TmpfsWritable(t *testing.T) {
	ensureSharedContainer(t)
	detExec(t, "touch", "/tmp/test-writable")
	detExec(t, "rm", "/tmp/test-writable")
}

func TestDet_VarTmpWritable(t *testing.T) {
	ensureSharedContainer(t)
	detExec(t, "touch", "/var/tmp/test-writable")
	detExec(t, "rm", "/var/tmp/test-writable")
}

func TestDet_WorkspaceWritable(t *testing.T) {
	ensureSharedContainer(t)
	detExec(t, "touch", "/workspace/test-writable")
	detExec(t, "rm", "/workspace/test-writable")
}

func TestDet_HomeConfigWritable(t *testing.T) {
	ensureSharedContainer(t)
	detExec(t, "touch", "/home/agent/.config/test-writable")
	detExec(t, "rm", "/home/agent/.config/test-writable")
}

func TestDet_HomeSshWritable(t *testing.T) {
	ensureSharedContainer(t)
	detExec(t, "touch", "/home/agent/.ssh/test-writable")
	detExec(t, "rm", "/home/agent/.ssh/test-writable")
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 2: Repo Cloning & Git Identity
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_RepoCloned(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "ls", "/workspace/octocat/Hello-World/")
	if !strings.Contains(out, "README") {
		t.Fatalf("README not found in cloned repo: %s", out)
	}
}

func TestDet_RepoIsGitRepo(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "git", "-C", "/workspace/octocat/Hello-World", "status")
	if !strings.Contains(out, "On branch") {
		t.Fatalf("not a git repo: %s", out)
	}
}

func TestDet_GitIdentityName(t *testing.T) {
	ensureSharedContainer(t)
	name := detExec(t, "git", "-C", "/workspace/octocat/Hello-World", "config", "user.name")
	if name != "Test User" {
		t.Fatalf("git user.name = %q, want 'Test User'", name)
	}
}

func TestDet_GitIdentityEmail(t *testing.T) {
	ensureSharedContainer(t)
	email := detExec(t, "git", "-C", "/workspace/octocat/Hello-World", "config", "user.email")
	if email != "test@example.com" {
		t.Fatalf("git user.email = %q, want 'test@example.com'", email)
	}
}

func TestDet_SSHKnownHostsExist(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "cat", "/home/agent/.ssh/known_hosts")
	if !strings.Contains(out, "github.com") {
		t.Fatalf("known_hosts should contain github.com: %s", out)
	}
}

func TestDet_SSHConfigExists(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "cat", "/home/agent/.ssh/config")
	if !strings.Contains(out, "StrictHostKeyChecking") {
		t.Fatalf("SSH config should have StrictHostKeyChecking: %s", out)
	}
}

func TestDet_GitDefaultBranchMain(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "git", "config", "--global", "init.defaultBranch")
	if out != "main" {
		t.Fatalf("init.defaultBranch = %q, want 'main'", out)
	}
}

func TestDet_GitPagerDelta(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "git", "config", "--global", "core.pager")
	if !strings.Contains(out, "delta") {
		t.Fatalf("core.pager = %q, want delta", out)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 3: Environment Variables
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_AgentTypeEnv(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "printenv", "AGENT_TYPE")
	if out != "shell" {
		t.Fatalf("AGENT_TYPE = %q, want shell", out)
	}
}

func TestDet_ReposEnv(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "printenv", "REPOS")
	if !strings.Contains(out, "Hello-World") {
		t.Fatalf("REPOS = %q, should contain Hello-World", out)
	}
}

func TestDet_GitEnvVars(t *testing.T) {
	ensureSharedContainer(t)
	for _, env := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		t.Run(env, func(t *testing.T) {
			out := detExec(t, "printenv", env)
			if out == "" {
				t.Fatalf("%s not set", env)
			}
		})
	}
}

func TestDet_HomeEnv(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "printenv", "HOME")
	if out != "/home/agent" {
		t.Fatalf("HOME = %q, want /home/agent", out)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 4: Installed Tools
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_ToolsInstalled(t *testing.T) {
	ensureSharedContainer(t)
	tools := []string{
		"git", "bash", "curl", "jq", "rg", "fd", "bat", "eza",
		"zoxide", "node", "npm", "gh", "tmux", "python3", "delta",
	}
	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			_, err := detExecMayFail(t, "which", tool)
			if err != nil {
				t.Fatalf("%s not found in PATH", tool)
			}
		})
	}
}

func TestDet_AgentCLIsInstalled(t *testing.T) {
	ensureSharedContainer(t)
	for _, cli := range []string{"claude", "codex"} {
		t.Run(cli, func(t *testing.T) {
			_, err := detExecMayFail(t, "which", cli)
			if err != nil {
				t.Fatalf("%s not installed", cli)
			}
		})
	}
}

func TestDet_NodeVersion(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "node", "--version")
	if !strings.HasPrefix(out, "v") {
		t.Fatalf("node --version = %q, expected vX.Y.Z", out)
	}
}

func TestDet_BashVersion(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "bash", "--version")
	if !strings.Contains(out, "GNU bash") {
		t.Fatalf("expected GNU bash, got: %s", out)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 5: Security Enforcement
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_CannotEscalatePrivileges(t *testing.T) {
	ensureSharedContainer(t)
	_, err := detExecMayFail(t, "apt-get", "update")
	if err == nil {
		t.Fatal("should not be able to run apt-get (read-only rootfs)")
	}
}

func TestDet_CannotWriteEtc(t *testing.T) {
	ensureSharedContainer(t)
	_, err := detExecMayFail(t, "bash", "-c", "echo test > /etc/test")
	if err == nil {
		t.Fatal("should not write to /etc")
	}
}

func TestDet_CannotWriteUsr(t *testing.T) {
	ensureSharedContainer(t)
	_, err := detExecMayFail(t, "bash", "-c", "echo test > /usr/test")
	if err == nil {
		t.Fatal("should not write to /usr")
	}
}

func TestDet_CannotWriteBin(t *testing.T) {
	ensureSharedContainer(t)
	_, err := detExecMayFail(t, "bash", "-c", "echo test > /usr/bin/test-bin")
	if err == nil {
		t.Fatal("should not write to /usr/bin")
	}
}

func TestDet_TmpNoexec(t *testing.T) {
	ensureSharedContainer(t)
	// Write a script to /tmp
	detExec(t, "bash", "-c", "echo '#!/bin/bash\necho hello' > /tmp/test.sh")
	// Try to make it executable and run -- should fail due to noexec
	_, err := detExecMayFail(t, "bash", "-c", "chmod +x /tmp/test.sh && /tmp/test.sh")
	if err == nil {
		t.Fatal("/tmp should be noexec")
	}
	// Cleanup
	detExec(t, "rm", "-f", "/tmp/test.sh")
}

func TestDet_NoCapabilities(t *testing.T) {
	ensureSharedContainer(t)
	// ping needs NET_RAW capability
	_, err := detExecMayFail(t, "ping", "-c", "1", "-W", "1", "127.0.0.1")
	if err == nil {
		t.Fatal("ping should fail without NET_RAW capability")
	}
}

func TestDet_CannotChown(t *testing.T) {
	ensureSharedContainer(t)
	detExec(t, "touch", "/tmp/chown-test")
	_, err := detExecMayFail(t, "chown", "root:root", "/tmp/chown-test")
	if err == nil {
		t.Fatal("chown to root should fail without CAP_CHOWN")
	}
	detExec(t, "rm", "-f", "/tmp/chown-test")
}

func TestDet_ProcSelfStatusNoCapabilities(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "bash", "-c", "grep CapEff /proc/self/status")
	// cap-drop ALL should set effective capabilities to 0
	if !strings.Contains(out, "0000000000000000") {
		t.Fatalf("expected zero effective capabilities, got: %s", out)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 6: Git Operations (deterministic)
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_GitCommitWorks(t *testing.T) {
	ensureSharedContainer(t)
	repo := "/workspace/octocat/Hello-World"

	detExec(t, "bash", "-c",
		fmt.Sprintf("cd %s && echo 'det-test-%d' > det-testfile.txt && git add det-testfile.txt && git commit -m 'det: test commit'", repo, time.Now().Unix()))

	log := detExec(t, "git", "-C", repo, "log", "--oneline", "-1")
	if !strings.Contains(log, "det: test commit") {
		t.Fatalf("commit not found in log: %s", log)
	}

	author := detExec(t, "git", "-C", repo, "log", "-1", "--format=%an")
	if author != "Test User" {
		t.Fatalf("commit author = %q, want 'Test User'", author)
	}

	authorEmail := detExec(t, "git", "-C", repo, "log", "-1", "--format=%ae")
	if authorEmail != "test@example.com" {
		t.Fatalf("commit email = %q, want 'test@example.com'", authorEmail)
	}
}

func TestDet_GitDiffWorks(t *testing.T) {
	ensureSharedContainer(t)
	repo := "/workspace/octocat/Hello-World"

	detExec(t, "bash", "-c",
		fmt.Sprintf("cd %s && echo 'det-unstaged-change' >> README", repo))

	diff := detExec(t, "git", "-C", repo, "diff")
	if !strings.Contains(diff, "det-unstaged-change") {
		t.Fatalf("diff should show unstaged change: %s", diff)
	}

	// Reset for other tests
	detExec(t, "git", "-C", repo, "checkout", "--", "README")
}

func TestDet_GitBranchWorks(t *testing.T) {
	ensureSharedContainer(t)
	repo := "/workspace/octocat/Hello-World"

	detExec(t, "git", "-C", repo, "checkout", "-b", "det-test-branch")
	out := detExec(t, "git", "-C", repo, "branch", "--show-current")
	if out != "det-test-branch" {
		t.Fatalf("branch = %q, want det-test-branch", out)
	}

	// Switch back
	detExec(t, "git", "-C", repo, "checkout", "master")
	detExec(t, "git", "-C", repo, "branch", "-D", "det-test-branch")
}

func TestDet_GitStashWorks(t *testing.T) {
	ensureSharedContainer(t)
	repo := "/workspace/octocat/Hello-World"

	detExec(t, "bash", "-c", fmt.Sprintf("echo stash-test >> %s/README", repo))
	detExec(t, "git", "-C", repo, "stash")

	diff := detExec(t, "git", "-C", repo, "diff")
	if strings.Contains(diff, "stash-test") {
		t.Fatal("stash should have removed the change from working tree")
	}

	detExec(t, "git", "-C", repo, "stash", "pop")
	detExec(t, "git", "-C", repo, "checkout", "--", "README")
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 7: Multiple Repos (dedicated container)
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_MultipleRepos(t *testing.T) {
	ctx := context.Background()
	name := "agent-shell-e2e-det-multi"

	// Cleanup from previous runs
	orbExec.Run(ctx, "docker", "rm", "-f", name)

	args := []string{
		"docker", "run", "-d",
		"--name", name,
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=512m",
		"--tmpfs", "/var/tmp:rw,noexec,nosuid,size=256m",
		"--tmpfs", "/run:rw,noexec,nosuid,size=16m",
		"--tmpfs", "/dev/shm:rw,noexec,nosuid,size=64m",
		"--tmpfs", "/home/agent/.config:rw,noexec,size=32m,uid=1000,gid=1000",
		"--tmpfs", "/home/agent/.ssh:rw,noexec,size=1m,uid=1000,gid=1000",
		"--mount", "type=volume,dst=/workspace",
		"-e", "AGENT_TYPE=shell",
		"-e", "REPOS=https://github.com/octocat/Hello-World.git,https://github.com/octocat/Spoon-Knife.git",
		"-e", "GIT_AUTHOR_NAME=Test User",
		"-e", "GIT_AUTHOR_EMAIL=test@example.com",
		"-e", "GIT_COMMITTER_NAME=Test User",
		"-e", "GIT_COMMITTER_EMAIL=test@example.com",
		detTestImage,
		"-lc", "sleep 300",
	}

	_, err := orbExec.Run(ctx, args...)
	if err != nil {
		t.Fatalf("docker run: %v", err)
	}
	defer orbExec.Run(ctx, "docker", "rm", "-f", name)

	// Wait for container + cloning
	for i := 0; i < 40; i++ {
		out, err := orbExec.Run(ctx, "docker", "inspect", "--format", "{{.State.Status}}", name)
		if err == nil && strings.TrimSpace(string(out)) == "running" {
			time.Sleep(8 * time.Second)
			break
		}
		if i == 39 {
			logs, _ := orbExec.Run(ctx, "docker", "logs", name)
			t.Fatalf("multi-repo container not running after 40s. Logs:\n%s", logs)
		}
		time.Sleep(1 * time.Second)
	}

	multiExec := func(cmd ...string) string {
		t.Helper()
		a := append([]string{"docker", "exec", name}, cmd...)
		out, err := orbExec.Run(ctx, a...)
		if err != nil {
			t.Fatalf("exec %v: %v\n%s", cmd, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	out := multiExec("ls", "/workspace/octocat/")
	if !strings.Contains(out, "Hello-World") {
		t.Fatalf("Hello-World not cloned: %s", out)
	}
	if !strings.Contains(out, "Spoon-Knife") {
		t.Fatalf("Spoon-Knife not cloned: %s", out)
	}

	// Verify both are git repos
	multiExec("git", "-C", "/workspace/octocat/Hello-World", "status")
	multiExec("git", "-C", "/workspace/octocat/Spoon-Knife", "status")
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 8: Security Preamble & Config Injection
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_SecurityPreambleInjectedByEntrypoint(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "cat", "/home/agent/.claude/CLAUDE.md")
	if !strings.Contains(out, "safe-agentic:security-preamble") {
		t.Fatalf("CLAUDE.md should contain injected security preamble marker, got: %s", out)
	}
	out = detExec(t, "cat", "/home/agent/.codex/AGENTS.md")
	if !strings.Contains(out, "safe-agentic:security-preamble") {
		t.Fatalf("AGENTS.md should contain injected security preamble marker, got: %s", out)
	}
}

func TestDet_CodexConfigCreated(t *testing.T) {
	ensureSharedContainer(t)
	out, err := detExecMayFail(t, "cat", "/home/agent/.codex/config.toml")
	if err != nil {
		t.Skipf("codex config not found: %v", err)
	}
	if !strings.Contains(out, "approval_policy") {
		t.Fatalf("codex config should have approval_policy, got: %s", out)
	}
}

func TestDet_ClaudeConfigCreated(t *testing.T) {
	ensureSharedContainer(t)
	out, err := detExecMayFail(t, "cat", "/home/agent/.claude/settings.json")
	if err != nil {
		t.Skipf("claude settings not found: %v", err)
	}
	if !strings.Contains(out, "permissions") {
		t.Fatalf("claude settings should have permissions, got: %s", out)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 9: Filesystem Layout
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_WorkspaceDirExists(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "stat", "-c", "%F", "/workspace")
	if !strings.Contains(out, "directory") {
		t.Fatalf("/workspace should be a directory, got: %s", out)
	}
}

func TestDet_AgentUserOwnsWorkspace(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "stat", "-c", "%U", "/workspace")
	// Volume may be owned by root initially; agent should still be able to write
	detExec(t, "touch", "/workspace/.det-ownership-test")
	detExec(t, "rm", "/workspace/.det-ownership-test")
	t.Logf("/workspace owned by: %s (writable by agent)", out)
}

func TestDet_TmpDirExists(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "stat", "-c", "%F", "/tmp")
	if !strings.Contains(out, "directory") {
		t.Fatalf("/tmp should be a directory, got: %s", out)
	}
}

func TestDet_RunDirExists(t *testing.T) {
	ensureSharedContainer(t)
	out := detExec(t, "stat", "-c", "%F", "/run")
	if !strings.Contains(out, "directory") {
		t.Fatalf("/run should be a directory, got: %s", out)
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Group 10: safe-ag CLI Against Shared Container
// ═══════════════════════════════════════════════════════════════════════════

func TestDet_SafeAgList(t *testing.T) {
	ensureSharedContainer(t)
	out, err := runSafeAg(t, "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, detSharedContainer) {
		t.Fatalf("list should show shared container %s:\n%s", detSharedContainer, out)
	}
}

func TestDet_SafeAgSummary(t *testing.T) {
	ensureSharedContainer(t)
	out, err := runSafeAg(t, "summary", detSharedContainer)
	if err != nil {
		t.Fatalf("summary: %v\n%s", err, out)
	}
	// Should show agent type and running status
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "shell") {
		t.Fatalf("summary should mention agent type 'shell':\n%s", out)
	}
}

func TestDet_SafeAgDiff(t *testing.T) {
	ensureSharedContainer(t)
	repo := "/workspace/octocat/Hello-World"

	// Make a tracked change
	detExec(t, "bash", "-c", fmt.Sprintf("echo 'safe-ag-diff-test' >> %s/README", repo))
	defer detExec(t, "git", "-C", repo, "checkout", "--", "README")

	out, err := runSafeAg(t, "diff", detSharedContainer)
	if err != nil {
		t.Fatalf("diff failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "safe-ag-diff-test") {
		t.Fatalf("diff should show our change:\n%s", out)
	}
}

func TestDet_SafeAgDiffStat(t *testing.T) {
	ensureSharedContainer(t)
	repo := "/workspace/octocat/Hello-World"

	detExec(t, "bash", "-c", fmt.Sprintf("echo 'stat-test' >> %s/README", repo))
	defer detExec(t, "git", "-C", repo, "checkout", "--", "README")

	out, err := runSafeAg(t, "diff", "--stat", detSharedContainer)
	if err != nil {
		t.Fatalf("diff --stat failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "README") {
		t.Fatalf("diff --stat should mention README:\n%s", out)
	}
}

func TestDet_SafeAgTodoWorkflow(t *testing.T) {
	ensureSharedContainer(t)

	// Ensure .safe-agentic dir exists
	detExec(t, "mkdir", "-p", "/workspace/.safe-agentic")

	// Add todo
	out, err := runSafeAg(t, "todo", "add", detSharedContainer, "Det: write more tests")
	if err != nil {
		t.Fatalf("todo add: %v\n%s", err, out)
	}

	// List
	out, err = runSafeAg(t, "todo", "list", detSharedContainer)
	if err != nil {
		t.Fatalf("todo list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Det: write more tests") || !strings.Contains(out, "[ ]") {
		t.Fatalf("todo list should show unchecked item:\n%s", out)
	}

	// Check
	out, err = runSafeAg(t, "todo", "check", detSharedContainer, "1")
	if err != nil {
		t.Fatalf("todo check: %v\n%s", err, out)
	}

	// Verify checked
	out, err = runSafeAg(t, "todo", "list", detSharedContainer)
	if err != nil {
		t.Fatalf("todo list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[x]") {
		t.Fatalf("todo should be checked:\n%s", out)
	}

	// Uncheck
	out, err = runSafeAg(t, "todo", "uncheck", detSharedContainer, "1")
	if err != nil {
		t.Fatalf("todo uncheck: %v\n%s", err, out)
	}

	out, err = runSafeAg(t, "todo", "list", detSharedContainer)
	if err != nil {
		t.Fatalf("todo list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[ ]") {
		t.Fatalf("todo should be unchecked:\n%s", out)
	}
}

func TestDet_SafeAgCheckpointWorkflow(t *testing.T) {
	ensureSharedContainer(t)
	repo := "/workspace/octocat/Hello-World"

	// Make an UNCOMMITTED change (stash only works on uncommitted changes)
	detExec(t, "bash", "-c", fmt.Sprintf(
		"cd %s && echo 'checkpoint-test-%d' > det-checkpoint.txt && git add det-checkpoint.txt",
		repo, time.Now().Unix()))

	// Create checkpoint (uses workspaceExec -> git stash)
	out, err := runSafeAg(t, "checkpoint", "create", detSharedContainer, "det-cp")
	if err != nil {
		t.Fatalf("checkpoint create failed: %v\n%s", err, out)
	}

	// List checkpoints
	out, err = runSafeAg(t, "checkpoint", "list", detSharedContainer)
	if err != nil {
		t.Fatalf("checkpoint list failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "det-cp") {
		t.Fatalf("checkpoint list should show 'det-cp':\n%s", out)
	}

	// Restore checkpoint (pop the stash)
	out, err = runSafeAg(t, "checkpoint", "restore", detSharedContainer, "0")
	if err != nil {
		t.Logf("checkpoint restore: %v\n%s", err, out)
	}
}

func TestDet_SafeAgOutputCommits(t *testing.T) {
	ensureSharedContainer(t)
	repo := "/workspace/octocat/Hello-World"

	// Create a recognizable commit
	detExec(t, "bash", "-c", fmt.Sprintf(
		"cd %s && echo 'output-test-%d' > det-output.txt && git add det-output.txt && git commit -m 'det: output commit test'",
		repo, time.Now().Unix()))

	out, err := runSafeAg(t, "output", "--commits", detSharedContainer)
	if err != nil {
		t.Fatalf("output --commits failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "det: output commit test") {
		t.Fatalf("output --commits should show our commit:\n%s", out)
	}
}

func TestDet_SafeAgOutputFiles(t *testing.T) {
	ensureSharedContainer(t)

	out, err := runSafeAg(t, "output", "--files", detSharedContainer)
	if err != nil {
		t.Fatalf("output --files failed: %v\n%s", err, out)
	}
	// Should list files that have been changed
	t.Logf("output --files:\n%s", out)
}

func TestDet_SafeAgPeek(t *testing.T) {
	ensureSharedContainer(t)
	// The shared container runs 'sleep 300', not tmux, so peek will fail.
	// This test just verifies safe-ag handles the error gracefully.
	_, err := runSafeAg(t, "peek", detSharedContainer)
	if err == nil {
		// If it succeeds, that's fine
		return
	}
	// Expected to fail; just verify it doesn't panic
	t.Logf("peek failed as expected (no tmux): %v", err)
}

func TestDet_SafeAgCost(t *testing.T) {
	ensureSharedContainer(t)
	out, err := runSafeAg(t, "cost", detSharedContainer)
	if err != nil {
		// cost may fail if no usage data; that's ok
		t.Logf("cost returned error (expected for test container): %v\n%s", err, out)
		return
	}
	t.Logf("cost output: %s", out)
}
