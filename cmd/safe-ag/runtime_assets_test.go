package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimePathsIncludeGoBinary(t *testing.T) {
	dockerfile, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	if !strings.Contains(string(dockerfile), `ENV PATH="/usr/local/go/bin:`) {
		t.Fatalf("Dockerfile PATH is missing /usr/local/go/bin")
	}

	bashrc, err := os.ReadFile(filepath.Join("..", "..", "config", "bashrc"))
	if err != nil {
		t.Fatalf("read config/bashrc: %v", err)
	}
	if !strings.Contains(string(bashrc), `export PATH="/usr/local/go/bin:`) {
		t.Fatalf("config/bashrc PATH is missing /usr/local/go/bin")
	}
}

func TestAgentSessionDoesNotResumeFleetOrBackgroundRuns(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "bin", "agent-session.sh"))
	if err != nil {
		t.Fatalf("read bin/agent-session.sh: %v", err)
	}
	content := string(script)
	if !strings.Contains(content, `[ "${SAFE_AGENTIC_BACKGROUND:-}" != "1" ]`) {
		t.Fatalf("agent-session.sh missing background resume guard")
	}
	if !strings.Contains(content, `[ "${SAFE_AGENTIC_FLEET:-}" != "1" ]`) {
		t.Fatalf("agent-session.sh missing fleet resume guard")
	}
}

func TestEntrypointPromptEnvOnlyUsedWithoutLaunchArgs(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "entrypoint.sh"))
	if err != nil {
		t.Fatalf("read entrypoint.sh: %v", err)
	}
	content := string(script)
	if !strings.Contains(content, `[ "${#launch_args[@]}" -eq 0 ]`) {
		t.Fatalf("entrypoint prompt env guard missing launch_args check")
	}
}

func TestEntrypointConfiguresGitHubCredentialHelper(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "entrypoint.sh"))
	if err != nil {
		t.Fatalf("read entrypoint.sh: %v", err)
	}
	content := string(script)
	if !strings.Contains(content, `gh auth setup-git -h github.com`) {
		t.Fatalf("entrypoint must configure gh git credentials before cloning private HTTPS repos")
	}
}

func TestEntrypointExtractsCodexSupportFiles(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "entrypoint.sh"))
	if err != nil {
		t.Fatalf("read entrypoint.sh: %v", err)
	}
	content := string(script)
	if !strings.Contains(content, `SAFE_AGENTIC_CODEX_SUPPORT_B64`) {
		t.Fatalf("entrypoint must extract Codex support files such as agents/*.toml")
	}
}

func TestDockerfileStripsVaultFileCapability(t *testing.T) {
	dockerfile, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	if !strings.Contains(string(dockerfile), `setcap -r /usr/bin/vault`) {
		t.Fatalf("Dockerfile must strip vault cap_ipc_lock so no-new-privileges containers can exec the CLI")
	}
}

func TestVMSetupRetriesDockerStart(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "vm", "setup.sh"))
	if err != nil {
		t.Fatalf("read vm/setup.sh: %v", err)
	}
	content := string(script)
	for _, want := range []string{
		"wait_for_docker_process_exit",
		"start_dockerd_once",
		"Docker did not become ready",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("vm/setup.sh missing Docker retry marker %q", want)
		}
	}
}
