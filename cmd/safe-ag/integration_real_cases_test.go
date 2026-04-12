//go:build integration

package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func sharedWorkspaceVolume(t *testing.T) string {
	t.Helper()
	ensureSharedContainer(t)
	ctx := context.Background()
	out, err := orbExec.Run(ctx, "docker", "inspect", "--format",
		`{{range .Mounts}}{{if eq .Destination "/workspace"}}{{.Name}}{{end}}{{end}}`,
		detSharedContainer)
	if err != nil {
		t.Fatalf("inspect shared workspace volume: %v", err)
	}
	vol := strings.TrimSpace(string(out))
	if vol == "" {
		t.Fatal("shared workspace volume not found")
	}
	return vol
}

func startTempShellContainer(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	orbExec.Run(ctx, "docker", "rm", "-f", name)
	workspaceVol := sharedWorkspaceVolume(t)

	args := []string{
		"docker", "run", "-d",
		"--name", name,
		"--hostname", name,
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
		"--mount", "type=volume,src=" + workspaceVol + ",dst=/workspace",
		"--label", "safe-agentic.agent-type=shell",
		"--label", "safe-agentic.auth=shared",
		"--label", "safe-agentic.ssh=false",
		"--label", "safe-agentic.docker=off",
		"--label", "safe-agentic.network-mode=none",
		"--label", "safe-agentic.repo-display=octocat/Hello-World",
		"--label", "safe-agentic.resources=cpu=4,mem=8g,pids=512",
		"--entrypoint", "bash",
		detTestImage,
		"-lc", "sleep 600",
	}

	if _, err := orbExec.Run(ctx, args...); err != nil {
		t.Fatalf("docker run %s: %v", name, err)
	}

	for i := 0; i < 20; i++ {
		out, err := orbExec.Run(ctx, "docker", "inspect", "--format", "{{.State.Status}}", name)
		if err == nil && strings.TrimSpace(string(out)) == "running" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	logs, _ := orbExec.Run(ctx, "docker", "logs", name)
	t.Fatalf("container %s not running. logs:\n%s", name, logs)
}

func TestE2E_RunQuickStartCreatesContainer(t *testing.T) {
	suffix := testPrefix + "-run"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	out, err := runSafeAg(t, "run",
		"--name", suffix,
		"--background",
		"https://github.com/octocat/Hello-World.git",
		"List the repository files")
	if err != nil {
		t.Fatalf("run failed: %v\n%s", err, out)
	}
	if !waitForContainer(t, fullName) {
		t.Fatal("run quick-start container did not appear")
	}

	if got := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.agent-type"}}`); got != "claude" {
		t.Fatalf("agent-type = %q, want claude", got)
	}
	if got := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.auth"}}`); got != "shared" {
		t.Fatalf("auth label = %q, want shared", got)
	}
	if got := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.prompt"}}`); !strings.Contains(got, "List the repository files") {
		t.Fatalf("prompt label = %q, want prompt snippet", got)
	}
}

func TestDet_SafeAgOutputFilesShowsTrackedAndUntrackedChanges(t *testing.T) {
	ensureSharedContainer(t)
	repo := "/workspace/octocat/Hello-World"

	detExec(t, "bash", "-lc", fmt.Sprintf(
		"cd %s && echo 'tracked-output-files' >> README && printf 'untracked\\n' > det-untracked.txt",
		repo))
	defer detExec(t, "bash", "-lc", fmt.Sprintf(
		"cd %s && git checkout -- README && rm -f det-untracked.txt",
		repo))

	out, err := runSafeAg(t, "output", "--files", detSharedContainer)
	if err != nil {
		t.Fatalf("output --files failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "README") {
		t.Fatalf("output --files should include README:\n%s", out)
	}
	if !strings.Contains(out, "det-untracked.txt") {
		t.Fatalf("output --files should include untracked file:\n%s", out)
	}
}

func TestDet_StoppedContainerDiffAndOutputFiles(t *testing.T) {
	name := "agent-shell-e2e-det-stopped"
	repo := "/workspace/octocat/Hello-World"
	startTempShellContainer(t, name)
	defer orbExec.Run(context.Background(), "docker", "rm", "-f", name)

	trackedOut, err := orbExec.Run(context.Background(), "docker", "exec", name, "bash", "-lc",
		fmt.Sprintf("cd %s && git ls-files | head -1", repo))
	if err != nil {
		t.Fatalf("resolve tracked file: %v", err)
	}
	trackedFile := strings.TrimSpace(string(trackedOut))
	if trackedFile == "" {
		t.Fatal("expected at least one tracked file in temp repo")
	}

	if _, err := orbExec.Run(context.Background(), "docker", "exec", name, "bash", "-lc",
		fmt.Sprintf("cd %s && echo 'stopped-diff-marker' >> %q && printf 'stopped\\n' > det-stopped.txt", repo, trackedFile)); err != nil {
		t.Fatalf("seed stopped-container changes: %v", err)
	}
	if _, err := orbExec.Run(context.Background(), "docker", "stop", "-t", "5", name); err != nil {
		t.Fatalf("stop temp container: %v", err)
	}

	out, err := runSafeAg(t, "diff", name)
	if err != nil {
		t.Fatalf("diff on stopped container failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "stopped-diff-marker") {
		t.Fatalf("stopped diff should show marker:\n%s", out)
	}

	out, err = runSafeAg(t, "output", "--files", name)
	if err != nil {
		t.Fatalf("output --files on stopped container failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, trackedFile) || !strings.Contains(out, "det-stopped.txt") {
		t.Fatalf("stopped output --files should show tracked and untracked changes:\n%s", out)
	}
}
