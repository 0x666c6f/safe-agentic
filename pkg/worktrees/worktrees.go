package worktrees

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/config"
)

type Options struct {
	RepoRoot      string
	ContainerName string
	Path          string
	Branch        string
	IncludeFile   string
	DryRun        bool
}

type Worktree struct {
	Container string   `json:"container"`
	RepoRoot  string   `json:"repo_root"`
	Path      string   `json:"path"`
	Branch    string   `json:"branch"`
	RemoteURL string   `json:"remote_url,omitempty"`
	Includes  []string `json:"includes,omitempty"`
	CreatedAt string   `json:"created_at"`
}

var branchRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._/-]*[A-Za-z0-9_-])?$`)

// VMMountPoint is the stable in-VM path where the host worktrees root is
// bind-mounted by vm/setup.sh. Managed worktrees under Root() appear here, and
// agent containers bind-mount the per-agent subdirectory into /workspace.
const VMMountPoint = "/worktrees"

// Root returns the host directory that holds all managed worktrees. It is the
// only host path exposed to the VM (as /worktrees).
func Root() string {
	return config.WorktreesDir()
}

func DefaultPath(containerName string) string {
	return filepath.Join(Root(), containerName)
}

// VMPath translates a host worktree path under root into its in-VM path under
// /worktrees. It fails for any path outside root, because the VM only mounts the
// worktrees root — a worktree elsewhere on the host would be invisible (or, worse,
// masked) inside the machine. root and hostPath are resolved to absolute paths
// before comparison.
func VMPath(root, hostPath string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve worktrees root: %w", err)
	}
	absPath, err := filepath.Abs(hostPath)
	if err != nil {
		return "", fmt.Errorf("resolve worktree path: %w", err)
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("worktree path %s is outside the safe-agentic worktrees root %s; the VM only mounts that root at %s, so worktrees must live under it (default ~/.safe-ag/worktrees). Relocate it or set a new root with: safe-ag config set defaults.worktrees_dir <path> (must be under your home directory)", absPath, absRoot, VMMountPoint)
	}
	if rel == "." {
		return VMMountPoint, nil
	}
	return path.Join(VMMountPoint, filepath.ToSlash(rel)), nil
}

func DefaultBranch(containerName string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, containerName)
	safe = strings.Trim(safe, ".-_")
	if safe == "" {
		safe = "agent"
	}
	return "safe-ag/" + safe
}

func RegistryPath() string {
	return filepath.Join(config.StateDir(), "worktrees.jsonl")
}

func Prepare(opts Options) (Worktree, error) {
	if opts.ContainerName == "" {
		return Worktree{}, fmt.Errorf("container name is required")
	}
	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		root, err := gitOutput("", "rev-parse", "--show-toplevel")
		if err != nil {
			return Worktree{}, fmt.Errorf("--worktree requires running from inside a git checkout: %w", err)
		}
		repoRoot = root
	}
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Worktree{}, fmt.Errorf("resolve repo root: %w", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err != nil {
		return Worktree{}, fmt.Errorf("repo root %s is not a git checkout", repoRoot)
	}

	path := opts.Path
	if path == "" {
		path = DefaultPath(opts.ContainerName)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return Worktree{}, fmt.Errorf("resolve worktree path: %w", err)
	}
	branch := opts.Branch
	if branch == "" {
		branch = DefaultBranch(opts.ContainerName)
	}
	if !branchRE.MatchString(branch) || strings.Contains(branch, "..") {
		return Worktree{}, fmt.Errorf("invalid worktree branch %q", branch)
	}
	remote, _ := gitOutput(repoRoot, "config", "--get", "remote.origin.url")

	wt := Worktree{
		Container: opts.ContainerName,
		RepoRoot:  repoRoot,
		Path:      path,
		Branch:    branch,
		RemoteURL: remote,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if opts.DryRun {
		return wt, nil
	}
	if _, err := os.Stat(path); err == nil {
		return Worktree{}, fmt.Errorf("worktree path already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return Worktree{}, fmt.Errorf("stat worktree path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Worktree{}, fmt.Errorf("create worktree parent: %w", err)
	}
	if _, err := gitOutput(repoRoot, "worktree", "add", "-b", branch, path, "HEAD"); err != nil {
		return Worktree{}, fmt.Errorf("git worktree add: %w", err)
	}
	includes, err := CopyIncludes(repoRoot, path, opts.IncludeFile)
	if err != nil {
		return Worktree{}, err
	}
	wt.Includes = includes
	if err := AppendRegistry(RegistryPath(), wt); err != nil {
		return Worktree{}, err
	}
	return wt, nil
}

func CopyIncludes(repoRoot, worktreePath, includeFile string) ([]string, error) {
	if includeFile == "" {
		includeFile = filepath.Join(repoRoot, ".safe-aginclude")
	}
	data, err := os.ReadFile(includeFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read include file: %w", err)
	}
	var copied []string
	for _, raw := range strings.Split(string(data), "\n") {
		pattern := strings.TrimSpace(raw)
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}
		matches, err := includeMatches(repoRoot, pattern)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("include pattern %q matched no files", pattern)
		}
		for _, src := range matches {
			rel, err := safeRel(repoRoot, src)
			if err != nil {
				return nil, err
			}
			dst := filepath.Join(worktreePath, rel)
			if err := copyRegularFile(src, dst); err != nil {
				return nil, err
			}
			copied = append(copied, rel)
		}
	}
	sort.Strings(copied)
	return copied, nil
}

func AppendRegistry(path string, wt Worktree) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create worktree registry dir: %w", err)
	}
	data, err := json.Marshal(wt)
	if err != nil {
		return fmt.Errorf("marshal worktree registry entry: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open worktree registry: %w", err)
	}
	defer f.Close()
	// Preserve private permissions if an older registry file already exists.
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod worktree registry: %w", err)
	}
	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		return fmt.Errorf("write worktree registry: %w", err)
	}
	return nil
}

func ReadRegistry(path string) ([]Worktree, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read worktree registry: %w", err)
	}
	var out []Worktree
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var wt Worktree
		if err := json.Unmarshal([]byte(line), &wt); err != nil {
			return nil, fmt.Errorf("parse worktree registry: %w", err)
		}
		out = append(out, wt)
	}
	return out, nil
}

func WriteRegistry(path string, entries []Worktree) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create worktree registry dir: %w", err)
	}
	var b strings.Builder
	for _, wt := range entries {
		data, err := json.Marshal(wt)
		if err != nil {
			return fmt.Errorf("marshal worktree registry entry: %w", err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write worktree registry: %w", err)
	}
	return nil
}

func includeMatches(repoRoot, pattern string) ([]string, error) {
	if filepath.IsAbs(pattern) {
		return nil, fmt.Errorf("include pattern must be relative: %s", pattern)
	}
	clean := filepath.Clean(pattern)
	if strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		return nil, fmt.Errorf("include pattern escapes repo: %s", pattern)
	}
	matches, err := filepath.Glob(filepath.Join(repoRoot, clean))
	if err != nil {
		return nil, fmt.Errorf("bad include pattern %q: %w", pattern, err)
	}
	return matches, nil
}

func safeRel(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", fmt.Errorf("path escapes repo: %s", path)
	}
	return rel, nil
}

func copyRegularFile(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat include %s: %w", src, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink include %s", src)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("include is not a regular file: %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create include dir: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open include: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create include copy: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy include: %w", err)
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
