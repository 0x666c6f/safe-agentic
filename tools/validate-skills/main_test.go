package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidatesMirroredSkills(t *testing.T) {
	root := t.TempDir()
	chdir(t, root)
	writeSkill(t, ".claude/skills/agent-spawn", "agent-spawn", "Spawn agents")
	writeSkill(t, ".codex/skills/agent-spawn", "agent-spawn", "Spawn agents")

	if err := run(); err != nil {
		t.Fatalf("run() error = %v", err)
	}
}

func TestSkillPairsDetectsRootDrift(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, ".claude/skills/a"), "a", "A")
	writeSkill(t, filepath.Join(root, ".codex/skills/b"), "b", "B")

	_, err := skillPairs(filepath.Join(root, ".claude/skills"), filepath.Join(root, ".codex/skills"))
	if err == nil || !strings.Contains(err.Error(), "skill roots differ") {
		t.Fatalf("skillPairs() error = %v, want root drift", err)
	}
}

func TestCompareDirsDetectsFileAndContentDrift(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	mkdir(t, left)
	mkdir(t, right)
	writeFile(t, filepath.Join(left, "SKILL.md"), "same")
	writeFile(t, filepath.Join(right, "SKILL.md"), "different")

	err := compareDirs(left, right)
	if err == nil || !strings.Contains(err.Error(), "skill mirror drift") {
		t.Fatalf("compareDirs() error = %v, want content drift", err)
	}

	writeFile(t, filepath.Join(left, "extra.md"), "extra")
	err = compareDirs(left, right)
	if err == nil || !strings.Contains(err.Error(), "skill file lists differ") {
		t.Fatalf("compareDirs() error = %v, want file list drift", err)
	}
}

func TestValidateSkillDirRejectsBadFrontmatterAndYAML(t *testing.T) {
	root := t.TempDir()
	missingDescription := filepath.Join(root, "missing-description")
	mkdir(t, missingDescription)
	writeFile(t, filepath.Join(missingDescription, "SKILL.md"), "---\nname: bad\n---\n")
	err := validateSkillDir(missingDescription)
	if err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("validateSkillDir() error = %v, want missing description", err)
	}

	badYAML := filepath.Join(root, "bad-yaml")
	writeSkill(t, badYAML, "bad-yaml", "Bad YAML")
	writeFile(t, filepath.Join(badYAML, "agents/openai.yaml"), "name: [unterminated\n")
	err = validateSkillDir(badYAML)
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("validateSkillDir() error = %v, want YAML parse error", err)
	}
}

func TestRegularFilesRejectsNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	mkdir(t, root)
	target := filepath.Join(root, "target")
	writeFile(t, target, "x")
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := regularFiles(root)
	if err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("regularFiles() error = %v, want non-regular", err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}

func writeSkill(t *testing.T, dir, name, description string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+name+"\ndescription: "+description+"\n---\n")
	writeFile(t, filepath.Join(dir, "agents/openai.yaml"), "name: "+name+"\n")
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
