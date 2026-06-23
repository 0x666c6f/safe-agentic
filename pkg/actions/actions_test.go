package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFilesMergesAndProjectOverridesUser(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "user.toml")
	projectPath := filepath.Join(dir, "project.toml")

	if err := os.WriteFile(userPath, []byte(`
[actions.test]
description = "user test"
command = "go test ./..."

[actions.lint]
command = "golangci-lint run"
`), 0o600); err != nil {
		t.Fatalf("write user actions: %v", err)
	}
	if err := os.WriteFile(projectPath, []byte(`
[actions.test]
description = "project test"
command = "go test ./cmd/safe-ag"
cwd = "cmd/safe-ag"
`), 0o600); err != nil {
		t.Fatalf("write project actions: %v", err)
	}

	catalog, err := LoadFiles([]string{userPath, projectPath})
	if err != nil {
		t.Fatalf("LoadFiles() error = %v", err)
	}
	if len(catalog.Actions) != 2 {
		t.Fatalf("actions len = %d, want 2", len(catalog.Actions))
	}
	action, ok := catalog.Get("test")
	if !ok {
		t.Fatal("missing test action")
	}
	if action.Description != "project test" || action.Command != "go test ./cmd/safe-ag" || action.CWD != "cmd/safe-ag" {
		t.Fatalf("project override not applied: %#v", action)
	}
	if action.Source != projectPath {
		t.Fatalf("source = %q, want %q", action.Source, projectPath)
	}
}

func TestLoadFilesRejectsInvalidAction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.toml")
	if err := os.WriteFile(path, []byte(`
[actions."bad name"]
command = "echo no"
`), 0o600); err != nil {
		t.Fatalf("write actions: %v", err)
	}

	_, err := LoadFiles([]string{path})
	if err == nil {
		t.Fatal("expected invalid action error")
	}
}

func TestLoadFilesRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "actions.toml")
	if err := os.WriteFile(path, []byte(`
[actions.test]
command = "echo ok"
bogus = true
`), 0o600); err != nil {
		t.Fatalf("write actions: %v", err)
	}

	_, err := LoadFiles([]string{path})
	if err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestLoadFilesRejectsEscapingCWD(t *testing.T) {
	for _, cwd := range []string{"/tmp", "../outside", "sub/../../outside"} {
		t.Run(cwd, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "actions.toml")
			if err := os.WriteFile(path, []byte(`
[actions.test]
command = "echo ok"
cwd = "`+cwd+`"
`), 0o600); err != nil {
				t.Fatalf("write actions: %v", err)
			}

			_, err := LoadFiles([]string{path})
			if err == nil {
				t.Fatal("expected invalid cwd error")
			}
		})
	}
}

func TestLoadFilesMissingFilesAreIgnored(t *testing.T) {
	catalog, err := LoadFiles([]string{filepath.Join(t.TempDir(), "missing.toml")})
	if err != nil {
		t.Fatalf("LoadFiles() error = %v", err)
	}
	if len(catalog.Actions) != 0 {
		t.Fatalf("actions len = %d, want 0", len(catalog.Actions))
	}
}
