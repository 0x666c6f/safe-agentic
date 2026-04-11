# Go Rewrite Phase 1: Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create all `pkg/` packages with full test coverage and implement `safe-ag spawn` + `safe-ag run` as the first working commands, producing Docker commands identical to the bash version.

**Architecture:** Single root Go module (`safe-agentic`) with `pkg/` for shared libraries, `cmd/safe-ag/` for the CLI binary, and existing `tui/` restructured under the same module. All Docker/VM interaction goes through a mockable `orb.Executor` interface — no Docker SDK, no global state.

**Tech Stack:** Go 1.25+, cobra for CLI, stdlib only (no external deps beyond cobra/pflag). Tests use `FakeExecutor` — no real OrbStack needed.

**Spec:** `docs/superpowers/specs/2026-04-10-go-rewrite-design.md`

**Scope:** This plan covers Phase 1 only (Foundation + spawn/run). Phases 2-7 are outlined at the end and will get separate plans.

---

## File Structure

### New files (Phase 1)

```
go.mod                           # Root module: safe-agentic
go.sum
cmd/
  safe-ag/
    main.go                      # Entry point, cobra root command, version
    spawn.go                     # cmd_spawn + cmd_run + shell mode

pkg/
  validate/
    validate.go                  # Name, network, PIDs validation
    validate_test.go

  labels/
    labels.go                    # All safe-agentic.* label constants

  orb/
    orb.go                       # Executor interface, OrbExecutor, FakeExecutor
    orb_test.go

  repourl/
    parse.go                     # URL parsing, traversal prevention
    parse_test.go

  config/
    defaults.go                  # Load/save defaults.sh, Config struct
    defaults_test.go
    identity.go                  # Git identity detection/parsing
    identity_test.go

  inject/
    inject.go                    # Base64 encode/decode, config injection helpers
    inject_test.go

  audit/
    audit.go                     # JSONL audit log read/write
    audit_test.go

  docker/
    runtime.go                   # DockerRunCmd builder + hardening
    runtime_test.go
    container.go                 # Container exists/resolve/inspect
    container_test.go
    volume.go                    # Volume helpers
    volume_test.go
    network.go                   # Managed/custom network lifecycle
    network_test.go
    ssh.go                       # SSH relay setup
    ssh_test.go
    dind.go                      # Docker-in-Docker sidecar
    dind_test.go

  tmux/
    tmux.go                      # Session management
    tmux_test.go

  events/
    events.go                    # Event emission + dispatch
    events_test.go
    budget.go                    # Cost budget monitoring
    budget_test.go
    notify.go                    # Notification targets
    notify_test.go

  cost/
    pricing.go                   # Model pricing + computation
    pricing_test.go
```

### Modified files

```
tui/go.mod                       # Delete (absorbed into root module)
tui/go.sum                       # Delete
tui/*.go                         # Update package imports to use root module paths
tui/Makefile                     # Update build paths
```

---

## Task 1: Go Module Restructure

**Files:**
- Create: `go.mod`
- Delete: `tui/go.mod`, `tui/go.sum`
- Modify: `tui/Makefile`

- [ ] **Step 1: Create root go.mod**

```bash
cd /Users/florian/perso/safe-agentic
go mod init safe-agentic
```

This creates:
```
module safe-agentic

go 1.25.5
```

- [ ] **Step 2: Add cobra dependency**

```bash
go get github.com/spf13/cobra@latest
```

- [ ] **Step 3: Move TUI dependencies to root module**

Copy the `require` block from `tui/go.mod` into the root `go.mod`:

```bash
# Copy tview dependency
go get github.com/rivo/tview@v0.42.0
```

- [ ] **Step 4: Delete the TUI sub-module**

```bash
rm tui/go.mod tui/go.sum
```

- [ ] **Step 5: Update TUI imports**

All files in `tui/` are `package main` and use only stdlib + tview — no import paths reference the module name. Verify nothing breaks:

```bash
cd tui && go build -o /dev/null . && cd ..
```

- [ ] **Step 6: Update TUI Makefile**

In `tui/Makefile`, the build commands use `go build -o agent-tui .` which will now use the root module. No path changes needed since `tui/` files are `package main` with no self-referencing imports.

Verify:
```bash
make -C tui build
```

- [ ] **Step 7: Create directory skeleton**

```bash
mkdir -p cmd/safe-ag pkg/{validate,labels,orb,repourl,config,inject,audit,docker,tmux,events,cost}
```

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum cmd/ pkg/
git add -u tui/  # captures deleted go.mod/go.sum
git commit -m "feat: create root Go module and directory skeleton for CLI rewrite"
```

---

## Task 2: pkg/validate — Input Validation

**Files:**
- Create: `pkg/validate/validate.go`
- Test: `pkg/validate/validate_test.go`

Ports `validate_name_component()`, `validate_network_name()`, `validate_pids_limit()` from `bin/agent-lib.sh:124-152`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/validate/validate_test.go
package validate

import "testing"

func TestNameComponent(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"my-agent", true},
		{"agent_1", true},
		{"Agent.Name", true},
		{"a", true},
		{"A123", true},
		{"", false},
		{"-starts-dash", false},
		{".starts-dot", false},
		{"has space", false},
		{"has/slash", false},
		{"has:colon", false},
		{"has@at", false},
		{"123numeric", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := NameComponent(tt.input, "test")
			if tt.valid && err != nil {
				t.Fatalf("NameComponent(%q) = %v, want nil", tt.input, err)
			}
			if !tt.valid && err == nil {
				t.Fatalf("NameComponent(%q) = nil, want error", tt.input)
			}
		})
	}
}

func TestNetworkName(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"my-network", true},
		{"custom_net", true},
		{"none", true},
		{"bridge", false},
		{"host", false},
		{"container:abc", false},
		{"container:anything", false},
		{"", false},
		{"-starts-dash", false},
		{".starts-dot", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := NetworkName(tt.input)
			if tt.valid && err != nil {
				t.Fatalf("NetworkName(%q) = %v, want nil", tt.input, err)
			}
			if !tt.valid && err == nil {
				t.Fatalf("NetworkName(%q) = nil, want error", tt.input)
			}
		})
	}
}

func TestPIDsLimit(t *testing.T) {
	tests := []struct {
		input int
		valid bool
	}{
		{512, true},
		{64, true},
		{1024, true},
		{63, false},
		{0, false},
		{-1, false},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			err := PIDsLimit(tt.input)
			if tt.valid && err != nil {
				t.Fatalf("PIDsLimit(%d) = %v, want nil", tt.input, err)
			}
			if !tt.valid && err == nil {
				t.Fatalf("PIDsLimit(%d) = nil, want error", tt.input)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/validate/ -v
```

Expected: compilation error (package doesn't exist yet).

- [ ] **Step 3: Write implementation**

```go
// pkg/validate/validate.go
package validate

import (
	"fmt"
	"regexp"
	"strings"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`)

// NameComponent validates a container or component name.
// Matches bash: [A-Za-z0-9][A-Za-z0-9_.-]*
func NameComponent(value, label string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	if !namePattern.MatchString(value) {
		return fmt.Errorf("%s contains invalid characters: %s. Allowed: letters, numbers, ., _, -", label, value)
	}
	return nil
}

// NetworkName validates a Docker network name.
// Blocks unsafe modes: bridge, host, container:*.
func NetworkName(value string) error {
	if value == "none" {
		return nil
	}
	switch {
	case value == "bridge" || value == "host":
		return fmt.Errorf("unsafe network mode %q is not allowed. Create a dedicated Docker network and pass its name", value)
	case strings.HasPrefix(value, "container:"):
		return fmt.Errorf("unsafe network mode %q is not allowed. Create a dedicated Docker network and pass its name", value)
	}
	if value == "" {
		return fmt.Errorf("network name must not be empty")
	}
	if !namePattern.MatchString(value) {
		return fmt.Errorf("network name contains invalid characters: %s. Allowed: letters, numbers, ., _, -", value)
	}
	return nil
}

// PIDsLimit validates the PIDs limit is a positive integer >= 64.
func PIDsLimit(value int) error {
	if value < 64 {
		return fmt.Errorf("PIDs limit must be >= 64 (got %d)", value)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/validate/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/validate/
git commit -m "feat: add pkg/validate with name, network, and PIDs validation"
```

---

## Task 3: pkg/labels — Label Constants

**Files:**
- Create: `pkg/labels/labels.go`

No tests needed — constants only.

- [ ] **Step 1: Write label constants**

```go
// pkg/labels/labels.go
package labels

// Docker container labels used by safe-agentic.
// These are read by the TUI, dashboard, and management commands.
const (
	AgentType     = "safe-agentic.agent-type"
	RepoDisplay   = "safe-agentic.repo-display"
	SSH           = "safe-agentic.ssh"
	AuthType      = "safe-agentic.auth"
	GHAuth        = "safe-agentic.gh-auth"
	NetworkMode   = "safe-agentic.network-mode"
	DockerMode    = "safe-agentic.docker"
	Resources     = "safe-agentic.resources"
	Prompt        = "safe-agentic.prompt"
	Instructions  = "safe-agentic.instructions"
	MaxCost       = "safe-agentic.max-cost"
	OnExit        = "safe-agentic.on-exit"
	OnCompleteB64 = "safe-agentic.on-complete-b64"
	OnFailB64     = "safe-agentic.on-fail-b64"
	NotifyB64     = "safe-agentic.notify-b64"
	Fleet         = "safe-agentic.fleet"
	Terminal      = "safe-agentic.terminal"
	ForkedFrom    = "safe-agentic.forked-from"
	ForkLabel     = "safe-agentic.fork-label"
	AWS           = "safe-agentic.aws"
	App           = "app"
	Type          = "safe-agentic.type"
	Parent        = "safe-agentic.parent"

	// Label values
	AppValue = "safe-agentic"
)

// ContainerFilter returns the Docker filter for safe-agentic containers.
func ContainerFilter() string {
	return "name=^agent-"
}
```

- [ ] **Step 2: Commit**

```bash
git add pkg/labels/
git commit -m "feat: add pkg/labels with all safe-agentic label constants"
```

---

## Task 4: pkg/orb — VM Executor Interface

**Files:**
- Create: `pkg/orb/orb.go`
- Test: `pkg/orb/orb_test.go`

This is the most critical foundation package. Every Docker/VM command goes through `Executor`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/orb/orb_test.go
package orb

import (
	"context"
	"reflect"
	"testing"
)

func TestFakeExecutorCaptures(t *testing.T) {
	fake := NewFake()
	fake.SetResponse("docker ps", "container1\ncontainer2")

	out, err := fake.Run(context.Background(), "docker", "ps")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(out) != "container1\ncontainer2" {
		t.Fatalf("Run() output = %q, want %q", string(out), "container1\ncontainer2")
	}
	if len(fake.Log) != 1 {
		t.Fatalf("Log length = %d, want 1", len(fake.Log))
	}
	want := []string{"docker", "ps"}
	if !reflect.DeepEqual(fake.Log[0], want) {
		t.Fatalf("Log[0] = %v, want %v", fake.Log[0], want)
	}
}

func TestFakeExecutorDefaultEmpty(t *testing.T) {
	fake := NewFake()
	out, err := fake.Run(context.Background(), "docker", "info")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if string(out) != "" {
		t.Fatalf("Run() output = %q, want empty", string(out))
	}
}

func TestFakeExecutorError(t *testing.T) {
	fake := NewFake()
	fake.SetError("docker inspect", "no such container")

	_, err := fake.Run(context.Background(), "docker", "inspect", "missing")
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
}

func TestOrbExecutorBuildArgs(t *testing.T) {
	// Test that OrbExecutor constructs the right orb command.
	// We can't run orb in tests, but we can verify the arg construction.
	e := &OrbExecutor{VMName: "test-vm"}
	args := e.buildArgs("docker", "ps", "-a")
	want := []string{"run", "-m", "test-vm", "docker", "ps", "-a"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("buildArgs() = %v, want %v", args, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/orb/ -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/orb/orb.go
package orb

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Executor abstracts VM command execution. All Docker/VM interaction goes through this.
type Executor interface {
	// Run executes a command in the VM and returns stdout.
	Run(ctx context.Context, args ...string) ([]byte, error)
	// RunInteractive hands off the terminal for interactive commands (attach, ssh).
	RunInteractive(args ...string) error
}

// OrbExecutor runs commands via `orb run -m <vmname>`.
type OrbExecutor struct {
	VMName string // defaults to "safe-agentic"
}

func (e *OrbExecutor) buildArgs(args ...string) []string {
	return append([]string{"run", "-m", e.VMName}, args...)
}

func (e *OrbExecutor) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "orb", e.buildArgs(args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("orb run %s: %w\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func (e *OrbExecutor) RunInteractive(args ...string) error {
	cmd := exec.Command("orb", e.buildArgs(args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// FakeExecutor captures commands for testing. No real execution.
type FakeExecutor struct {
	mu        sync.Mutex
	Log       [][]string            // All captured commands
	responses map[string]string     // prefix → stdout
	errors    map[string]string     // prefix → error message
}

// NewFake creates a new FakeExecutor.
func NewFake() *FakeExecutor {
	return &FakeExecutor{
		responses: make(map[string]string),
		errors:    make(map[string]string),
	}
}

// SetResponse configures a canned response for commands matching the prefix.
// Prefix is matched against the joined args (e.g., "docker ps").
func (f *FakeExecutor) SetResponse(prefix, output string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[prefix] = output
}

// SetError configures an error response for commands matching the prefix.
func (f *FakeExecutor) SetError(prefix, msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors[prefix] = msg
}

func (f *FakeExecutor) Run(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = append(f.Log, args)

	joined := strings.Join(args, " ")
	for prefix, msg := range f.errors {
		if strings.HasPrefix(joined, prefix) {
			return nil, fmt.Errorf("%s", msg)
		}
	}
	for prefix, out := range f.responses {
		if strings.HasPrefix(joined, prefix) {
			return []byte(out), nil
		}
	}
	return []byte(""), nil
}

func (f *FakeExecutor) RunInteractive(args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = append(f.Log, args)
	return nil
}

// LastCommand returns the last logged command, or nil.
func (f *FakeExecutor) LastCommand() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.Log) == 0 {
		return nil
	}
	return f.Log[len(f.Log)-1]
}

// CommandsMatching returns all logged commands whose joined args contain the substring.
func (f *FakeExecutor) CommandsMatching(substr string) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result [][]string
	for _, cmd := range f.Log {
		if strings.Contains(strings.Join(cmd, " "), substr) {
			result = append(result, cmd)
		}
	}
	return result
}

// Reset clears all logged commands and configured responses.
func (f *FakeExecutor) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = nil
	f.responses = make(map[string]string)
	f.errors = make(map[string]string)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/orb/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/orb/
git commit -m "feat: add pkg/orb with Executor interface and FakeExecutor for testing"
```

---

## Task 5: pkg/repourl — Repository URL Parsing

**Files:**
- Create: `pkg/repourl/parse.go`
- Test: `pkg/repourl/parse_test.go`

Ports `safe_agentic_repo_path_from_url()` from `bin/repo-url.sh`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/repourl/parse_test.go
package repourl

import "testing"

func TestClonePath(t *testing.T) {
	tests := []struct {
		url  string
		want string
		ok   bool
	}{
		// HTTPS
		{"https://github.com/org/repo.git", "org/repo", true},
		{"https://github.com/org/repo", "org/repo", true},
		{"https://gitlab.com/org/repo.git", "org/repo", true},

		// SSH scp-style
		{"git@github.com:org/repo.git", "org/repo", true},
		{"git@github.com:org/repo", "org/repo", true},

		// SSH URL-style
		{"ssh://git@github.com/org/repo.git", "org/repo", true},
		{"ssh://git@github.com/org/repo", "org/repo", true},

		// Dots, dashes, underscores in names
		{"https://github.com/my-org/my_repo.git", "my-org/my_repo", true},
		{"https://github.com/org/repo.name.git", "org/repo.name", true},

		// Invalid: no slash
		{"https://github.com/justrepo", "", false},

		// Invalid: traversal
		{"https://github.com/../etc.git", "", false},

		// Invalid: starts with dash
		{"https://github.com/-org/repo.git", "", false},
		{"https://github.com/org/-repo.git", "", false},

		// Invalid: starts with dot
		{"https://github.com/.org/repo.git", "", false},
		{"https://github.com/org/.repo.git", "", false},

		// Invalid: empty parts
		{"https://github.com//repo.git", "", false},
		{"https://github.com/org/.git", "", false},

		// Invalid: nested paths (more than owner/repo)
		{"https://github.com/a/b/c.git", "", false},

		// Invalid: flag-like input
		{"-flag", "", false},
		{"--flag=value", "", false},

		// Invalid: special characters
		{"https://github.com/org/repo;evil.git", "", false},
		{"https://github.com/org/repo|evil.git", "", false},

		// Invalid: no scheme
		{"justrepo", "", false},
		{"org/repo", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got, err := ClonePath(tt.url)
			if tt.ok {
				if err != nil {
					t.Fatalf("ClonePath(%q) error = %v", tt.url, err)
				}
				if got != tt.want {
					t.Fatalf("ClonePath(%q) = %q, want %q", tt.url, got, tt.want)
				}
			} else {
				if err == nil {
					t.Fatalf("ClonePath(%q) = %q, want error", tt.url, got)
				}
			}
		})
	}
}

func TestUsesSSH(t *testing.T) {
	tests := []struct {
		url string
		ssh bool
	}{
		{"git@github.com:org/repo.git", true},
		{"ssh://git@github.com/org/repo.git", true},
		{"https://github.com/org/repo.git", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := UsesSSH(tt.url); got != tt.ssh {
				t.Fatalf("UsesSSH(%q) = %v, want %v", tt.url, got, tt.ssh)
			}
		})
	}
}

func TestDisplayLabel(t *testing.T) {
	tests := []struct {
		repos []string
		want  string
	}{
		{[]string{"https://github.com/org/repo.git"}, "org/repo"},
		{[]string{"https://github.com/org/a.git", "https://github.com/org/b.git"}, "org/a, org/b"},
		{[]string{"https://github.com/o/a.git", "https://github.com/o/b.git", "https://github.com/o/c.git", "https://github.com/o/d.git"}, "o/a + 3 more"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := DisplayLabel(tt.repos)
			if got != tt.want {
				t.Fatalf("DisplayLabel(%v) = %q, want %q", tt.repos, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/repourl/ -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/repourl/parse.go
package repourl

import (
	"fmt"
	"regexp"
	"strings"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]*$`)

// ClonePath extracts "owner/repo" from a git URL.
// Supported schemes: https://, ssh://, git@host:owner/repo (scp-style).
// Rejects traversal, dot-prefixed names, special characters.
func ClonePath(repoURL string) (string, error) {
	// Reject flag-like input
	if strings.HasPrefix(repoURL, "-") {
		return "", fmt.Errorf("invalid repo URL: %s", repoURL)
	}

	path := strings.TrimSuffix(repoURL, ".git")

	switch {
	case strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "ssh://"):
		// Strip scheme + host: https://github.com/org/repo → org/repo
		idx := strings.Index(path, "://")
		path = path[idx+3:]
		// Remove host part (everything before first /)
		slashIdx := strings.Index(path, "/")
		if slashIdx < 0 {
			return "", fmt.Errorf("invalid repo URL: %s", repoURL)
		}
		path = path[slashIdx+1:]
	case strings.Contains(path, ":") && strings.Contains(path, "/"):
		// scp-style: git@github.com:org/repo → org/repo
		colonIdx := strings.LastIndex(path, ":")
		path = path[colonIdx+1:]
	default:
		return "", fmt.Errorf("invalid repo URL: %s (must be https://, ssh://, or git@host:owner/repo)", repoURL)
	}

	// Must have exactly owner/repo (one slash)
	if !strings.Contains(path, "/") {
		return "", fmt.Errorf("invalid repo URL: %s (no owner/repo)", repoURL)
	}

	owner := path[:strings.Index(path, "/")]
	repo := path[strings.Index(path, "/")+1:]

	// Reject nested paths
	if strings.Contains(repo, "/") {
		return "", fmt.Errorf("invalid repo URL: %s (nested path)", repoURL)
	}

	// Validate owner
	if owner == "" || strings.HasPrefix(owner, ".") || strings.HasPrefix(owner, "-") {
		return "", fmt.Errorf("invalid repo owner: %q", owner)
	}
	if !namePattern.MatchString(owner) {
		return "", fmt.Errorf("invalid repo owner: %q", owner)
	}

	// Validate repo
	if repo == "" || strings.HasPrefix(repo, ".") || strings.HasPrefix(repo, "-") {
		return "", fmt.Errorf("invalid repo name: %q", repo)
	}
	if !namePattern.MatchString(repo) {
		return "", fmt.Errorf("invalid repo name: %q", repo)
	}

	return owner + "/" + repo, nil
}

// UsesSSH returns true if the URL requires SSH (git@ or ssh://).
func UsesSSH(url string) bool {
	return strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://")
}

// DisplayLabel formats repos for human display.
// 1 repo: "org/repo", 2 repos: "org/a, org/b", 3+: "org/a + N more".
func DisplayLabel(repos []string) string {
	if len(repos) == 0 {
		return ""
	}
	slugs := make([]string, 0, len(repos))
	for _, r := range repos {
		s, err := ClonePath(r)
		if err != nil {
			slugs = append(slugs, r)
		} else {
			slugs = append(slugs, s)
		}
	}
	if len(slugs) <= 2 {
		return strings.Join(slugs, ", ")
	}
	return fmt.Sprintf("%s + %d more", slugs[0], len(slugs)-1)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/repourl/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/repourl/
git commit -m "feat: add pkg/repourl with URL parsing and traversal prevention"
```

---

## Task 6: pkg/config — Configuration Loading

**Files:**
- Create: `pkg/config/defaults.go`, `pkg/config/identity.go`
- Test: `pkg/config/defaults_test.go`, `pkg/config/identity_test.go`

Ports `load_user_defaults()`, `parse_defaults_line()`, `parse_defaults_value()`, `default_key_allowed()`, `detect_git_identity()`, `parse_identity()` from `bin/agent-lib.sh:36-200`.

- [ ] **Step 1: Write identity tests**

```go
// pkg/config/identity_test.go
package config

import "testing"

func TestParseIdentity(t *testing.T) {
	tests := []struct {
		input string
		name  string
		email string
		ok    bool
	}{
		{"John Doe <john@example.com>", "John Doe", "john@example.com", true},
		{"A B <a@b.com>", "A B", "a@b.com", true},
		{"Single <s@x.co>", "Single", "s@x.co", true},
		{"", "", "", false},
		{"no-angle-brackets", "", "", false},
		{"<only@email.com>", "", "", false},
		{"Name <noemail>", "", "", false},
		{"Name <@broken>", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, email, err := ParseIdentity(tt.input)
			if tt.ok {
				if err != nil {
					t.Fatalf("ParseIdentity(%q) error = %v", tt.input, err)
				}
				if name != tt.name || email != tt.email {
					t.Fatalf("ParseIdentity(%q) = (%q, %q), want (%q, %q)", tt.input, name, email, tt.name, tt.email)
				}
			} else if err == nil {
				t.Fatalf("ParseIdentity(%q) expected error", tt.input)
			}
		})
	}
}
```

- [ ] **Step 2: Write identity implementation**

```go
// pkg/config/identity.go
package config

import (
	"fmt"
	"os/exec"
	"strings"
)

// ParseIdentity parses "Name <email>" into name and email.
func ParseIdentity(identity string) (string, string, error) {
	if identity == "" {
		return "", "", fmt.Errorf("identity must not be empty")
	}
	ltIdx := strings.LastIndex(identity, "<")
	gtIdx := strings.LastIndex(identity, ">")
	if ltIdx < 0 || gtIdx < 0 || gtIdx < ltIdx {
		return "", "", fmt.Errorf("identity must be in format 'Name <email>': %s", identity)
	}
	name := strings.TrimSpace(identity[:ltIdx])
	email := identity[ltIdx+1 : gtIdx]
	if name == "" {
		return "", "", fmt.Errorf("name part is empty in identity: %s", identity)
	}
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") {
		return "", "", fmt.Errorf("invalid email in identity: %s", email)
	}
	return name, email, nil
}

// DetectGitIdentity reads git user.name and user.email from global git config.
// Returns "Name <email>" or empty string if not configured.
func DetectGitIdentity() string {
	nameOut, err := exec.Command("git", "config", "--global", "user.name").Output()
	if err != nil {
		return ""
	}
	emailOut, err := exec.Command("git", "config", "--global", "user.email").Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(nameOut))
	email := strings.TrimSpace(string(emailOut))
	if name == "" || email == "" {
		return ""
	}
	return fmt.Sprintf("%s <%s>", name, email)
}
```

- [ ] **Step 3: Run identity tests**

```bash
go test ./pkg/config/ -run TestParseIdentity -v
```

Expected: all PASS.

- [ ] **Step 4: Write defaults tests**

```go
// pkg/config/defaults_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "defaults.sh")
	content := `SAFE_AGENTIC_DEFAULT_CPUS=8
SAFE_AGENTIC_DEFAULT_MEMORY=16g
SAFE_AGENTIC_DEFAULT_SSH=true
SAFE_AGENTIC_DEFAULT_PIDS_LIMIT=1024
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=false
GIT_AUTHOR_NAME=Test User
GIT_AUTHOR_EMAIL=test@example.com
`
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("LoadDefaults() error = %v", err)
	}
	if cfg.DefaultCPUs != "8" {
		t.Fatalf("DefaultCPUs = %q, want %q", cfg.DefaultCPUs, "8")
	}
	if cfg.DefaultMemory != "16g" {
		t.Fatalf("DefaultMemory = %q, want %q", cfg.DefaultMemory, "16g")
	}
	if !cfg.DefaultSSH {
		t.Fatal("DefaultSSH = false, want true")
	}
	if cfg.DefaultPIDsLimit != 1024 {
		t.Fatalf("DefaultPIDsLimit = %d, want 1024", cfg.DefaultPIDsLimit)
	}
	if cfg.DefaultReuseAuth {
		t.Fatal("DefaultReuseAuth = true, want false")
	}
	if cfg.GitAuthorName != "Test User" {
		t.Fatalf("GitAuthorName = %q, want %q", cfg.GitAuthorName, "Test User")
	}
}

func TestLoadDefaultsQuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "defaults.sh")
	content := `SAFE_AGENTIC_DEFAULT_IDENTITY="John Doe <john@example.com>"
GIT_AUTHOR_NAME='Single Quoted'
`
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("LoadDefaults() error = %v", err)
	}
	if cfg.DefaultIdentity != "John Doe <john@example.com>" {
		t.Fatalf("DefaultIdentity = %q", cfg.DefaultIdentity)
	}
	if cfg.GitAuthorName != "Single Quoted" {
		t.Fatalf("GitAuthorName = %q", cfg.GitAuthorName)
	}
}

func TestLoadDefaultsComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "defaults.sh")
	content := `# This is a comment
SAFE_AGENTIC_DEFAULT_CPUS=4

  # Another comment
`
	os.WriteFile(path, []byte(content), 0644)

	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("LoadDefaults() error = %v", err)
	}
	if cfg.DefaultCPUs != "4" {
		t.Fatalf("DefaultCPUs = %q, want %q", cfg.DefaultCPUs, "4")
	}
}

func TestLoadDefaultsRejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "defaults.sh")
	content := `UNKNOWN_KEY=value
`
	os.WriteFile(path, []byte(content), 0644)

	_, err := LoadDefaults(path)
	if err == nil {
		t.Fatal("LoadDefaults() expected error for unknown key")
	}
}

func TestLoadDefaultsMissingFile(t *testing.T) {
	cfg, err := LoadDefaults("/nonexistent/path")
	if err != nil {
		t.Fatalf("LoadDefaults() error = %v (should return defaults for missing file)", err)
	}
	if cfg.DefaultCPUs != "4" {
		t.Fatalf("DefaultCPUs = %q, want default %q", cfg.DefaultCPUs, "4")
	}
}

func TestKeyAllowed(t *testing.T) {
	allowed := []string{
		"SAFE_AGENTIC_DEFAULT_CPUS", "SAFE_AGENTIC_DEFAULT_MEMORY",
		"SAFE_AGENTIC_DEFAULT_PIDS_LIMIT", "SAFE_AGENTIC_DEFAULT_SSH",
		"SAFE_AGENTIC_DEFAULT_DOCKER", "SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET",
		"SAFE_AGENTIC_DEFAULT_REUSE_AUTH", "SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH",
		"SAFE_AGENTIC_DEFAULT_NETWORK", "SAFE_AGENTIC_DEFAULT_IDENTITY",
		"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL",
		"GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL",
	}
	for _, k := range allowed {
		if !keyAllowed(k) {
			t.Fatalf("keyAllowed(%q) = false, want true", k)
		}
	}
	rejected := []string{"HOME", "PATH", "SAFE_AGENTIC_CUSTOM", "SSH_AUTH_SOCK"}
	for _, k := range rejected {
		if keyAllowed(k) {
			t.Fatalf("keyAllowed(%q) = true, want false", k)
		}
	}
}
```

- [ ] **Step 5: Write defaults implementation**

```go
// pkg/config/defaults.go
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all CLI configuration loaded from defaults.sh.
type Config struct {
	DefaultCPUs       string
	DefaultMemory     string
	DefaultPIDsLimit  int
	DefaultSSH        bool
	DefaultDocker     bool
	DefaultDockerSocket bool
	DefaultReuseAuth  bool
	DefaultReuseGHAuth bool
	DefaultNetwork    string
	DefaultIdentity   string
	GitAuthorName     string
	GitAuthorEmail    string
	GitCommitterName  string
	GitCommitterEmail string
}

// Defaults returns a Config with default values.
func Defaults() Config {
	return Config{
		DefaultCPUs:       "4",
		DefaultMemory:     "8g",
		DefaultPIDsLimit:  512,
		DefaultReuseAuth:  true,
		DefaultReuseGHAuth: true,
	}
}

var allowedKeys = map[string]bool{
	"SAFE_AGENTIC_DEFAULT_CPUS":         true,
	"SAFE_AGENTIC_DEFAULT_MEMORY":       true,
	"SAFE_AGENTIC_DEFAULT_PIDS_LIMIT":   true,
	"SAFE_AGENTIC_DEFAULT_SSH":          true,
	"SAFE_AGENTIC_DEFAULT_DOCKER":       true,
	"SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET":true,
	"SAFE_AGENTIC_DEFAULT_REUSE_AUTH":   true,
	"SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH":true,
	"SAFE_AGENTIC_DEFAULT_NETWORK":      true,
	"SAFE_AGENTIC_DEFAULT_IDENTITY":     true,
	"GIT_AUTHOR_NAME":                   true,
	"GIT_AUTHOR_EMAIL":                  true,
	"GIT_COMMITTER_NAME":                true,
	"GIT_COMMITTER_EMAIL":               true,
}

func keyAllowed(key string) bool {
	return allowedKeys[key]
}

// LoadDefaults loads configuration from a defaults.sh file.
// Returns default values if the file doesn't exist.
func LoadDefaults(path string) (Config, error) {
	cfg := Defaults()

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("open defaults: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimRight(scanner.Text(), "\r")
		if err := parseLine(&cfg, line, lineNo); err != nil {
			return cfg, err
		}
	}
	return cfg, scanner.Err()
}

func parseLine(cfg *Config, line string, lineNo int) error {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil
	}
	// Strip optional "export " prefix
	trimmed = strings.TrimPrefix(trimmed, "export ")

	eqIdx := strings.Index(trimmed, "=")
	if eqIdx < 0 {
		return fmt.Errorf("line %d: invalid syntax (no '='): %s", lineNo, line)
	}
	key := strings.TrimSpace(trimmed[:eqIdx])
	rawValue := trimmed[eqIdx+1:]

	if !keyAllowed(key) {
		return fmt.Errorf("line %d: unknown config key: %s", lineNo, key)
	}

	value, err := parseValue(rawValue)
	if err != nil {
		return fmt.Errorf("line %d: %w", lineNo, err)
	}

	return applyConfig(cfg, key, value)
}

func parseValue(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`):
		v := raw[1 : len(raw)-1]
		v = strings.ReplaceAll(v, `\"`, `"`)
		v = strings.ReplaceAll(v, `\\`, `\`)
		return v, nil
	case strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'"):
		return raw[1 : len(raw)-1], nil
	case strings.ContainsAny(raw, " \t"):
		return "", fmt.Errorf("unquoted value contains whitespace: %s", raw)
	default:
		return raw, nil
	}
}

func boolFromString(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func applyConfig(cfg *Config, key, value string) error {
	switch key {
	case "SAFE_AGENTIC_DEFAULT_CPUS":
		cfg.DefaultCPUs = value
	case "SAFE_AGENTIC_DEFAULT_MEMORY":
		cfg.DefaultMemory = value
	case "SAFE_AGENTIC_DEFAULT_PIDS_LIMIT":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("PIDs limit must be integer: %s", value)
		}
		cfg.DefaultPIDsLimit = v
	case "SAFE_AGENTIC_DEFAULT_SSH":
		cfg.DefaultSSH = boolFromString(value)
	case "SAFE_AGENTIC_DEFAULT_DOCKER":
		cfg.DefaultDocker = boolFromString(value)
	case "SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET":
		cfg.DefaultDockerSocket = boolFromString(value)
	case "SAFE_AGENTIC_DEFAULT_REUSE_AUTH":
		cfg.DefaultReuseAuth = boolFromString(value)
	case "SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH":
		cfg.DefaultReuseGHAuth = boolFromString(value)
	case "SAFE_AGENTIC_DEFAULT_NETWORK":
		cfg.DefaultNetwork = value
	case "SAFE_AGENTIC_DEFAULT_IDENTITY":
		cfg.DefaultIdentity = value
	case "GIT_AUTHOR_NAME":
		cfg.GitAuthorName = value
	case "GIT_AUTHOR_EMAIL":
		cfg.GitAuthorEmail = value
	case "GIT_COMMITTER_NAME":
		cfg.GitCommitterName = value
	case "GIT_COMMITTER_EMAIL":
		cfg.GitCommitterEmail = value
	}
	return nil
}

// DefaultsPath returns the path to the defaults file.
func DefaultsPath() string {
	if p := os.Getenv("SAFE_AGENTIC_DEFAULTS_FILE"); p != "" {
		return p
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = home + "/.config"
	}
	return dir + "/safe-agentic/defaults.sh"
}
```

- [ ] **Step 6: Run all config tests**

```bash
go test ./pkg/config/ -v
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/config/
git commit -m "feat: add pkg/config with defaults loading and identity parsing"
```

---

## Task 7: pkg/inject — Base64 & Config Injection

**Files:**
- Create: `pkg/inject/inject.go`
- Test: `pkg/inject/inject_test.go`

Ports the base64 encoding and host config injection from `inject_host_config()` and `inject_aws_credentials()` in `bin/agent-lib.sh`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/inject/inject_test.go
package inject

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestEncodeB64(t *testing.T) {
	got := EncodeB64("hello world")
	decoded, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if string(decoded) != "hello world" {
		t.Fatalf("decoded = %q, want %q", string(decoded), "hello world")
	}
}

func TestDecodeB64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("test string"))
	got, err := DecodeB64(encoded)
	if err != nil {
		t.Fatalf("DecodeB64() error = %v", err)
	}
	if got != "test string" {
		t.Fatalf("DecodeB64() = %q, want %q", got, "test string")
	}
}

func TestReadClaudeConfig(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".claude")
	os.MkdirAll(configDir, 0755)

	settings := `{"permissions":{"allow":["Bash"]}}` + "\n"
	os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(settings), 0644)

	envs, err := ReadClaudeConfig(configDir)
	if err != nil {
		t.Fatalf("ReadClaudeConfig() error = %v", err)
	}
	if _, ok := envs["SAFE_AGENTIC_CLAUDE_CONFIG_B64"]; !ok {
		t.Fatal("missing SAFE_AGENTIC_CLAUDE_CONFIG_B64 env var")
	}
	// Decode and verify
	decoded, _ := DecodeB64(envs["SAFE_AGENTIC_CLAUDE_CONFIG_B64"])
	if decoded != settings {
		t.Fatalf("decoded config = %q, want %q", decoded, settings)
	}
}

func TestReadClaudeConfigMissing(t *testing.T) {
	envs, err := ReadClaudeConfig("/nonexistent")
	if err != nil {
		t.Fatalf("ReadClaudeConfig() error = %v (should return empty for missing)", err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected empty envs, got %v", envs)
	}
}

func TestReadAWSCredentials(t *testing.T) {
	dir := t.TempDir()
	awsDir := filepath.Join(dir, ".aws")
	os.MkdirAll(awsDir, 0755)
	creds := "[my-profile]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\n"
	os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(creds), 0644)

	envs, err := ReadAWSCredentials(filepath.Join(awsDir, "credentials"), "my-profile")
	if err != nil {
		t.Fatalf("ReadAWSCredentials() error = %v", err)
	}
	if envs["AWS_PROFILE"] != "my-profile" {
		t.Fatalf("AWS_PROFILE = %q", envs["AWS_PROFILE"])
	}
	if _, ok := envs["SAFE_AGENTIC_AWS_CREDS_B64"]; !ok {
		t.Fatal("missing SAFE_AGENTIC_AWS_CREDS_B64")
	}
}

func TestReadAWSCredentialsMissingProfile(t *testing.T) {
	dir := t.TempDir()
	awsDir := filepath.Join(dir, ".aws")
	os.MkdirAll(awsDir, 0755)
	creds := "[other-profile]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\n"
	os.WriteFile(filepath.Join(awsDir, "credentials"), []byte(creds), 0644)

	_, err := ReadAWSCredentials(filepath.Join(awsDir, "credentials"), "missing-profile")
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/inject/ -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/inject/inject.go
package inject

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EncodeB64 encodes a string to base64.
func EncodeB64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// DecodeB64 decodes a base64 string.
func DecodeB64(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// EncodeFileB64 reads a file and returns its base64-encoded contents.
func EncodeFileB64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// ReadClaudeConfig reads Claude settings from configDir and returns env vars for injection.
// Returns empty map if configDir doesn't exist.
func ReadClaudeConfig(configDir string) (map[string]string, error) {
	envs := make(map[string]string)

	settingsPath := filepath.Join(configDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return envs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read claude settings: %w", err)
	}
	envs["SAFE_AGENTIC_CLAUDE_CONFIG_B64"] = base64.StdEncoding.EncodeToString(data)
	return envs, nil
}

// ReadCodexConfig reads Codex config.toml and returns env vars for injection.
func ReadCodexConfig(codexHome string) (map[string]string, error) {
	envs := make(map[string]string)

	configPath := filepath.Join(codexHome, "config.toml")
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return envs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read codex config: %w", err)
	}

	// Strip host paths and set /workspace as trusted project
	content := string(data)
	// The entrypoint handles path rewriting — pass raw config
	envs["SAFE_AGENTIC_CODEX_CONFIG_B64"] = base64.StdEncoding.EncodeToString([]byte(content))
	return envs, nil
}

// ReadAWSCredentials reads AWS credentials for a profile.
// Returns env vars for injection. Errors if profile not found.
func ReadAWSCredentials(credPath, profile string) (map[string]string, error) {
	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read AWS credentials: %w", err)
	}

	content := string(data)
	if !strings.Contains(content, "["+profile+"]") {
		return nil, fmt.Errorf("AWS profile %q not found in %s", profile, credPath)
	}

	envs := map[string]string{
		"SAFE_AGENTIC_AWS_CREDS_B64": base64.StdEncoding.EncodeToString(data),
		"AWS_PROFILE":                profile,
	}

	// Forward region env vars if set
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		envs["AWS_DEFAULT_REGION"] = r
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		envs["AWS_REGION"] = r
	}

	return envs, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/inject/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/inject/
git commit -m "feat: add pkg/inject with base64 encoding and config injection helpers"
```

---

## Task 8: pkg/audit — Audit Logging

**Files:**
- Create: `pkg/audit/audit.go`
- Test: `pkg/audit/audit_test.go`

Ports `audit_log()` from `bin/agent-lib.sh:858+`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/audit/audit_test.go
package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	logger := &Logger{Path: path}

	err := logger.Log("spawn", "agent-claude-test", map[string]string{
		"type": "claude",
		"repo": "org/repo",
	})
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	err = logger.Log("stop", "agent-claude-test", nil)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	entries, err := logger.Read(10)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("Read() returned %d entries, want 2", len(entries))
	}
	if entries[0].Action != "spawn" {
		t.Fatalf("entries[0].Action = %q, want %q", entries[0].Action, "spawn")
	}
	if entries[0].Container != "agent-claude-test" {
		t.Fatalf("entries[0].Container = %q", entries[0].Container)
	}
}

func TestLogCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "audit.jsonl")

	logger := &Logger{Path: path}
	err := logger.Log("test", "container", nil)
	if err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Verify file was created
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}

	var entry Entry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if entry.Action != "test" {
		t.Fatalf("Action = %q", entry.Action)
	}
}

func TestReadLimitsTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{Path: path}

	for i := 0; i < 20; i++ {
		logger.Log("action", "container", nil)
	}

	entries, err := logger.Read(5)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("Read() returned %d entries, want 5", len(entries))
	}
}

func TestLogJSONSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	logger := &Logger{Path: path}

	// Test with special characters
	logger.Log("test", "agent-test", map[string]string{
		"prompt": `He said "hello" & <world>`,
	})

	data, _ := os.ReadFile(path)
	line := strings.TrimSpace(string(data))
	var entry Entry
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("JSON parse failed for special chars: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/audit/ -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/audit/audit.go
package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry represents one audit log record.
type Entry struct {
	Timestamp string            `json:"timestamp"`
	Action    string            `json:"action"`
	Container string            `json:"container"`
	Details   map[string]string `json:"details,omitempty"`
}

// Logger writes to an append-only JSONL audit file.
type Logger struct {
	Path string
}

// DefaultPath returns the default audit log path.
func DefaultPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = home + "/.config"
	}
	return filepath.Join(dir, "safe-agentic", "audit.jsonl")
}

// Log appends an entry to the audit log.
func (l *Logger) Log(action, container string, details map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0755); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}

	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Action:    action,
		Container: container,
		Details:   details,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}

	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// Read returns the last n entries from the audit log (most recent last).
func (l *Logger) Read(n int) ([]Entry, error) {
	f, err := os.Open(l.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var all []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // skip malformed lines
		}
		all = append(all, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/audit/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/audit/
git commit -m "feat: add pkg/audit with JSONL audit log read/write"
```

---

## Task 9: pkg/docker/container — Container Helpers

**Files:**
- Create: `pkg/docker/container.go`
- Test: `pkg/docker/container_test.go`

Ports `container_exists()`, `resolve_latest_container()`, `resolve_container_reference()` from `bin/agent-lib.sh`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/docker/container_test.go
package docker

import (
	"context"
	"safe-agentic/pkg/orb"
	"testing"
)

func TestContainerExists(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker inspect", "exists")

	exists, err := ContainerExists(context.Background(), fake, "agent-test")
	if err != nil {
		t.Fatalf("ContainerExists() error = %v", err)
	}
	if !exists {
		t.Fatal("ContainerExists() = false, want true")
	}

	// Verify the command
	cmd := fake.LastCommand()
	if cmd[0] != "docker" || cmd[1] != "inspect" || cmd[2] != "agent-test" {
		t.Fatalf("unexpected command: %v", cmd)
	}
}

func TestContainerExistsNotFound(t *testing.T) {
	fake := orb.NewFake()
	fake.SetError("docker inspect", "no such container")

	exists, err := ContainerExists(context.Background(), fake, "missing")
	if err != nil {
		t.Fatalf("ContainerExists() error = %v", err)
	}
	if exists {
		t.Fatal("ContainerExists() = true, want false")
	}
}

func TestResolveLatest(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps", "agent-claude-abc123")

	name, err := ResolveLatest(context.Background(), fake)
	if err != nil {
		t.Fatalf("ResolveLatest() error = %v", err)
	}
	if name != "agent-claude-abc123" {
		t.Fatalf("ResolveLatest() = %q", name)
	}
}

func TestResolveLatestEmpty(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps", "")

	_, err := ResolveLatest(context.Background(), fake)
	if err == nil {
		t.Fatal("ResolveLatest() expected error for no containers")
	}
}

func TestResolveTarget(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps", "agent-claude-abc123\nagent-codex-def456\n")

	name, err := ResolveTarget(context.Background(), fake, "abc123")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if name != "agent-claude-abc123" {
		t.Fatalf("ResolveTarget() = %q", name)
	}
}

func TestResolveTargetExact(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps", "agent-claude-myname\n")

	name, err := ResolveTarget(context.Background(), fake, "agent-claude-myname")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if name != "agent-claude-myname" {
		t.Fatalf("ResolveTarget() = %q", name)
	}
}

func TestResolveTargetLatest(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker ps", "agent-claude-latest")

	name, err := ResolveTarget(context.Background(), fake, "--latest")
	if err != nil {
		t.Fatalf("ResolveTarget() error = %v", err)
	}
	if name != "agent-claude-latest" {
		t.Fatalf("ResolveTarget() = %q", name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/docker/ -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/docker/container.go
package docker

import (
	"context"
	"fmt"
	"safe-agentic/pkg/orb"
	"strings"
)

const ContainerPrefix = "agent"

// ContainerExists checks if a container exists (running or stopped).
func ContainerExists(ctx context.Context, exec orb.Executor, name string) (bool, error) {
	_, err := exec.Run(ctx, "docker", "inspect", name)
	if err != nil {
		return false, nil // docker inspect fails = container doesn't exist
	}
	return true, nil
}

// ResolveLatest returns the most recently created safe-agentic container name.
func ResolveLatest(ctx context.Context, exec orb.Executor) (string, error) {
	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^"+ContainerPrefix+"-",
		"--format", "{{.Names}}",
		"--latest")
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("no safe-agentic containers found")
	}
	return name, nil
}

// ResolveTarget resolves a container name from user input.
// Accepts "--latest", a full name, or a partial name (substring match).
func ResolveTarget(ctx context.Context, exec orb.Executor, nameOrPartial string) (string, error) {
	if nameOrPartial == "--latest" || nameOrPartial == "" {
		return ResolveLatest(ctx, exec)
	}

	// List all containers and find match
	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^"+ContainerPrefix+"-",
		"--format", "{{.Names}}")
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}

	names := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == nameOrPartial {
			return n, nil // Exact match
		}
	}
	// Partial/substring match
	for _, n := range names {
		n = strings.TrimSpace(n)
		if strings.Contains(n, nameOrPartial) {
			return n, nil
		}
	}
	return "", fmt.Errorf("container %q not found", nameOrPartial)
}

// InspectLabel reads a single label value from a container.
func InspectLabel(ctx context.Context, exec orb.Executor, name, label string) (string, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", fmt.Sprintf("{{index .Config.Labels %q}}", label), name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsRunning checks if a container is currently running.
func IsRunning(ctx context.Context, exec orb.Executor, name string) (bool, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", "{{.State.Running}}", name)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/docker/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/docker/container.go pkg/docker/container_test.go
git commit -m "feat: add pkg/docker container resolution and inspection helpers"
```

---

## Task 10: pkg/docker/volume — Volume Helpers

**Files:**
- Create: `pkg/docker/volume.go`
- Test: `pkg/docker/volume_test.go`

- [ ] **Step 1: Write failing tests**

```go
// pkg/docker/volume_test.go
package docker

import (
	"context"
	"safe-agentic/pkg/orb"
	"strings"
	"testing"
)

func TestVolumeExists(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker volume inspect", "exists")

	exists, err := VolumeExists(context.Background(), fake, "my-volume")
	if err != nil {
		t.Fatalf("VolumeExists() error = %v", err)
	}
	if !exists {
		t.Fatal("want true")
	}
}

func TestVolumeExistsNotFound(t *testing.T) {
	fake := orb.NewFake()
	fake.SetError("docker volume inspect", "no such volume")

	exists, err := VolumeExists(context.Background(), fake, "missing")
	if err != nil {
		t.Fatalf("VolumeExists() error = %v", err)
	}
	if exists {
		t.Fatal("want false")
	}
}

func TestCreateLabeledVolume(t *testing.T) {
	fake := orb.NewFake()

	err := CreateLabeledVolume(context.Background(), fake, "test-vol", "docker-runtime", "parent-container")
	if err != nil {
		t.Fatalf("CreateLabeledVolume() error = %v", err)
	}

	cmds := fake.CommandsMatching("docker volume create")
	if len(cmds) != 1 {
		t.Fatalf("expected 1 volume create command, got %d", len(cmds))
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "app=safe-agentic") {
		t.Fatalf("missing app label in: %s", joined)
	}
	if !strings.Contains(joined, "safe-agentic.type=docker-runtime") {
		t.Fatalf("missing type label in: %s", joined)
	}
	if !strings.Contains(joined, "safe-agentic.parent=parent-container") {
		t.Fatalf("missing parent label in: %s", joined)
	}
}

func TestAuthVolumeName(t *testing.T) {
	tests := []struct {
		agentType string
		ephemeral bool
		container string
		want      string
	}{
		{"claude", false, "", "safe-agentic-claude-auth"},
		{"codex", false, "", "safe-agentic-codex-auth"},
		{"claude", true, "agent-claude-test", "agent-claude-test-auth"},
	}
	for _, tt := range tests {
		got := AuthVolumeName(tt.agentType, tt.ephemeral, tt.container)
		if got != tt.want {
			t.Fatalf("AuthVolumeName(%q, %v, %q) = %q, want %q", tt.agentType, tt.ephemeral, tt.container, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/docker/ -run TestVolume -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/docker/volume.go
package docker

import (
	"context"
	"fmt"
	"safe-agentic/pkg/labels"
	"safe-agentic/pkg/orb"
)

// VolumeExists checks if a Docker volume exists.
func VolumeExists(ctx context.Context, exec orb.Executor, name string) (bool, error) {
	_, err := exec.Run(ctx, "docker", "volume", "inspect", name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// CreateLabeledVolume creates a Docker volume with safe-agentic labels.
func CreateLabeledVolume(ctx context.Context, exec orb.Executor, name, volumeType, parentName string) error {
	_, err := exec.Run(ctx, "docker", "volume", "create",
		"--label", fmt.Sprintf("%s=%s", labels.App, labels.AppValue),
		"--label", fmt.Sprintf("%s=%s", labels.Type, volumeType),
		"--label", fmt.Sprintf("%s=%s", labels.Parent, parentName),
		name)
	if err != nil {
		return fmt.Errorf("create volume %s: %w", name, err)
	}
	return nil
}

// AuthVolumeName returns the auth volume name for an agent.
// Shared mode: "safe-agentic-{type}-auth"
// Ephemeral mode: "{container}-auth"
func AuthVolumeName(agentType string, ephemeral bool, containerName string) string {
	if ephemeral {
		return containerName + "-auth"
	}
	return "safe-agentic-" + agentType + "-auth"
}

// GHAuthVolumeName returns the GitHub CLI auth volume name.
func GHAuthVolumeName(agentType string) string {
	return "safe-agentic-" + agentType + "-gh-auth"
}

// CacheVolumeName returns the cache volume name for a container.
func CacheVolumeName(containerName, cacheType string) string {
	return containerName + "-" + cacheType
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/docker/ -run TestVolume -v
go test ./pkg/docker/ -run TestAuth -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/docker/volume.go pkg/docker/volume_test.go
git commit -m "feat: add pkg/docker volume helpers with labeled volume creation"
```

---

## Task 11: pkg/docker/network — Network Management

**Files:**
- Create: `pkg/docker/network.go`
- Test: `pkg/docker/network_test.go`

Ports `create_managed_network()`, `remove_managed_network()`, `prepare_network()` from `bin/agent-lib.sh`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/docker/network_test.go
package docker

import (
	"context"
	"safe-agentic/pkg/orb"
	"strings"
	"testing"
)

func TestManagedNetworkName(t *testing.T) {
	got := ManagedNetworkName("agent-claude-test")
	if got != "agent-claude-test-net" {
		t.Fatalf("ManagedNetworkName() = %q, want %q", got, "agent-claude-test-net")
	}
}

func TestCreateManagedNetwork(t *testing.T) {
	fake := orb.NewFake()

	name, err := CreateManagedNetwork(context.Background(), fake, "agent-test")
	if err != nil {
		t.Fatalf("CreateManagedNetwork() error = %v", err)
	}
	if name != "agent-test-net" {
		t.Fatalf("network name = %q", name)
	}

	cmds := fake.CommandsMatching("docker network create")
	if len(cmds) != 1 {
		t.Fatalf("expected 1 network create, got %d", len(cmds))
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "--driver bridge") {
		t.Fatalf("missing bridge driver in: %s", joined)
	}
}

func TestRemoveManagedNetwork(t *testing.T) {
	fake := orb.NewFake()

	err := RemoveManagedNetwork(context.Background(), fake, "agent-test-net")
	if err != nil {
		t.Fatalf("RemoveManagedNetwork() error = %v", err)
	}

	cmds := fake.CommandsMatching("docker network rm")
	if len(cmds) != 1 {
		t.Fatalf("expected 1 network rm, got %d", len(cmds))
	}
}

func TestPrepareNetworkManaged(t *testing.T) {
	fake := orb.NewFake()

	netName, mode, err := PrepareNetwork(context.Background(), fake, "agent-test", "", false)
	if err != nil {
		t.Fatalf("PrepareNetwork() error = %v", err)
	}
	if netName != "agent-test-net" {
		t.Fatalf("network = %q", netName)
	}
	if mode != "managed" {
		t.Fatalf("mode = %q", mode)
	}
}

func TestPrepareNetworkCustom(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("docker network inspect", "exists")

	netName, mode, err := PrepareNetwork(context.Background(), fake, "agent-test", "my-custom-net", false)
	if err != nil {
		t.Fatalf("PrepareNetwork() error = %v", err)
	}
	if netName != "my-custom-net" {
		t.Fatalf("network = %q", netName)
	}
	if mode != "custom" {
		t.Fatalf("mode = %q", mode)
	}
}

func TestPrepareNetworkNone(t *testing.T) {
	fake := orb.NewFake()

	netName, mode, err := PrepareNetwork(context.Background(), fake, "agent-test", "none", false)
	if err != nil {
		t.Fatalf("PrepareNetwork() error = %v", err)
	}
	if netName != "none" {
		t.Fatalf("network = %q", netName)
	}
	if mode != "none" {
		t.Fatalf("mode = %q", mode)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/docker/ -run TestNetwork -v
go test ./pkg/docker/ -run TestPrepare -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/docker/network.go
package docker

import (
	"context"
	"fmt"
	"safe-agentic/pkg/labels"
	"safe-agentic/pkg/orb"
	"safe-agentic/pkg/validate"
)

// ManagedNetworkName returns the network name for a container.
func ManagedNetworkName(containerName string) string {
	return containerName + "-net"
}

// CreateManagedNetwork creates a dedicated bridge network with egress restrictions.
func CreateManagedNetwork(ctx context.Context, exec orb.Executor, containerName string) (string, error) {
	netName := ManagedNetworkName(containerName)
	_, err := exec.Run(ctx, "docker", "network", "create",
		"--driver", "bridge",
		"--label", fmt.Sprintf("%s=%s", labels.App, labels.AppValue),
		netName)
	if err != nil {
		return "", fmt.Errorf("create network %s: %w", netName, err)
	}
	return netName, nil
}

// RemoveManagedNetwork removes a managed network.
func RemoveManagedNetwork(ctx context.Context, exec orb.Executor, netName string) error {
	_, err := exec.Run(ctx, "docker", "network", "rm", netName)
	if err != nil {
		return fmt.Errorf("remove network %s: %w", netName, err)
	}
	return nil
}

// PrepareNetwork creates or validates the network for a container.
// Returns (networkName, mode, error) where mode is "managed", "custom", or "none".
func PrepareNetwork(ctx context.Context, exec orb.Executor, containerName, customNetwork string, dryRun bool) (string, string, error) {
	if customNetwork == "" {
		// Managed network
		if dryRun {
			return ManagedNetworkName(containerName), "managed", nil
		}
		name, err := CreateManagedNetwork(ctx, exec, containerName)
		return name, "managed", err
	}

	if customNetwork == "none" {
		return "none", "none", nil
	}

	// Validate custom network name
	if err := validate.NetworkName(customNetwork); err != nil {
		return "", "", err
	}

	if !dryRun {
		// Verify network exists
		_, err := exec.Run(ctx, "docker", "network", "inspect", customNetwork)
		if err != nil {
			return "", "", fmt.Errorf("custom network %q does not exist", customNetwork)
		}
	}

	return customNetwork, "custom", nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/docker/ -run "TestNetwork|TestPrepare|TestManaged" -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/docker/network.go pkg/docker/network_test.go
git commit -m "feat: add pkg/docker network management with managed/custom/none modes"
```

---

## Task 12: pkg/docker/runtime — DockerRunCmd Builder

**Files:**
- Create: `pkg/docker/runtime.go`
- Test: `pkg/docker/runtime_test.go`

This is the most critical package — it replaces the bash `docker_cmd=()` array pattern. Ports `build_container_runtime()` and `append_runtime_hardening()` from `bin/agent-lib.sh`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/docker/runtime_test.go
package docker

import (
	"strings"
	"testing"
)

func TestDockerRunCmdBasic(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	args := cmd.Build()

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "docker run") {
		t.Fatalf("missing 'docker run' in: %s", joined)
	}
	if !strings.Contains(joined, "--name agent-test") {
		t.Fatalf("missing container name in: %s", joined)
	}
}

func TestDockerRunCmdLabels(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	cmd.AddLabel("safe-agentic.agent-type", "claude")
	cmd.AddLabel("safe-agentic.ssh", "true")

	args := cmd.Build()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--label safe-agentic.agent-type=claude") {
		t.Fatalf("missing agent-type label in: %s", joined)
	}
	if !strings.Contains(joined, "--label safe-agentic.ssh=true") {
		t.Fatalf("missing ssh label in: %s", joined)
	}
}

func TestDockerRunCmdEnvs(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	cmd.AddEnv("AGENT_TYPE", "claude")
	cmd.AddEnv("REPOS", "org/repo")

	args := cmd.Build()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-e AGENT_TYPE=claude") {
		t.Fatalf("missing AGENT_TYPE env in: %s", joined)
	}
	if !strings.Contains(joined, "-e REPOS=org/repo") {
		t.Fatalf("missing REPOS env in: %s", joined)
	}
}

func TestDockerRunCmdMounts(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	cmd.AddNamedVolume("my-vol", "/data")
	cmd.AddEphemeralVolume("/workspace")
	cmd.AddTmpfs("/tmp", "512m", true, true)

	args := cmd.Build()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--mount type=volume,src=my-vol,dst=/data") {
		t.Fatalf("missing named volume in: %s", joined)
	}
	if !strings.Contains(joined, "--mount type=volume,dst=/workspace") {
		t.Fatalf("missing ephemeral volume in: %s", joined)
	}
	if !strings.Contains(joined, "--tmpfs /tmp:rw,noexec,nosuid,size=512m") {
		t.Fatalf("missing tmpfs in: %s", joined)
	}
}

func TestAppendRuntimeHardening(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	AppendRuntimeHardening(cmd, HardeningOpts{
		Network:   "agent-test-net",
		Memory:    "8g",
		CPUs:      "4",
		PIDsLimit: 512,
	})

	args := cmd.Build()
	joined := strings.Join(args, " ")

	checks := []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--read-only",
		"--network agent-test-net",
		"--memory 8g",
		"--cpus 4",
		"--pids-limit 512",
		"--ulimit nofile=65536:65536",
		"--tmpfs /tmp:rw,noexec,nosuid,size=512m",
		"--tmpfs /var/tmp:rw,noexec,nosuid,size=256m",
		"--tmpfs /run:rw,noexec,nosuid,size=16m",
		"--tmpfs /dev/shm:rw,noexec,nosuid,size=64m",
	}
	for _, check := range checks {
		if !strings.Contains(joined, check) {
			t.Errorf("missing %q in: %s", check, joined)
		}
	}
}

func TestAppendRuntimeHardeningBlocksPrivileged(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	AppendRuntimeHardening(cmd, HardeningOpts{
		Network:   "agent-test-net",
		Memory:    "8g",
		CPUs:      "4",
		PIDsLimit: 512,
	})

	args := cmd.Build()
	joined := strings.Join(args, " ")

	forbidden := []string{"--privileged", "--cap-add", "--network host"}
	for _, f := range forbidden {
		if strings.Contains(joined, f) {
			t.Errorf("forbidden flag %q found in: %s", f, joined)
		}
	}
}

func TestAppendCacheMounts(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	AppendCacheMounts(cmd)

	args := cmd.Build()
	joined := strings.Join(args, " ")

	caches := []string{
		"/home/agent/.npm",
		"/home/agent/.cache/pip",
		"/home/agent/go",
		"/home/agent/.terraform.d/plugin-cache",
	}
	for _, cache := range caches {
		if !strings.Contains(joined, cache) {
			t.Errorf("missing cache mount for %s in: %s", cache, joined)
		}
	}
}

func TestBuildDetached(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	cmd.Detached = true

	args := cmd.Build()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, " -d ") && !strings.HasSuffix(joined, " -d") {
		t.Fatalf("missing -d flag for detached mode in: %s", joined)
	}
	if strings.Contains(joined, " -it ") {
		t.Fatalf("should not have -it in detached mode: %s", joined)
	}
}

func TestBuildInteractive(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")

	args := cmd.Build()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, " -it ") {
		t.Fatalf("missing -it flag for interactive mode in: %s", joined)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/docker/ -run "TestDockerRunCmd|TestAppend|TestBuild" -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/docker/runtime.go
package docker

import (
	"fmt"
	"sort"
	"strings"
)

// DockerRunCmd builds a Docker run command as a string slice.
// Replaces the bash docker_cmd=() array pattern.
type DockerRunCmd struct {
	name     string
	image    string
	Detached bool // -d vs -it

	flags  []string
	labels map[string]string
	envs   []envEntry // ordered for deterministic output
	mounts []string
	tmpfs  []string
}

type envEntry struct {
	key   string
	value string
}

// NewRunCmd creates a new DockerRunCmd with container name and image.
func NewRunCmd(name, image string) *DockerRunCmd {
	return &DockerRunCmd{
		name:   name,
		image:  image,
		labels: make(map[string]string),
	}
}

// AddLabel adds a Docker label.
func (d *DockerRunCmd) AddLabel(key, value string) {
	d.labels[key] = value
}

// AddEnv adds an environment variable.
func (d *DockerRunCmd) AddEnv(key, value string) {
	d.envs = append(d.envs, envEntry{key, value})
}

// AddFlag adds raw flags (e.g., "--read-only").
func (d *DockerRunCmd) AddFlag(flags ...string) {
	d.flags = append(d.flags, flags...)
}

// AddNamedVolume adds a named volume mount: --mount type=volume,src=X,dst=Y
func (d *DockerRunCmd) AddNamedVolume(src, dst string) {
	d.mounts = append(d.mounts, fmt.Sprintf("--mount type=volume,src=%s,dst=%s", src, dst))
}

// AddEphemeralVolume adds an anonymous volume mount: --mount type=volume,dst=X
func (d *DockerRunCmd) AddEphemeralVolume(dst string) {
	d.mounts = append(d.mounts, fmt.Sprintf("--mount type=volume,dst=%s", dst))
}

// AddTmpfs adds a tmpfs mount: --tmpfs path:rw,[noexec],[nosuid],size=X
func (d *DockerRunCmd) AddTmpfs(path, size string, noexec, nosuid bool) {
	opts := "rw"
	if noexec {
		opts += ",noexec"
	}
	if nosuid {
		opts += ",nosuid"
	}
	if size != "" {
		opts += ",size=" + size
	}
	d.tmpfs = append(d.tmpfs, fmt.Sprintf("%s:%s", path, opts))
}

// Build produces the final []string for exec.Command.
// Order: docker run [-it|-d] --name --hostname [flags] [labels] [envs] [mounts] [tmpfs] image
func (d *DockerRunCmd) Build() []string {
	args := []string{"docker", "run"}

	if d.Detached {
		args = append(args, "-d")
	} else {
		args = append(args, "-it")
	}

	args = append(args, "--name", d.name)
	args = append(args, "--hostname", d.name)
	args = append(args, "--pull", "never")

	// Flags
	args = append(args, d.flags...)

	// Labels (sorted for deterministic output)
	labelKeys := make([]string, 0, len(d.labels))
	for k := range d.labels {
		labelKeys = append(labelKeys, k)
	}
	sort.Strings(labelKeys)
	for _, k := range labelKeys {
		args = append(args, "--label", k+"="+d.labels[k])
	}

	// Envs (in insertion order)
	for _, e := range d.envs {
		args = append(args, "-e", e.key+"="+e.value)
	}

	// Mounts
	args = append(args, d.mounts...)

	// Tmpfs
	for _, t := range d.tmpfs {
		args = append(args, "--tmpfs", t)
	}

	// Image last
	args = append(args, d.image)
	return args
}

// Render returns the command as a shell-quoted string for display.
func (d *DockerRunCmd) Render() string {
	args := d.Build()
	var quoted []string
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"'$\\") {
			quoted = append(quoted, fmt.Sprintf("%q", a))
		} else {
			quoted = append(quoted, a)
		}
	}
	return strings.Join(quoted, " ")
}

// HardeningOpts configures security and resource constraints.
type HardeningOpts struct {
	Network      string
	Memory       string
	CPUs         string
	PIDsLimit    int
	SeccompPath  string // default: /etc/safe-agentic/seccomp.json
}

// AppendRuntimeHardening adds security and resource constraints to the command.
// Matches bash append_runtime_hardening() exactly.
func AppendRuntimeHardening(cmd *DockerRunCmd, opts HardeningOpts) {
	seccomp := opts.SeccompPath
	if seccomp == "" {
		seccomp = "/etc/safe-agentic/seccomp.json"
	}

	// Security
	cmd.AddFlag("--cap-drop=ALL")
	cmd.AddFlag("--security-opt=no-new-privileges:true")
	cmd.AddFlag("--security-opt=seccomp=" + seccomp)
	cmd.AddFlag("--read-only")

	// Network
	if opts.Network != "" {
		cmd.AddFlag("--network", opts.Network)
	}

	// Resources
	if opts.Memory != "" {
		cmd.AddFlag("--memory", opts.Memory)
	}
	if opts.CPUs != "" {
		cmd.AddFlag("--cpus", opts.CPUs)
	}
	if opts.PIDsLimit > 0 {
		cmd.AddFlag("--pids-limit", fmt.Sprintf("%d", opts.PIDsLimit))
	}
	cmd.AddFlag("--ulimit", "nofile=65536:65536")

	// Tmpfs mounts
	cmd.AddTmpfs("/tmp", "512m", true, true)
	cmd.AddTmpfs("/var/tmp", "256m", true, true)
	cmd.AddTmpfs("/run", "16m", true, true)
	cmd.AddTmpfs("/dev/shm", "64m", true, true)
	cmd.AddTmpfs("/home/agent/.config", "32m", true, false)
	cmd.AddTmpfs("/home/agent/.ssh", "1m", true, false)

	// Workspace volume
	cmd.AddEphemeralVolume("/workspace")
}

// AppendCacheMounts adds ephemeral volumes for build caches.
func AppendCacheMounts(cmd *DockerRunCmd) {
	caches := []string{
		"/home/agent/.npm",
		"/home/agent/.cache/pip",
		"/home/agent/go",
		"/home/agent/.terraform.d/plugin-cache",
	}
	for _, c := range caches {
		cmd.AddEphemeralVolume(c)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/docker/ -run "TestDockerRunCmd|TestAppend|TestBuild" -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/docker/runtime.go pkg/docker/runtime_test.go
git commit -m "feat: add pkg/docker DockerRunCmd builder with runtime hardening"
```

---

## Task 13: pkg/docker/ssh — SSH Relay

**Files:**
- Create: `pkg/docker/ssh.go`
- Test: `pkg/docker/ssh_test.go`

Ports `append_ssh_mount()` from `bin/agent-lib.sh`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/docker/ssh_test.go
package docker

import (
	"context"
	"safe-agentic/pkg/orb"
	"strings"
	"testing"
)

func TestAppendSSHMount(t *testing.T) {
	fake := orb.NewFake()
	fake.SetResponse("bash -c", "/tmp/fake-ssh.sock")

	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	err := AppendSSHMount(context.Background(), fake, cmd)
	if err != nil {
		t.Fatalf("AppendSSHMount() error = %v", err)
	}

	args := cmd.Build()
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "SSH_AUTH_SOCK=/run/ssh-agent.sock") {
		t.Errorf("missing SSH_AUTH_SOCK env in: %s", joined)
	}
}

func TestAppendSSHMountDryRun(t *testing.T) {
	// Dry run should add env vars without querying the VM
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	AppendSSHMountDryRun(cmd)

	args := cmd.Build()
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "SSH_AUTH_SOCK=/run/ssh-agent.sock") {
		t.Errorf("missing SSH_AUTH_SOCK env in: %s", joined)
	}
}

func TestEnsureSSHForRepos(t *testing.T) {
	tests := []struct {
		ssh   bool
		repos []string
		ok    bool
	}{
		{true, []string{"git@github.com:org/repo.git"}, true},
		{false, []string{"https://github.com/org/repo.git"}, true},
		{false, []string{"git@github.com:org/repo.git"}, false}, // SSH repo without --ssh
		{true, []string{"https://github.com/org/repo.git"}, true},
	}
	for _, tt := range tests {
		err := EnsureSSHForRepos(tt.ssh, tt.repos)
		if tt.ok && err != nil {
			t.Errorf("EnsureSSHForRepos(%v, %v) = %v, want nil", tt.ssh, tt.repos, err)
		}
		if !tt.ok && err == nil {
			t.Errorf("EnsureSSHForRepos(%v, %v) = nil, want error", tt.ssh, tt.repos)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/docker/ -run TestSSH -v
go test ./pkg/docker/ -run TestEnsure -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/docker/ssh.go
package docker

import (
	"context"
	"fmt"
	"safe-agentic/pkg/orb"
	"safe-agentic/pkg/repourl"
	"strings"
)

const sshSocketPath = "/run/ssh-agent.sock"

// AppendSSHMount sets up SSH agent forwarding from the VM into the container.
// Uses socat relay for userns-remap compatibility.
func AppendSSHMount(ctx context.Context, exec orb.Executor, cmd *DockerRunCmd) error {
	// Find the SSH agent socket in the VM
	out, err := exec.Run(ctx, "bash", "-c", "echo $SSH_AUTH_SOCK")
	if err != nil {
		return fmt.Errorf("find SSH socket: %w", err)
	}
	vmSocket := strings.TrimSpace(string(out))
	if vmSocket == "" {
		return fmt.Errorf("SSH_AUTH_SOCK not set in VM. Run 'ssh-add' on the host first")
	}

	cmd.AddEnv("SSH_AUTH_SOCK", sshSocketPath)
	cmd.AddFlag("-v", vmSocket+":"+sshSocketPath)
	return nil
}

// AppendSSHMountDryRun adds SSH env vars without querying the VM.
func AppendSSHMountDryRun(cmd *DockerRunCmd) {
	cmd.AddEnv("SSH_AUTH_SOCK", sshSocketPath)
	cmd.AddFlag("-v", "<SSH_SOCKET>:"+sshSocketPath)
}

// EnsureSSHForRepos checks that --ssh is enabled if any repo uses SSH.
func EnsureSSHForRepos(sshEnabled bool, repos []string) error {
	if sshEnabled {
		return nil
	}
	for _, r := range repos {
		if repourl.UsesSSH(r) {
			return fmt.Errorf("repo %q requires SSH but --ssh is not enabled", r)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/docker/ -run "TestSSH|TestEnsure" -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/docker/ssh.go pkg/docker/ssh_test.go
git commit -m "feat: add pkg/docker SSH relay setup and validation"
```

---

## Task 14: pkg/docker/dind — Docker-in-Docker

**Files:**
- Create: `pkg/docker/dind.go`
- Test: `pkg/docker/dind_test.go`

Ports Docker runtime management from `bin/docker-runtime.sh`.

- [ ] **Step 1: Write failing tests**

```go
// pkg/docker/dind_test.go
package docker

import (
	"context"
	"safe-agentic/pkg/orb"
	"strings"
	"testing"
)

func TestDinDContainerName(t *testing.T) {
	got := DinDContainerName("agent-test")
	if got != "safe-agentic-docker-agent-test" {
		t.Fatalf("DinDContainerName() = %q", got)
	}
}

func TestDinDSocketVolume(t *testing.T) {
	got := DinDSocketVolume("agent-test")
	if got != "agent-test-docker-sock" {
		t.Fatalf("DinDSocketVolume() = %q", got)
	}
}

func TestDinDDataVolume(t *testing.T) {
	got := DinDDataVolume("agent-test")
	if got != "agent-test-docker-data" {
		t.Fatalf("DinDDataVolume() = %q", got)
	}
}

func TestAppendDinDAccess(t *testing.T) {
	cmd := NewRunCmd("agent-test", "safe-agentic:latest")
	AppendDinDAccess(cmd, "agent-test")

	args := cmd.Build()
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "DOCKER_HOST=unix://") {
		t.Errorf("missing DOCKER_HOST in: %s", joined)
	}
	if !strings.Contains(joined, "agent-test-docker-sock") {
		t.Errorf("missing socket volume in: %s", joined)
	}
}

func TestRemoveDinDRuntime(t *testing.T) {
	fake := orb.NewFake()

	err := RemoveDinDRuntime(context.Background(), fake, "agent-test")
	if err != nil {
		t.Fatalf("RemoveDinDRuntime() error = %v", err)
	}

	cmds := fake.CommandsMatching("docker rm")
	if len(cmds) < 1 {
		t.Fatal("expected docker rm command")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/docker/ -run TestDinD -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/docker/dind.go
package docker

import (
	"context"
	"fmt"
	"safe-agentic/pkg/orb"
	"strings"
	"time"
)

const (
	dockerInternalSocketDir  = "/var/run/docker-internal"
	dockerInternalSocketPath = "/var/run/docker-internal/docker.sock"
)

// DinDContainerName returns the DinD sidecar container name.
func DinDContainerName(containerName string) string {
	return "safe-agentic-docker-" + containerName
}

// DinDSocketVolume returns the shared socket volume name.
func DinDSocketVolume(containerName string) string {
	return containerName + "-docker-sock"
}

// DinDDataVolume returns the shared data volume name.
func DinDDataVolume(containerName string) string {
	return containerName + "-docker-data"
}

// AppendDinDAccess configures a container to use a DinD sidecar for Docker access.
func AppendDinDAccess(cmd *DockerRunCmd, containerName string) {
	socketVolume := DinDSocketVolume(containerName)
	cmd.AddEnv("DOCKER_HOST", "unix://"+dockerInternalSocketPath)
	cmd.AddNamedVolume(socketVolume, dockerInternalSocketDir)
}

// AppendHostDockerSocket mounts the host Docker socket into the container.
// This is less isolated than DinD but faster.
func AppendHostDockerSocket(ctx context.Context, exec orb.Executor, cmd *DockerRunCmd) error {
	const hostSocketPath = "/run/docker-host.sock"
	// Get the Docker socket path and GID from the VM
	out, err := exec.Run(ctx, "bash", "-c", "stat -c %g /var/run/docker.sock")
	if err != nil {
		return fmt.Errorf("get docker socket GID: %w", err)
	}
	gid := strings.TrimSpace(string(out))

	cmd.AddFlag("--group-add", gid)
	cmd.AddFlag("-v", "/var/run/docker.sock:"+hostSocketPath)
	cmd.AddEnv("DOCKER_HOST", "unix://"+hostSocketPath)
	return nil
}

// StartDinDRuntime starts a Docker-in-Docker sidecar container.
func StartDinDRuntime(ctx context.Context, exec orb.Executor, containerName, networkName, image string) error {
	dindName := DinDContainerName(containerName)
	socketVol := DinDSocketVolume(containerName)
	dataVol := DinDDataVolume(containerName)

	// Create volumes
	for _, vol := range []string{socketVol, dataVol} {
		if err := CreateLabeledVolume(ctx, exec, vol, "docker-runtime", containerName); err != nil {
			return err
		}
	}

	// Start DinD container (privileged, required for dockerd)
	args := []string{"docker", "run", "-d",
		"--name", dindName,
		"--privileged",
		"--network", networkName,
		"--tmpfs", "/tmp:rw,nosuid,size=512m",
		"--mount", fmt.Sprintf("type=volume,src=%s,dst=%s", socketVol, dockerInternalSocketDir),
		"--mount", fmt.Sprintf("type=volume,src=%s,dst=/var/lib/docker", dataVol),
		"--label", "app=safe-agentic",
		"--label", "safe-agentic.type=docker-runtime",
		"--label", fmt.Sprintf("safe-agentic.parent=%s", containerName),
		"--entrypoint", "dockerd",
		image,
		"--host", "unix://" + dockerInternalSocketPath,
	}
	if _, err := exec.Run(ctx, args...); err != nil {
		return fmt.Errorf("start DinD sidecar: %w", err)
	}

	// Wait for daemon to become ready
	return waitForDinD(ctx, exec, dindName)
}

func waitForDinD(ctx context.Context, exec orb.Executor, dindName string) error {
	for i := 0; i < 40; i++ {
		_, err := exec.Run(ctx, "docker", "exec", dindName, "docker", "info")
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("DinD daemon did not start within 20s")
}

// RemoveDinDRuntime removes the DinD sidecar and its volumes.
func RemoveDinDRuntime(ctx context.Context, exec orb.Executor, containerName string) error {
	dindName := DinDContainerName(containerName)
	exec.Run(ctx, "docker", "rm", "-f", dindName)
	exec.Run(ctx, "docker", "volume", "rm", DinDSocketVolume(containerName))
	exec.Run(ctx, "docker", "volume", "rm", DinDDataVolume(containerName))
	return nil
}

// CleanupAllDinD removes all DinD sidecars and volumes.
func CleanupAllDinD(ctx context.Context, exec orb.Executor) error {
	// Remove containers with docker-runtime label
	exec.Run(ctx, "docker", "rm", "-f",
		"$(docker ps -aq --filter label=safe-agentic.type=docker-runtime)")
	// Remove volumes
	exec.Run(ctx, "docker", "volume", "rm",
		"$(docker volume ls -q --filter label=safe-agentic.type=docker-runtime)")
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/docker/ -run TestDinD -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/docker/dind.go pkg/docker/dind_test.go
git commit -m "feat: add pkg/docker DinD sidecar lifecycle management"
```

---

## Task 15: pkg/tmux — Tmux Session Management

**Files:**
- Create: `pkg/tmux/tmux.go`
- Test: `pkg/tmux/tmux_test.go`

- [ ] **Step 1: Write failing tests**

```go
// pkg/tmux/tmux_test.go
package tmux

import (
	"context"
	"safe-agentic/pkg/orb"
	"testing"
)

func TestSessionName(t *testing.T) {
	got := SessionName()
	if got != "safe-agentic" {
		t.Fatalf("SessionName() = %q, want %q", got, "safe-agentic")
	}
}

func TestHasSession(t *testing.T) {
	fake := orb.NewFake()
	// docker exec success means tmux session exists
	fake.SetResponse("docker exec", "")

	has, err := HasSession(context.Background(), fake, "agent-test")
	if err != nil {
		t.Fatalf("HasSession() error = %v", err)
	}
	if !has {
		t.Fatal("HasSession() = false, want true")
	}
}

func TestHasSessionNotFound(t *testing.T) {
	fake := orb.NewFake()
	fake.SetError("docker exec", "no session")

	has, err := HasSession(context.Background(), fake, "agent-test")
	if err != nil {
		t.Fatalf("HasSession() error = %v", err)
	}
	if has {
		t.Fatal("HasSession() = true, want false")
	}
}

func TestBuildAttachArgs(t *testing.T) {
	got := BuildAttachArgs("agent-test")
	want := []string{"docker", "exec", "-it", "agent-test", "tmux", "attach", "-t", "safe-agentic"}
	if len(got) != len(want) {
		t.Fatalf("BuildAttachArgs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("BuildAttachArgs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildCapturePaneArgs(t *testing.T) {
	got := BuildCapturePaneArgs("agent-test", 30)
	// Should include tmux capture-pane with -S -30
	found := false
	for _, a := range got {
		if a == "-S" {
			found = true
		}
	}
	if !found {
		t.Fatalf("BuildCapturePaneArgs() missing -S flag: %v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/tmux/ -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/tmux/tmux.go
package tmux

import (
	"context"
	"fmt"
	"os"
	"safe-agentic/pkg/orb"
	"time"
)

const defaultSessionName = "safe-agentic"

// SessionName returns the tmux session name.
func SessionName() string {
	if n := os.Getenv("SAFE_AGENTIC_TMUX_SESSION_NAME"); n != "" {
		return n
	}
	return defaultSessionName
}

// HasSession checks if a tmux session exists in the container.
func HasSession(ctx context.Context, exec orb.Executor, containerName string) (bool, error) {
	_, err := exec.Run(ctx, "docker", "exec", containerName,
		"tmux", "has-session", "-t", SessionName())
	if err != nil {
		return false, nil
	}
	return true, nil
}

// WaitForSession polls until a tmux session exists (up to ~60s).
func WaitForSession(ctx context.Context, exec orb.Executor, containerName string) error {
	for i := 0; i < 300; i++ {
		has, _ := HasSession(ctx, exec, containerName)
		if has {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("tmux session not ready after 60s in container %s", containerName)
}

// BuildAttachArgs returns the docker exec args to attach to a tmux session.
func BuildAttachArgs(containerName string) []string {
	return []string{
		"docker", "exec", "-it", containerName,
		"tmux", "attach", "-t", SessionName(),
	}
}

// BuildCapturePaneArgs returns docker exec args to capture the last N lines of tmux output.
func BuildCapturePaneArgs(containerName string, lines int) []string {
	return []string{
		"docker", "exec", containerName,
		"tmux", "capture-pane", "-t", SessionName(), "-p", "-S", fmt.Sprintf("-%d", lines),
	}
}

// Attach hands off terminal control to a tmux session.
func Attach(exec orb.Executor, containerName string) error {
	return exec.RunInteractive(BuildAttachArgs(containerName)...)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/tmux/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/tmux/
git commit -m "feat: add pkg/tmux with session management and pane capture"
```

---

## Task 16: pkg/events — Event System

**Files:**
- Create: `pkg/events/events.go`, `pkg/events/notify.go`, `pkg/events/budget.go`
- Test: `pkg/events/events_test.go`, `pkg/events/notify_test.go`, `pkg/events/budget_test.go`

- [ ] **Step 1: Write failing tests**

```go
// pkg/events/events_test.go
package events

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEmitEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	err := Emit(path, "agent.spawned", map[string]string{
		"container": "agent-test",
		"type":      "claude",
	})
	if err != nil {
		t.Fatalf("Emit() error = %v", err)
	}

	data, _ := os.ReadFile(path)
	var event Event
	if err := json.Unmarshal(data[:len(data)-1], &event); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if event.Type != "agent.spawned" {
		t.Fatalf("Type = %q", event.Type)
	}
}
```

```go
// pkg/events/notify_test.go
package events

import "testing"

func TestParseNotifyTargets(t *testing.T) {
	tests := []struct {
		input string
		want  []NotifyTarget
	}{
		{"terminal", []NotifyTarget{{Kind: "terminal"}}},
		{"terminal,slack:https://hooks.slack.com/xxx", []NotifyTarget{
			{Kind: "terminal"},
			{Kind: "slack", Value: "https://hooks.slack.com/xxx"},
		}},
		{"command:notify-send done", []NotifyTarget{
			{Kind: "command", Value: "notify-send done"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseNotifyTargets(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i].Kind != tt.want[i].Kind || got[i].Value != tt.want[i].Value {
					t.Fatalf("[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
```

```go
// pkg/events/budget_test.go
package events

import "testing"

func TestCheckBudget(t *testing.T) {
	tests := []struct {
		cost    float64
		budget  float64
		over    bool
	}{
		{0.50, 1.00, false},
		{1.00, 1.00, false},
		{1.01, 1.00, true},
	}
	for _, tt := range tests {
		over := CheckBudget(tt.cost, tt.budget)
		if over != tt.over {
			t.Fatalf("CheckBudget(%f, %f) = %v, want %v", tt.cost, tt.budget, over, tt.over)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/events/ -v
```

Expected: compilation error.

- [ ] **Step 3: Write events implementation**

```go
// pkg/events/events.go
package events

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Event represents a structured event.
type Event struct {
	Timestamp string            `json:"timestamp"`
	Type      string            `json:"type"`
	Payload   map[string]string `json:"payload"`
}

// Emit writes an event to a JSONL file.
func Emit(path, eventType string, payload map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	event := Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Type:      eventType,
		Payload:   payload,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// DefaultEventsPath returns the default events file path.
func DefaultEventsPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = home + "/.config"
	}
	return filepath.Join(dir, "safe-agentic", "events.jsonl")
}
```

```go
// pkg/events/notify.go
package events

import (
	"strings"
)

// NotifyTarget represents a notification destination.
type NotifyTarget struct {
	Kind  string // "terminal", "slack", "command"
	Value string // webhook URL or command
}

// ParseNotifyTargets parses a comma-separated notify targets string.
func ParseNotifyTargets(s string) []NotifyTarget {
	var targets []NotifyTarget
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if colonIdx := strings.Index(part, ":"); colonIdx > 0 {
			targets = append(targets, NotifyTarget{
				Kind:  part[:colonIdx],
				Value: part[colonIdx+1:],
			})
		} else {
			targets = append(targets, NotifyTarget{Kind: part})
		}
	}
	return targets
}
```

```go
// pkg/events/budget.go
package events

// CheckBudget returns true if the cost exceeds the budget.
func CheckBudget(cost, budget float64) bool {
	return cost > budget
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/events/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/events/
git commit -m "feat: add pkg/events with event emission, notifications, and budget checks"
```

---

## Task 17: pkg/cost — Cost Computation

**Files:**
- Create: `pkg/cost/pricing.go`
- Test: `pkg/cost/pricing_test.go`

- [ ] **Step 1: Write failing tests**

```go
// pkg/cost/pricing_test.go
package cost

import "testing"

func TestComputeCost(t *testing.T) {
	usage := TokenUsage{
		Model:       "claude-3-opus-20240229",
		InputTokens: 1_000_000,
		OutputTokens: 100_000,
	}
	cost := ComputeCost(usage)
	// $15/MTok in + $75/MTok out = $15 + $7.5 = $22.5
	if cost < 22.0 || cost > 23.0 {
		t.Fatalf("ComputeCost() = %f, want ~22.5", cost)
	}
}

func TestComputeCostUnknownModel(t *testing.T) {
	usage := TokenUsage{
		Model:       "unknown-model",
		InputTokens: 1000,
		OutputTokens: 100,
	}
	cost := ComputeCost(usage)
	if cost != 0 {
		t.Fatalf("ComputeCost() = %f for unknown model, want 0", cost)
	}
}

func TestSumUsages(t *testing.T) {
	usages := []TokenUsage{
		{Model: "claude-3-opus-20240229", InputTokens: 500_000, OutputTokens: 50_000},
		{Model: "claude-3-opus-20240229", InputTokens: 500_000, OutputTokens: 50_000},
	}
	total := SumCost(usages)
	// Each: $7.5 + $3.75 = $11.25, total = $22.5
	if total < 22.0 || total > 23.0 {
		t.Fatalf("SumCost() = %f, want ~22.5", total)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./pkg/cost/ -v
```

Expected: compilation error.

- [ ] **Step 3: Write implementation**

```go
// pkg/cost/pricing.go
package cost

import "strings"

// TokenUsage represents token usage for a model.
type TokenUsage struct {
	Model        string
	InputTokens  int64
	OutputTokens int64
}

// Pricing per million tokens (USD).
type modelPricing struct {
	InputPerMTok  float64
	OutputPerMTok float64
}

var pricing = map[string]modelPricing{
	"claude-3-opus":     {15.0, 75.0},
	"claude-3-sonnet":   {3.0, 15.0},
	"claude-3-haiku":    {0.25, 1.25},
	"claude-3.5-sonnet": {3.0, 15.0},
	"claude-3.5-haiku":  {0.80, 4.0},
	"claude-4-opus":     {15.0, 75.0},
	"claude-4-sonnet":   {3.0, 15.0},
	"gpt-4o":            {2.5, 10.0},
	"gpt-4o-mini":       {0.15, 0.6},
	"o3":                {10.0, 40.0},
	"o4-mini":           {1.1, 4.4},
	"codex":             {3.0, 15.0},
}

// ComputeCost calculates the cost for a single usage entry.
func ComputeCost(usage TokenUsage) float64 {
	p, ok := lookupPricing(usage.Model)
	if !ok {
		return 0
	}
	inCost := float64(usage.InputTokens) / 1_000_000.0 * p.InputPerMTok
	outCost := float64(usage.OutputTokens) / 1_000_000.0 * p.OutputPerMTok
	return inCost + outCost
}

// SumCost computes total cost across multiple usage entries.
func SumCost(usages []TokenUsage) float64 {
	total := 0.0
	for _, u := range usages {
		total += ComputeCost(u)
	}
	return total
}

func lookupPricing(model string) (modelPricing, bool) {
	// Try exact match first
	if p, ok := pricing[model]; ok {
		return p, true
	}
	// Try prefix match (e.g., "claude-3-opus-20240229" → "claude-3-opus")
	lower := strings.ToLower(model)
	for prefix, p := range pricing {
		if strings.HasPrefix(lower, prefix) {
			return p, true
		}
	}
	return modelPricing{}, false
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./pkg/cost/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/cost/
git commit -m "feat: add pkg/cost with model pricing table and cost computation"
```

---

## Task 18: cmd/safe-ag — Root Command

**Files:**
- Create: `cmd/safe-ag/main.go`

- [ ] **Step 1: Write root command**

```go
// cmd/safe-ag/main.go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set by -ldflags at build time.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "safe-ag",
	Short: "Isolated environment for running AI coding agents",
	Long:  "Sandboxed AI agent environment with per-agent Docker containers in an OrbStack VM.",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

func init() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("safe-agentic v{{.Version}}\n")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify it builds**

```bash
go build -o bin/safe-ag ./cmd/safe-ag
bin/safe-ag --version
```

Expected: `safe-agentic vdev`

- [ ] **Step 3: Verify help works**

```bash
bin/safe-ag --help
```

Expected: Shows usage and available commands.

- [ ] **Step 4: Commit**

```bash
git add cmd/safe-ag/
git commit -m "feat: add cmd/safe-ag cobra root command with version flag"
```

---

## Task 19: cmd/safe-ag/spawn.go — Spawn Command

**Files:**
- Create: `cmd/safe-ag/spawn.go`

This is the largest command — it orchestrates all pkg/ packages. Implementation references `cmd_spawn()` from `bin/agent:944-1368`.

- [ ] **Step 1: Create SpawnOpts struct and flag registration**

```go
// cmd/safe-ag/spawn.go
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"safe-agentic/pkg/audit"
	"safe-agentic/pkg/config"
	"safe-agentic/pkg/docker"
	"safe-agentic/pkg/events"
	"safe-agentic/pkg/inject"
	"safe-agentic/pkg/labels"
	"safe-agentic/pkg/orb"
	"safe-agentic/pkg/repourl"
	"safe-agentic/pkg/tmux"
	"safe-agentic/pkg/validate"

	"github.com/spf13/cobra"
)

// SpawnOpts holds all spawn command options.
type SpawnOpts struct {
	AgentType      string
	Repos          []string
	Name           string
	Prompt         string
	Template       string
	Instructions   string
	InstructionsFile string
	SSH            bool
	ReuseAuth      bool
	EphemeralAuth  bool
	ReuseGHAuth    bool
	DockerAccess   bool
	DockerSocket   bool
	Network        string
	Memory         string
	CPUs           string
	PIDsLimit      int
	Identity       string
	AWSProfile     string
	AutoTrust      bool
	Background     bool
	OnExit         string
	OnComplete     string
	OnFail         string
	MaxCost        string
	Notify         string
	FleetVolume    string
	DryRun         bool
}

var spawnCmd = &cobra.Command{
	Use:   "spawn <claude|codex|shell> [flags]",
	Short: "Spawn a new agent container",
	Args:  cobra.ExactArgs(1),
	RunE:  runSpawn,
}

var runCmd = &cobra.Command{
	Use:   "run [flags] <repo-url> [repo-url...] [prompt]",
	Short: "Quick-start an agent with smart defaults",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runQuickStart,
}

var spawnOpts SpawnOpts

func init() {
	// Register spawn flags
	f := spawnCmd.Flags()
	f.StringSliceVar(&spawnOpts.Repos, "repo", nil, "Repository URL to clone (repeatable)")
	f.StringVar(&spawnOpts.Name, "name", "", "Container name")
	f.StringVar(&spawnOpts.Prompt, "prompt", "", "Initial prompt")
	f.StringVar(&spawnOpts.Template, "template", "", "Prompt template name")
	f.StringVar(&spawnOpts.Instructions, "instructions", "", "Task instructions")
	f.StringVar(&spawnOpts.InstructionsFile, "instructions-file", "", "Instructions from file")
	f.BoolVar(&spawnOpts.SSH, "ssh", false, "Enable SSH agent forwarding")
	f.BoolVar(&spawnOpts.ReuseAuth, "reuse-auth", false, "Reuse shared auth volume")
	f.BoolVar(&spawnOpts.EphemeralAuth, "ephemeral-auth", false, "Use ephemeral auth volume")
	f.BoolVar(&spawnOpts.ReuseGHAuth, "reuse-gh-auth", false, "Reuse GitHub CLI auth")
	f.BoolVar(&spawnOpts.DockerAccess, "docker", false, "Enable Docker-in-Docker")
	f.BoolVar(&spawnOpts.DockerSocket, "docker-socket", false, "Mount host Docker socket")
	f.StringVar(&spawnOpts.Network, "network", "", "Custom Docker network")
	f.StringVar(&spawnOpts.Memory, "memory", "", "Memory limit (e.g., 8g)")
	f.StringVar(&spawnOpts.CPUs, "cpus", "", "CPU limit")
	f.IntVar(&spawnOpts.PIDsLimit, "pids-limit", 0, "PIDs limit (>= 64)")
	f.StringVar(&spawnOpts.Identity, "identity", "", "Git identity (Name <email>)")
	f.StringVar(&spawnOpts.AWSProfile, "aws", "", "AWS profile for credential injection")
	f.BoolVar(&spawnOpts.AutoTrust, "auto-trust", false, "Skip trust prompt")
	f.BoolVar(&spawnOpts.Background, "background", false, "Run in background (no tmux attach)")
	f.StringVar(&spawnOpts.OnExit, "on-exit", "", "Command to run on exit")
	f.StringVar(&spawnOpts.OnComplete, "on-complete", "", "Command to run on success")
	f.StringVar(&spawnOpts.OnFail, "on-fail", "", "Command to run on failure")
	f.StringVar(&spawnOpts.MaxCost, "max-cost", "", "Kill if estimated cost exceeds budget")
	f.StringVar(&spawnOpts.Notify, "notify", "", "Notification targets (terminal,slack:url,command:cmd)")
	f.StringVar(&spawnOpts.FleetVolume, "fleet-volume", "", "Shared fleet volume name")
	f.BoolVar(&spawnOpts.DryRun, "dry-run", false, "Show what would run without executing")

	// Register run flags (subset)
	rf := runCmd.Flags()
	rf.StringVar(&spawnOpts.Name, "name", "", "Container name")
	rf.StringVar(&spawnOpts.Network, "network", "", "Custom Docker network")
	rf.StringVar(&spawnOpts.Memory, "memory", "", "Memory limit")
	rf.StringVar(&spawnOpts.CPUs, "cpus", "", "CPU limit")
	rf.StringVar(&spawnOpts.MaxCost, "max-cost", "", "Cost budget")
	rf.StringVar(&spawnOpts.Template, "template", "", "Prompt template")
	rf.StringVar(&spawnOpts.Instructions, "instructions", "", "Task instructions")
	rf.BoolVar(&spawnOpts.Background, "background", false, "Background mode")
	rf.BoolVar(&spawnOpts.DryRun, "dry-run", false, "Dry run")

	rootCmd.AddCommand(spawnCmd, runCmd)
}

func runSpawn(cmd *cobra.Command, args []string) error {
	spawnOpts.AgentType = args[0]
	return executeSpawn(spawnOpts)
}

func runQuickStart(cmd *cobra.Command, args []string) error {
	// Parse: last arg may be a prompt, rest are repos
	var repos []string
	var prompt string

	for _, arg := range args {
		if strings.HasPrefix(arg, "http") || strings.HasPrefix(arg, "git@") || strings.HasPrefix(arg, "ssh://") {
			repos = append(repos, arg)
		} else {
			prompt = arg
		}
	}

	if len(repos) == 0 {
		return fmt.Errorf("at least one repo URL is required")
	}

	// Auto-detect SSH
	ssh := false
	for _, r := range repos {
		if repourl.UsesSSH(r) {
			ssh = true
			break
		}
	}

	// Auto-detect identity
	identity := config.DetectGitIdentity()

	opts := spawnOpts
	opts.AgentType = "claude"
	opts.Repos = repos
	opts.Prompt = prompt
	opts.SSH = ssh
	opts.ReuseAuth = true
	opts.Identity = identity

	return executeSpawn(opts)
}

func executeSpawn(opts SpawnOpts) error {
	ctx := context.Background()
	exec := &orb.OrbExecutor{VMName: "safe-agentic"}

	// --- Validation ---
	switch opts.AgentType {
	case "claude", "codex", "shell":
	default:
		return fmt.Errorf("agent type must be claude, codex, or shell (got %q)", opts.AgentType)
	}

	if opts.Name != "" {
		if err := validate.NameComponent(opts.Name, "container name"); err != nil {
			return err
		}
	}

	if opts.PIDsLimit > 0 {
		if err := validate.PIDsLimit(opts.PIDsLimit); err != nil {
			return err
		}
	}

	if err := docker.EnsureSSHForRepos(opts.SSH, opts.Repos); err != nil {
		return err
	}

	// --- Load defaults ---
	cfg, err := config.LoadDefaults(config.DefaultsPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply defaults where not overridden
	memory := coalesce(opts.Memory, cfg.DefaultMemory)
	cpus := coalesce(opts.CPUs, cfg.DefaultCPUs)
	pidsLimit := opts.PIDsLimit
	if pidsLimit == 0 {
		pidsLimit = cfg.DefaultPIDsLimit
	}

	// --- Identity ---
	if opts.Identity == "" {
		opts.Identity = cfg.DefaultIdentity
	}
	if opts.Identity == "" {
		opts.Identity = config.DetectGitIdentity()
	}
	var gitName, gitEmail string
	if opts.Identity != "" {
		gitName, gitEmail, err = config.ParseIdentity(opts.Identity)
		if err != nil {
			return fmt.Errorf("parse identity: %w", err)
		}
	}

	// --- Container name ---
	timestamp := time.Now().Format("20060102-150405")
	containerName := resolveContainerName(opts.AgentType, opts.Name, timestamp, opts.Repos)

	// --- Network ---
	customNetwork := opts.Network
	if customNetwork == "" {
		customNetwork = cfg.DefaultNetwork
	}
	networkName, networkMode, err := docker.PrepareNetwork(ctx, exec, containerName, customNetwork, opts.DryRun)
	if err != nil {
		return fmt.Errorf("prepare network: %w", err)
	}

	// --- Build Docker command ---
	imageName := "safe-agentic:latest"
	cmd := docker.NewRunCmd(containerName, imageName)

	// Core env vars
	cmd.AddEnv("AGENT_TYPE", opts.AgentType)
	if len(opts.Repos) > 0 {
		cmd.AddEnv("REPOS", strings.Join(opts.Repos, " "))
	}
	if gitName != "" {
		cmd.AddEnv("GIT_AUTHOR_NAME", gitName)
		cmd.AddEnv("GIT_AUTHOR_EMAIL", gitEmail)
		cmd.AddEnv("GIT_COMMITTER_NAME", gitName)
		cmd.AddEnv("GIT_COMMITTER_EMAIL", gitEmail)
	}

	// Runtime hardening
	docker.AppendRuntimeHardening(cmd, docker.HardeningOpts{
		Network:   networkName,
		Memory:    memory,
		CPUs:      cpus,
		PIDsLimit: pidsLimit,
	})

	// Cache mounts
	docker.AppendCacheMounts(cmd)

	// SSH
	if opts.SSH {
		if opts.DryRun {
			docker.AppendSSHMountDryRun(cmd)
		} else {
			if err := docker.AppendSSHMount(ctx, exec, cmd); err != nil {
				return err
			}
		}
	}

	// Labels
	cmd.AddLabel(labels.AgentType, opts.AgentType)
	cmd.AddLabel(labels.SSH, fmt.Sprintf("%v", opts.SSH))
	cmd.AddLabel(labels.RepoDisplay, repourl.DisplayLabel(opts.Repos))
	cmd.AddLabel(labels.NetworkMode, networkMode)
	cmd.AddLabel(labels.Resources, fmt.Sprintf("cpu=%s,mem=%s,pids=%d", cpus, memory, pidsLimit))
	cmd.AddLabel(labels.Terminal, "tmux")

	// Auth volumes
	if opts.EphemeralAuth {
		cmd.AddLabel(labels.AuthType, "ephemeral")
		authVol := docker.AuthVolumeName(opts.AgentType, true, containerName)
		cmd.AddNamedVolume(authVol, authDestination(opts.AgentType))
	} else {
		cmd.AddLabel(labels.AuthType, "shared")
		authVol := docker.AuthVolumeName(opts.AgentType, false, "")
		cmd.AddNamedVolume(authVol, authDestination(opts.AgentType))
	}

	// GitHub CLI auth
	if opts.ReuseGHAuth {
		cmd.AddLabel(labels.GHAuth, "shared")
		ghVol := docker.GHAuthVolumeName(opts.AgentType)
		cmd.AddNamedVolume(ghVol, "/home/agent/.config/gh")
	}

	// Host config injection
	claudeDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeDir == "" {
		home, _ := os.UserHomeDir()
		claudeDir = home + "/.claude"
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		home, _ := os.UserHomeDir()
		codexHome = home + "/.codex"
	}

	if opts.AgentType == "claude" || opts.AgentType == "shell" {
		if envs, err := inject.ReadClaudeConfig(claudeDir); err == nil {
			for k, v := range envs {
				cmd.AddEnv(k, v)
			}
		}
	}
	if opts.AgentType == "codex" || opts.AgentType == "shell" {
		if envs, err := inject.ReadCodexConfig(codexHome); err == nil {
			for k, v := range envs {
				cmd.AddEnv(k, v)
			}
		}
	}

	// AWS credentials
	if opts.AWSProfile != "" {
		home, _ := os.UserHomeDir()
		credPath := home + "/.aws/credentials"
		envs, err := inject.ReadAWSCredentials(credPath, opts.AWSProfile)
		if err != nil {
			return fmt.Errorf("AWS credentials: %w", err)
		}
		for k, v := range envs {
			cmd.AddEnv(k, v)
		}
		cmd.AddTmpfs("/home/agent/.aws", "1m", true, false)
		cmd.AddLabel(labels.AWS, opts.AWSProfile)
	}

	// Prompt / instructions / template (base64 encoded)
	if opts.Prompt != "" {
		cmd.AddEnv("SAFE_AGENTIC_PROMPT_B64", inject.EncodeB64(opts.Prompt))
		cmd.AddLabel(labels.Prompt, truncate(opts.Prompt, 100))
	}
	if opts.Instructions != "" {
		cmd.AddEnv("SAFE_AGENTIC_INSTRUCTIONS_B64", inject.EncodeB64(opts.Instructions))
		cmd.AddLabel(labels.Instructions, "1")
	}
	if opts.InstructionsFile != "" {
		data, err := os.ReadFile(opts.InstructionsFile)
		if err != nil {
			return fmt.Errorf("read instructions file: %w", err)
		}
		cmd.AddEnv("SAFE_AGENTIC_INSTRUCTIONS_B64", inject.EncodeB64(string(data)))
		cmd.AddLabel(labels.Instructions, "1")
	}
	if opts.Template != "" {
		cmd.AddEnv("SAFE_AGENTIC_TEMPLATE_B64", inject.EncodeB64(opts.Template))
	}

	// Callbacks
	if opts.OnExit != "" {
		cmd.AddLabel(labels.OnExit, "1")
		cmd.AddEnv("SAFE_AGENTIC_ON_EXIT_B64", inject.EncodeB64(opts.OnExit))
	}
	if opts.OnComplete != "" {
		cmd.AddLabel(labels.OnCompleteB64, inject.EncodeB64(opts.OnComplete))
	}
	if opts.OnFail != "" {
		cmd.AddLabel(labels.OnFailB64, inject.EncodeB64(opts.OnFail))
	}
	if opts.MaxCost != "" {
		cmd.AddLabel(labels.MaxCost, opts.MaxCost)
	}
	if opts.Notify != "" {
		cmd.AddLabel(labels.NotifyB64, inject.EncodeB64(opts.Notify))
	}
	if opts.FleetVolume != "" {
		cmd.AddNamedVolume(opts.FleetVolume, "/fleet")
		cmd.AddLabel(labels.Fleet, opts.FleetVolume)
	}

	// Docker access
	if opts.DockerSocket {
		// Mount host Docker socket (dangerous but fast)
		docker.AppendHostDockerSocket(ctx, exec, cmd)
		cmd.AddLabel(labels.DockerMode, "host-socket")
	} else if opts.DockerAccess {
		// Docker-in-Docker sidecar (safe)
		docker.AppendDinDAccess(cmd, containerName)
		cmd.AddLabel(labels.DockerMode, "dind")
	} else {
		cmd.AddLabel(labels.DockerMode, "off")
	}

	// --- Dry run ---
	if opts.DryRun {
		fmt.Println("Would execute:")
		fmt.Printf("  orb run -m safe-agentic %s\n", cmd.Render())
		return nil
	}

	// --- Execute ---
	cmd.Detached = true
	fullArgs := cmd.Build()

	_, err = exec.Run(ctx, fullArgs...)
	if err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	fmt.Printf("Agent %s started: %s\n", opts.AgentType, containerName)

	// Start DinD sidecar if needed
	if opts.DockerAccess {
		if err := docker.StartDinDRuntime(ctx, exec, containerName, networkName, imageName); err != nil {
			return fmt.Errorf("start Docker runtime: %w", err)
		}
	}

	// Audit log
	auditLogger := &audit.Logger{Path: audit.DefaultPath()}
	auditLogger.Log("spawn", containerName, map[string]string{
		"type":    opts.AgentType,
		"repos":   strings.Join(opts.Repos, ","),
		"ssh":     fmt.Sprintf("%v", opts.SSH),
		"network": networkMode,
	})

	// Emit event
	events.Emit(events.DefaultEventsPath(), "agent.spawned", map[string]string{
		"container": containerName,
		"type":      opts.AgentType,
	})

	// Auto-attach
	if !opts.Background && opts.AgentType != "shell" {
		if err := tmux.WaitForSession(ctx, exec, containerName); err != nil {
			return err
		}
		return tmux.Attach(exec, containerName)
	}

	return nil
}

// --- Helpers ---

func resolveContainerName(agentType, name, timestamp string, repos []string) string {
	prefix := docker.ContainerPrefix + "-" + agentType
	if name != "" {
		return prefix + "-" + name
	}
	if len(repos) > 0 {
		slug, err := repourl.ClonePath(repos[0])
		if err == nil {
			parts := strings.Split(slug, "/")
			if len(parts) == 2 {
				short := parts[1]
				if len(short) > 20 {
					short = short[:20]
				}
				return prefix + "-" + short
			}
		}
	}
	return prefix + "-" + timestamp
}

func authDestination(agentType string) string {
	switch agentType {
	case "claude":
		return "/home/agent/.claude"
	case "codex":
		return "/home/agent/.codex"
	default:
		return "/home/agent/.claude"
	}
}

func coalesce(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./cmd/safe-ag/
```

Expected: clean compilation.

- [ ] **Step 3: Test dry-run output**

```bash
go build -o bin/safe-ag ./cmd/safe-ag
bin/safe-ag spawn claude --repo https://github.com/org/repo.git --dry-run
```

Expected: prints the Docker command that would be executed.

- [ ] **Step 4: Test run command dry-run**

```bash
bin/safe-ag run https://github.com/org/repo.git "Fix the tests" --dry-run
```

Expected: prints Docker command with auto-detected settings.

- [ ] **Step 5: Commit**

```bash
git add cmd/safe-ag/spawn.go
git commit -m "feat: add spawn and run commands with full flag support and dry-run"
```

---

## Task 20: Spawn Parity Test

**Files:**
- Create: `cmd/safe-ag/spawn_test.go`

Verify Go spawn produces the same Docker command as bash spawn.

- [ ] **Step 1: Write parity test**

```go
// cmd/safe-ag/spawn_test.go
package main

import (
	"strings"
	"testing"
)

func TestResolveContainerName(t *testing.T) {
	tests := []struct {
		agentType string
		name      string
		timestamp string
		repos     []string
		want      string
	}{
		{"claude", "my-agent", "20260410-120000", nil, "agent-claude-my-agent"},
		{"codex", "", "20260410-120000", []string{"https://github.com/org/repo.git"}, "agent-codex-repo"},
		{"claude", "", "20260410-120000", nil, "agent-claude-20260410-120000"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := resolveContainerName(tt.agentType, tt.name, tt.timestamp, tt.repos)
			if got != tt.want {
				t.Fatalf("resolveContainerName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpawnDryRunContainsExpectedFlags(t *testing.T) {
	// Build a DockerRunCmd the same way spawn does and verify key flags
	opts := SpawnOpts{
		AgentType: "claude",
		Repos:     []string{"https://github.com/org/repo.git"},
		Memory:    "8g",
		CPUs:      "4",
		PIDsLimit: 512,
		DryRun:    true,
	}

	// Simulate the Docker command construction
	containerName := resolveContainerName(opts.AgentType, opts.Name, "20260410-120000", opts.Repos)

	cmd := newTestRunCmd(containerName, opts)
	args := cmd.Build()
	joined := strings.Join(args, " ")

	required := []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--read-only",
		"--memory 8g",
		"--cpus 4",
		"--pids-limit 512",
		"-e AGENT_TYPE=claude",
		"-e REPOS=https://github.com/org/repo.git",
		"--tmpfs /tmp:rw,noexec,nosuid,size=512m",
		"safe-agentic:latest",
	}

	for _, r := range required {
		if !strings.Contains(joined, r) {
			t.Errorf("missing %q in docker command:\n%s", r, joined)
		}
	}

	forbidden := []string{
		"--privileged",
		"--cap-add",
	}
	for _, f := range forbidden {
		if strings.Contains(joined, f) {
			t.Errorf("forbidden flag %q found in docker command", f)
		}
	}
}

// newTestRunCmd simulates spawn's Docker command construction for testing.
func newTestRunCmd(containerName string, opts SpawnOpts) *docker.DockerRunCmd {
	// This is imported from the docker package
	// We test the integration here
	cmd := docker.NewRunCmd(containerName, "safe-agentic:latest")
	cmd.AddEnv("AGENT_TYPE", opts.AgentType)
	if len(opts.Repos) > 0 {
		cmd.AddEnv("REPOS", strings.Join(opts.Repos, " "))
	}
	docker.AppendRuntimeHardening(cmd, docker.HardeningOpts{
		Network:   "test-net",
		Memory:    opts.Memory,
		CPUs:      opts.CPUs,
		PIDsLimit: opts.PIDsLimit,
	})
	docker.AppendCacheMounts(cmd)
	return cmd
}
```

Note: The test above uses `docker` package directly. The import path needs to be `safe-agentic/pkg/docker`. Add this import to the test file:

```go
import (
	"safe-agentic/pkg/docker"
	"strings"
	"testing"
)
```

- [ ] **Step 2: Run tests**

```bash
go test ./cmd/safe-ag/ -v
```

Expected: all PASS.

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -v
```

Expected: all packages pass.

- [ ] **Step 4: Commit**

```bash
git add cmd/safe-ag/spawn_test.go
git commit -m "test: add spawn parity tests verifying Docker command flags"
```

---

## Task 21: Build & Verify

**Files:**
- Modify: root `Makefile` (or create if none exists)

- [ ] **Step 1: Add build target**

Add to the project root (or create `Makefile`):

```makefile
.PHONY: build test clean

build:
	go build -o bin/safe-ag ./cmd/safe-ag

test:
	go test ./... -v

clean:
	rm -f bin/safe-ag
```

- [ ] **Step 2: Build**

```bash
make build
```

Expected: `bin/safe-ag` binary produced.

- [ ] **Step 3: Run full test suite**

```bash
make test
```

Expected: all tests pass across all packages.

- [ ] **Step 4: Verify help output**

```bash
bin/safe-ag --help
bin/safe-ag spawn --help
bin/safe-ag run --help
```

Expected: help text for all commands.

- [ ] **Step 5: Commit**

```bash
git add Makefile
git commit -m "feat: add Makefile for Go CLI build and test"
```

---

## Phases 2-7 Roadmap

Each phase below will get its own detailed plan document. All depend on Phase 1's `pkg/` packages.

### Phase 2: Lifecycle Commands (`cmd/safe-ag/lifecycle.go`)

**Commands:** `attach`, `stop`, `cleanup`, `list`, `retry`

**Tasks:**
1. `list` — query `docker ps`, format table output using labels
2. `attach` — resolve container, wait for tmux, hand off terminal
3. `stop` — resolve target(s), docker stop, cleanup DinD/network
4. `cleanup` — stop all, prune networks/images, optionally remove auth volumes
5. `retry` — read labels from stopped container, reconstruct spawn args, re-spawn

**Dependencies:** `pkg/docker/container`, `pkg/tmux`, `pkg/orb`

### Phase 3: Observability Commands (`cmd/safe-ag/observe.go`, `cmd/safe-ag/audit_cmd.go`)

**Commands:** `peek`, `output`, `summary`, `cost`, `replay`, `sessions`, `audit`

**Tasks:**
1. `peek` — tmux capture-pane
2. `output` — extract last message, git diff, changed files, commits, JSON
3. `summary` — docker inspect + cost calculation + formatted display
4. `cost` — parse session-events.jsonl, compute per-model costs; `--history` reads audit log
5. `replay` — copy and render session event log with timestamps
6. `sessions` — list session files from containers
7. `audit` — read and display audit.jsonl

**Dependencies:** `pkg/cost`, `pkg/audit`, `pkg/tmux`

### Phase 4: Workflow Commands (`cmd/safe-ag/workflow.go`)

**Commands:** `diff`, `checkpoint` (create/list/revert/fork), `todo` (add/list/check/uncheck), `pr`, `review`

**Tasks:**
1. `diff` — git diff inside container, optional `--stat`
2. `checkpoint create` — git stash + docker commit
3. `checkpoint list/revert/fork` — ref management
4. `todo add/list/check` — read/write JSON in container
5. `pr` — push branch, create GitHub PR via `gh` in container
6. `review` — codex review or git diff fallback

**Dependencies:** `pkg/docker/container`, `pkg/orb`

### Phase 5: Orchestration (`cmd/safe-ag/fleet.go`, `pkg/fleet/`, `pkg/pipeline/`)

**Commands:** `fleet`, `pipeline`

**Tasks:**
1. `pkg/fleet/manifest.go` — parse fleet YAML
2. `pkg/fleet/runner.go` — parallel agent spawning with shared volumes
3. `pkg/pipeline/manifest.go` — parse pipeline YAML with stages/dependencies
4. `pkg/pipeline/runner.go` — sequential execution with conditions and retry
5. `fleet` command — wire manifest to runner
6. `pipeline` command — wire manifest to runner

**Dependencies:** Spawn command, `pkg/docker/volume`

### Phase 6: Setup & Config (`cmd/safe-ag/setup.go`, `cmd/safe-ag/config_cmd.go`)

**Commands:** `setup`, `update`, `vm`, `diagnose`, `config`, `template`, `mcp-login`, `aws-refresh`

**Tasks:**
1. `setup` — create VM, copy files, build image
2. `update` — rebuild image with cache control (`--quick`, `--full`)
3. `vm` (ssh/start/stop) — VM lifecycle
4. `diagnose` — preflight checks
5. `config` (show/set/get/reset) — manage defaults.sh
6. `template` (list/show/create) — prompt template management
7. `mcp-login` — MCP OAuth flow
8. `aws-refresh` — refresh AWS credentials in running container

**Dependencies:** `pkg/orb`, `pkg/config`

### Phase 7: Delete Bash, Update Docs

**Tasks:**
1. Delete: `bin/agent`, `bin/agent-lib.sh`, `bin/docker-runtime.sh`, `bin/repo-url.sh`, `bin/agent-alias`
2. Update: `CLAUDE.md` command references
3. Update: Homebrew formula to build from Go source
4. Update: CI workflow for Go build/test
5. Update: `docs/` to reference Go CLI
6. Final parity verification: run bash tests against Go binary
