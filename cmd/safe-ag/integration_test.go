//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/orb"
)

const (
	testPrefix = "agent-e2e-test"
)

var orbExec *orb.OrbExecutor
var testVMName = "safe-agentic"

// binaryPath holds the path to the built safe-ag binary.
// Set relative to the module root by TestMain.
var binaryPath string

func TestMain(m *testing.M) {
	if os.Getenv("SAFE_AGENTIC_INTEGRATION") != "1" {
		fmt.Println("SKIP: set SAFE_AGENTIC_INTEGRATION=1 to run integration tests")
		os.Exit(0)
	}
	if v := os.Getenv("SAFE_AGENTIC_INTEGRATION_VM"); v != "" {
		testVMName = v
	}

	// Build the binary
	fmt.Println("Building safe-ag binary...")
	build := exec.Command("go", "build", "-o", "../../bin/safe-ag", "./")
	build.Dir = "" // current dir is cmd/safe-ag/
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build failed: %s\n%s", err, out)
		os.Exit(1)
	}

	// Resolve absolute path
	abs, err := exec.Command("go", "env", "GOMOD").Output()
	if err == nil {
		modDir := strings.TrimSpace(string(abs))
		if strings.HasSuffix(modDir, "go.mod") {
			modDir = modDir[:len(modDir)-len("go.mod")]
		}
		binaryPath = modDir + "bin/safe-ag"
	} else {
		binaryPath = "../../bin/safe-ag"
	}

	// Verify VM + Docker
	orbExec = &orb.OrbExecutor{VMName: testVMName}
	ctx := context.Background()
	var infoErr error
	for i := 0; i < 6; i++ {
		if _, infoErr = orbExec.Run(ctx, "docker", "info"); infoErr == nil {
			break
		}
		time.Sleep(5 * time.Second)
	}
	if infoErr != nil {
		fmt.Fprintf(os.Stderr, "docker not available in VM %s: %v\n", testVMName, infoErr)
		os.Exit(1)
	}
	fmt.Printf("VM %s and Docker OK\n", testVMName)

	code := m.Run()

	cleanupTestContainers()
	cleanupDetContainers()
	os.Exit(code)
}

func cleanupTestContainers() {
	ctx := context.Background()
	// Find all containers with test prefix
	out, _ := orbExec.Run(ctx, "docker", "ps", "-aq", "--filter", "name="+testPrefix)
	ids := strings.TrimSpace(string(out))
	if ids != "" {
		for _, id := range strings.Split(ids, "\n") {
			id = strings.TrimSpace(id)
			if id != "" {
				orbExec.Run(ctx, "docker", "rm", "-f", id)
			}
		}
	}
	// Cleanup networks
	out, _ = orbExec.Run(ctx, "docker", "network", "ls", "-q", "--filter", "name="+testPrefix)
	nets := strings.TrimSpace(string(out))
	if nets != "" {
		for _, n := range strings.Split(nets, "\n") {
			n = strings.TrimSpace(n)
			if n != "" {
				orbExec.Run(ctx, "docker", "network", "rm", n)
			}
		}
	}
}

// runSafeAg executes the safe-ag binary with given args and returns combined output.
func runSafeAg(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "SAFE_AGENTIC_VM_NAME="+testVMName)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// waitForContainer polls until a container exists (up to 30s).
func waitForContainer(t *testing.T, name string) bool {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 60; i++ {
		_, err := orbExec.Run(ctx, "docker", "inspect", name)
		if err == nil {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// dockerInspectField returns a docker inspect field value.
func dockerInspectField(t *testing.T, container, format string) string {
	t.Helper()
	ctx := context.Background()
	out, err := orbExec.Run(ctx, "docker", "inspect", "--format", format, container)
	if err != nil {
		t.Fatalf("inspect %s with %q: %v", container, format, err)
	}
	return strings.TrimSpace(string(out))
}

// stopAndRemove stops and removes a container + its managed network.
func stopAndRemove(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	orbExec.Run(ctx, "docker", "rm", "-f", name)
	orbExec.Run(ctx, "docker", "network", "rm", name+"-net")
}

// containerFullName returns the full container name as created by spawn.
// spawn creates containers named: agent-<type>-<name>
func containerFullName(agentType, suffix string) string {
	return "agent-" + agentType + "-" + suffix
}

// ─── Version & Help ─────────────────────────────────────────────────────────

func TestE2E_VersionOutput(t *testing.T) {
	out, err := runSafeAg(t, "--version")
	if err != nil {
		t.Fatalf("version failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "safe-agentic v") {
		t.Fatalf("version output unexpected: %s", out)
	}
}

func TestE2E_HelpOutput(t *testing.T) {
	out, err := runSafeAg(t, "--help")
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, out)
	}

	expectedCmds := []string{
		"spawn", "run", "list", "attach", "stop", "cleanup",
		"peek", "output", "summary", "cost", "audit", "diff",
		"checkpoint", "todo", "pr", "review", "fleet", "pipeline",
		"setup", "update", "vm", "diagnose", "config", "template",
		"retry", "sessions", "replay", "mcp-login", "aws-refresh",
	}
	for _, cmd := range expectedCmds {
		if !strings.Contains(out, cmd) {
			t.Errorf("help missing command %q", cmd)
		}
	}
}

// ─── Diagnose ───────────────────────────────────────────────────────────────

func TestE2E_DiagnoseChecks(t *testing.T) {
	out, err := runSafeAg(t, "diagnose")
	if err != nil {
		t.Fatalf("diagnose failed: %v\n%s", err, out)
	}
	// Should show at least one checkmark
	if !strings.Contains(out, "✓") {
		t.Fatalf("diagnose should show check marks:\n%s", out)
	}
	if !strings.Contains(out, "orb installed") {
		t.Fatalf("diagnose should check orb:\n%s", out)
	}
}

// ─── Dry Run ────────────────────────────────────────────────────────────────

func TestE2E_SpawnDryRunDoesNotCreateContainer(t *testing.T) {
	suffix := testPrefix + "-dryrun"
	fullName := containerFullName("claude", suffix)

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Would execute") {
		t.Fatalf("dry-run should print 'Would execute', got: %s", out)
	}

	// Verify no container was created
	ctx := context.Background()
	_, err = orbExec.Run(ctx, "docker", "inspect", fullName)
	if err == nil {
		stopAndRemove(t, fullName)
		t.Fatal("dry-run should not create container")
	}
}

// ─── Spawn + Security Verification ─────────────────────────────────────────

func TestE2E_SpawnCreatesContainer(t *testing.T) {
	suffix := testPrefix + "-spawn"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if err != nil {
		t.Fatalf("spawn failed: %v\n%s", err, out)
	}

	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear within 30s")
	}

	t.Run("CapDropAll", func(t *testing.T) {
		caps := dockerInspectField(t, fullName, "{{json .HostConfig.CapDrop}}")
		if !strings.Contains(caps, "ALL") {
			t.Fatalf("CapDrop = %s, want ALL", caps)
		}
	})

	t.Run("ReadOnlyRootfs", func(t *testing.T) {
		ro := dockerInspectField(t, fullName, "{{.HostConfig.ReadonlyRootfs}}")
		if ro != "true" {
			t.Fatalf("ReadonlyRootfs = %s, want true", ro)
		}
	})

	t.Run("NoNewPrivileges", func(t *testing.T) {
		secopt := dockerInspectField(t, fullName, "{{json .HostConfig.SecurityOpt}}")
		if !strings.Contains(secopt, "no-new-privileges:true") {
			t.Fatalf("SecurityOpt = %s, want no-new-privileges:true", secopt)
		}
	})

	t.Run("MemoryLimit", func(t *testing.T) {
		mem := dockerInspectField(t, fullName, "{{.HostConfig.Memory}}")
		// 8g = 8589934592
		if mem != "8589934592" {
			t.Fatalf("Memory = %s, want 8589934592 (8g)", mem)
		}
	})

	t.Run("CPULimit", func(t *testing.T) {
		cpus := dockerInspectField(t, fullName, "{{.HostConfig.NanoCpus}}")
		// 4 CPUs = 4000000000
		if cpus != "4000000000" {
			t.Fatalf("NanoCpus = %s, want 4000000000 (4 cpus)", cpus)
		}
	})

	t.Run("PIDsLimit", func(t *testing.T) {
		pids := dockerInspectField(t, fullName, "{{.HostConfig.PidsLimit}}")
		if pids != "512" {
			t.Fatalf("PidsLimit = %s, want 512", pids)
		}
	})

	t.Run("LabelAgentType", func(t *testing.T) {
		v := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.agent-type"}}`)
		if v != "claude" {
			t.Fatalf("agent-type label = %q, want claude", v)
		}
	})

	t.Run("LabelSSH", func(t *testing.T) {
		v := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.ssh"}}`)
		if v != "false" {
			t.Fatalf("ssh label = %q, want false", v)
		}
	})

	t.Run("LabelAuth", func(t *testing.T) {
		v := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.auth"}}`)
		if v != "shared" {
			t.Fatalf("auth label = %q, want shared", v)
		}
	})

	t.Run("LabelTerminal", func(t *testing.T) {
		v := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.terminal"}}`)
		if v != "tmux" {
			t.Fatalf("terminal label = %q, want tmux", v)
		}
	})

	t.Run("LabelNetworkMode", func(t *testing.T) {
		v := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.network-mode"}}`)
		if v != "managed" {
			t.Fatalf("network-mode label = %q, want managed", v)
		}
	})

	t.Run("RunsAsNonRoot", func(t *testing.T) {
		ctx := context.Background()
		out, err := orbExec.Run(ctx, "docker", "exec", fullName, "id", "-u")
		if err != nil {
			t.Skipf("container may have exited: %v", err)
		}
		uid := strings.TrimSpace(string(out))
		if uid == "0" {
			t.Fatal("container running as root (uid=0)")
		}
	})

	t.Run("NetworkCreated", func(t *testing.T) {
		ctx := context.Background()
		_, err := orbExec.Run(ctx, "docker", "network", "inspect", fullName+"-net")
		if err != nil {
			t.Fatal("managed network not created")
		}
	})

	t.Run("SeccompProfile", func(t *testing.T) {
		secopt := dockerInspectField(t, fullName, "{{json .HostConfig.SecurityOpt}}")
		if !strings.Contains(secopt, "seccomp=") {
			t.Fatalf("SecurityOpt should contain seccomp profile, got: %s", secopt[:min(len(secopt), 100)])
		}
	})
}

func TestE2E_SpawnWithCustomResources(t *testing.T) {
	suffix := testPrefix + "-resources"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--memory", "4g", "--cpus", "2", "--pids-limit", "256",
		"--background")
	if err != nil {
		t.Fatalf("spawn failed: %v\n%s", err, out)
	}

	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	mem := dockerInspectField(t, fullName, "{{.HostConfig.Memory}}")
	if mem != "4294967296" { // 4g
		t.Fatalf("Memory = %s, want 4294967296 (4g)", mem)
	}

	cpus := dockerInspectField(t, fullName, "{{.HostConfig.NanoCpus}}")
	if cpus != "2000000000" { // 2 cpus
		t.Fatalf("NanoCpus = %s, want 2000000000 (2 cpus)", cpus)
	}

	pids := dockerInspectField(t, fullName, "{{.HostConfig.PidsLimit}}")
	if pids != "256" {
		t.Fatalf("PidsLimit = %s, want 256", pids)
	}
}

func TestE2E_SpawnCodex(t *testing.T) {
	suffix := testPrefix + "-codex"
	fullName := containerFullName("codex", suffix)
	defer stopAndRemove(t, fullName)

	out, err := runSafeAg(t, "spawn", "codex",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if err != nil {
		t.Fatalf("spawn codex failed: %v\n%s", err, out)
	}

	if !waitForContainer(t, fullName) {
		t.Fatal("codex container did not appear")
	}

	agentType := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.agent-type"}}`)
	if agentType != "codex" {
		t.Fatalf("agent-type = %q, want codex", agentType)
	}
}

func TestE2E_SpawnWithPrompt(t *testing.T) {
	suffix := testPrefix + "-prompt"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--prompt", "List all files in the repo",
		"--background")
	if err != nil {
		t.Fatalf("spawn with prompt failed: %v\n%s", err, out)
	}

	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	prompt := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.prompt"}}`)
	if !strings.Contains(prompt, "List all files") {
		t.Fatalf("prompt label = %q, should contain 'List all files'", prompt)
	}
}

func TestE2E_SpawnWithEphemeralAuth(t *testing.T) {
	suffix := testPrefix + "-ephauth"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--ephemeral-auth",
		"--background")
	if err != nil {
		t.Fatalf("spawn with ephemeral auth failed: %v\n%s", err, out)
	}

	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	auth := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.auth"}}`)
	if auth != "ephemeral" {
		t.Fatalf("auth label = %q, want ephemeral", auth)
	}
}

func TestE2E_SpawnWithCustomNetwork(t *testing.T) {
	suffix := testPrefix + "-customnet"
	fullName := containerFullName("claude", suffix)
	netName := testPrefix + "-custom-net"

	ctx := context.Background()
	// Create custom network
	orbExec.Run(ctx, "docker", "network", "create", netName)
	defer func() {
		stopAndRemove(t, fullName)
		orbExec.Run(ctx, "docker", "network", "rm", netName)
	}()

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--network", netName,
		"--background")
	if err != nil {
		t.Fatalf("spawn with custom network failed: %v\n%s", err, out)
	}

	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	networkMode := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.network-mode"}}`)
	if networkMode != "custom" {
		t.Fatalf("network-mode = %q, want custom", networkMode)
	}
}

// ─── Lifecycle Commands ─────────────────────────────────────────────────────

func TestE2E_ListShowsContainers(t *testing.T) {
	suffix := testPrefix + "-list"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	out, err := runSafeAg(t, "list")
	if err != nil {
		t.Fatalf("list failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, fullName) {
		t.Fatalf("list output should contain %s:\n%s", fullName, out)
	}
	if !strings.Contains(out, "claude") {
		t.Fatalf("list output should contain 'claude':\n%s", out)
	}
}

func TestE2E_ListJSON(t *testing.T) {
	suffix := testPrefix + "-listjson"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	out, err := runSafeAg(t, "list", "--json")
	if err != nil {
		t.Fatalf("list --json failed: %v\n%s", err, out)
	}

	// Should be valid JSON lines
	lines := strings.Split(strings.TrimSpace(out), "\n")
	foundOurs := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("invalid JSON line: %s\nerror: %v", line, err)
		}
		// Docker JSON output uses "Names" field
		if names, ok := obj["Names"].(string); ok && strings.Contains(names, suffix) {
			foundOurs = true
		}
	}
	if !foundOurs {
		t.Fatalf("JSON output should contain our container %s:\n%s", suffix, out)
	}
}

func TestE2E_StopRemovesContainer(t *testing.T) {
	suffix := testPrefix + "-stop"
	fullName := containerFullName("claude", suffix)

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if err != nil {
		t.Fatalf("spawn failed: %v\n%s", err, out)
	}
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	// Stop it
	out, err = runSafeAg(t, "stop", fullName)
	if err != nil {
		t.Fatalf("stop failed: %v\n%s", err, out)
	}

	// Verify container is gone
	ctx := context.Background()
	time.Sleep(2 * time.Second)
	_, err = orbExec.Run(ctx, "docker", "inspect", fullName)
	if err == nil {
		stopAndRemove(t, fullName)
		t.Fatal("container should be removed after stop")
	}

	// Verify network is also gone
	_, err = orbExec.Run(ctx, "docker", "network", "inspect", fullName+"-net")
	if err == nil {
		t.Fatal("managed network should be removed after stop")
	}
}

func TestE2E_CleanupRemovesAll(t *testing.T) {
	suffix1 := testPrefix + "-clean1"
	suffix2 := testPrefix + "-clean2"
	fullName1 := containerFullName("claude", suffix1)
	fullName2 := containerFullName("claude", suffix2)

	runSafeAg(t, "spawn", "claude",
		"--name", suffix1,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	runSafeAg(t, "spawn", "claude",
		"--name", suffix2,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if !waitForContainer(t, fullName1) {
		t.Fatal("container 1 did not appear")
	}
	if !waitForContainer(t, fullName2) {
		t.Fatal("container 2 did not appear")
	}

	// Cleanup
	out, err := runSafeAg(t, "cleanup")
	if err != nil {
		t.Fatalf("cleanup failed: %v\n%s", err, out)
	}

	// Both should be gone
	ctx := context.Background()
	time.Sleep(2 * time.Second)
	_, err = orbExec.Run(ctx, "docker", "inspect", fullName1)
	if err == nil {
		t.Fatal("container 1 should be removed after cleanup")
	}
	_, err = orbExec.Run(ctx, "docker", "inspect", fullName2)
	if err == nil {
		t.Fatal("container 2 should be removed after cleanup")
	}
}

// ─── Observability ──────────────────────────────────────────────────────────

func TestE2E_SummaryShowsInfo(t *testing.T) {
	suffix := testPrefix + "-summary"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if err != nil {
		t.Fatalf("spawn failed: %v\n%s", err, out)
	}
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	out, err = runSafeAg(t, "summary", fullName)
	if err != nil {
		t.Fatalf("summary failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "claude") {
		t.Fatalf("summary should contain 'claude':\n%s", out)
	}
	if !strings.Contains(out, fullName) {
		t.Fatalf("summary should contain container name %s:\n%s", fullName, out)
	}
	// Check for status or key fields
	if !strings.Contains(out, "Status:") {
		t.Fatalf("summary should show Status field:\n%s", out)
	}
}

func TestE2E_PeekShowsOutput(t *testing.T) {
	suffix := testPrefix + "-peek"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	// Wait a bit for tmux to start
	time.Sleep(5 * time.Second)

	out, err := runSafeAg(t, "peek", fullName, "--lines", "10")
	if err != nil {
		t.Skipf("peek failed (tmux may not be ready): %v", err)
	}
	// Just verify we got some output (even empty is fine if tmux is up)
	t.Logf("peek output (%d bytes): %s", len(out), out)
}

func TestE2E_DiffCommand(t *testing.T) {
	suffix := testPrefix + "-diff"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	// Wait for repo clone to complete
	time.Sleep(5 * time.Second)

	// diff on a fresh clone should return empty (no changes) or an error
	// if the container has already exited
	out, err := runSafeAg(t, "diff", fullName)
	if err != nil {
		t.Logf("diff returned error (container may have exited): %v\n%s", err, out)
		// Not fatal: the container may exit quickly without agent auth
	}
}

func TestE2E_AuditShowsSpawnEntry(t *testing.T) {
	suffix := testPrefix + "-audit"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	out, err := runSafeAg(t, "audit", "--lines", "10")
	if err != nil {
		t.Fatalf("audit failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "spawn") {
		t.Fatalf("audit should contain spawn entry:\n%s", out)
	}
}

// ─── Workflow Commands ──────────────────────────────────────────────────────

func TestE2E_TodoWorkflow(t *testing.T) {
	suffix := testPrefix + "-todo"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if err != nil {
		t.Fatalf("spawn failed: %v\n%s", err, out)
	}
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	// Wait for container to be ready
	time.Sleep(3 * time.Second)

	// Add a todo
	out, err = runSafeAg(t, "todo", "add", fullName, "Fix the tests")
	if err != nil {
		t.Skipf("todo add failed (container may have exited): %v\n%s", err, out)
	}
	if !strings.Contains(out, "Added") {
		t.Fatalf("todo add should confirm addition:\n%s", out)
	}

	// List todos
	out, err = runSafeAg(t, "todo", "list", fullName)
	if err != nil {
		t.Fatalf("todo list failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Fix the tests") {
		t.Fatalf("todo list should show our item:\n%s", out)
	}
	if !strings.Contains(out, "[ ]") {
		t.Fatalf("todo should be unchecked:\n%s", out)
	}

	// Check it
	out, err = runSafeAg(t, "todo", "check", fullName, "1")
	if err != nil {
		t.Fatalf("todo check failed: %v\n%s", err, out)
	}

	// Verify checked
	out, err = runSafeAg(t, "todo", "list", fullName)
	if err != nil {
		t.Fatalf("todo list failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "[x]") {
		t.Fatalf("todo should be checked:\n%s", out)
	}
}

// ─── Stop --all ─────────────────────────────────────────────────────────────

func TestE2E_StopAll(t *testing.T) {
	suffix1 := testPrefix + "-stopall1"
	suffix2 := testPrefix + "-stopall2"
	fullName1 := containerFullName("claude", suffix1)
	fullName2 := containerFullName("claude", suffix2)

	runSafeAg(t, "spawn", "claude",
		"--name", suffix1,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	runSafeAg(t, "spawn", "claude",
		"--name", suffix2,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if !waitForContainer(t, fullName1) {
		t.Fatal("container 1 did not appear")
	}
	if !waitForContainer(t, fullName2) {
		t.Fatal("container 2 did not appear")
	}

	out, err := runSafeAg(t, "stop", "--all")
	if err != nil {
		t.Fatalf("stop --all failed: %v\n%s", err, out)
	}

	ctx := context.Background()
	time.Sleep(2 * time.Second)
	_, err = orbExec.Run(ctx, "docker", "inspect", fullName1)
	if err == nil {
		stopAndRemove(t, fullName1)
		t.Fatal("container 1 should be removed after stop --all")
	}
	_, err = orbExec.Run(ctx, "docker", "inspect", fullName2)
	if err == nil {
		stopAndRemove(t, fullName2)
		t.Fatal("container 2 should be removed after stop --all")
	}
}

// ─── Docker mode label ──────────────────────────────────────────────────────

func TestE2E_SpawnDockerModeOff(t *testing.T) {
	suffix := testPrefix + "-dockeroff"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	dockerMode := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.docker"}}`)
	if dockerMode != "off" {
		t.Fatalf("docker label = %q, want off", dockerMode)
	}
}

// ─── Environment variables ──────────────────────────────────────────────────

func TestE2E_SpawnSetsEnvVars(t *testing.T) {
	suffix := testPrefix + "-env"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if err != nil {
		t.Fatalf("spawn failed: %v\n%s", err, out)
	}
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	// Check AGENT_TYPE env var
	envs := dockerInspectField(t, fullName, "{{range .Config.Env}}{{println .}}{{end}}")
	if !strings.Contains(envs, "AGENT_TYPE=claude") {
		t.Fatalf("env should contain AGENT_TYPE=claude, got:\n%s", envs)
	}
	if !strings.Contains(envs, "REPOS=https://github.com/octocat/Hello-World.git") {
		t.Fatalf("env should contain REPOS, got:\n%s", envs)
	}
}

// ─── Hostname matches container name ────────────────────────────────────────

func TestE2E_HostnameMatchesContainerName(t *testing.T) {
	suffix := testPrefix + "-hostname"
	fullName := containerFullName("claude", suffix)
	defer stopAndRemove(t, fullName)

	runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--background")
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	hostname := dockerInspectField(t, fullName, "{{.Config.Hostname}}")
	if hostname != fullName {
		t.Fatalf("hostname = %q, want %q", hostname, fullName)
	}
}

// ─── Pull policy ────────────────────────────────────────────────────────────

func TestE2E_ImagePullPolicyNever(t *testing.T) {
	// The dry-run output should contain --pull never
	suffix := testPrefix + "-pullnever"
	out, err := runSafeAg(t, "spawn", "claude",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--dry-run")
	if err != nil {
		t.Fatalf("dry-run failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "--pull never") {
		t.Fatalf("should contain --pull never:\n%s", out)
	}
}

// ─── Helper ─────────────────────────────────────────────────────────────────

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
