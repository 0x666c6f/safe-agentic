# Safe-Agentic Deep Exploration Summary

## What Was Explored
Complete codebase exploration of safe-agentic bash CLI, library functions, container setup, and Go TUI code.

## Output Files Created

### 1. **REWRITE-REFERENCE.md** (1,502 lines)
Comprehensive source-of-truth document covering:
- **§1**: Architecture overview (macOS + OrbStack + Docker + tmux)
- **§2**: ALL 42+ `cmd_*` functions in bin/agent with:
  - Line number, purpose, arguments, Docker commands, env vars, file I/O
  - **cmd_spawn** (944 lines) — detailed breakdown of all labels/flags/logic
- **§3**: ALL exported functions from agent-lib.sh with:
  - Validation, config, container resolution, network, volume, runtime building
  - SSH mounting, config injection, container execution
  - Audit, events, budget enforcement, tmux session management
  - Notification system
- **§4**: bin/agent-session.sh — session events, resumption, auto-trust, agent launch modes
- **§5**: entrypoint.sh — complete init sequence with config restoration
- **§6**: Docker labels — complete namespace reference (safe-agentic.*)
- **§7**: TUI Go code — App, Model, Poller, Dashboard structures
- **§8**: Config files & formats — defaults.sh, audit.jsonl, session-events.jsonl, Fleet/Pipeline YAML schemas
- **§9**: Test infrastructure — fake orb binary, test helpers
- **§10**: Env vars summary (user-set, host-to-container, internal)
- **§11**: Critical Docker commands & patterns
- **§12**: End-to-end execution flow example
- **§13**: Go rewrite checklist (high-level)
- **§14**: Critical design patterns (array-based docker commands, base64 injection, append-only logging)

### 2. **GO-REWRITE-CHECKLIST.md** (200 lines)
Actionable implementation guide with:
- 9 implementation phases with detailed task breakdowns
- Data structures to define
- Priority order (Phase 1-9)
- Key files to reference
- Design pattern notes

### 3. **EXPLORATION-SUMMARY.md** (this file)
Quick reference summary.

## Key Discoveries

### 1. Docker Runtime Building Pattern
- Bash uses `docker_cmd=()` array, appended to incrementally
- Each function adds flags: `append_runtime_hardening`, `append_ssh_mount`, etc.
- Final execution: `orb run -m safe-agentic "${docker_cmd[@]}"`
- Go rewrite must map this to `[]string` with careful append order

### 2. Label Namespace (safe-agentic.*)
Complete set of labels for containers, networks, volumes:
- Container: 20 label types (agent-type, ssh, auth, docker, network-mode, terminal, prompt, on-exit/complete/fail, max-cost, notify, mcp-oauth)
- Network: type marking for cleanup filtering
- Volume: type + parent container tracking
- Used heavily by TUI poller for agent classification and by cleanup for safety

### 3. SSH Relay Complexity
- Bash creates `/tmp/safe-agentic-ssh-relay.sh` script
- Uses `start-stop-daemon` to daemonize `socat` relay
- Mounts relay socket (world-readable) into container
- Fallback to direct mount if relay fails
- Go rewrite must handle both paths

### 4. Base64 Injection Pattern (Critical)
Multiple fields transported base64-encoded as Docker labels or env vars:
- Prompt: `--label "safe-agentic.prompt=<base64>"`
- AWS credentials: `-e "SAFE_AGENTIC_AWS_CREDS_B64=<base64>"`
- Claude config: `-e "SAFE_AGENTIC_CLAUDE_CONFIG_B64=<base64>"`
- Claude support files: tar+gzip+base64
- Instructions: `-e "SAFE_AGENTIC_INSTRUCTIONS_B64=<base64>"`
- Callbacks: `--label "safe-agentic.on-complete-b64=<base64>"`
- Notifications: `-e "SAFE_AGENTIC_NOTIFY_B64=<base64>"`
- No `tr -d '\n'` needed in Go (encoding/base64 handles it)

### 5. cmd_spawn is THE Critical Command
- ~400 lines of bash logic
- 25+ command-line flags
- Sets 40+ Docker labels + env vars
- Orchestrates 5+ sub-functions:
  - Network creation
  - Volume setup
  - SSH relay
  - Config injection (Claude/Codex/AWS)
  - Docker runtime hardening
- Must be implemented with extreme care for correctness

### 6. Budget Monitoring
- Separate background subprocess (backgrounded with `&`)
- Runs every 10 seconds
- Collects JSONL files from inside container via `docker exec`
- Parses token usage + model type
- Applies hard-coded pricing table (6 models)
- Stops container if budget exceeded
- Emits event to events file

### 7. Fleet & Pipeline Orchestration
- **Fleet**: Parallel agent spawning from YAML manifest
  - Python inline parser (no PyYAML dependency)
  - Shared volume for task coordination
  - All agents spawn in parallel
- **Pipeline**: Sequential steps with dependencies
  - Topological sort of `depends_on` graph
  - Waits for step completion before spawning dependent steps
  - Retry logic per step
  - on_failure: stop | continue

### 8. Container Lifecycle
- Containers persist after agent exits (no `--rm`)
- Cleanup is manual or via `agent cleanup`
- Network & volumes cleaned up separately (with label filtering)
- Audit logged for all actions

### 9. Poller Architecture (TUI)
- Every 2 seconds:
  - `docker ps -a --filter "name=^agent-"` — get all containers
  - `docker stats --no-stream --format '{{json .}}'` — get resource stats
  - `docker exec <container> pgrep -x <agent>` — activity probing
  - Parses JSON for each, merges into Agent struct
- Activity detection:
  - Reads `/proc/<pid>/stat` field 14+15 (utime+stime)
  - Measures delta over 1 second window
  - "Working" if delta > 0, else "Idle"

### 10. Test Infrastructure
- Fake `orb` binary (bash script) captures all commands in log
- Checks specific patterns: SSH socket, Docker socket, hardening verification
- Test helpers: `run_ok`, `run_fails`, `assert_output_contains`
- No need for real orb/docker in tests

---

## Critical Files for Go Implementation

### Read First
1. **REWRITE-REFERENCE.md** — Complete spec (this is your bible)
2. **bin/agent** — lines 944-1370 (cmd_spawn is most complex)
3. **bin/agent-lib.sh** — functions in order of dependency

### Reference During Implementation
1. **tui/poller.go** — Shows how to execute Docker commands with timeouts
2. **bin/docker-runtime.sh** — DinD setup logic
3. **tests/test-cli-dispatch.sh** — Test patterns

### Keep As-Is (No Rewrite Needed)
1. **entrypoint.sh** — Runs inside container, bash is fine
2. **bin/agent-session.sh** — Runs inside container, bash is fine
3. **tui/** — Already Go, no changes needed (only integration points)

---

## Bash-to-Go Translation Patterns

### Array Appending
```bash
docker_cmd=(docker run -it)
docker_cmd+=(--name container)
```

Maps to:
```go
dockerCmd := []string{"docker", "run", "-it"}
dockerCmd = append(dockerCmd, "--name", "container")
```

### Variable Interpolation + Base64
```bash
prompt_b64=$(printf '%s' "$prompt" | base64 | tr -d '\n')
docker_cmd+=(--label "safe-agentic.prompt=$prompt_b64")
```

Maps to:
```go
promptB64 := base64.StdEncoding.EncodeToString([]byte(prompt))
dockerCmd = append(dockerCmd, "--label", fmt.Sprintf("safe-agentic.prompt=%s", promptB64))
```

### Command Execution with Timeout
```bash
vm_exec() {
  orb run -m "$VM_NAME" "$@"
}
```

Maps to:
```go
func vmExec(args ...string) ([]byte, error) {
  ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
  defer cancel()
  fullArgs := append([]string{"run", "-m", "safe-agentic"}, args...)
  cmd := exec.CommandContext(ctx, "orb", fullArgs...)
  return cmd.CombinedOutput()
}
```

### Append-Only Logging
```bash
audit_log() {
  mkdir -p "$(dirname "$AUDIT_LOG_FILE")"
  python3 -c "import json, sys; print(json.dumps({...}))" >> "$AUDIT_LOG_FILE"
}
```

Maps to:
```go
func auditLog(action, container, details string) error {
  entry := map[string]string{
    "timestamp": time.Now().UTC().Format(time.RFC3339),
    "action": action,
    "container": container,
    "details": details,
  }
  file, _ := os.OpenFile(auditLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
  defer file.Close()
  return json.NewEncoder(file).Encode(entry)
}
```

---

## Line Count Summary

| File | Lines | Scope |
|------|-------|-------|
| bin/agent | 4,949 | 42+ cmd_* functions, main dispatch |
| bin/agent-lib.sh | 968 | 70+ functions (validation, docker, ssh, budget, audit) |
| bin/docker-runtime.sh | 169 | DinD setup & cleanup |
| entrypoint.sh | 400+ | Container init (keep as-is) |
| bin/agent-session.sh | 193 | Agent launch (keep as-is) |
| tui/*.go | 2,000+ | TUI app, poller, dashboard (keep, just integrate) |
| tests/ | 5,000+ lines across 30+ files | Test patterns visible in test-*.sh |

**Total bash to rewrite: ~6,000 lines → ~3,000-4,000 lines Go** (more expressive, cleaner error handling)

---

## Success Criteria for Go Rewrite

1. ✅ **Functional parity** — Every bash command works identically
2. ✅ **Label exactness** — Every Docker label matches exactly
3. ✅ **Docker command precision** — Flags order preserved, all args present
4. ✅ **Error handling** — Errors don't crash, are reported clearly
5. ✅ **Performance** — No significant slowdown vs bash
6. ✅ **Test coverage** — Unit tests for critical paths
7. ✅ **Integration** — TUI still works via syscall.Exec

---

## Recommended Go Dependencies

- **CLI parsing**: cobra (or custom, given complexity)
- **YAML**: gopkg.in/yaml.v3 (for fleet/pipeline)
- **Docker API**: Standard exec.Command (no SDK needed, CLI is sufficient)
- **Config file parsing**: Custom (simple KEY=value format)
- **JSON**: encoding/json (stdlib)
- **Base64**: encoding/base64 (stdlib)
- **Timestamps**: time (stdlib)
- **Exec with timeout**: context + exec.CommandContext (stdlib)

No need for Docker Go SDK — CLI interface is sufficient and preferred for consistency.

---

## Next Steps

1. Read REWRITE-REFERENCE.md thoroughly
2. Review GO-REWRITE-CHECKLIST.md for phase structure
3. Start with Phase 1 (CLI dispatcher + validation)
4. Implement Phase 2 (Docker runtime builder) in parallel
5. Move to Phase 3 (execution) once Phases 1-2 solid
6. Test heavily with fake orb binary (provided in tests/)
7. Integration test against real TUI

Good luck! This codebase is well-structured and this reference should make the Go rewrite straightforward.
