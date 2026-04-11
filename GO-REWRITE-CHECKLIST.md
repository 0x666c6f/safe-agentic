# Go Rewrite Implementation Checklist

## Overview
This document maps the comprehensive reference (REWRITE-REFERENCE.md) to actionable Go implementation tasks.

## Phase 1: Core CLI Framework
- [ ] **CLI Dispatcher** 
  - [ ] Command parsing (cobra/pflag or custom)
  - [ ] Help system
  - [ ] Error handling (die/warn/info patterns)
  - [ ] All ~42 cmd_* functions (see REWRITE-REFERENCE.md §2)

- [ ] **Validation Functions** (REWRITE-REFERENCE.md §3)
  - [ ] validate_name_component (regex: `^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
  - [ ] validate_pids_limit (min 64)
  - [ ] validate_network_name (reject bridge/host/container:*, allow none)

- [ ] **Config Management** (REWRITE-REFERENCE.md §3, §8)
  - [ ] Load defaults.sh (parser for KEY=value format)
  - [ ] Detect & apply git identity
  - [ ] Allowed keys whitelist validation

## Phase 2: Docker Runtime Builder
- [ ] **Container Name Resolution** (REWRITE-REFERENCE.md §3)
  - [ ] resolve_container_name (with repo slug detection)
  - [ ] resolve_latest_container
  - [ ] container_exists checks

- [ ] **Network Management** (REWRITE-REFERENCE.md §3)
  - [ ] create_managed_network (with bridge naming & ICC disabled)
  - [ ] remove_managed_network (with label safety check)
  - [ ] prepare_network (orchestration)
  - [ ] ensure_custom_network

- [ ] **Volume Management** (REWRITE-REFERENCE.md §3)
  - [ ] append_ephemeral_volume
  - [ ] append_named_volume
  - [ ] append_cache_mounts (npm, pip, go, terraform)
  - [ ] auth_volume_exists
  - [ ] volume_contains_file

- [ ] **Runtime Hardening** (REWRITE-REFERENCE.md §3)
  - [ ] append_runtime_hardening (all caps, seccomp, tmpfs, limits)
  - [ ] build_container_runtime (orchestrates all above)

- [ ] **SSH Mounting** (REWRITE-REFERENCE.md §3)
  - [ ] SSH relay via socat (complex)
  - [ ] append_ssh_mount with fallback

- [ ] **Config Injection** (REWRITE-REFERENCE.md §3)
  - [ ] inject_host_config (Claude/Codex)
  - [ ] inject_aws_credentials
  - [ ] Base64 encoding utilities

## Phase 3: Container Execution
- [ ] **orb Integration** (REWRITE-REFERENCE.md §3, §11)
  - [ ] vm_exec wrapper
  - [ ] vm_copy_from_host
  - [ ] Timeout handling (5s, 30s variants)

- [ ] **Container Lifecycle** (REWRITE-REFERENCE.md §3)
  - [ ] run_container (with docker_cmd array execution)
  - [ ] run_container_detached (-it → -d conversion)

- [ ] **tmux Session Management** (REWRITE-REFERENCE.md §3)
  - [ ] container_has_tmux_session
  - [ ] wait_for_tmux_session
  - [ ] attach_tmux_session
  - [ ] container_terminal_mode

## Phase 4: Audit, Events & Budget (REWRITE-REFERENCE.md §3)
- [ ] **Audit Logging** 
  - [ ] audit_log (append-only JSONL with timestamp, action, container, details)
  - [ ] File location: `$SAFE_AGENTIC_AUDIT_LOG` or `~/.config/safe-agentic/audit.jsonl`

- [ ] **Event System**
  - [ ] emit_event (JSONL to arbitrary file)
  - [ ] dispatch_event (command/webhook/file sinks)

- [ ] **Budget Enforcement**
  - [ ] compute_running_cost (parse session JSONL, apply pricing table)
  - [ ] check_budget (cost ≤ max_cost)
  - [ ] start_budget_monitor (background loop, 10s polling)

- [ ] **Notifications**
  - [ ] send_notification (terminal/slack/command)
  - [ ] parse_notify_targets (comma-separated)

## Phase 5: Complex Commands
- [ ] **cmd_spawn** (REWRITE-REFERENCE.md §2)
  - [ ] All argument parsing (25+ flags)
  - [ ] Label generation (safe-agentic.*)
  - [ ] Env var preparation
  - [ ] Network + volume setup
  - [ ] Docker command building

- [ ] **cmd_fleet** (REWRITE-REFERENCE.md §2)
  - [ ] YAML manifest parsing (manual or stdlib)
  - [ ] Parallel agent spawning
  - [ ] Shared volume management

- [ ] **cmd_pipeline** (REWRITE-REFERENCE.md §2)
  - [ ] YAML manifest parsing
  - [ ] Dependency graph (topological sort)
  - [ ] Sequential execution with waits
  - [ ] Retry logic
  - [ ] on_failure handling

- [ ] **cmd_docker** (docker-runtime.sh integration)
  - [ ] Internal Docker daemon setup (DinD)
  - [ ] Volume for socket + data
  - [ ] Wait for daemon ready

- [ ] **Other Commands**
  - [ ] cmd_list (docker ps with labels)
  - [ ] cmd_attach (docker exec tmux)
  - [ ] cmd_stop / cmd_cleanup
  - [ ] cmd_cost (budget + pricing)
  - [ ] cmd_audit (JSONL viewer)
  - [ ] All others (~30 more)

## Phase 6: Docker-in-Docker (docker-runtime.sh)
- [ ] **Internal Docker Daemon** (REWRITE-REFERENCE.md §3, bin/docker-runtime.sh)
  - [ ] Privileged daemon container lifecycle
  - [ ] Socket + data volumes
  - [ ] Wait-for-ready polling (40 retries × 0.5s)
  - [ ] Cleanup for containers

- [ ] **Host Socket Access**
  - [ ] Detect VM docker socket path
  - [ ] Get socket GID
  - [ ] Mount & group-add

## Phase 7: Testing Infrastructure
- [ ] **Test Helpers**
  - [ ] Fake orb binary (Go version)
  - [ ] Test fixture setup/teardown
  - [ ] Command logging capture

- [ ] **Unit Tests**
  - [ ] Validation functions (name, pids, network)
  - [ ] Container name resolution
  - [ ] Cost computation (with pricing table)
  - [ ] YAML parsing (fleet/pipeline)

- [ ] **Integration Tests**
  - [ ] End-to-end spawn flow (with fake orb)
  - [ ] Network creation & cleanup
  - [ ] Label verification

## Phase 8: TUI Integration
- [ ] **Keep Existing** (tui/ Go code already done)
  - [ ] Agent model, table, poller
  - [ ] Dashboard HTTP API

- [ ] **New CLI ↔ TUI Integration**
  - [ ] syscall.Exec for attach/spawn from TUI
  - [ ] JSON output mode for programmatic use

## Phase 9: Container Entrypoint (bin/agent-session.sh)
**Status**: Keep as-is (shell scripts inside container)
- Bash-based, runs inside container
- No need to rewrite to Go (different environment)

## Priority Order for Implementation
1. **Core CLI + validation** (Phase 1)
2. **Docker runtime builder** (Phase 2)
3. **Container execution** (Phase 3)
4. **cmd_spawn** (Phase 5) — most critical
5. **Audit + budget** (Phase 4)
6. **cmd_fleet + cmd_pipeline** (Phase 5) — high complexity
7. **All other commands** (Phase 5)
8. **Testing** (Phase 7)
9. **TUI integration** (Phase 8)
10. **Docker-in-Docker** (Phase 6) — optional first pass

## Data Structures to Define
```go
// From REWRITE-REFERENCE.md §3, §6
type Agent struct {
  Name        string
  Type        string  // claude, codex, shell
  Repo        string
  SSH         string  // on/off
  Status      string
  // ... more fields
}

type DockerRunCmd []string  // Array of args

type Label struct {
  Key   string
  Value string
}

type Config struct {
  CPUS         string
  Memory       string
  PIDsLimit    int
  // ... etc
}

type BudgetMonitor struct {
  ContainerName string
  MaxCost       float64
  EventsFile    string
}

type FleetManifest struct {
  Agents []AgentSpec
  // ...
}

type PipelineManifest struct {
  Name  string
  Steps []StepSpec
  // ...
}
```

## Key Files to Reference During Implementation
- `REWRITE-REFERENCE.md` — Complete source of truth
- `bin/agent` — Current bash implementation
- `bin/agent-lib.sh` — All library functions
- `bin/docker-runtime.sh` — Docker daemon setup
- `tui/poller.go` — Docker command patterns
- `tests/test-cli-dispatch.sh` — Test patterns

## Notes
- Every Docker command goes through `orb run -m safe-agentic`
- Label namespace is `safe-agentic.*` (see complete list in REWRITE-REFERENCE.md §6)
- Bash's `docker_cmd=()` array pattern must map to Go's `[]string`
- All base64 injections use encoding/base64, no tr -d needed
- Audit log format: JSON objects, one per line, NEVER modified after write
- Cost pricing table hard-coded (see REWRITE-REFERENCE.md §3)

