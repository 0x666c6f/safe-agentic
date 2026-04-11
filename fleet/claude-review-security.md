# Security Review: Go CLI Rewrite — Claude Opus 4.6 (Second Pass)

**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`
**Reviewer:** Claude Opus 4.6 (automated, independent review)
**Date:** 2026-04-11
**Scope:** All Go source under `cmd/safe-ag/` and `pkg/` (67 files)
**Baseline:** Compared against `fleet/review-security.md` (prior review, 2026-04-10)

---

## Executive Summary

The Go rewrite correctly enforces the vast majority of the CLAUDE.md security model. The typed `DockerRunCmd` builder and `orb.Executor` abstraction eliminate shell-injection classes that existed in the bash version. Since the prior review, **H3/H4 (branch-name injection)** and **H5 (stash ref injection)** have been fixed via `validBranchName` and `validStashRef` regexes. However, **3 critical**, **3 high**, and **7 medium** issues remain or are newly identified.

### Delta from Prior Review (2026-04-10)

| Prior ID | Status | Notes |
|----------|--------|-------|
| C1 (callback injection) | **Still open** | Unchanged |
| C2 (DinD --privileged) | **Still open** | Inherent to DinD design |
| C3 (host Docker socket) | **Still open** | Unchanged |
| C4 (AWS heredoc injection) | **Still open** | Unchanged |
| H1 (notify command: target) | **Still open** | Unchanged |
| H2 (SSH relay shell interp) | **Still open** | Unchanged |
| H3 (prBase injection) | **Fixed** | `validBranchName` regex added at `workflow.go:445` |
| H4 (reviewBase injection) | **Fixed** | `validBranchName` check at `workflow.go:517` |
| H5 (stash label injection) | **Partially fixed** | `validStashRef` covers revert; create `label` still unvalidated |
| M1-M8 | **Still open** | Unchanged |
| **New: N1** | Template path traversal | `findTemplate` does not sanitize name |
| **New: N2** | AWS profile injection in bashrc | Profile name unescaped in shell string |
| **New: N3** | Cron command splitting | `strings.Fields` mishandles quoted args |

---

## 1. CRITICAL Findings

### C1. Shell Injection via `--on-exit` / `--on-complete` / `--on-fail` Callbacks

**Files:** `cmd/safe-ag/spawn.go:372-382`

Callback values are base64-encoded and stored as Docker labels / env vars without any content validation. The security impact depends on _where_ these callbacks execute:

- If **host-side** (Go CLI invokes them after `waitForContainers`): arbitrary host command execution.
- If **container-side only** (entrypoint runs them): blast radius is contained to the sandbox.

**Current code path:** The Go CLI does _not_ execute callbacks itself — they are passed to the container via env vars (`SAFE_AGENTIC_ON_EXIT_B64`, etc.) and consumed by `entrypoint.sh` / `agent-session.sh`. This makes the attack container-side only.

**Remaining risk:** A malicious fleet manifest author can craft callbacks that exfiltrate data from inside the container (e.g., base64 AWS creds → curl). Mitigated by network isolation if `--network none` is used.

**Recommendation:** Add a comment in `spawn.go` documenting that callbacks execute container-side. Consider logging the callback hash to the audit log for forensics.

### C2. DinD Sidecar Runs `--privileged`

**File:** `pkg/docker/dind.go:56-69`

```go
args := []string{"docker", "run", "-d",
    "--name", dindName,
    "--privileged",  // full host capabilities in VM
```

The DinD sidecar gets `--privileged` while the agent container gets `--cap-drop=ALL`. But the agent container communicates with the DinD daemon via a shared Docker socket volume. An agent can issue `docker run --privileged -v /:/host ...` to the sidecar's daemon, escaping all hardening.

**Impact:** Any agent spawned with `--docker` can achieve root-equivalent access in the VM. This is a _designed_ trade-off (the CLAUDE.md documents `--docker` as an opt-in), but the escalation path should be explicit.

**Recommendation:**
- Document in CLAUDE.md security table: `--docker` grants VM-root-equivalent
- Apply `--security-opt=seccomp=<path>` and `--memory` limits to the DinD sidecar
- Long-term: evaluate rootless Docker or Sysbox

### C3. Host Docker Socket Mount Bypasses All Hardening

**File:** `pkg/docker/dind.go:34-45`

`AppendHostDockerSocket` grants the container full Docker daemon access via `/var/run/docker.sock`. This bypasses _every_ hardening measure (read-only rootfs, cap-drop, seccomp, userns-remap).

**Impact:** VM compromise. The `--docker-socket` flag is the most dangerous option in the entire CLI.

**Recommendation:** Print a warning to stderr when `--docker-socket` is used. Consider requiring `--docker-socket --i-understand-the-risks` double-opt-in.

---

## 2. HIGH Findings

### H1. AWS Credentials Heredoc Injection (was C4)

**File:** `cmd/safe-ag/config_cmd.go:563-566`

```go
"mkdir -p ~/.aws && cat > ~/.aws/credentials <<'__CREDS__'\n"+credsContent+"\n__CREDS__"
```

If `~/.aws/credentials` contains the literal line `__CREDS__`, the heredoc terminates early and subsequent lines become shell commands. Downgraded from CRITICAL to HIGH because:
- The attacker must control the host's `~/.aws/credentials` content
- The injection runs inside the container, not on the host

**Recommendation:** Replace with base64 pipe: `echo '<b64>' | base64 -d > ~/.aws/credentials`

### H2. AWS Profile Name Shell Injection in `aws-refresh`

**File:** `cmd/safe-ag/config_cmd.go:573-574`

```go
orbRunner.Run(ctx, "docker", "exec", name,
    "bash", "-c", "echo 'export AWS_PROFILE="+p+"' >> ~/.bashrc")
```

The profile name `p` is interpolated into a single-quoted shell string. If a profile name contains `'`, the quoting breaks:
- Profile `foo'; rm -rf /; echo '` would execute arbitrary commands.

AWS profile names are _typically_ `[A-Za-z0-9_-]`, but the code does not enforce this.

**Recommendation:** Validate `p` against `^[A-Za-z0-9_.-]+$`, or use `%q` and adjust the shell quoting.

### H3. Template Name Path Traversal

**File:** `cmd/safe-ag/config_cmd.go:392-416`

```go
func findTemplate(name string) (string, error) {
    candidates = append(candidates,
        filepath.Join(userDir, name+".md"),
        filepath.Join(userDir, name),
    )
```

The `name` argument comes directly from `args[0]` (user CLI input). If `name` is `../../etc/passwd`, `filepath.Join` resolves the `..` components, and `os.Stat` + `os.ReadFile` happily reads any file on the host filesystem.

```
$ safe-ag template show ../../etc/passwd
# reads /etc/passwd
```

**Impact:** Arbitrary file read on the host (the CLI runs on macOS, not inside the container).

**Recommendation:** Validate template names against `validate.NameComponent` or reject names containing `/` or `..`.

---

## 3. MEDIUM Findings

### M1. No Validation on `--memory` / `--cpus` Values

**File:** `cmd/safe-ag/spawn.go:180-182`

Values are passed directly to Docker. Docker will reject malformed values, but malicious values like `--memory 0` (disables limit) or `--cpus 9999` could be used to abuse VM resources.

**Recommendation:** Validate `memory` matches `^\d+[gmkGMK]?$` and `cpus` matches `^\d+(\.\d+)?$`.

### M2. Prompt Leaked in Docker Label (First 100 Chars)

**File:** `cmd/safe-ag/spawn.go:354`

```go
cmd.AddLabel(labels.Prompt, truncate(opts.Prompt, 100))
```

The prompt label is visible via `docker inspect` to any VM user. Prompts may contain API keys, internal URLs, or proprietary instructions.

**Recommendation:** Base64-encode the label value (like `on-complete-b64`) or remove it.

### M3. Checkpoint Create Label Not Validated

**File:** `cmd/safe-ag/workflow.go:104-105, 116-117`

```go
label := "snapshot"
if len(args) >= 2 {
    label = args[1]  // user input, no validation
}
stashMsg := fmt.Sprintf("checkpoint: %s", label)
stashCmd := fmt.Sprintf("git stash push -m %q", stashMsg)
```

While revert validates via `validStashRef`, the create `label` is not validated. Go's `%q` provides _Go-style_ quoting (backslash escapes), which is not identical to shell quoting. For most inputs this is safe, but edge cases with backticks, `$()`, or nested quotes could behave unexpectedly inside `bash -c`.

**Recommendation:** Validate `label` against `^[A-Za-z0-9_.-]+$` (same pattern as `validate.NameComponent`).

### M4. `runQuickStart` URL Detection Too Broad

**File:** `cmd/safe-ag/spawn.go:124`

```go
if strings.HasPrefix(arg, "http") || ...
```

The prefix `"http"` matches `httpfoo`, `httpd-config`, etc. These would be incorrectly treated as repo URLs.

**Recommendation:** Use `strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "http://")`.

### M5. Audit and Events Logs World-Readable

**Files:** `pkg/audit/audit.go:46`, `pkg/events/events.go:30`

Both files are created with `0644`. On shared systems, other users can read the audit trail (spawn times, repo URLs, container names).

**Recommendation:** Use `0600`.

### M6. `containerEnvVar` Processes Full Env Dump

**File:** `cmd/safe-ag/lifecycle.go:490-501`

All container env vars (including base64-encoded AWS creds, Claude config) are loaded into memory to find a single key. If this output is ever logged (e.g., debug mode, error wrapping), secrets leak.

**Recommendation:** Use a targeted Docker inspect template: `{{range .Config.Env}}{{println .}}{{end}}` is already minimal; alternatively filter server-side with a Go template condition.

### M7. Cron Job Command Splitting Mishandles Quoted Arguments

**File:** `cmd/safe-ag/cron.go:318-319`

```go
parts := strings.Fields(job.Command)
c := exec.Command(safeAgBin, parts...)
```

`strings.Fields` splits on whitespace without respecting quotes. A cron command like:
```
spawn claude --prompt "Fix the CI tests" --repo git@...
```
becomes `["spawn", "claude", "--prompt", "\"Fix", "the", "CI", "tests\"", "--repo", ...]`.

**Impact:** Cron jobs with quoted arguments silently malfunction.

**Recommendation:** Use a proper shell-style tokenizer (e.g., `github.com/kballard/go-shellquote`) or document that cron commands must not contain spaces in argument values.

---

## 4. CLAUDE.md Security Model vs. Go Code Enforcement

| CLAUDE.md Claim | Go Enforcement | Status |
|---|---|---|
| SSH agent OFF by default | `spawnOpts.SSH` defaults `false` | **Enforced** |
| SSH requires `--ssh` flag | `EnsureSSHForRepos()` checks SSH URLs | **Enforced** |
| Per-session auth (ephemeral) | `AuthVolumeName(_, true, _)` | **Enforced** |
| `--reuse-auth` for shared auth | `AuthVolumeName(_, false, _)` | **Enforced** |
| AWS credentials OFF by default | Only when `AWSProfile != ""` | **Enforced** |
| AWS on tmpfs | `AddTmpfs("/home/agent/.aws", ...)` | **Enforced** |
| Read-only rootfs | `--read-only` in `AppendRuntimeHardening` | **Enforced** |
| cap-drop ALL | `--cap-drop=ALL` | **Enforced** |
| no-new-privileges | `--security-opt=no-new-privileges:true` | **Enforced** |
| Seccomp profile | `--security-opt=seccomp=<path>` | **Enforced** |
| Dedicated bridge network | `CreateManagedNetwork` per container | **Enforced** |
| Block host/bridge/container: networks | `validate.NetworkName` | **Enforced** |
| Memory 8g, CPU 4, PIDs 512 | `config.Defaults()` | **Enforced** |
| No sudo, no supplemental groups | Dockerfile-enforced | **Deferred** |
| Docker userns-remap | `vm/setup.sh`-enforced | **Deferred** |
| Git identity from host env | `DetectGitIdentity()` + env injection | **Enforced** |
| Block `--privileged` flag | No explicit blocklist on `DockerRunCmd` | **GAP** |
| Block `--` passthrough | Cobra arg parsing prevents | **Enforced** |
| Container name validation | `validate.NameComponent` | **Enforced** |
| Repo URL traversal rejection | `repourl.ClonePath` | **Enforced** |
| Build context from tracked files | `orb run docker build` (not Go) | **Deferred** |

### Key Gap: No `DockerRunCmd.Validate()` Blocklist

The `DockerRunCmd` builder correctly adds hardening flags by construction — no code path adds `--privileged` to agent containers. However, there is no defense-in-depth mechanism that _prevents_ a future code change from accidentally adding forbidden flags.

**Recommendation:** Add a `Validate() error` method to `DockerRunCmd` that scans `d.flags` for:
- `--privileged`
- `--cap-add` (any)
- `--security-opt=apparmor:unconfined`
- `--network=host`
- `--pid=host`
- `--userns=host`

Call `Validate()` in `Build()` before returning the args.

---

## 5. Positive Security Observations

1. **Type-safe command builder** — `DockerRunCmd` passes args as `[]string` to `exec.Command`, not shell strings. This eliminates the #1 injection class from bash.

2. **Executor abstraction** — `orb.Executor` channels all VM commands through `exec.CommandContext`. No shell concatenation for Docker commands.

3. **Comprehensive input validation** — Container names, network names, PIDs, repo URLs, stash refs, branch names, config keys all validated. The regex patterns are tight (`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`).

4. **Tar extraction zip-slip protection** — `extractTar` (`observe.go:829-856`) uses `filepath.Clean` + `strings.HasPrefix` to prevent path traversal. Correctly handles both files and directories.

5. **Config key allowlist** — `config.KeyAllowed` prevents arbitrary env var injection from the defaults file.

6. **Branch name validation** — `validBranchName` (`workflow.go:445`) now validates both `prBase` and `reviewBase` before shell interpolation. This was missing in the prior review and has been fixed.

7. **Stash ref validation** — `validStashRef` (`workflow.go:189`) prevents injection via the revert ref argument.

8. **SSH relay uses constants** — `sshRelaySocket` and `sshSocketPath` are package-level constants, not user input. The relay setup comment correctly documents this.

9. **Auth volume isolation for fleet** — Fleet agents get per-container auth volumes seeded from shared (spawn.go:266-279), preventing cross-agent credential interference.

10. **FakeExecutor enables security testing** — The `orb.FakeExecutor` captures all commands for assertion, enabling tests to verify no forbidden flags are passed.

---

## 6. Recommendations Summary (Prioritized)

| # | Severity | Finding | Recommended Fix |
|---|----------|---------|-----------------|
| 1 | **CRITICAL** | C2: DinD `--privileged` | Document as VM-root-equivalent; apply seccomp/memory to sidecar |
| 2 | **CRITICAL** | C3: Host Docker socket | Add stderr warning; consider double-opt-in |
| 3 | **CRITICAL** | C1: Callback commands | Document container-side execution; log callback hash |
| 4 | **HIGH** | H1: AWS heredoc injection | Replace with base64 pipe |
| 5 | **HIGH** | H2: AWS profile shell injection | Validate profile name against `[A-Za-z0-9_.-]+` |
| 6 | **HIGH** | H3: Template path traversal | Reject names containing `/` or `..` |
| 7 | **MEDIUM** | M3: Checkpoint label unvalidated | Apply `validate.NameComponent` |
| 8 | **MEDIUM** | M4: `http` prefix too broad | Use `https://` / `http://` |
| 9 | **MEDIUM** | M5: World-readable logs | Use `0600` permissions |
| 10 | **MEDIUM** | M7: Cron command splitting | Use shell-aware tokenizer |
| 11 | **MEDIUM** | M1: No memory/cpus validation | Add format regex |
| 12 | **MEDIUM** | M2: Prompt in label | Base64-encode or omit |
| 13 | **MEDIUM** | M6: Full env dump | Document risk; no code change needed |
| — | **GAP** | No `DockerRunCmd.Validate()` | Add flag blocklist method |

---

## 7. Methodology

- Read all 67 Go source files (cmd + pkg + tui)
- Searched for `exec.Command`, `fmt.Sprintf` with `bash -c`, `Sprintf` with user input
- Traced all user-input paths from cobra flags → validation → Docker command construction
- Compared each CLAUDE.md security claim against the corresponding Go code
- Diffed findings against prior review (`fleet/review-security.md`) to identify fixes and regressions
- Focused on: command injection, path traversal, unsafe Docker flags, secret exposure, input validation gaps
