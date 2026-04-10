# Go Rewrite Design Spec

**Date:** 2026-04-10
**Status:** Draft
**Scope:** Rewrite `bin/agent` (4,949 lines), `bin/agent-lib.sh` (968 lines), `bin/docker-runtime.sh` (169 lines) from bash to Go. No legacy fallback. Clean break.

---

## Goal

Replace the bash CLI with a single Go binary (`safe-ag`) that has full functional parity with the current 42+ commands, proper argument parsing via cobra, shared code with the existing TUI and dashboard, and Go-native testing.

## What stays bash

- `entrypoint.sh` — runs inside containers, not on host
- `bin/agent-session.sh` — runs inside containers
- `config/bashrc` — shell config inside containers

## What becomes Go

Everything in `bin/agent`, `bin/agent-lib.sh`, `bin/docker-runtime.sh`, `bin/agent-claude`, `bin/agent-codex`, `bin/agent-alias`, `bin/repo-url.sh`.

## Architecture

```
cmd/
  safe-ag/
    main.go              # Entry point, cobra root command
    spawn.go             # cmd_spawn + cmd_run + cmd_shell
    lifecycle.go         # cmd_attach, cmd_stop, cmd_cleanup, cmd_list, cmd_retry
    observe.go           # cmd_peek, cmd_output, cmd_summary, cmd_cost, cmd_replay, cmd_sessions
    workflow.go          # cmd_diff, cmd_checkpoint, cmd_todo, cmd_pr, cmd_review
    fleet.go             # cmd_fleet, cmd_pipeline
    setup.go             # cmd_setup, cmd_update, cmd_vm, cmd_diagnose
    config_cmd.go        # cmd_config, cmd_template, cmd_mcp_login, cmd_aws_refresh
    audit_cmd.go         # cmd_audit
    help.go              # Custom help system

pkg/
  orb/
    orb.go               # vm_exec, vm_copy_from_host — all orb interactions
    orb_test.go

  docker/
    runtime.go           # DockerRunCmd builder, append_runtime_hardening
    runtime_test.go
    network.go           # create/remove managed network
    network_test.go
    volume.go            # volume helpers (auth, cache, fleet)
    volume_test.go
    ssh.go               # SSH relay setup, append_ssh_mount
    ssh_test.go
    dind.go              # Docker-in-Docker setup
    dind_test.go
    container.go         # container_exists, resolve_latest, inspect helpers
    container_test.go

  config/
    defaults.go          # Load/save defaults.sh, config get/set
    defaults_test.go
    identity.go          # Git identity detection, parsing "Name <email>"
    identity_test.go

  validate/
    validate.go          # validate_name_component, validate_network_name, validate_pids_limit
    validate_test.go

  labels/
    labels.go            # All safe-agentic.* label constants and helpers
    labels_test.go

  inject/
    inject.go            # Base64 encode/decode, config injection (claude/codex/aws)
    inject_test.go

  events/
    events.go            # emit_event, dispatch_event, event types
    events_test.go
    budget.go            # compute_running_cost, check_budget, start_budget_monitor
    budget_test.go
    notify.go            # send_notification, parse_notify_targets
    notify_test.go

  audit/
    audit.go             # audit_log, read audit entries
    audit_test.go

  tmux/
    tmux.go              # tmux session management (has_session, wait, attach, capture)
    tmux_test.go

  fleet/
    manifest.go          # Parse fleet YAML manifests
    manifest_test.go
    runner.go            # Parallel agent spawning, shared volumes
    runner_test.go

  pipeline/
    manifest.go          # Parse pipeline YAML manifests
    manifest_test.go
    runner.go            # Sequential execution, depends_on, retry, when/outputs
    runner_test.go

  repourl/
    parse.go             # repo_clone_path, URL validation, traversal prevention
    parse_test.go

  cost/
    pricing.go           # Model pricing table, cost computation
    pricing_test.go
```

## Key Design Decisions

### 1. No Docker Go SDK

All Docker interaction goes through `orb run -m safe-agentic docker ...` via `exec.Command`. Reasons:
- Matches existing behavior exactly (Docker runs inside OrbStack VM, not on host)
- No need for Docker socket on macOS host
- The TUI already uses this pattern successfully
- Simpler dependency tree

### 2. `pkg/orb` is the single execution gateway

Every Docker/VM command goes through `pkg/orb`. This makes testing trivial — mock `orb.Executor` interface:

```go
type Executor interface {
    Run(ctx context.Context, args ...string) ([]byte, error)
    RunInteractive(args ...string) error
}

type OrbExecutor struct{ VMName string }
type FakeExecutor struct{ Log [][]string; Responses map[string]string }
```

All command implementations receive an `Executor` — no global state.

### 3. `DockerRunCmd` builder pattern

```go
type DockerRunCmd struct {
    args   []string
    labels map[string]string
    envs   map[string]string
    mounts []Mount
}

func (d *DockerRunCmd) AddLabel(key, value string)
func (d *DockerRunCmd) AddEnv(key, value string)
func (d *DockerRunCmd) AddMount(mount Mount)
func (d *DockerRunCmd) AddFlag(flags ...string)
func (d *DockerRunCmd) Build() []string  // Returns final []string for exec
```

This replaces the bash `docker_cmd=()` array pattern with a type-safe builder.

### 4. Cobra for CLI

```go
var rootCmd = &cobra.Command{Use: "safe-ag"}

// Sub-projects register their commands
func init() {
    rootCmd.AddCommand(spawnCmd, runCmd, shellCmd)      // spawn.go
    rootCmd.AddCommand(attachCmd, stopCmd, cleanupCmd)  // lifecycle.go
    rootCmd.AddCommand(fleetCmd, pipelineCmd)           // fleet.go
    // ...
}
```

Flags use cobra's pflag. `--repo` is a StringSlice, `--ssh` is a Bool, etc.

### 5. Configuration

```go
type Config struct {
    DefaultCPUs      string  `key:"SAFE_AGENTIC_DEFAULT_CPUS" default:"4"`
    DefaultMemory    string  `key:"SAFE_AGENTIC_DEFAULT_MEMORY" default:"8g"`
    DefaultPIDsLimit int     `key:"SAFE_AGENTIC_DEFAULT_PIDS_LIMIT" default:"512"`
    DefaultReuseAuth bool    `key:"SAFE_AGENTIC_DEFAULT_REUSE_AUTH" default:"true"`
    DefaultReuseGHAuth bool  `key:"SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH" default:"true"`
    DefaultSSH       bool    `key:"SAFE_AGENTIC_DEFAULT_SSH" default:"false"`
    DefaultDocker    bool    `key:"SAFE_AGENTIC_DEFAULT_DOCKER" default:"false"`
    DefaultNetwork   string  `key:"SAFE_AGENTIC_DEFAULT_NETWORK" default:""`
    GitAuthorName    string
    GitAuthorEmail   string
}
```

Loaded from `~/.config/safe-agentic/defaults.sh` (same KEY=value format for backward compat).

### 6. Labels as constants

```go
const (
    LabelAgentType    = "safe-agentic.agent-type"
    LabelRepos        = "safe-agentic.repos"
    LabelSSH          = "safe-agentic.ssh"
    LabelAuthType     = "safe-agentic.auth-type"
    LabelNetworkMode  = "safe-agentic.network-mode"
    LabelDockerMode   = "safe-agentic.docker-mode"
    LabelResources    = "safe-agentic.resources"
    LabelPrompt       = "safe-agentic.prompt"
    LabelMaxCost      = "safe-agentic.max-cost"
    LabelOnExit       = "safe-agentic.on-exit-b64"
    LabelOnComplete   = "safe-agentic.on-complete-b64"
    LabelOnFail       = "safe-agentic.on-fail-b64"
    LabelNotify       = "safe-agentic.notify-b64"
    LabelFleet        = "safe-agentic.fleet"
    LabelTerminal     = "safe-agentic.terminal"
    LabelForkedFrom   = "safe-agentic.forked-from"
    // ... complete set
)
```

### 7. Testing strategy

- **Unit tests** per package (`*_test.go`) using `FakeExecutor`
- **Integration tests** using the same fake orb pattern as bash tests — but Go's `exec.Command` with a test binary
- **Go test binary trick**: set `OrbBinary` to a compiled test helper that logs commands
- **Table-driven tests** for validation, URL parsing, cost computation
- No dependency on real OrbStack/Docker for tests

### 8. Build & distribution

```makefile
# Makefile
build:
    go build -o bin/safe-ag ./cmd/safe-ag
    go build -o bin/agent-tui ./tui  # existing

install: build
    cp bin/safe-ag /usr/local/bin/
    cp bin/agent-tui /usr/local/bin/
```

Homebrew formula updated to build from Go source. Version injected via `-ldflags`.

### 9. TUI integration

The TUI binary (`tui/agent-tui`) stays separate. `safe-ag tui` does `syscall.Exec` to hand off. Shared packages (like `pkg/orb`, `pkg/docker/container`) imported by both binaries. Eventually the TUI code could move under `cmd/safe-ag` as a subcommand, but that's not in scope for this rewrite.

### 10. Entrypoint / agent-session stay bash

These run inside the Docker container where Go isn't installed. No change needed. The Go CLI only runs on the macOS host.

---

## Implementation Phases

### Phase 1: Foundation (pkg/ packages + `spawn`)
Create all `pkg/` packages with unit tests, implement `cmd_spawn` and `cmd_run` as the first working commands. This is the largest phase — spawn touches every package.

**Files:** All of `pkg/`, `cmd/safe-ag/main.go`, `cmd/safe-ag/spawn.go`
**Test:** Full spawn dry-run produces identical Docker commands to bash version
**Deliverable:** `safe-ag spawn` and `safe-ag run` work

### Phase 2: Lifecycle commands
Port `cmd_attach`, `cmd_stop`, `cmd_cleanup`, `cmd_list`, `cmd_retry`.

**Files:** `cmd/safe-ag/lifecycle.go`
**Test:** All lifecycle operations match bash behavior
**Deliverable:** Can manage agent lifecycle entirely from Go binary

### Phase 3: Observability commands
Port `cmd_peek`, `cmd_output`, `cmd_summary`, `cmd_cost`, `cmd_replay`, `cmd_sessions`, `cmd_audit`.

**Files:** `cmd/safe-ag/observe.go`, `cmd/safe-ag/audit_cmd.go`
**Deliverable:** All monitoring/inspection commands work

### Phase 4: Workflow commands
Port `cmd_diff`, `cmd_checkpoint` (including fork), `cmd_todo`, `cmd_pr`, `cmd_review`.

**Files:** `cmd/safe-ag/workflow.go`
**Deliverable:** Developer workflow commands work

### Phase 5: Orchestration
Port `cmd_fleet`, `cmd_pipeline` with YAML parsing, parallel/sequential execution, shared volumes, conditions.

**Files:** `cmd/safe-ag/fleet.go`, `pkg/fleet/`, `pkg/pipeline/`
**Deliverable:** Fleet and pipeline orchestration works

### Phase 6: Setup & config
Port `cmd_setup`, `cmd_update`, `cmd_vm`, `cmd_diagnose`, `cmd_config`, `cmd_template`, `cmd_mcp_login`, `cmd_aws_refresh`.

**Files:** `cmd/safe-ag/setup.go`, `cmd/safe-ag/config_cmd.go`
**Deliverable:** Full setup and configuration works

### Phase 7: Delete bash, update docs
Remove `bin/agent`, `bin/agent-lib.sh`, `bin/docker-runtime.sh`, `bin/repo-url.sh`, `bin/agent-alias`. Update CLAUDE.md, Homebrew formula, CI.

**Deliverable:** Clean Go-only CLI

---

## Functional parity checklist

Every bash command must produce identical Docker commands (same flags, labels, env vars, mounts in the same order). The test strategy is:

1. Run bash version with `--dry-run`, capture docker command
2. Run Go version with `--dry-run`, capture docker command
3. Compare — must match exactly (modulo whitespace)

This ensures zero behavioral regression.
