package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "defaults-*.sh")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	return f.Name()
}

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.CPUs != "4" {
		t.Errorf("default CPUs should be '4', got %q", d.CPUs)
	}
	if d.Memory != "8g" {
		t.Errorf("default Memory should be '8g', got %q", d.Memory)
	}
	if d.PIDsLimit != "512" {
		t.Errorf("default PIDsLimit should be '512', got %q", d.PIDsLimit)
	}
}

func TestLoadDefaults_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.sh")
	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("LoadDefaults with missing file should return no error, got: %v", err)
	}
	// Missing file returns hardcoded defaults
	if cfg.CPUs != "4" {
		t.Errorf("expected default CPUs '4' for missing file, got %q", cfg.CPUs)
	}
}

func TestLoadDefaults_Values(t *testing.T) {
	content := `
SAFE_AGENTIC_DEFAULT_CPUS=8
SAFE_AGENTIC_DEFAULT_MEMORY=16g
SAFE_AGENTIC_DEFAULT_PIDS_LIMIT=1024
SAFE_AGENTIC_DEFAULT_SSH=true
SAFE_AGENTIC_DEFAULT_NETWORK=agent-net
GIT_AUTHOR_NAME="Agent Bot"
GIT_AUTHOR_EMAIL=bot@example.com
GIT_COMMITTER_NAME="Agent Bot"
GIT_COMMITTER_EMAIL=bot@example.com
`
	path := writeTemp(t, content)
	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CPUs != "8" {
		t.Errorf("CPUs = %q, want %q", cfg.CPUs, "8")
	}
	if cfg.Memory != "16g" {
		t.Errorf("Memory = %q, want %q", cfg.Memory, "16g")
	}
	if cfg.PIDsLimit != "1024" {
		t.Errorf("PIDsLimit = %q, want %q", cfg.PIDsLimit, "1024")
	}
	if cfg.SSH != "true" {
		t.Errorf("SSH = %q, want %q", cfg.SSH, "true")
	}
	if cfg.Network != "agent-net" {
		t.Errorf("Network = %q, want %q", cfg.Network, "agent-net")
	}
	if cfg.GitAuthorName != "Agent Bot" {
		t.Errorf("GitAuthorName = %q, want %q", cfg.GitAuthorName, "Agent Bot")
	}
	if cfg.GitAuthorEmail != "bot@example.com" {
		t.Errorf("GitAuthorEmail = %q, want %q", cfg.GitAuthorEmail, "bot@example.com")
	}
	if cfg.GitCommitterName != "Agent Bot" {
		t.Errorf("GitCommitterName = %q, want %q", cfg.GitCommitterName, "Agent Bot")
	}
	if cfg.GitCommitterEmail != "bot@example.com" {
		t.Errorf("GitCommitterEmail = %q, want %q", cfg.GitCommitterEmail, "bot@example.com")
	}
}

func TestLoadDefaults_QuotedValues(t *testing.T) {
	content := `
SAFE_AGENTIC_DEFAULT_MEMORY="16g"
SAFE_AGENTIC_DEFAULT_IDENTITY='Agent Bot <bot@example.com>'
GIT_AUTHOR_NAME="Agent \"Bot\""
`
	path := writeTemp(t, content)
	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Memory != "16g" {
		t.Errorf("Memory = %q, want %q", cfg.Memory, "16g")
	}
	if cfg.Identity != "Agent Bot <bot@example.com>" {
		t.Errorf("Identity = %q, want %q", cfg.Identity, "Agent Bot <bot@example.com>")
	}
	if cfg.GitAuthorName != `Agent "Bot"` {
		t.Errorf("GitAuthorName = %q, want %q", cfg.GitAuthorName, `Agent "Bot"`)
	}
}

func TestLoadDefaults_CommentsAndBlanks(t *testing.T) {
	content := `
# This is a comment
# SAFE_AGENTIC_DEFAULT_CPUS=ignored

SAFE_AGENTIC_DEFAULT_CPUS=4

# another comment
SAFE_AGENTIC_DEFAULT_MEMORY=8g
`
	path := writeTemp(t, content)
	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CPUs != "4" {
		t.Errorf("CPUs = %q, want %q", cfg.CPUs, "4")
	}
	if cfg.Memory != "8g" {
		t.Errorf("Memory = %q, want %q", cfg.Memory, "8g")
	}
}

func TestLoadDefaults_ExportPrefix(t *testing.T) {
	content := `export SAFE_AGENTIC_DEFAULT_CPUS=6
export GIT_AUTHOR_NAME="Exported Bot"
`
	path := writeTemp(t, content)
	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CPUs != "6" {
		t.Errorf("CPUs = %q, want %q", cfg.CPUs, "6")
	}
	if cfg.GitAuthorName != "Exported Bot" {
		t.Errorf("GitAuthorName = %q, want %q", cfg.GitAuthorName, "Exported Bot")
	}
}

func TestLoadDefaults_UnknownKeyRejected(t *testing.T) {
	content := `UNKNOWN_KEY=value
`
	path := writeTemp(t, content)
	_, err := LoadDefaults(path)
	if err == nil {
		t.Error("expected error for unknown key, got nil")
	}
}

func TestLoadDefaults_WhitespaceInBareValue(t *testing.T) {
	content := `SAFE_AGENTIC_DEFAULT_CPUS=has space
`
	path := writeTemp(t, content)
	_, err := LoadDefaults(path)
	if err == nil {
		t.Error("expected error for bare value with whitespace, got nil")
	}
}

func TestLoadDefaults_NoEqualsSign(t *testing.T) {
	// A line with no '=' sign should return an error.
	content := "SAFE_AGENTIC_DEFAULT_CPUS\n"
	path := writeTemp(t, content)
	_, err := LoadDefaults(path)
	if err == nil {
		t.Error("expected error for line without '=', got nil")
	}
}

func TestLoadDefaults_OpenError(t *testing.T) {
	// Create a file and remove read permission so Open fails with a non-IsNotExist error.
	dir := t.TempDir()
	path := filepath.Join(dir, "defaults.sh")
	if err := os.WriteFile(path, []byte("SAFE_AGENTIC_DEFAULT_CPUS=4\n"), 0o000); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := LoadDefaults(path)
	if err == nil {
		t.Error("expected error opening unreadable file, got nil")
	}
}

func TestKeyAllowed(t *testing.T) {
	allowed := []string{
		"SAFE_AGENTIC_DEFAULT_CPUS",
		"SAFE_AGENTIC_DEFAULT_MEMORY",
		"SAFE_AGENTIC_DEFAULT_PIDS_LIMIT",
		"SAFE_AGENTIC_DEFAULT_SSH",
		"SAFE_AGENTIC_DEFAULT_DOCKER",
		"SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET",
		"SAFE_AGENTIC_DEFAULT_REUSE_AUTH",
		"SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH",
		"SAFE_AGENTIC_DEFAULT_NETWORK",
		"SAFE_AGENTIC_DEFAULT_IDENTITY",
		"GIT_AUTHOR_NAME",
		"GIT_AUTHOR_EMAIL",
		"GIT_COMMITTER_NAME",
		"GIT_COMMITTER_EMAIL",
	}
	for _, k := range allowed {
		if !keyAllowed(k) {
			t.Errorf("keyAllowed(%q) = false, want true", k)
		}
	}

	notAllowed := []string{
		"",
		"SAFE_AGENTIC_DEFAULT_UNKNOWN",
		"HOME",
		"PATH",
		"GIT_AUTHOR_UNKNOWN",
		"SOME_RANDOM_KEY",
	}
	for _, k := range notAllowed {
		if keyAllowed(k) {
			t.Errorf("keyAllowed(%q) = true, want false", k)
		}
	}
}

func TestDefaultsPath(t *testing.T) {
	// Test with explicit XDG_CONFIG_HOME
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := DefaultsPath()
	expected := filepath.Join(dir, "safe-agentic", "defaults.sh")
	if path != expected {
		t.Errorf("DefaultsPath() = %q, want %q", path, expected)
	}
}

func TestDefaultsPath_FallbackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	path := DefaultsPath()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "safe-agentic", "defaults.sh")
	if path != expected {
		t.Errorf("DefaultsPath() = %q, want %q", path, expected)
	}
}

func TestLoadDefaults_AllDefaultKeys(t *testing.T) {
	content := `SAFE_AGENTIC_DEFAULT_CPUS=2
SAFE_AGENTIC_DEFAULT_MEMORY=4g
SAFE_AGENTIC_DEFAULT_PIDS_LIMIT=512
SAFE_AGENTIC_DEFAULT_SSH=false
SAFE_AGENTIC_DEFAULT_DOCKER=true
SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET=false
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=true
SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH=false
SAFE_AGENTIC_DEFAULT_NETWORK=my-net
SAFE_AGENTIC_DEFAULT_IDENTITY='Bot <bot@example.com>'
`
	path := writeTemp(t, content)
	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CPUs != "2" {
		t.Errorf("CPUs = %q, want %q", cfg.CPUs, "2")
	}
	if cfg.Memory != "4g" {
		t.Errorf("Memory = %q, want %q", cfg.Memory, "4g")
	}
	if cfg.PIDsLimit != "512" {
		t.Errorf("PIDsLimit = %q, want %q", cfg.PIDsLimit, "512")
	}
	if cfg.SSH != "false" {
		t.Errorf("SSH = %q, want %q", cfg.SSH, "false")
	}
	if cfg.Docker != "true" {
		t.Errorf("Docker = %q, want %q", cfg.Docker, "true")
	}
	if cfg.DockerSocket != "false" {
		t.Errorf("DockerSocket = %q, want %q", cfg.DockerSocket, "false")
	}
	if cfg.ReuseAuth != "true" {
		t.Errorf("ReuseAuth = %q, want %q", cfg.ReuseAuth, "true")
	}
	if cfg.ReuseGHAuth != "false" {
		t.Errorf("ReuseGHAuth = %q, want %q", cfg.ReuseGHAuth, "false")
	}
	if cfg.Network != "my-net" {
		t.Errorf("Network = %q, want %q", cfg.Network, "my-net")
	}
	if cfg.Identity != "Bot <bot@example.com>" {
		t.Errorf("Identity = %q, want %q", cfg.Identity, "Bot <bot@example.com>")
	}
}
