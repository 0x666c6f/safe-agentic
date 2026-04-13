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
