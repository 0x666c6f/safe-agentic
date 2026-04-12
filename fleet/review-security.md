╭─ safe-agentic container
│  Agent: claude
│  Workspace: /workspace
╰─ Ready.
# Security Review: Go CLI Rewrite (Phase 1 Foundation)

**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`
**Reviewer:** Claude Code (automated)
**Date:** 2026-04-10
**Scope:** All Go code under `cmd/safe-ag/` and `pkg/`

---

## Executive Summary

The Go rewrite is structurally sound and faithfully ports the bash security model. The type-safe `DockerRunCmd` builder eliminates an entire class of command-injection bugs that existed in the bash version. However, this review identifies **4 critical**, **5 high**, and **8 medium** findings that should be addressed before merging.

---

## 1. CRITICAL Findings

### C1. Shell Injection via `--on-exit` / `--on-complete` / `--on-fail` Callbacks

**Files:** `cmd/safe-ag/spawn.go:369-378`, `pkg/labels/labels.go:16-17`

The `--on-exit`, `--on-complete`, and `--on-fail` values are stored as base64-encoded Docker labels (`safe-agentic.on-complete-b64`, `safe-agentic.on-fail-b64`) and as environment variables. These are arbitrary shell commands provided by the user. While this is by-design (the user controls what runs on exit), there is **no validation** of the command content before it is base64-encoded and injected. A malicious fleet manifest could inject callbacks that execute arbitrary commands on the **host** (not inside the container), depending on how the entrypoint/session script consumes them.

**Risk:** If the callback is executed by the Go CLI itself (e.g., after `waitForContainers` completes in pipeline mode), this becomes a host-side command injection vector. If it's only executed inside the container, the blast radius is contained.

**Recommendation:** Document clearly where callbacks execute. If host-side: validate against a restricted command set or require explicit opt-in. If container-side only: acceptable, but add a comment.

### C2. DinD Sidecar Runs `--privileged`

**File:** `pkg/docker/dind.go:56-69`

The DinD sidecar container is started with `--privileged`:
```go
args := []string{"docker", "run", "-d",
    "--name", dindName,
    "--privileged",  // <-- full host capabilities
```

This is inherent to Docker-in-Docker but it means a container with `--docker` access gains root-equivalent capabilities in the VM. Combined with the shared socket volume, the agent container can issue arbitrary Docker commands to the privileged sidecar's daemon.

**Risk:** An agent with `--docker` can escape its hardening (read-only rootfs, cap-drop ALL) by creating a new privileged container via the DinD daemon.

**Recommendation:**
- Use rootless Docker or Sysbox for DinD instead of `--privileged`
- At minimum, apply seccomp profile and limit capabilities on the DinD sidecar
- Document this as a known privilege escalation path when `--docker` is used

### C3. Host Docker Socket Mount Bypasses All Container Hardening

**File:** `pkg/docker/dind.go:34-45`

`AppendHostDockerSocket` mounts `/var/run/docker.sock` into the container and adds the docker group GID:
```go
cmd.AddFlag("--group-add", gid)
cmd.AddFlag("-v", "/var/run/docker.sock:"+hostSocketPath)
```

This grants full Docker daemon access, letting the container create new containers without any of the hardening flags (no cap-drop, no read-only rootfs, no seccomp). This is effectively root-equivalent access to the VM.

**Risk:** Complete VM compromise. Any agent with `--docker-socket` can create privileged containers, mount the host filesystem, and escape all isolation.

**Recommendation:**
- Gate `--docker-socket` behind an explicit `--i-understand-this-is-dangerous` flag
- Log a prominent warning when this flag is used
- Consider removing this option entirely in favor of DinD

### C4. AWS Credentials Written to Container via Heredoc with Unsanitized Content

**File:** `cmd/safe-ag/config_cmd.go:560-569`

```go
"docker", "exec", name,
"bash", "-c",
"mkdir -p ~/.aws && cat > ~/.aws/credentials <<'__CREDS__'\n"+credsContent+"\n__CREDS__",
```

The `credsContent` variable is the raw content of `~/.aws/credentials` read from the host. If this file contains the literal string `__CREDS__` on its own line, the heredoc terminates early, and the remaining content is interpreted as shell commands.

**Risk:** If a malicious actor can influence the AWS credentials file content (unlikely but possible in shared environments), they can achieve command execution inside the container.

**Recommendation:** Use a more robust injection method:
- Base64-encode the content and decode inside the container: `echo <b64> | base64 -d > ~/.aws/credentials`
- Or use `docker cp` instead of `docker exec bash -c`

---

## 2. HIGH Findings

### H1. `--notify` with `command:` Target Enables Arbitrary Command Execution

**File:** `pkg/events/notify.go:10-27`

The `ParseNotifyTargets` function parses `command:some-script.sh` from the `--notify` flag. There is no validation on the `Value` field. If the notification system eventually invokes this as a shell command (either host-side or container-side), it's an injection vector.

**Risk:** Depends on the notification execution path, which is not fully implemented in Go yet. The parsed target is stored in a Docker label as base64.

**Recommendation:** Validate the `command:` target against a restricted pattern (e.g., no shell metacharacters, must be an absolute path).

### H2. SSH Relay Setup Uses Shell Interpolation

**File:** `pkg/docker/ssh.go:34-47`

```go
relayScript := fmt.Sprintf(
    "#!/bin/bash\nexec socat UNIX-LISTEN:%s,fork,mode=666 UNIX-CONNECT:%s\n",
    sshRelaySocket, vmSocket)

setupCmd := fmt.Sprintf(
    "pkill -f 'socat.*safe-agentic-ssh-agent' 2>/dev/null || true; "+
        "rm -f %s; "+
        "printf '%%s' '%s' > /tmp/safe-agentic-ssh-relay.sh; "+
        ...
    sshRelaySocket, relayScript)
```

The `vmSocket` value comes from `$SSH_AUTH_SOCK` in the VM. While this is not user-controlled input, the value is interpolated into a shell command string without escaping. If `SSH_AUTH_SOCK` contains single quotes or shell metacharacters, this could break or inject commands.

**Risk:** Low probability (SSH_AUTH_SOCK is typically a clean path), but the pattern is fragile.

**Recommendation:** Use `exec.Command` with discrete arguments instead of shell string interpolation, or at minimum shell-escape the socket path.

### H3. `prTitle` and `prBase` Interpolated into Shell Command

**File:** `cmd/safe-ag/workflow.go:468-470`

```go
ghArgs = fmt.Sprintf("gh pr create --title %q --base %s --fill", prTitle, prBase)
```

While `%q` quotes the title, `prBase` is inserted unquoted. A branch name like `main; rm -rf /` would execute arbitrary commands inside the container.

**Risk:** The `prBase` flag defaults to `"main"` and users must explicitly set it, but branch names are not validated.

**Recommendation:** Validate `prBase` against a branch name pattern (e.g., `^[A-Za-z0-9/_.-]+$`) or use `%q` for it as well.

### H4. `reviewBase` Interpolated into Shell Command Without Validation

**File:** `cmd/safe-ag/workflow.go:511, 519`

```go
codexCmd := fmt.Sprintf("codex review --base %s 2>/dev/null", reviewBase)
diffCmd := fmt.Sprintf("git diff %s...HEAD", reviewBase)
```

Same issue as H3. The `--base` value is passed to `fmt.Sprintf` and embedded in a shell command string executed via `docker exec bash -c`.

**Recommendation:** Validate against `^[A-Za-z0-9/_.\-]+$` or use `%q`.

### H5. `stashMsg` and `label` Interpolated into Shell Command

**File:** `cmd/safe-ag/workflow.go:116-117`

```go
stashMsg := fmt.Sprintf("checkpoint: %s", label)
stashCmd := fmt.Sprintf("git stash push -m %q", stashMsg)
```

The `label` comes from `args[1]` which is user-provided CLI input. While `%q` provides Go-style quoting, this is passed to `bash -c` which interprets shell escapes differently. Go's `%q` uses backslash escaping which may not be shell-safe in all edge cases.

**Recommendation:** Use the validated stash ref approach (like `validStashRef` for revert) — validate `label` against a safe pattern.

---

## 3. MEDIUM Findings

### M1. No Validation on `--memory` and `--cpus` Values

**File:** `cmd/safe-ag/spawn.go:180-182`

The `memory` and `cpus` values from user flags or config are passed directly to Docker's `--memory` and `--cpus` flags without validation. While Docker itself will reject malformed values, there's no check against unreasonable values (e.g., `--memory 999999g`).

**Recommendation:** Add basic format validation (e.g., memory matches `^\d+[gmk]$`, cpus matches `^\d+(\.\d+)?$`).

### M2. Prompt Stored in Docker Label (Truncated but Unescaped)

**File:** `cmd/safe-ag/spawn.go:351`

```go
cmd.AddLabel(labels.Prompt, truncate(opts.Prompt, 100))
```

Docker labels are visible to anyone who can run `docker inspect`. The prompt may contain sensitive information (API keys, credentials, internal URLs). Labels are also preserved across `docker commit` (checkpoints).

**Recommendation:** Consider whether the prompt label should be base64-encoded (like `--on-complete`) or omitted entirely. Currently it leaks the first 100 chars of the prompt to any VM user.

### M3. Tar Extraction Path Traversal Check Has Edge Case

**File:** `cmd/safe-ag/observe.go:827-851`

The tar extraction uses `filepath.Clean` and `strings.HasPrefix` for path traversal prevention:
```go
target := filepath.Join(destDir, filepath.Clean(hdr.Name))
if !strings.HasPrefix(target, cleanDest) {
```

This is correctly implemented for regular files (line 849 uses `target` without trailing separator), but for directories (line 839) it appends `os.PathSeparator`. The logic appears correct but the asymmetry between directory and file checks is worth noting.

**Recommendation:** Unify the check — always compare `target + os.PathSeparator` prefix against `cleanDest` for both files and directories, or use `filepath.Rel` and check for `..` components.

### M4. Fleet/Pipeline Manifest Parsing Trusts YAML Without Validation

**File:** `pkg/fleet/manifest.go:70-107`

Fleet and pipeline manifests are parsed from user-provided YAML files with no schema validation. A malicious manifest could specify agent types beyond `claude/codex/shell`, set arbitrary network names, or include very long prompt strings.

**Risk:** The `executeSpawn` function does validate agent type and network name, so this is defense-in-depth rather than a direct vulnerability. But manifests from untrusted sources could cause confusing errors.

**Recommendation:** Add validation in `ParseFleet`/`ParsePipeline` (e.g., check agent types, validate repo URLs).

### M5. `runQuickStart` Treats Non-URL Arguments as Prompt Without Validation

**File:** `cmd/safe-ag/spawn.go:120-149`

```go
for _, arg := range args {
    if strings.HasPrefix(arg, "http") || strings.HasPrefix(arg, "git@") || strings.HasPrefix(arg, "ssh://") {
        repos = append(repos, arg)
    } else {
        prompt = arg
    }
}
```

The last non-URL argument becomes the prompt. If a user passes multiple non-URL arguments, only the last one is used (silently dropping earlier ones). More importantly, the `http` prefix check is too broad — `httpfoo` would be treated as a repo URL.

**Recommendation:** Use `strings.HasPrefix(arg, "https://") || strings.HasPrefix(arg, "http://")` for URL detection.

### M6. Audit Log Written with World-Readable Permissions

**File:** `pkg/audit/audit.go:46`

```go
f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
```

The audit log (which records spawn operations, container names, repo URLs) is created with 0644 permissions. On shared systems, other users can read the audit trail.

**Recommendation:** Use 0600 for the audit log file.

### M7. Events Log Written with World-Readable Permissions

**File:** `pkg/events/events.go:30`

Same as M6 — events log created with 0644.

### M8. `containerEnvVar` Exposes All Environment Variables

**File:** `cmd/safe-ag/lifecycle.go:448-461`

The `containerEnvVar` function dumps all container environment variables via `docker inspect` to search for one specific key. While this runs within the VM (not exposed externally), the full env dump includes base64-encoded secrets (AWS creds, config, support files) in the process.

**Risk:** If executor output is ever logged or leaked, all secrets are exposed.

**Recommendation:** Use Docker's Go template to extract only the needed variable: `--format '{{range .Config.Env}}{{if eq (index (split . "=") 0) "KEY"}}{{.}}{{end}}{{end}}'`

---

## 4. CLAUDE.md Security Model vs. Go Code Enforcement

| CLAUDE.md Claim | Go Code Status | Gap? |
|---|---|---|
| SSH agent OFF by default | `spawnOpts.SSH` defaults to `false` | **Enforced** |
| `--ssh` required for SSH forwarding | `EnsureSSHForRepos` checks SSH URLs vs flag | **Enforced** |
| Per-session auth (ephemeral volume) | `AuthVolumeName` with `ephemeral=true` | **Enforced** |
| `--reuse-auth` for shared auth | `AuthVolumeName` with `ephemeral=false` | **Enforced** |
| AWS credentials OFF by default | Only injected when `opts.AWSProfile != ""` | **Enforced** |
| AWS on tmpfs | `cmd.AddTmpfs("/home/agent/.aws", ...)` in spawn | **Enforced** |
| Read-only rootfs | `--read-only` in `AppendRuntimeHardening` | **Enforced** |
| cap-drop ALL | `--cap-drop=ALL` in `AppendRuntimeHardening` | **Enforced** |
| no-new-privileges | `--security-opt=no-new-privileges:true` | **Enforced** |
| Seccomp profile | `--security-opt=seccomp=<path>` | **Enforced** |
| Dedicated bridge network per container | `CreateManagedNetwork` per container | **Enforced** |
| Block `host`/`bridge`/`container:` networks | `validate.NetworkName` blocks all three | **Enforced** |
| Memory 8g, CPU 4, PIDs 512 defaults | `config.Defaults()` returns these values | **Enforced** |
| No sudo, no supplemental groups | Set in Dockerfile (not Go code) | **Deferred to Dockerfile** |
| Docker userns-remap in VM | Set in `vm/setup.sh` (not Go code) | **Deferred to VM setup** |
| Git identity from host env vars | `config.DetectGitIdentity()` + env injection | **Enforced** |
| Block `--privileged` / host network / `--` passthrough | **NOT ENFORCED** in Go code | **GAP** |
| Container names validated | `validate.NameComponent` | **Enforced** |
| Repo clone paths validated (traversal) | `repourl.ClonePath` rejects `..`, dot-prefix | **Enforced** |
| Build context uses `git ls-files -c` | Not in Go code (build is via `orb run docker build`) | **Deferred to build process** |

### Key Gap: Unsafe Docker Flag Blocking

CLAUDE.md states: *"Unsafe Docker flags (`--privileged`, `host` network, `--` passthrough) are blocked."*

In the bash version, `bin/agent-lib.sh` explicitly checked for and rejected these flags. In the Go code:

- **Network blocking:** `validate.NetworkName` correctly blocks `host`, `bridge`, and `container:*`. **Enforced.**
- **`--privileged` blocking:** The `DockerRunCmd` builder never adds `--privileged` to agent containers (only to DinD sidecars). However, there is no explicit check preventing `AddFlag("--privileged")` from being called. The builder is a whitelist-by-construction approach rather than a blocklist approach. **Partially enforced** (by omission, not by active blocking).
- **`--` passthrough:** The Go CLI uses Cobra for argument parsing, which does not pass raw `--` args to Docker. **Enforced** (by architecture).

**Recommendation:** Add a `Validate()` method to `DockerRunCmd` that scans the built args for forbidden flags (`--privileged`, `--cap-add`, `--security-opt=apparmor:unconfined`, etc.) before execution.

---

## 5. Positive Security Observations

1. **Type-safe command builder:** `DockerRunCmd` eliminates shell injection risks that existed in bash array construction. Arguments are passed as discrete `[]string` elements to `exec.Command`, never concatenated into a shell string for the Docker command itself.

2. **Executor abstraction:** All VM commands go through `orb.Executor.Run()` which uses `exec.CommandContext`, not shell interpolation. This is a significant improvement over the bash version.

3. **Input validation coverage:** Container names, network names, PIDs limits, repo URLs, and stash refs all have validation. The validation patterns (`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`) are appropriate.

4. **Tar extraction path traversal protection:** The `extractTar` function in `observe.go` correctly prevents zip-slip attacks.

5. **Config key allowlist:** The `KeyAllowed` function prevents arbitrary environment variable injection via the config file.

6. **FakeExecutor for testing:** The mock executor enables comprehensive testing without a real VM, and the test suite is extensive (714 assertions).

---

## 6. Recommendations Summary

| Priority | Finding | Fix |
|---|---|---|
| **CRITICAL** | C1: Callback commands unvalidated | Document execution context; validate if host-side |
| **CRITICAL** | C2: DinD runs `--privileged` | Use rootless Docker or document escalation path |
| **CRITICAL** | C3: Host Docker socket mount | Gate behind danger flag or remove |
| **CRITICAL** | C4: AWS creds heredoc injection | Use base64 or `docker cp` |
| **HIGH** | H1: `command:` notify target | Validate command pattern |
| **HIGH** | H2: SSH relay shell interpolation | Use discrete exec args |
| **HIGH** | H3: `prBase` shell injection | Validate branch name pattern |
| **HIGH** | H4: `reviewBase` shell injection | Validate branch name pattern |
| **HIGH** | H5: `label` in stash cmd | Validate label pattern |
| **MEDIUM** | M1: No memory/cpus validation | Add format regex |
| **MEDIUM** | M2: Prompt in Docker label | Base64-encode or omit |
| **MEDIUM** | M3: Tar extraction edge case | Unify traversal check |
| **MEDIUM** | M4: Fleet manifest no validation | Add schema validation |
| **MEDIUM** | M5: `http` prefix too broad | Use `https://` / `http://` |
| **MEDIUM** | M6-M7: World-readable logs | Use 0600 permissions |
| **MEDIUM** | M8: Full env dump | Extract only needed var |
| **GAP** | Missing `--privileged` blocklist | Add `Validate()` to `DockerRunCmd` |
