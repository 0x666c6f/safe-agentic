//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requireDeepIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("SAFE_AGENTIC_DEEP_INTEGRATION") != "1" {
		t.Skip("set SAFE_AGENTIC_DEEP_INTEGRATION=1 to run deep live integration cases")
	}
}

func cleanupNamedArtifacts(name string) {
	ctx := context.Background()
	orbExec.Run(ctx, "docker", "rm", "-f", name)
	orbExec.Run(ctx, "docker", "rm", "-f", "safe-agentic-docker-"+name)
	orbExec.Run(ctx, "docker", "network", "rm", name+"-net")
	orbExec.Run(ctx, "docker", "volume", "rm", name+"-docker-sock")
	orbExec.Run(ctx, "docker", "volume", "rm", name+"-docker-data")
	orbExec.Run(ctx, "docker", "volume", "rm", name+"-auth")
}

func runSafeAgEnv(t *testing.T, env map[string]string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "SAFE_AGENTIC_VM_NAME="+testVMName)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func dockerInspectAllEnv(t *testing.T, name string) string {
	t.Helper()
	return dockerInspectField(t, name, "{{range .Config.Env}}{{println .}}{{end}}")
}

func dockerInspectMountsJSON(t *testing.T, name string) string {
	t.Helper()
	return dockerInspectField(t, name, "{{json .Mounts}}")
}

func dockerInspectTmpfsJSON(t *testing.T, name string) string {
	t.Helper()
	return dockerInspectField(t, name, "{{json .HostConfig.Tmpfs}}")
}

func ensureTUIBinary(t *testing.T) string {
	t.Helper()
	tuiPath := filepath.Join(filepath.Dir(binaryPath), "safe-ag-tui")
	if _, err := os.Stat(tuiPath); err == nil {
		return tuiPath
	}
	build := exec.Command("go", "build", "-o", "../../bin/safe-ag-tui", "../../tui")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build safe-ag-tui failed: %v\n%s", err, out)
	}
	return tuiPath
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitHTTPContains(t *testing.T, url, needle string) string {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	var body string
	for i := 0; i < 30; i++ {
		resp, err := client.Get(url)
		if err == nil {
			data, _ := ioReadAllAndClose(resp)
			body = string(data)
			if strings.Contains(body, needle) {
				return body
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("url %s never contained %q; last body:\n%s", url, needle, body)
	return ""
}

func ioReadAllAndClose(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func waitForDockerLogs(t *testing.T, name, needle string) string {
	t.Helper()
	ctx := context.Background()
	var logs string
	for i := 0; i < 30; i++ {
		out, _ := orbExec.Run(ctx, "docker", "logs", name)
		logs = string(out)
		if strings.Contains(logs, needle) {
			return logs
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("docker logs for %s never contained %q:\n%s", name, needle, logs)
	return ""
}

func TestE2E_DashboardHTTPListsAndStopsAgent(t *testing.T) {
	requireDeepIntegration(t)
	ensureTUIBinary(t)
	tempName := "agent-shell-e2e-dashboard"
	startTempShellContainer(t, tempName)
	defer orbExec.Run(context.Background(), "docker", "rm", "-f", tempName)

	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryPath, "dashboard", "--bind", addr)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dashboard: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	apiURL := "http://" + addr + "/api/agents"
	body := waitHTTPContains(t, apiURL, tempName)
	var agents []map[string]any
	if err := json.Unmarshal([]byte(body), &agents); err != nil {
		t.Fatalf("dashboard api json: %v\n%s", err, body)
	}

	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/api/agents/stop/"+tempName, nil)
	if err != nil {
		t.Fatalf("build stop request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST stop via dashboard: %v", err)
	}
	resp.Body.Close()
	for i := 0; i < 20; i++ {
		if state := dockerInspectField(t, tempName, "{{.State.Status}}"); state != "running" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("dashboard stop should stop agent")
}

func TestE2E_SpawnInjectsHostConfigsAndAuthMounts(t *testing.T) {
	requireDeepIntegration(t)
	suffix := testPrefix + "-configinject"
	fullName := containerFullName("shell", suffix)
	cleanupNamedArtifacts(fullName)
	defer stopAndRemove(t, fullName)

	tmp := t.TempDir()
	claudeDir := filepath.Join(tmp, "claude")
	codexDir := filepath.Join(tmp, "codex")
	if err := os.MkdirAll(filepath.Join(claudeDir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"env":{"A":"B"}}`), 0o644)
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("host claude doc"), 0o644)
	os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte("approval_policy = \"never\"\n"), 0o644)

	out, err := runSafeAgEnv(t, map[string]string{
		"CLAUDE_CONFIG_DIR": claudeDir,
		"CODEX_HOME":        codexDir,
	}, "spawn", "shell",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--reuse-auth",
		"--reuse-gh-auth",
		"--background")
	if err != nil {
		t.Fatalf("spawn shell with config injection failed: %v\n%s", err, out)
	}
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	envs := dockerInspectAllEnv(t, fullName)
	for _, key := range []string{
		"SAFE_AGENTIC_CLAUDE_CONFIG_B64=",
		"SAFE_AGENTIC_CLAUDE_SUPPORT_B64=",
		"SAFE_AGENTIC_CODEX_CONFIG_B64=",
	} {
		if !strings.Contains(envs, key) {
			t.Fatalf("expected env %s in container envs:\n%s", key, envs)
		}
	}

	mounts := dockerInspectMountsJSON(t, fullName)
	if !strings.Contains(mounts, "safe-agentic-shell-auth") {
		t.Fatalf("shared auth volume missing:\n%s", mounts)
	}
	if !strings.Contains(mounts, "safe-agentic-shell-gh-auth") {
		t.Fatalf("shared gh auth volume missing:\n%s", mounts)
	}
}

func TestE2E_SpawnAWSInjectsEnvAndTmpfs(t *testing.T) {
	requireDeepIntegration(t)
	suffix := testPrefix + "-aws"
	fullName := containerFullName("shell", suffix)
	cleanupNamedArtifacts(fullName)
	defer stopAndRemove(t, fullName)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	awsDir := filepath.Join(home, ".aws")
	if err := os.MkdirAll(awsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	credPath := filepath.Join(awsDir, "credentials")
	var backup []byte
	if existing, err := os.ReadFile(credPath); err == nil {
		backup = existing
	}
	defer func() {
		if backup != nil {
			_ = os.WriteFile(credPath, backup, 0o600)
		} else {
			_ = os.Remove(credPath)
		}
	}()
	creds := "[integ]\naws_access_key_id = test\naws_secret_access_key = secret\n"
	if err := os.WriteFile(credPath, []byte(creds), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runSafeAg(t, "spawn", "shell",
		"--name", suffix,
		"--repo", "https://github.com/octocat/Hello-World.git",
		"--aws", "integ",
		"--background")
	if err != nil {
		t.Fatalf("spawn shell with aws failed: %v\n%s", err, out)
	}
	if !waitForContainer(t, fullName) {
		t.Fatal("container did not appear")
	}

	envs := dockerInspectAllEnv(t, fullName)
	if !strings.Contains(envs, "SAFE_AGENTIC_AWS_CREDS_B64=") || !strings.Contains(envs, "AWS_PROFILE=integ") {
		t.Fatalf("expected aws env injection:\n%s", envs)
	}
	tmpfs := dockerInspectTmpfsJSON(t, fullName)
	if !strings.Contains(tmpfs, "/home/agent/.aws") {
		t.Fatalf("expected /home/agent/.aws tmpfs:\n%s", tmpfs)
	}
}

func TestE2E_SpawnDockerModes(t *testing.T) {
	requireDeepIntegration(t)
	t.Run("DinD", func(t *testing.T) {
		suffix := testPrefix + "-dind"
		fullName := containerFullName("shell", suffix)
		dindName := "safe-agentic-docker-" + fullName
		cleanupNamedArtifacts(fullName)
		defer stopAndRemove(t, fullName)
		defer orbExec.Run(context.Background(), "docker", "rm", "-f", dindName)

		out, err := runSafeAg(t, "spawn", "shell",
			"--name", suffix,
			"--repo", "https://github.com/octocat/Hello-World.git",
			"--docker",
			"--background")
		if err != nil {
			if strings.Contains(out, "privileged mode is incompatible with user namespaces") {
				return
			}
			t.Fatalf("spawn shell --docker failed: %v\n%s", err, out)
		}
		if !waitForContainer(t, fullName) {
			t.Fatal("parent container did not appear")
		}
		if !waitForContainer(t, dindName) {
			t.Fatal("dind sidecar did not appear")
		}
		if got := dockerInspectField(t, fullName, `{{index .Config.Labels "safe-agentic.docker"}}`); got != "dind" {
			t.Fatalf("docker label = %q, want dind", got)
		}
		if _, err := orbExec.Run(context.Background(), "docker", "exec", dindName, "docker", "info"); err != nil {
			t.Fatalf("dind docker info failed: %v", err)
		}
	})

	t.Run("HostSocket", func(t *testing.T) {
		suffix := testPrefix + "-hostsock"
		fullName := containerFullName("shell", suffix)
		cleanupNamedArtifacts(fullName)
		defer stopAndRemove(t, fullName)

		out, err := runSafeAg(t, "spawn", "shell",
			"--name", suffix,
			"--repo", "https://github.com/octocat/Hello-World.git",
			"--docker-socket",
			"--background")
		if err != nil {
			t.Fatalf("spawn shell --docker-socket failed: %v\n%s", err, out)
		}
		if !waitForContainer(t, fullName) {
			t.Fatal("container did not appear")
		}
		envs := dockerInspectAllEnv(t, fullName)
		if !strings.Contains(envs, "DOCKER_HOST=unix:///run/docker-host.sock") {
			t.Fatalf("expected DOCKER_HOST env:\n%s", envs)
		}
		mounts := dockerInspectMountsJSON(t, fullName)
		if !strings.Contains(mounts, "/var/run/docker.sock") || !strings.Contains(mounts, "/run/docker-host.sock") {
			t.Fatalf("expected docker socket bind mount:\n%s", mounts)
		}
	})
}

func TestE2E_LiveFleetAndPipelineExecution(t *testing.T) {
	requireDeepIntegration(t)
	t.Run("FleetShellManifest", func(t *testing.T) {
		dir := t.TempDir()
		manifest := filepath.Join(dir, "fleet.yaml")
		data := `name: integ-shell-fleet
agents:
  - name: fleet-one
    type: shell
    repo: https://github.com/octocat/Hello-World.git
  - name: fleet-two
    type: shell
    repo: https://github.com/octocat/Hello-World.git
`
		if err := os.WriteFile(manifest, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"agent-shell-fleet-one", "agent-shell-fleet-two"} {
			cleanupNamedArtifacts(name)
		}

		out, err := runSafeAg(t, "fleet", manifest)
		if err != nil {
			t.Fatalf("fleet run failed: %v\n%s", err, out)
		}
		if !strings.Contains(out, "Fleet volume:") || !strings.Contains(out, "Fleet spawned 2 agent(s).") {
			t.Fatalf("unexpected fleet output:\n%s", out)
		}
		for _, name := range []string{"agent-shell-fleet-one", "agent-shell-fleet-two"} {
			cleanupNamedArtifacts(name)
			defer stopAndRemove(t, name)
			if !waitForContainer(t, name) {
				t.Fatalf("fleet container %s did not appear", name)
			}
		}
	})

	t.Run("PipelineShellManifest", func(t *testing.T) {
		dir := t.TempDir()
		child := filepath.Join(dir, "child.yaml")
		parent := filepath.Join(dir, "parent.yaml")
		if err := os.WriteFile(child, []byte(`name: child-pipe
steps:
  - name: child-step
    type: shell
    repo: https://github.com/octocat/Hello-World.git
`), 0o644); err != nil {
			t.Fatal(err)
		}
		parentBody := fmt.Sprintf(`name: integ-shell-pipeline
stages:
  - name: root-step
    agents:
      - name: root-step
        type: shell
        repo: https://github.com/octocat/Hello-World.git
  - name: nested
    depends_on: [root-step]
    pipeline: %q
`, child)
		if err := os.WriteFile(parent, []byte(parentBody), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"agent-shell-root-step", "agent-shell-child-step"} {
			cleanupNamedArtifacts(name)
		}

		out, err := runSafeAg(t, "pipeline", parent)
		if err != nil {
			t.Fatalf("pipeline run failed: %v\n%s", err, out)
		}
		if !strings.Contains(out, `Pipeline "integ-shell-pipeline" complete.`) {
			t.Fatalf("unexpected pipeline output:\n%s", out)
		}
		for _, name := range []string{"agent-shell-root-step", "agent-shell-child-step"} {
			cleanupNamedArtifacts(name)
			defer stopAndRemove(t, name)
			if !waitForContainer(t, name) {
				t.Fatalf("pipeline container %s did not appear", name)
			}
		}
	})
}

func TestE2E_ClaudeAndCodexStartupLogs(t *testing.T) {
	requireDeepIntegration(t)
	cases := []struct {
		agentType string
	}{
		{agentType: "claude"},
		{agentType: "codex"},
	}

	for _, tc := range cases {
		t.Run(tc.agentType, func(t *testing.T) {
			suffix := testPrefix + "-" + tc.agentType + "-startup"
			fullName := containerFullName(tc.agentType, suffix)
			cleanupNamedArtifacts(fullName)
			defer stopAndRemove(t, fullName)

			out, err := runSafeAg(t, "spawn", tc.agentType,
				"--name", suffix,
				"--repo", "https://github.com/octocat/Hello-World.git",
				"--auto-trust",
				"--background")
			if err != nil {
				t.Fatalf("spawn %s failed: %v\n%s", tc.agentType, err, out)
			}
			if !waitForContainer(t, fullName) {
				t.Fatalf("%s container did not appear", tc.agentType)
			}
			logs := waitForDockerLogs(t, fullName, "[entrypoint] Cloning https://github.com/octocat/Hello-World.git")
			if strings.Contains(logs, "Do you trust this project") {
				t.Fatalf("%s logs should not contain trust prompt:\n%s", tc.agentType, logs)
			}
		})
	}
}
