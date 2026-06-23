package profiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirsMergesProjectOverridesUser(t *testing.T) {
	dir := t.TempDir()
	userDir := filepath.Join(dir, "user")
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatalf("mkdir user: %v", err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "reviewer.toml"), []byte(`
agent_type = "codex"
repos = ["git@github.com:org/repo.git"]
prompt = "Review this"
ssh = true
reuse_auth = true
`), 0o600); err != nil {
		t.Fatalf("write user profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "reviewer.toml"), []byte(`
agent_type = "claude"
repo = ["https://github.com/org/project.git"]
prompt = "Project review"
background = true
`), 0o600); err != nil {
		t.Fatalf("write project profile: %v", err)
	}

	catalog, err := LoadDirs([]string{userDir, projectDir})
	if err != nil {
		t.Fatalf("LoadDirs() error = %v", err)
	}
	if len(catalog.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(catalog.Profiles))
	}
	profile, ok := catalog.Get("reviewer")
	if !ok {
		t.Fatal("missing reviewer")
	}
	if profile.AgentType != "claude" || profile.Prompt != "Project review" || len(profile.Repos) != 1 {
		t.Fatalf("project override not applied: %#v", profile)
	}
	if profile.Background == nil || !*profile.Background {
		t.Fatalf("background not decoded: %#v", profile.Background)
	}
	if profile.Source != filepath.Join(projectDir, "reviewer.toml") {
		t.Fatalf("source = %q", profile.Source)
	}
}

func TestLoadFileRejectsUnsupportedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(path, []byte("unknown = true\n"), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected unsupported key error")
	}
}

func TestLoadDirsMissingIgnored(t *testing.T) {
	catalog, err := LoadDirs([]string{filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("LoadDirs() error = %v", err)
	}
	if len(catalog.Profiles) != 0 {
		t.Fatalf("profiles len = %d, want 0", len(catalog.Profiles))
	}
}
