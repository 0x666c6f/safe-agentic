# Security Review: Go CLI Rewrite — Claude Opus 4.6 (Third Pass)

**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`
**Reviewer:** Claude Opus 4.6 (automated, independent review)
**Date:** 2026-04-11
**Scope:** All Go source under `cmd/safe-ag/` and `pkg/` (67 files), plus `entrypoint.sh`
**Baseline:** Compared against `fleet/claude-review-security.md` (second pass, same date)

---

## Executive Summary

The Go rewrite enforces the vast majority of the CLAUDE.md security model. The typed `DockerRunCmd` builder and `orb.Executor` abstraction eliminate shell-injection classes that existed in the bash version. Since the second pass, no new fixes have landed. This review provides deeper analysis of existing findings, re-validates severity assignments, and identifies **1 additional medium finding** (dry-run credential leakage). Totals: **3 critical**, **3 high**, **8 medium** issues.

### Delta from Second Pass

| Prior ID | Status | Notes |
|----------|--------|-------|
| C1 (callback injection) | **Still open** | Container-side only; severity confirmed |
| C2 (DinD --privileged) | **Still open** | Inherent to DinD design |
| C3 (host Docker socket) | **Still open** | Most dangerous flag |
| H1 (AWS heredoc injection) | **Still open** | `config_cmd.go:563-566` |
| H2 (AWS profile shell injection) | **Still open** | `config_cmd.go:573-575` |
| H3 (template path traversal) | **Still open** | `config_cmd.go:392-416` — host-side file read |
| M1-M7 | **Still open** | Unchanged |
| **New: M8** | Dry-run leaks credentials | `spawn.go:414` prints full `cmd.Render()` including env vars |

---

## 1. CRITICAL Findings

### C1. Shell Injection via `--on-exit` / `--on-complete` / `--on-fail` Callbacks

**File:** `cmd/safe-ag/spawn.go:372-382`

Callback values are base64-encoded and stored as Docker labels/env vars without content validation. Callbacks execute **container-side** (via `entrypoint.sh`/`agent-session.sh`), not on the host. A malicious fleet manifest author could craft callbacks that exfiltrate container data.

**Mitigating factors:** Container network isolation (especially `--network none`), read-only rootfs, cap-drop ALL. The attack requires authoring a fleet manifest.

**Recommendation:** Document container-side execution in code comments. Log callback hash to audit log. Consider a callback allowlist for fleet manifests.

### C2. DinD Sidecar Runs `--privileged`

**File:** `pkg/docker/dind.go:56-69`

```go
args := []string{"docker", "run", "-d",
    "--name", dindName,
    "--privileged",  // full host capabilities in VM
```

The agent container communicates with the DinD daemon via a shared Docker socket volume. An agent can issue `docker run --privileged -v /:/host ...` to the sidecar's daemon, escaping all hardening.

**Impact:** Any agent spawned with `--docker` can achieve root-equivalent access in the VM.

**Recommendation:**
- Document in CLAUDE.md security table: `--docker` grants VM-root-equivalent
- Apply `--security-opt=seccomp=<path>` and `--memory` limits to the DinD sidecar
- Long-term: evaluate rootless Docker or Sysbox

### C3. Host Docker Socket Mount Bypasses All Hardening

**File:** `pkg/docker/dind.go:34-45`

`AppendHostDockerSocket` grants full Docker daemon access via `/var/run/docker.sock`. This bypasses every hardening measure (read-only rootfs, cap-drop, seccomp, userns-remap).

**Impact:** VM compromise. `--docker-socket` is the most dangerous option in the CLI.

**Recommendation:** Print a warning to stderr. Consider requiring `--docker-socket --i-understand-the-risks` double-opt-in.

---

## 2. HIGH Findings

### H1. AWS Credentials Heredoc Injection

**File:** `cmd/safe-ag/config_cmd.go:563-566`

```go
"mkdir -p ~/.aws && cat > ~/.aws/credentials <<'__CREDS__'\n"+credsContent+"\n__CREDS__"
```

If `~/.aws/credentials` contains the literal line `__CREDS__`, the heredoc terminates early and subsequent lines become shell commands inside the container. Downgraded from CRITICAL because:
- Attacker must control host's `~/.aws/credentials` content
- Injection runs inside the container, not on the host

**Recommendation:** Replace with base64 pipe: `echo '<b64>' | base64 -d > ~/.aws/credentials`

### H2. AWS Profile Name Shell Injection in `aws-refresh`

**File:** `cmd/safe-ag/config_cmd.go:573-575`

```go
orbRunner.Run(ctx, "docker", "exec", name,
    "bash", "-c", "echo 'export AWS_PROFILE="+p+"' >> ~/.bashrc")
```

**Verified:** The profile name `p` comes from `envs["AWS_PROFILE"]` which originates from either CLI argument (`args[1]`) or container label (`docker.InspectLabel`). Neither path validates the profile name against shell metacharacters.

A profile name like `foo'; rm -rf /; echo '` breaks the single quoting and executes arbitrary commands.

**Recommendation:** Validate `p` against `^[A-Za-z0-9_.-]+$` before interpolation, or use env var passing instead of shell string construction.

### H3. Template Name Path Traversal (Host-Side)

**File:** `cmd/safe-ag/config_cmd.go:392-416`

```go
func findTemplate(name string) (string, error) {
    candidates = append(candidates,
        filepath.Join(userDir, name+".md"),
        filepath.Join(userDir, name),
    )
```

**Verified:** `name` comes directly from `args[0]` with zero validation. `filepath.Join` resolves `..` components, so `../../etc/passwd` reads `/etc/passwd` on the **host macOS machine** (not inside a container).

```
$ safe-ag template show ../../etc/passwd  → reads /etc/passwd
$ safe-ag template create ../../tmp/pwned → writes to /tmp/pwned.md
```

**Impact:** Arbitrary file read on host via `template show`; arbitrary file write on host via `template create`.

**Recommendation:** Validate template names with `validate.NameComponent` (rejects `/`, `..`, special chars). Additionally, after `filepath.Join`, verify the result is still under the expected template directory with `strings.HasPrefix(resolved, expectedDir)`.

---

## 3. MEDIUM Findings

### M1. No Validation on `--memory` / `--cpus` Values

**File:** `cmd/safe-ag/spawn.go:180-182`

Values are passed directly to Docker's `--memory` and `--cpus` flags. Docker rejects malformed values, but:
- `--memory 0` disables the memory limit entirely
- `--cpus 9999` could starve the VM
- No lower bounds enforced

**Recommendation:** Validate `memory` matches `^\d+[gmkGMK]?$` with minimum 512m; validate `cpus` matches `^\d+(\.\d+)?$` with maximum equal to host CPUs.

### M2. Prompt Leaked in Docker Label (First 100 Chars)

**File:** `cmd/safe-ag/spawn.go:354`

```go
cmd.AddLabel(labels.Prompt, truncate(opts.Prompt, 100))
```

Visible via `docker inspect` to any VM user. Prompts may contain API keys, internal URLs, or proprietary instructions.

**Recommendation:** Base64-encode the label value or remove it.

### M3. Checkpoint Create Label Not Validated

**File:** `cmd/safe-ag/workflow.go:104-105, 116-117`

The `label` argument from `args[1]` is passed to `fmt.Sprintf("git stash push -m %q", stashMsg)` without validation. Go's `%q` provides Go-style quoting (backslash escapes), not shell quoting. Edge cases with backticks or `$()` could behave unexpectedly inside `bash -c`.

**Recommendation:** Validate `label` against `^[A-Za-z0-9_. -]+$`.

### M4. `runQuickStart` URL Detection Too Broad

**File:** `cmd/safe-ag/spawn.go:124`

```go
if strings.HasPrefix(arg, "http") || ...
```

The prefix `"http"` matches `httpfoo`, `httpd-config`, etc.

**Recommendation:** Use `strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "http://")`.

### M5. Audit and Events Logs World-Readable

**Files:** `pkg/audit/audit.go:46`, `pkg/events/events.go:30`

Both files created with `0644`. On shared systems, other users can read the audit trail.

**Recommendation:** Use `0600`.

### M6. `containerEnvVar` Processes Full Env Dump

**File:** `cmd/safe-ag/lifecycle.go:490-501`

All container env vars (including base64-encoded AWS creds) are loaded into memory. If this output is ever logged, secrets leak.

**Recommendation:** Document risk. Consider filtering to requested key server-side.

### M7. Cron Job Command Splitting Mishandles Quoted Arguments

**File:** `cmd/safe-ag/cron.go:318-319`

```go
parts := strings.Fields(job.Command)
c := exec.Command(safeAgBin, parts...)
```

`strings.Fields` splits on whitespace without respecting quotes. A command like `spawn claude --prompt "Fix the CI tests"` becomes `["spawn", "claude", "--prompt", "\"Fix", "the", "CI", "tests\"", ...]`.

**Recommendation:** Use `github.com/kballard/go-shellquote` or `github.com/google/shlex`.

### M8. Dry-Run Leaks Credentials to Stdout (NEW)

**File:** `cmd/safe-ag/spawn.go:412-414`

```go
if opts.DryRun {
    fmt.Println("Would execute:")
    fmt.Printf("  orb run -m safe-agentic %s\n", cmd.Render())
```

`cmd.Render()` includes all `--env` flags, which contain `SAFE_AGENTIC_AWS_CREDS_B64` (base64-encoded AWS credentials) and other secrets. These are printed to stdout, visible in terminal history, CI logs, and screen recordings.

**Recommendation:** Redact env var values in dry-run output. Show `--env SAFE_AGENTIC_AWS_CREDS_B64=<redacted>` instead of the actual value.

---

## 4. CLAUDE.md Security Model vs. Go Code Enforcement

| CLAUDE.md Claim | Go Code | Status |
|---|---|---|
| SSH agent OFF by default | `spawnOpts.SSH` defaults `false` (spawn.go:79) | **Enforced** |
| SSH requires `--ssh` flag | `EnsureSSHForRepos()` validates (ssh.go:80-90) | **Enforced** |
| Per-session auth (ephemeral volume) | Default uses shared volume (`ReuseAuth: "true"` in defaults.go:41) | **Mismatch** (see note 1) |
| `--reuse-auth` for shared auth | `AuthVolumeName(_, false, _)` | **Enforced** |
| AWS credentials OFF by default | Only injected when `AWSProfile != ""` (spawn.go:330) | **Enforced** |
| AWS credentials on tmpfs | `AddTmpfs("/home/agent/.aws", "1m", true, false)` (spawn.go:340) | **Enforced** |
| Read-only rootfs | `--read-only` in `AppendRuntimeHardening` (runtime.go:136) | **Enforced** |
| cap-drop ALL | `--cap-drop=ALL` (runtime.go:133) | **Enforced** |
| no-new-privileges | `--security-opt=no-new-privileges:true` (runtime.go:134) | **Enforced** |
| Seccomp profile | `--security-opt=seccomp=<path>` (runtime.go:135) | **Enforced** |
| Dedicated bridge network per container | `CreateManagedNetwork` in network.go:35-41 | **Enforced** |
| Block host/bridge/container: networks | `validate.NetworkName` (validate.go:26-29) | **Enforced** |
| Memory 8g default | `config.Defaults().Memory = "8g"` (defaults.go:39) | **Enforced** |
| CPU 4 default | `config.Defaults().CPUs = "4"` (defaults.go:38) | **Enforced** |
| PIDs 512 default | `config.Defaults().PIDsLimit = "512"` (defaults.go:40) | **Enforced** |
| No sudo, no supplemental groups | Dockerfile-enforced (not in Go code) | **Deferred** |
| Docker userns-remap | `vm/setup.sh`-enforced (not in Go code) | **Deferred** |
| Git identity from host env | `DetectGitIdentity()` + env injection (identity.go) | **Enforced** |
| Unsafe Docker flags blocked | No runtime blocklist on `DockerRunCmd` | **GAP** (see note 2) |
| Container name validation | `validate.NameComponent` (validate.go:9-19) | **Enforced** |
| Repo URL traversal rejection | `repourl.ClonePath` (parse.go:11-52) | **Enforced** |

### Note 1: Auth Persistence Default Mismatch

CLAUDE.md states: "Per-session OAuth + ephemeral cache volumes" as the default. However, `pkg/config/defaults.go:41-42` sets:

```go
ReuseAuth:   "true",
ReuseGHAuth: "true",
```

This means auth volumes **persist across sessions by default**. The CLAUDE.md security table shows "Per-session auth (ephemeral volume)" as the default and `--reuse-auth` as the override — but the Go code inverts this.

**Recommendation:** Either change defaults to `"false"` (matching docs) or update CLAUDE.md to reflect the actual default.

### Note 2: No `DockerRunCmd.Validate()` Blocklist

The `DockerRunCmd` builder adds hardening flags by construction — no code path adds `--privileged` to agent containers. However, there is no defense-in-depth mechanism preventing a future code change from accidentally adding forbidden flags.

**Recommendation:** Add a `Validate() error` method to `DockerRunCmd` that scans `d.flags` for: `--privileged`, `--cap-add`, `--security-opt=apparmor:unconfined`, `--network=host`, `--pid=host`, `--userns=host`. Call it in `Build()`.

---

## 5. Additional Observations

### 5a. Todo JSON Shell Escaping (Low Risk)

**File:** `cmd/safe-ag/workflow.go:255-260`

```go
jsonStr := strings.ReplaceAll(string(data), "'", "'\\''")
writeCmd := fmt.Sprintf(
    "mkdir -p /workspace/.safe-agentic && printf '%%s' '%s' > /workspace/.safe-agentic/todos.json",
    jsonStr,
)
```

The single-quote escaping (`'` → `'\''`) is the standard POSIX technique and is correct for single-quoted shell contexts. The `printf '%s'` prevents interpretation of escape sequences. **This is safe** for well-formed JSON, but if an attacker can control todo text to inject `'\''`, the quoting remains intact because Go's `strings.ReplaceAll` handles all occurrences.

**Risk:** Low. The attacker would need to control todo text AND the JSON marshaling would need to produce shell-dangerous output, which `json.MarshalIndent` does not (it escapes special characters).

### 5b. `--instructions-file` Reads Arbitrary Host Files (Low Risk)

**File:** `cmd/safe-ag/spawn.go:360-367`

```go
if opts.InstructionsFile != "" {
    data, err := os.ReadFile(opts.InstructionsFile)
```

The file path is not validated. However, this is a CLI flag provided by the user themselves, and the content is base64-encoded and injected into the container. The user already has full host access. **This is expected behavior** for a CLI tool — users can read their own files.

### 5c. Fleet Manifest Field Validation Deferred

**File:** `pkg/fleet/manifest.go`

Fleet manifests are parsed via YAML unmarshaling without field-level validation. Invalid values (bad names, negative memory, etc.) are caught later during `executeSpawn`. This is acceptable but means error messages point to spawn failures rather than manifest parse errors.

**Recommendation:** Add a `Validate()` method to `FleetManifest` for early feedback.

---

## 6. Positive Security Observations

1. **Type-safe command builder** — `DockerRunCmd` passes args as `[]string` to `exec.Command`, not shell strings. This eliminates the primary injection class from bash.

2. **Executor abstraction** — `orb.Executor` channels all VM commands through `exec.CommandContext`. No shell concatenation for Docker commands.

3. **Comprehensive input validation** — Container names, network names, PIDs, repo URLs, stash refs, branch names, config keys all validated with tight regexes (`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`).

4. **Tar extraction zip-slip protection** — `extractTar` (`observe.go:829-856`) uses `filepath.Clean` + `strings.HasPrefix` to prevent path traversal.

5. **Config key allowlist** — `config.KeyAllowed` prevents arbitrary env var injection.

6. **Branch name + stash ref validation** — `validBranchName` and `validStashRef` added since the first review, closing prior H3/H4/H5.

7. **SSH relay uses constants** — Socket paths are package-level constants, not user input.

8. **Auth volume isolation for fleet** — Fleet agents get per-container auth volumes seeded from shared, preventing cross-agent credential interference.

9. **FakeExecutor enables security testing** — Tests verify no forbidden flags are present in Docker commands.

10. **AWS credentials on tmpfs** — `/home/agent/.aws` is a 1MB tmpfs mount with `noexec`, preventing credential persistence on disk.

---

## 7. Recommendations Summary (Prioritized)

| # | Severity | ID | Finding | Fix |
|---|----------|----|---------|-----|
| 1 | **CRITICAL** | C2 | DinD `--privileged` | Document as VM-root-equivalent; seccomp/memory on sidecar |
| 2 | **CRITICAL** | C3 | Host Docker socket | Stderr warning; double-opt-in |
| 3 | **CRITICAL** | C1 | Callback commands | Document container-side; log callback hash |
| 4 | **HIGH** | H1 | AWS heredoc injection | Replace with `base64 -d` pipe |
| 5 | **HIGH** | H2 | AWS profile shell injection | Validate against `[A-Za-z0-9_.-]+` |
| 6 | **HIGH** | H3 | Template path traversal | `validate.NameComponent` + `HasPrefix` check |
| 7 | **MEDIUM** | M8 | Dry-run leaks credentials | Redact env values in `cmd.Render()` |
| 8 | **MEDIUM** | M1 | No memory/cpus validation | Add format regex with bounds |
| 9 | **MEDIUM** | M2 | Prompt in Docker label | Base64-encode or remove |
| 10 | **MEDIUM** | M3 | Checkpoint label unvalidated | Apply `validate.NameComponent` |
| 11 | **MEDIUM** | M4 | `http` prefix too broad | Use `https://` / `http://` |
| 12 | **MEDIUM** | M5 | World-readable logs | Use `0600` permissions |
| 13 | **MEDIUM** | M7 | Cron command splitting | Use shell-aware tokenizer |
| 14 | **MEDIUM** | M6 | Full env dump in memory | Document risk |
| — | **GAP** | — | Auth default mismatch | Align defaults.go with CLAUDE.md |
| — | **GAP** | — | No `DockerRunCmd.Validate()` | Add flag blocklist method |

---

## 8. Methodology

- Read all 67 Go source files (cmd + pkg + tui)
- Searched for `exec.Command`, `bash -c`, `fmt.Sprintf` with user input in all call sites
- Traced all user-input paths from Cobra flags → validation → Docker command construction
- Compared each CLAUDE.md security claim against corresponding Go code with line references
- Verified each finding by reading actual source (not just grep output)
- Diffed findings against second-pass review to track fixes and identify new issues
- Reviewed test coverage for security-critical validation functions
- Focused on: command injection, path traversal, unsafe Docker flags, secret exposure, input validation gaps
