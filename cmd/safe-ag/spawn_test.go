package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
)

func TestWaitForSessionOrExit_SessionReady(t *testing.T) {
	fake := vmexec.NewFake()
	// Default fake: docker exec (tmux has-session) succeeds → session is ready.
	if err := waitForSessionOrExit(context.Background(), fake, "c"); err != nil {
		t.Fatalf("expected nil when session ready, got %v", err)
	}
}

func TestWaitForSessionOrExit_ContainerExitedFailsFast(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("docker exec", "no session yet")                          // HasSession → false
	fake.SetResponse("docker inspect --format {{.State.Running}}", "false") // not running
	fake.SetResponse("docker inspect --format {{.State.ExitCode}}", "128")  // exited 128
	fake.SetResponse("bash -lc docker logs",
		"ssh: connect to host github.com port 22: Connection timed out")

	err := waitForSessionOrExit(context.Background(), fake, "c")
	if err == nil {
		t.Fatal("expected error when container exited before session")
	}
	if !strings.Contains(err.Error(), "128") {
		t.Errorf("error should report exit code 128, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "port 22") {
		t.Errorf("error should surface the container logs, got %q", err.Error())
	}
}

func TestResolveContainerName(t *testing.T) {
	tests := []struct {
		agentType string
		name      string
		timestamp string
		repos     []string
		want      string
	}{
		{"claude", "my-agent", "20260410-120000", nil, "agent-claude-my-agent"},
		{"codex", "", "20260410-120000", []string{"https://github.com/org/repo.git"}, "agent-codex-repo-20260410-120000"},
		{"claude", "", "20260410-120000", nil, "agent-claude-20260410-120000"},
		{"shell", "debug", "20260410-120000", nil, "agent-shell-debug"},
		{"claude", "", "20260410-120000", []string{"https://github.com/org/very-long-repository-name-here.git"}, "agent-claude-very-long-repository-20260410-120000"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := resolveContainerName(tt.agentType, tt.name, tt.timestamp, tt.repos)
			if got != tt.want {
				t.Fatalf("resolveContainerName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpawnDryRunContainsSecurityFlags(t *testing.T) {
	containerName := "agent-claude-test"
	cmd := docker.NewRunCmd(containerName, "safe-agentic:latest")
	cmd.AddEnv("AGENT_TYPE", "claude")
	cmd.AddEnv("REPOS", "https://github.com/org/repo.git")

	docker.AppendRuntimeHardening(cmd, docker.HardeningOpts{
		Network:   "agent-claude-test-net",
		Memory:    "8g",
		CPUs:      "4",
		PIDsLimit: 512,
	})
	docker.AppendCacheMounts(cmd)

	args := cmd.Build()
	joined := strings.Join(args, " ")

	required := []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--security-opt=seccomp=/etc/safe-agentic/seccomp.json",
		"--read-only",
		"--network agent-claude-test-net",
		"--memory 8g",
		"--cpus 4",
		"--pids-limit 512",
		"--ulimit nofile=65536:65536",
		"-e AGENT_TYPE=claude",
		"-e REPOS=https://github.com/org/repo.git",
		"--tmpfs /tmp:rw,noexec,nosuid,size=512m",
		"--tmpfs /var/tmp:rw,noexec,nosuid,size=256m",
		"--tmpfs /run:rw,noexec,nosuid,size=16m",
		"--tmpfs /dev/shm:rw,noexec,nosuid,size=64m",
		"--mount type=volume,dst=/workspace",
		"--mount type=volume,dst=/home/agent/.npm",
		"--mount type=volume,dst=/home/agent/.cache/pip",
		"--mount type=volume,dst=/home/agent/go",
		"safe-agentic:latest",
	}
	for _, r := range required {
		if !strings.Contains(joined, r) {
			t.Errorf("missing %q in docker command:\n%s", r, joined)
		}
	}

	forbidden := []string{
		"--privileged",
		"--cap-add",
		"--network host",
	}
	for _, f := range forbidden {
		if strings.Contains(joined, f) {
			t.Errorf("forbidden flag %q found in docker command", f)
		}
	}
}

func TestPrepareSpawnResourceLimits_OmitsDefaultMemoryAndCPUsOnThreadedCgroup(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -lc cat /sys/fs/cgroup/docker/cgroup.type", "threaded\n")
	resolved := spawnResolved{Memory: "8g", CPUs: "4"}

	err := prepareSpawnResourceLimits(context.Background(), fake, SpawnOpts{}, &resolved)
	if err != nil {
		t.Fatalf("prepareSpawnResourceLimits() error = %v", err)
	}
	if resolved.Memory != "" || resolved.CPUs != "" {
		t.Fatalf("expected memory/cpus omitted, got memory=%q cpus=%q", resolved.Memory, resolved.CPUs)
	}
}

func TestPrepareSpawnResourceLimits_OmitsDefaultMemoryAndCPUsOnRootThreadedCgroup(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -lc cat /sys/fs/cgroup/docker/cgroup.type", "domain threaded\n")
	resolved := spawnResolved{Memory: "8g", CPUs: "4"}

	err := prepareSpawnResourceLimits(context.Background(), fake, SpawnOpts{}, &resolved)
	if err != nil {
		t.Fatalf("prepareSpawnResourceLimits() error = %v", err)
	}
	if resolved.Memory != "" || resolved.CPUs != "" {
		t.Fatalf("expected memory/cpus omitted, got memory=%q cpus=%q", resolved.Memory, resolved.CPUs)
	}
}

func TestPrepareSpawnResourceLimits_RejectsExplicitLimitsOnThreadedCgroup(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("bash -lc cat /sys/fs/cgroup/docker/cgroup.type", "threaded\n")
	resolved := spawnResolved{Memory: "8g", CPUs: "4"}

	err := prepareSpawnResourceLimits(context.Background(), fake, SpawnOpts{Memory: "8g"}, &resolved)
	if err == nil {
		t.Fatal("expected explicit memory limit to fail on threaded cgroup")
	}
	if !strings.Contains(err.Error(), "threaded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartSpawnContainerStartsDinDBeforeAgent(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker exec safe-agentic-docker-agent-claude-test docker info", "ok")
	cmd := docker.NewRunCmd("agent-claude-test", "safe-agentic:latest")
	resolved := spawnResolved{
		ContainerName: "agent-claude-test",
		NetworkName:   "agent-claude-test-net",
		ImageName:     "safe-agentic:latest",
	}

	if err := startSpawnContainer(context.Background(), fake, cmd, SpawnOpts{AgentType: "claude", DockerAccess: true}, resolved); err != nil {
		t.Fatalf("startSpawnContainer() error = %v", err)
	}
	runCmds := fake.CommandsMatching("docker run")
	if len(runCmds) < 2 {
		t.Fatalf("expected DinD and agent docker runs, got %d", len(runCmds))
	}
	first := strings.Join(runCmds[0], " ")
	second := strings.Join(runCmds[1], " ")
	if !strings.Contains(first, "safe-agentic-docker-agent-claude-test") {
		t.Fatalf("first docker run should start DinD, got:\n%s", first)
	}
	if !strings.Contains(second, "--name agent-claude-test") {
		t.Fatalf("second docker run should start agent, got:\n%s", second)
	}
}

// worktreeUnderRoot returns a worktree path under the (HOME-derived) worktrees
// root, plus its expected in-VM path, for the given leaf name.
func worktreeUnderRoot(home, leaf string) (string, string) {
	return filepath.Join(home, ".safe-ag", "worktrees", leaf), "/worktrees/" + leaf
}

func TestExecuteSpawnWorktreeDryRun(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	repo := initSpawnGitRepo(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Worktrees must live under the mounted root; the container bind uses the
	// translated in-VM path (/worktrees/...), not the host path.
	worktreePath, vmPath := worktreeUnderRoot(home, "agent-worktree")
	output := captureOutput(func() {
		err := executeSpawn(SpawnOpts{
			AgentType:      "claude",
			Name:           "worktree-test",
			Worktree:       true,
			WorktreePath:   worktreePath,
			WorktreeBranch: "safe-ag/worktree-test",
			DryRun:         true,
		})
		if err != nil {
			t.Fatalf("executeSpawn worktree dry-run error = %v", err)
		}
	})
	if !strings.Contains(output, "Worktree: "+worktreePath) {
		t.Fatalf("dry-run output missing worktree path:\n%s", output)
	}
	if !strings.Contains(output, "→ VM "+vmPath) {
		t.Fatalf("dry-run output missing VM path translation:\n%s", output)
	}
	if !strings.Contains(output, "type=bind,src="+vmPath+",dst=/workspace") {
		t.Fatalf("dry-run output missing translated workspace bind:\n%s", output)
	}
	if !strings.Contains(output, "SAFE_AGENTIC_WORKTREE=1") {
		t.Fatalf("dry-run output missing worktree env:\n%s", output)
	}
	// The label keeps the host path so host-side git ops (diff/snapshot) work.
	if !strings.Contains(output, "safe-agentic.worktree="+worktreePath) {
		t.Fatalf("dry-run output missing worktree label:\n%s", output)
	}
	if strings.Contains(output, "type=volume,dst=/workspace") {
		t.Fatalf("dry-run output should not include ephemeral workspace:\n%s", output)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("dry-run created worktree path or unexpected stat error: %v", err)
	}
}

func TestExecuteSpawnWorktreeRejectsOutOfRootPath(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	repo := initSpawnGitRepo(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD)
	t.Setenv("HOME", t.TempDir())

	// A path outside the worktrees root cannot be mounted (the VM only exposes
	// the root), so it must be rejected before any worktree is created.
	err = executeSpawn(SpawnOpts{
		AgentType:    "claude",
		Name:         "out-of-root-worktree",
		Worktree:     true,
		WorktreePath: filepath.Join(t.TempDir(), "elsewhere"),
		DryRun:       true,
	})
	if err == nil || !strings.Contains(err.Error(), "outside the safe-agentic worktrees root") {
		t.Fatalf("executeSpawn() error = %v, want out-of-root worktree error", err)
	}
}

func TestExecuteSpawnWorktreeRejectsMountOptionCharacters(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	repo := initSpawnGitRepo(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A comma in the leaf survives translation into the /worktrees bind path and
	// must be rejected because Docker --mount cannot encode it.
	badPath, _ := worktreeUnderRoot(home, "bad,path")
	err = executeSpawn(SpawnOpts{
		AgentType:    "claude",
		Name:         "bad-worktree-path",
		Worktree:     true,
		WorktreePath: badPath,
		DryRun:       true,
	})
	if err == nil || !strings.Contains(err.Error(), "Docker --mount cannot safely encode") {
		t.Fatalf("executeSpawn() error = %v, want Docker mount path error", err)
	}
}

func TestExecuteSpawnWorktreeChecksVMMountBeforeCreate(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	// Simulate a machine that was never migrated: /worktrees is not mounted.
	fake.SetError("sh -c test -d /worktrees", "no mount")

	repo := initSpawnGitRepo(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD)
	home := t.TempDir()
	t.Setenv("HOME", home)

	worktreePath, _ := worktreeUnderRoot(home, "agent-worktree")
	err = executeSpawn(SpawnOpts{
		AgentType:    "claude",
		Name:         "unmounted-worktree",
		Worktree:     true,
		WorktreePath: worktreePath,
	})
	if err == nil || !strings.Contains(err.Error(), "has no /worktrees mount") {
		t.Fatalf("executeSpawn() error = %v, want worktree mount error", err)
	}
	if _, err := os.Stat(filepath.Join(worktreePath, ".git")); !os.IsNotExist(err) {
		t.Fatalf("worktree should not be created before the mount check passes, stat err=%v", err)
	}
}

func TestExecuteSpawnPolicyDeniesDockerBeforeDryRun(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	rulesDir := filepath.Join(home, ".safe-ag")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatalf("mkdir rules dir: %v", err)
	}
	rules := `[allow]
docker_modes = ["off"]
networks = ["managed"]
`
	if err := os.WriteFile(filepath.Join(rulesDir, "rules.toml"), []byte(rules), 0o600); err != nil {
		t.Fatalf("write rules: %v", err)
	}

	err := executeSpawn(SpawnOpts{
		AgentType:    "claude",
		Name:         "policy-test",
		DockerAccess: true,
		DryRun:       true,
	})
	if err == nil || !strings.Contains(err.Error(), `denies docker mode "dind"`) {
		t.Fatalf("executeSpawn() error = %v, want docker policy denial", err)
	}
}

func TestValidateSpawnWorktreeRejectsRepos(t *testing.T) {
	err := validateSpawnOpts(SpawnOpts{
		AgentType: "claude",
		Worktree:  true,
		Repos:     []string{"https://github.com/org/repo.git"},
	})
	if err == nil || !strings.Contains(err.Error(), "omit --repo") {
		t.Fatalf("validateSpawnOpts() error = %v", err)
	}
}

func TestAuthDestination(t *testing.T) {
	tests := []struct {
		agentType string
		want      string
	}{
		{"claude", "/home/agent/.claude"},
		{"codex", "/home/agent/.codex"},
		{"shell", "/home/agent/.claude"},
	}
	for _, tt := range tests {
		got := authDestination(tt.agentType)
		if got != tt.want {
			t.Fatalf("authDestination(%q) = %q, want %q", tt.agentType, got, tt.want)
		}
	}
}

func initSpawnGitRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runSpawnGit(t, repo, "init")
	runSpawnGit(t, repo, "config", "user.email", "agent@example.com")
	runSpawnGit(t, repo, "config", "user.name", "Agent")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatalf("write tracked: %v", err)
	}
	runSpawnGit(t, repo, "add", "tracked.txt")
	runSpawnGit(t, repo, "commit", "-m", "initial")
	return repo
}

func runSpawnGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestAppendAuthVolumes_DefaultEphemeralUsesTmpfs(t *testing.T) {
	cmd := docker.NewRunCmd("agent-claude-test", "safe-agentic:latest")

	if err := appendAuthVolumes(context.Background(), vmexec.NewFake(), cmd, SpawnOpts{
		AgentType: "claude",
	}, spawnResolved{
		ContainerName: "agent-claude-test",
	}); err != nil {
		t.Fatalf("appendAuthVolumes() error = %v", err)
	}

	joined := strings.Join(cmd.Build(), " ")
	if !strings.Contains(joined, "--tmpfs /home/agent/.claude:rw,noexec,nosuid,size=8m,uid=1000,gid=1000") {
		t.Fatalf("expected ephemeral auth tmpfs in docker command, got:\n%s", joined)
	}
	if strings.Contains(joined, "src=agent-claude-test-auth,dst=/home/agent/.claude") {
		t.Fatalf("did not expect named auth volume in docker command, got:\n%s", joined)
	}
}

func TestAppendAuthVolumes_ShellEphemeralMountsClaudeAndCodex(t *testing.T) {
	cmd := docker.NewRunCmd("agent-shell-test", "safe-agentic:latest")

	if err := appendAuthVolumes(context.Background(), vmexec.NewFake(), cmd, SpawnOpts{
		AgentType: "shell",
	}, spawnResolved{
		ContainerName: "agent-shell-test",
	}); err != nil {
		t.Fatalf("appendAuthVolumes() error = %v", err)
	}

	joined := strings.Join(cmd.Build(), " ")
	for _, want := range []string{
		"--tmpfs /home/agent/.claude:rw,noexec,nosuid,size=8m,uid=1000,gid=1000",
		"--tmpfs /home/agent/.codex:rw,noexec,nosuid,size=8m,uid=1000,gid=1000",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected shell auth mount %q in docker command:\n%s", want, joined)
		}
	}
}

func TestAppendAuthVolumes_FleetCopyFailsClosed(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("docker run --rm", "copy failed")
	cmd := docker.NewRunCmd("agent-claude-test", "safe-agentic:latest")

	err := appendAuthVolumes(context.Background(), fake, cmd, SpawnOpts{
		AgentType:   "claude",
		ReuseAuth:   true,
		FleetVolume: "fleet-vol",
	}, spawnResolved{
		ContainerName: "agent-claude-test",
	})
	if err == nil || !strings.Contains(err.Error(), "copy shared auth volume") {
		t.Fatalf("appendAuthVolumes() error = %v, want copy failure", err)
	}
}

func TestAppendAWSConfig_UsesNosuidTmpfs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o755); err != nil {
		t.Fatalf("mkdir aws dir: %v", err)
	}
	creds := []byte("[test]\naws_access_key_id = key\naws_secret_access_key = secret\n")
	if err := os.WriteFile(filepath.Join(awsDir, "credentials"), creds, 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	cmd := docker.NewRunCmd("agent-shell-test", "safe-agentic:latest")
	if err := appendAWSConfig(cmd, SpawnOpts{AWSProfile: "test"}); err != nil {
		t.Fatalf("appendAWSConfig() error = %v", err)
	}

	joined := strings.Join(cmd.Build(), " ")
	if !strings.Contains(joined, "--tmpfs /home/agent/.aws:rw,noexec,nosuid,size=1m") {
		t.Fatalf("expected nosuid aws tmpfs in docker command, got:\n%s", joined)
	}
}

func TestCoalesce(t *testing.T) {
	if coalesce("a", "b") != "a" {
		t.Error("coalesce should prefer first non-empty")
	}
	if coalesce("", "b") != "b" {
		t.Error("coalesce should fall back to second")
	}
	if coalesce("", "") != "" {
		t.Error("coalesce should return empty if both empty")
	}
}

func TestTruncate(t *testing.T) {
	if truncate("short", 100) != "short" {
		t.Error("truncate should pass through short strings")
	}
	got := truncate("this is a long string", 10)
	if got != "this is a ..." {
		t.Fatalf("truncate() = %q", got)
	}
}

func TestExecuteSpawnWorktreeRejectsStaleRoot(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	// /worktrees is mounted (mount check passes by default) but the VM's sentinel
	// records a different (stale) root than the one this checkout resolves to —
	// defaults.worktrees_dir changed after setup. Spawn must refuse before it
	// creates a worktree Docker would then bind from the wrong VM path.
	fake.SetResponse("sh -c cat /run/safe-ag-worktrees-source", "/some/old/worktrees\n")

	repo := initSpawnGitRepo(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir repo: %v", err)
	}
	defer os.Chdir(oldWD)
	home := t.TempDir()
	t.Setenv("HOME", home)

	worktreePath, _ := worktreeUnderRoot(home, "agent-worktree")
	err = executeSpawn(SpawnOpts{
		AgentType:    "claude",
		Name:         "stale-root-worktree",
		Worktree:     true,
		WorktreePath: worktreePath,
	})
	if err == nil || !strings.Contains(err.Error(), "worktrees root changed") {
		t.Fatalf("executeSpawn() error = %v, want stale worktrees root error", err)
	}
	if _, err := os.Stat(filepath.Join(worktreePath, ".git")); !os.IsNotExist(err) {
		t.Fatalf("worktree must not be created on a stale root, stat err=%v", err)
	}
}
