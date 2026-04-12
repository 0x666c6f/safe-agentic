╭─ safe-agentic container
│  Agent: claude
│  Workspace: /workspace
╰─ Ready.
# Documentation Review: Stale Bash References & Go Doc Comments

Branch: `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`

Confirmed removed files: `bin/agent`, `bin/agent-lib.sh`, `bin/agent-claude`, `bin/agent-codex`.
Remaining bash scripts (container-side, still valid): `bin/agent-session.sh`, `bin/repo-url.sh`.

---

## 1. CLAUDE.md

### Stale reference

| Line | Issue |
|------|-------|
| 189 | Security Model table says `refresh with `agent aws-refresh`` — should be `safe-ag aws-refresh` (rest of doc uses `safe-ag`) |

Otherwise well-updated for the Go rewrite.

---

## 2. docs/quickstart.md

### Stale references

| Lines | Issue |
|-------|-------|
| 19-24 | "From source" instructions say `add bin/ to your PATH` and `commands are agent, agent-claude, and agent-codex`. Should say `run make build` and the binary is `safe-ag` (matches CLAUDE.md line 19). |
| 29 | "The rest of this guide uses `agent` as the command name. Substitute `safe-ag` if installed via Homebrew." — With Go rewrite, from-source binary is also `safe-ag`. The entire Homebrew-vs-source distinction for command names no longer applies. |
| 33-66 | All command examples use `agent`, `agent-claude`, `agent-codex` — should use `safe-ag`, `safe-ag run`, etc. to match the Go CLI. |

---

## 3. docs/usage.md

### Stale references

| Lines | Issue |
|-------|-------|
| 5 | Note says Homebrew commands are `safe-ag` "instead of `agent`, `agent-claude`, and `agent-codex`". The from-source binary is now also `safe-ag`, so this distinction is wrong. |
| 9-47 | Command table uses `agent` throughout (e.g. `agent setup`, `agent spawn claude`, `agent-claude`). Should be `safe-ag`. |
| 54-651 | Every code block and example uses `agent` as the command. Full find-and-replace needed. |
| 75 | References "safe-agentic-managed networks" — this phrasing is fine (refers to the project name, not the CLI). |

This file needs a bulk `s/agent spawn/safe-ag spawn/`, `s/agent-claude/safe-ag run/`, `s/agent-codex/safe-ag run .* --type codex/`, `s/^agent /safe-ag /` pass, plus updating the intro note.

---

## 4. docs/security.md

No stale CLI command references found. All content describes the security model, flags, and architecture — no `agent` used as a command invocation. Clean.

---

## 5. Go Doc Comments on Exported Functions in pkg/

### 5a. Stale `bin/agent-lib.sh` / `bin/agent` references

These doc comments reference the now-removed bash files:

| File | Function | Stale text |
|------|----------|------------|
| `pkg/config/defaults.go:35` | `Defaults()` | "same hardcoded defaults as bin/agent" |
| `pkg/config/defaults.go:48` | `DefaultsPath()` | "mirrors the DEFAULTS_FILE variable in bin/agent-lib.sh" |
| `pkg/config/defaults.go:58` | `LoadDefaults()` | "mirrors load_user_defaults() / parse_defaults_line() / parse_defaults_value() in bin/agent-lib.sh" |
| `pkg/config/defaults.go:132` | `parseValue()` (unexported) | "mirrors parse_defaults_value() in bin/agent-lib.sh" |
| `pkg/config/defaults.go:158` | `KeyAllowed()` | "mirrors default_key_allowed() in bin/agent-lib.sh" |
| `pkg/config/identity.go:10` | `ParseIdentity()` | "mirrors the parse_identity() function in bin/agent-lib.sh" |
| `pkg/config/identity.go:34` | `DetectGitIdentity()` | "mirrors detect_git_identity() in bin/agent-lib.sh" |

**Recommendation:** Remove all "mirrors ... in bin/agent-lib.sh" phrases. These were useful during porting but are now dead references. Keep the functional descriptions.

### 5b. Missing doc comments on exported symbols

Per Go convention (`go vet`, `golint`), every exported function, type, const, and method should have a doc comment. The following are missing:

**pkg/audit/audit.go:**
- `Entry` (struct)
- `Logger` (struct)
- `DefaultPath()`, `Logger.Log()`, `Logger.Read()`

**pkg/cost/pricing.go:**
- `TokenUsage` (struct)
- `ComputeCost()`, `SumCost()`

**pkg/docker/container.go:**
- `ContainerExists()`, `ResolveLatest()`, `ResolveTarget()`, `InspectLabel()`, `IsRunning()`

**pkg/docker/dind.go:**
- `DinDContainerName()`, `DinDSocketVolume()`, `DinDDataVolume()`, `AppendDinDAccess()`, `AppendHostDockerSocket()`, `StartDinDRuntime()`, `RemoveDinDRuntime()`, `CleanupAllDinD()`

**pkg/docker/network.go:**
- `ManagedNetworkName()`, `CreateManagedNetwork()`, `RemoveManagedNetwork()`, `PrepareNetwork()`

**pkg/docker/runtime.go:**
- `DockerRunCmd` (struct), `NewRunCmd()`, `Render()`, `HardeningOpts` (struct), `AppendRuntimeHardening()`, `AppendCacheMounts()`
- Setter methods: `AddLabel()`, `AddEnv()`, `AddFlag()`, `AddCmdArgs()`, `AddNamedVolume()`, `AddEphemeralVolume()`, `AddTmpfs()`

**pkg/docker/ssh.go:**
- `AppendSSHMountDryRun()`, `EnsureSSHForRepos()`

**pkg/docker/volume.go:**
- `VolumeExists()`, `CreateLabeledVolume()`, `AuthVolumeName()`, `GHAuthVolumeName()`

**pkg/events/budget.go:**
- `CheckBudget()`

**pkg/events/events.go:**
- `Event` (struct), `Emit()`, `DefaultEventsPath()`

**pkg/events/notify.go:**
- `NotifyTarget` (struct), `ParseNotifyTargets()`

**pkg/inject/inject.go:**
- `EncodeB64()`, `DecodeB64()`, `EncodeFileB64()`, `ReadClaudeConfig()`, `ReadCodexConfig()`, `ReadAWSCredentials()`

**pkg/labels/labels.go:**
- All 20+ constants (consider a package-level doc comment grouping them)
- `ContainerFilter()`

**pkg/orb/orb.go:**
- `Executor` (interface), `OrbExecutor` (struct), `FakeExecutor` (struct)
- `NewFake()`, `SetResponse()`, `SetError()`, `LastCommand()`, `CommandsMatching()`, `Reset()`

**pkg/repourl/parse.go:**
- `ClonePath()`, `UsesSSH()`, `DisplayLabel()`

**pkg/tmux/tmux.go:**
- `SessionName()`, `HasSession()`, `WaitForSession()`, `BuildAttachArgs()`, `BuildCapturePaneArgs()`, `Attach()`

**pkg/validate/validate.go:**
- `NameComponent()`, `NetworkName()`, `PIDsLimit()`

### 5c. Symbols with good doc comments (no issues)

- `pkg/config/defaults.go`: `Config`, `Defaults`, `DefaultsPath`, `LoadDefaults`, `KeyAllowed` (content is good, just needs bash ref removal)
- `pkg/docker/runtime.go`: `Build()`, `AddTmpfsOwned()`
- `pkg/docker/ssh.go`: `AppendSSHMount()`
- `pkg/fleet/manifest.go`: `AgentSpec`, `FleetManifest`, `PipelineStage`, `PipelineManifest`, `ParseFleet()`, `ParsePipeline()`
- `pkg/inject/inject.go`: `ReadClaudeSupportFiles()`

---

## Summary

| Area | Severity | Count |
|------|----------|-------|
| CLAUDE.md stale `agent` command | Low | 1 |
| quickstart.md stale bash references | High | Entire "from source" section + all examples |
| usage.md stale `agent` commands | High | ~100 occurrences across 651 lines |
| security.md | Clean | 0 |
| Go doc comments referencing removed bash files | Medium | 7 functions in `pkg/config/` |
| Go exported symbols missing doc comments | Medium | ~70 symbols across 12 packages |
