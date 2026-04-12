╭─ safe-agentic container
│  Agent: claude
│  Workspace: /workspace
╰─ Ready.
# Design Spec Review: Go Rewrite (Phase 1)

**Spec:** `docs/superpowers/specs/2026-04-10-go-rewrite-design.md`
**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`
**Reviewed:** 2026-04-10

---

## 1. Package Structure Deviations

### Missing packages

| Spec declares | Status | Notes |
|---------------|--------|-------|
| `pkg/pipeline/` (manifest.go, runner.go) | **Missing** | Pipeline types (`PipelineManifest`, `PipelineStage`) live in `pkg/fleet/manifest.go`. No separate package. |
| `pkg/fleet/runner.go` | **Missing** | Spec lists a fleet runner for parallel agent spawning and shared volumes. Not implemented. |
| `pkg/pipeline/runner.go` | **Missing** | Spec lists a pipeline runner for sequential execution, `depends_on`, retry, `when`/`outputs`. Not implemented. |

**Impact:** Fleet/pipeline parsing exists but there is no orchestration logic. `cmd/safe-ag/fleet.go` calls `fleet.ParseFleet()` and `fleet.ParsePipeline()` but must implement all spawning/execution inline or delegate to functions that don't exist yet. This is the biggest structural gap.

### Missing cmd files

| Spec declares | Status |
|---------------|--------|
| `cmd/safe-ag/audit_cmd.go` | **Missing** — audit command lives inside `observe.go` |
| `cmd/safe-ag/help.go` | **Missing** — no custom help system file |

These are minor organizational differences, not functional gaps.

### Extra files not in spec

| File | Notes |
|------|-------|
| `cmd/safe-ag/executor.go` | Global `newExecutor` factory. Spec doesn't mention this file. |

---

## 2. Executor Pattern Deviations

### Spec says
> "All command implementations receive an `Executor` — no global state."

### Actual
Commands do **not** receive an Executor. A **package-level variable** `newExecutor` (in `executor.go`) is a factory function that each `RunE` handler calls:

```go
var newExecutor = func() orb.Executor {
    return &orb.OrbExecutor{VMName: "safe-agentic"}
}
```

Every command does `exec := newExecutor()` at the top of its `RunE`. Tests override the package variable:

```go
original := newExecutor
newExecutor = func() orb.Executor { return fake }
```

**Assessment:** This is a pragmatic choice — it's testable via variable replacement, and avoids threading the executor through cobra's `RunE` signature. But it is global mutable state, contrary to the spec's "no global state" claim. The spec's `Executor` interface itself (signature, methods, `FakeExecutor`) matches exactly.

### Recommendation
Consider a `struct App { exec orb.Executor }` approach where commands are methods on `App`, eliminating the mutable global. Not blocking, but the spec should be updated to match reality.

---

## 3. DockerRunCmd Builder Deviations

### Spec says
```go
type DockerRunCmd struct {
    args   []string
    labels map[string]string
    envs   map[string]string
    mounts []Mount
}
func (d *DockerRunCmd) AddMount(mount Mount)
```

### Actual
```go
type DockerRunCmd struct {
    name     string      // added
    image    string      // added
    Detached bool        // added
    flags    []string
    labels   map[string]string
    envs     []envEntry  // []envEntry, not map (preserves insertion order)
    mounts   []string    // []string, not []Mount
    tmpfs    []string    // separate from mounts
    cmdArgs  []string    // added: args after image name
}
```

### Method differences

| Spec method | Actual | Notes |
|-------------|--------|-------|
| `AddMount(mount Mount)` | **Does not exist** | Replaced by `AddNamedVolume(src, dst)` and `AddEphemeralVolume(dst)` |
| — | `AddTmpfs(path, size, noexec, nosuid)` | **Extra**: tmpfs handled separately from mounts |
| — | `AddTmpfsOwned(path, size, noexec, nosuid, uid, gid)` | **Extra**: owner-aware tmpfs |
| — | `AddCmdArgs(args ...string)` | **Extra**: entrypoint arguments |
| — | `Render() string` | **Extra**: shell-quoted string for dry-run display |
| — | `NewRunCmd(name, image)` | **Extra**: constructor (spec had no constructor) |

**Assessment:** The actual implementation is **better** than the spec. Using `envEntry` structs instead of a map preserves insertion order (important for deterministic docker commands). Splitting tmpfs from mounts avoids type unions. The specialized `AddNamedVolume`/`AddEphemeralVolume` methods are more type-safe than a generic `AddMount(Mount)`. The spec should be updated to reflect these improvements.

---

## 4. Label Naming Deviations

| Spec constant name | Actual constant name | Label string |
|-------------------|---------------------|--------------|
| `LabelRepos` | `RepoDisplay` | `safe-agentic.repo-display` (spec said `safe-agentic.repos`) |
| `LabelAuthType` | `AuthType` | `safe-agentic.auth` (spec said `safe-agentic.auth-type`) |
| `LabelOnExit` | `OnExit` | `safe-agentic.on-exit` (spec said `safe-agentic.on-exit-b64`) |
| — | `GHAuth` | `safe-agentic.gh-auth` (not in spec) |
| — | `Instructions` | `safe-agentic.instructions` (not in spec) |
| — | `ForkLabel` | `safe-agentic.fork-label` (not in spec) |
| — | `AWS` | `safe-agentic.aws` (not in spec) |
| — | `App` | `app` (not in spec) |
| — | `Type` | `safe-agentic.type` (not in spec) |
| — | `Parent` | `safe-agentic.parent` (not in spec) |
| — | `AppValue` | `"safe-agentic"` (not in spec) |

The Go code also drops the `Label` prefix from constant names (e.g., `labels.AgentType` vs spec's `LabelAgentType`). This is idiomatic Go since the package name already provides the `labels.` qualifier.

**Assessment:** The naming is cleaner in the implementation. Three label string values differ from the spec (`repos` vs `repo-display`, `auth-type` vs `auth`, `on-exit-b64` vs `on-exit`). These must be reconciled with entrypoint.sh and any TUI code that reads these labels.

---

## 5. Config Struct Deviations

### Spec says
```go
type Config struct {
    DefaultCPUs      string  `key:"SAFE_AGENTIC_DEFAULT_CPUS" default:"4"`
    DefaultMemory    string  `key:"SAFE_AGENTIC_DEFAULT_MEMORY" default:"8g"`
    // ... struct tags with key: and default:
    GitAuthorName    string
    GitAuthorEmail   string
}
```

### Actual
```go
type Config struct {
    CPUs, Memory, PIDsLimit, SSH, Docker, DockerSocket string
    ReuseAuth, ReuseGHAuth, Network, Identity string
    GitAuthorName, GitAuthorEmail, GitCommitterName, GitCommitterEmail string
}
```

Differences:
- **No struct tags** — uses a manual `setField()` switch instead of reflection
- **Field names** drop the `Default` prefix (cleaner)
- **All fields are `string`** — spec had `bool` for `DefaultReuseAuth`, `DefaultSSH`, etc., and `int` for `DefaultPIDsLimit`
- **Extra fields:** `DockerSocket`, `GitCommitterName`, `GitCommitterEmail` (not in spec)

**Assessment:** Using strings everywhere avoids parsing complexity in the config layer; consumers convert as needed. The spec should be updated. The manual `setField()` switch is fine for 14 keys — reflection would be over-engineering.

---

## 6. Command Coverage: Spec vs Implementation

### Commands in implementation but NOT in spec architecture

| Command | File | Notes |
|---------|------|-------|
| `logs` | observe.go | Conversation log viewer with `--follow`. Spec doesn't mention it. |
| `dashboard` | setup.go | Web dashboard with `--bind` flag. Spec doesn't mention it. |
| `tui` | setup.go | TUI launcher. Spec mentions TUI integration (section 9) but doesn't list it as a command in the architecture. |

### Commands in spec but NOT implemented (or differ)

| Spec command | Status |
|--------------|--------|
| `shell` (standalone cobra command) | **Not a separate command** — "shell" is a valid agent type for `spawn` (`spawn shell`), not its own top-level command |
| `checkpoint fork` (subcommand) | **Missing** — `checkpoint` has `create`, `list`, `revert` but no `fork` |

### Missing helper functions (compilation blockers)

| Function | Called from | Status |
|----------|------------|--------|
| `addLatestFlag(cmd)` | observe.go (7 call sites) | **Never defined anywhere** |
| `targetFromArgs(cmd, args)` | observe.go (6 call sites) | **Never defined anywhere** |

These are called in `init()` blocks and `RunE` handlers respectively. **The code will not compile.** These need to be implemented — likely:

```go
func addLatestFlag(cmd *cobra.Command) {
    cmd.Flags().Bool("latest", false, "Target most recent container")
}

func targetFromArgs(cmd *cobra.Command, args []string) string {
    if latest, _ := cmd.Flags().GetBool("latest"); latest {
        return "--latest"
    }
    if len(args) > 0 {
        return args[0]
    }
    return ""
}
```

---

## 7. Testing

### Test file coverage

Every `pkg/` package has corresponding `*_test.go` files — **19 test files total**. This matches the spec's requirement of "unit tests per package using FakeExecutor."

`cmd/safe-ag/` has test files:
- `command_test.go` (~2997 lines)
- `deterministic_test.go` (~1089 lines)
- `lifecycle_test.go`
- `spawn_test.go`
- `integration_test.go` (~317 lines)

**Assessment:** Test infrastructure is solid. The `FakeExecutor` with `SetResponse`/`SetError`/`CommandsMatching` provides good mock capability. The deterministic test file likely validates that spawn produces identical Docker commands to the bash version (matching the spec's parity checklist).

---

## 8. Summary of Findings

### Blocking issues (must fix)

1. **`addLatestFlag()` and `targetFromArgs()` are undefined** — code won't compile. Implement these two helpers.

### Spec deviations to reconcile (update spec or code)

2. **`pkg/pipeline/` doesn't exist** — pipeline types are in `pkg/fleet/`. Update spec.
3. **`pkg/fleet/runner.go` doesn't exist** — no orchestration runtime. Implement or descope from Phase 1.
4. **`checkpoint fork` subcommand missing** — CLAUDE.md documents it, but it's not implemented.
5. **Executor is global state** via `newExecutor` var, not injected per-command. Update spec.
6. **Label string values differ** — `repos` vs `repo-display`, `auth-type` vs `auth`, `on-exit-b64` vs `on-exit`. Verify compatibility with `entrypoint.sh` and TUI.
7. **`audit_cmd.go` and `help.go` don't exist** as separate files. Update spec.
8. **DockerRunCmd API differs** significantly (better than spec). Update spec.
9. **Config struct** uses all-strings, no struct tags. Update spec.

### Good deviations (implementation is better than spec)

10. `envEntry` struct preserves env var ordering — important for deterministic builds.
11. Specialized `AddNamedVolume`/`AddEphemeralVolume`/`AddTmpfs` methods are more type-safe than generic `AddMount`.
12. `Render()` method enables clean `--dry-run` output.
13. `FakeExecutor` has richer API than spec (`SetResponse`, `SetError`, `CommandsMatching`, `Reset`).
14. `logs` command with `--follow` is a valuable addition not in spec.
15. Thread-safe `FakeExecutor` with `sync.Mutex` supports concurrent test scenarios.
