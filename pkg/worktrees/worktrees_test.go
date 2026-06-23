package worktrees

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareCreatesWorktreeAndCopiesIncludes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := initRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".safe-aginclude"), []byte("ignored.env\nnested/*.local\n"), 0o600); err != nil {
		t.Fatalf("write include file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "ignored.env"), []byte("TOKEN=test\n"), 0o600); err != nil {
		t.Fatalf("write ignored.env: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "nested", "app.local"), []byte("local=true\n"), 0o600); err != nil {
		t.Fatalf("write nested include: %v", err)
	}

	path := filepath.Join(t.TempDir(), "agent-worktree")
	wt, err := Prepare(Options{
		RepoRoot:      repo,
		ContainerName: "agent-claude-test",
		Path:          path,
		Branch:        "safe-ag/test",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if wt.Path != path || wt.Branch != "safe-ag/test" {
		t.Fatalf("worktree = %#v", wt)
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		t.Fatalf("worktree .git missing: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(path, "ignored.env")); err != nil || !strings.Contains(string(data), "TOKEN=test") {
		t.Fatalf("ignored.env copy data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(path, "nested", "app.local")); err != nil || !strings.Contains(string(data), "local=true") {
		t.Fatalf("nested include data=%q err=%v", data, err)
	}
	entries, err := ReadRegistry(RegistryPath())
	if err != nil {
		t.Fatalf("ReadRegistry() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Container != "agent-claude-test" {
		t.Fatalf("registry entries = %#v", entries)
	}
}

func TestPrepareDryRunDoesNotCreatePath(t *testing.T) {
	repo := initRepo(t)
	path := filepath.Join(t.TempDir(), "dry")
	wt, err := Prepare(Options{
		RepoRoot:      repo,
		ContainerName: "agent-claude-dry",
		Path:          path,
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("Prepare(dry) error = %v", err)
	}
	if wt.Path != path || wt.Branch == "" {
		t.Fatalf("dry worktree = %#v", wt)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("dry run created path or unexpected error: %v", err)
	}
}

func TestPrepareReportsBranchConflict(t *testing.T) {
	repo := initRepo(t)
	runGit(t, repo, "branch", "safe-ag/conflict")
	_, err := Prepare(Options{
		RepoRoot:      repo,
		ContainerName: "agent-claude-conflict",
		Path:          filepath.Join(t.TempDir(), "conflict"),
		Branch:        "safe-ag/conflict",
	})
	if err == nil || !strings.Contains(err.Error(), "git worktree add") {
		t.Fatalf("Prepare() error = %v, want git worktree add branch conflict", err)
	}
}

func TestPrepareRejectsUnsafeBranchSuffix(t *testing.T) {
	repo := initRepo(t)
	for _, branch := range []string{"feature.", "feature/"} {
		t.Run(branch, func(t *testing.T) {
			_, err := Prepare(Options{
				RepoRoot:      repo,
				ContainerName: "agent-claude-branch",
				Path:          filepath.Join(t.TempDir(), "branch"),
				Branch:        branch,
				DryRun:        true,
			})
			if err == nil || !strings.Contains(err.Error(), "invalid worktree branch") {
				t.Fatalf("Prepare() error = %v, want invalid branch", err)
			}
		})
	}
}

func TestCopyIncludesRejectsEscapesAndSymlinks(t *testing.T) {
	repo := initRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatalf("mkdir wt: %v", err)
	}
	include := filepath.Join(repo, ".safe-aginclude")
	if err := os.WriteFile(include, []byte("../outside\n"), 0o600); err != nil {
		t.Fatalf("write include: %v", err)
	}
	if _, err := CopyIncludes(repo, wt, include); err == nil {
		t.Fatal("expected escaping include error")
	}

	link := filepath.Join(repo, "link.env")
	if err := os.Symlink(filepath.Join(repo, "tracked.txt"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.WriteFile(include, []byte("link.env\n"), 0o600); err != nil {
		t.Fatalf("write include symlink: %v", err)
	}
	if _, err := CopyIncludes(repo, wt, include); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("CopyIncludes symlink error = %v", err)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "agent@example.com")
	runGit(t, repo, "config", "user.name", "Agent")
	runGit(t, repo, "config", "commit.gpgsign", "false")
	runGit(t, repo, "config", "tag.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatalf("write tracked: %v", err)
	}
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-m", "initial")
	runGit(t, repo, "remote", "add", "origin", "git@github.com:org/repo.git")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}
