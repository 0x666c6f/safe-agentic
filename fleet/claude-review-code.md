# Code Quality Review: Go Rewrite (Phase 1)

Reviewer: Claude Opus 4.6
Date: 2026-04-11
Scope: `cmd/safe-ag/` and `pkg/` -- all `.go` files on `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`

---

## Critical

### C1. `retry` command loses the original prompt

`reconstructSpawnOpts` reads `SAFE_AGENTIC_PROMPT_B64` from the container environment (lifecycle.go:430), but `executeSpawn` never sets that env var -- the prompt is passed as CMD args (spawn.go:348-353). Every `retry` silently drops the prompt.

**Fix:** Either store the prompt as an env var during spawn (like instructions/template), or reconstruct it from the container's CMD args.

### C2. Cron daemon has race condition on config file

`runCronDaemon` (cron.go:281-315) reads and writes the cron config file in a tight loop without file locking. If two daemon instances run simultaneously, or a user runs `cron add` while the daemon is writing, the config file can be corrupted or updates lost. Additionally, the daemon loop runs forever with `context.Background()` and no signal handler -- no graceful shutdown mechanism exists.

**Fix:** Use `flock(2)` via `syscall.Flock` or a lockfile before reading/writing the config. Add a signal handler or accept a cancellable `context.Context`.

### C3. `FakeExecutor.Run` has non-deterministic prefix matching

`FakeExecutor.Run` (orb/orb.go:71-87) iterates over both `errors` and `responses` maps with `range`. When a command matches prefixes in both maps, the result depends on Go's random map iteration order. The test at orb_test.go:185 acknowledges this with a comment.

**Fix:** Check errors first with sorted longest-prefix-first matching, or require that the same prefix not appear in both maps.

### C4. `waitForContainers` polls indefinitely with no timeout

`waitForContainers` (fleet.go:306-339) uses `context.Background()` which never cancels. If a container hangs (neither exits nor dies), this function blocks forever in a 5-second polling loop.

**Fix:** Add a configurable timeout, e.g., `context.WithTimeout(ctx, 2*time.Hour)`.

### C5. `RemoveDinDRuntime` and `CleanupAllDinD` silently swallow all errors

docker/dind.go:91-97 and 99-123 -- all `exec.Run` errors are discarded. Both functions always return `nil` despite having an `error` return type. Orphaned containers and volumes accumulate silently with no indication of cleanup failure.

**Fix:** Collect errors and return a combined error (e.g., `errors.Join`), or remove the `error` return type if cleanup is best-effort.

### C6. `AppendSSHMount` ignores errors from relay setup commands

docker/ssh.go:48-50 -- `exec.Run` for the relay setup script and `start-stop-daemon` are called without checking errors. If the setup script fails, the function silently falls through to the relay-socket wait loop, which then falls back to direct mount without any indication of _why_.

**Fix:** Check these errors and return a descriptive error or log a warning.

---

## Important

### I1. `runQuickStart` silently discards all but the last non-URL argument as the prompt

spawn.go:120-129 -- the loop overwrites `prompt` on each non-URL arg instead of joining them. Running `safe-ag run "fix the bug" repo-url "and add tests"` silently drops `"fix the bug"`.

**Fix:** Join non-URL args with spaces: `prompt = strings.Join(nonURLArgs, " ")`.

### I2. `ContainerExists` / `VolumeExists` conflate "error" with "not found"

docker/container.go:12-18, docker/volume.go:10-16 -- any error from `docker inspect` returns `(false, nil)`. A Docker daemon crash, permission error, or network failure would be silently interpreted as "does not exist", potentially leading to duplicate container creation.

**Fix:** Parse the error/exit code to distinguish "not found" from other failures. Return `(false, err)` for unexpected errors.

### I3. `parseValue` escape processing order is wrong for `\\"`

config/defaults.go:140-141 -- processes `\"` -> `"` before `\\` -> `\`. For input `\\"`, step 1 turns `\"` into `"` (consuming the second backslash), then step 2 has nothing to replace. Result: `"` instead of correct `\"`.

**Fix:** Swap the two `ReplaceAll` lines -- process `\\` -> `\` first, then `\"` -> `"`.

### I4. `mustReadFile` misnamed -- doesn't panic, returns nil on error

config_cmd.go:582-586 -- Go convention is that `must*` functions panic on error. This one silently returns nil, which causes empty credentials to be written to the container.

**Fix:** Rename to `readFileOrNil`, or make it actually panic, or return the error.

### I5. AWS credential data passed via heredoc is vulnerable to content injection

config_cmd.go:560-568 -- credential file contents are interpolated into a heredoc with delimiter `__CREDS__`. If the credentials file contains the literal string `__CREDS__` on a line by itself, the heredoc terminates early and subsequent content becomes shell commands.

**Fix:** Use a more unique delimiter, or pipe the data through stdin rather than a heredoc.

### I6. `runCleanup` silently ignores all errors

lifecycle.go:276-328 -- errors from `docker.ContainerExists`, `docker.RemoveNetwork`, etc. are all swallowed. A total Docker daemon failure is indistinguishable from a clean run.

**Fix:** Accumulate errors and report them at the end.

### I7. `audit.Logger.Log` is not safe for concurrent use

pkg/audit/audit.go:32-80 -- `Log` opens, writes, and closes the file without synchronization. Concurrent goroutines calling `Log` can interleave writes.

**Fix:** Add a `sync.Mutex` to `Logger`, or document that concurrent use is not supported.

### I8. `events.Emit` is not safe for concurrent writes

events/events.go:30-36 -- opens the file with `O_APPEND` and writes via `fmt.Fprintf`, which may issue multiple `write()` syscalls. Concurrent callers can interleave lines.

**Fix:** Write to a `bytes.Buffer` first, then do a single `f.Write(buf.Bytes())` call, or add a `sync.Mutex`.

### I9. `inject.ReadClaudeSupportFiles` doesn't check `tw.Close()` / `gw.Close()` errors

inject/inject.go:99-100 -- on the success path, the tar writer and gzip writer are closed without checking return values. A gzip finalization error would produce a silently corrupted archive.

**Fix:** Check both return values and propagate errors.

### I10. `inject.tarDir` ignores error from `filepath.Rel`

inject/inject.go:128 -- `rel, _ := filepath.Rel(baseDir, path)` discards the error. If `Rel` fails, `rel` contains the full path, producing incorrect tar entries.

**Fix:** Check and return the error.

### I11. `cost.lookupPricing` prefix matching is non-deterministic

cost/pricing.go:56-67 -- iterates over a map, so for model IDs that match multiple prefixes, the result depends on random map iteration order. Currently safe with the existing table but fragile as models are added.

**Fix:** Sort prefixes by length (longest first) and return the first match.

### I12. `fleet.mergeDefaults` cannot override boolean defaults back to `false`

fleet/manifest.go:199-240 -- uses `if !agent.SSH && defaults.SSH { agent.SSH = true }`. YAML unmarshals a missing `ssh:` field as `false`, so an agent can't explicitly set `ssh: false` to override `defaults.ssh: true`.

**Fix:** Use `*bool` for optional boolean fields where `nil` means "not specified".

### I13. Multiple `os.UserHomeDir()` errors silently ignored

spawn.go:300,305,331; cron.go:107; config_cmd.go:308; audit/audit.go:25; config/defaults.go:52; events/events.go:41 -- all use `home, _ := os.UserHomeDir()`. If `$HOME` is unset, `home` is `""`, producing paths like `"/.config/safe-agentic/..."` that would write to the filesystem root.

**Fix:** Return errors from functions that call `UserHomeDir`, or fail fast with a clear error message.

### I14. `events.Emit` returns unwrapped errors

events/events.go:17-37 -- four error returns for `os.MkdirAll`, `json.Marshal`, `os.OpenFile`, and `fmt.Fprintf` are returned without wrapping. Callers can't determine which operation failed.

**Fix:** Wrap each with `fmt.Errorf("emit event: <op>: %w", err)`.

### I15. `saveCronConfig` error ignored in daemon and run paths

cron.go:274,309 -- both `runCronRun` and `runCronDaemon` call `saveCronConfig(cfg)` without checking the returned error. If the write fails, job status (LastRun/LastErr) is lost.

**Fix:** Log or return the error from `saveCronConfig`.

### I16. `ensureRunning` swallows the `docker.IsRunning` error

observe.go:139 -- `running, _ := docker.IsRunning(ctx, exec, name)` discards the error. If the container doesn't exist, the code attempts `docker start` which also fails, producing a confusing error message.

**Fix:** Check and propagate the error from `IsRunning`.

### I17. Dead code: `createdAt` fetched but never used

observe.go:87,101 -- `createdAt, _ := inspectField(...)` fetches data via a Docker API call, then is immediately suppressed with `_ = createdAt`. This is a wasted network round-trip.

**Fix:** Remove the `createdAt` fetch or implement the container matching it was intended for.

### I18. `inject.ReadAWSCredentials` uses naive substring match for profile detection

inject/inject.go:174 -- `strings.Contains(content, "["+profile+"]")` would match partial profile names (e.g., `[my-profile-extended]` when searching for `[my-profile]`) or strings inside comments/values.

**Fix:** Use a line-by-line scan or regex `(?m)^\s*\[profile-name\]\s*$`.

### I19. `bufio.Scanner` buffer too small for JSONL session files

observe.go:603-604 -- `bufio.NewScanner` has a default 64KB line limit. Claude session JSONL files can have very long lines (large content blocks). Lines exceeding 64KB are silently truncated, causing `json.Unmarshal` to fail and token usage to be lost.

**Fix:** Use `scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)` to increase the limit.

---

## Minor

### M1. Dead code: `tool_use` case always returns empty string

observe.go:219-224 -- the `"tool_use"` subtype branch returns `""` regardless of the `hookInfos` check. The if/else paths both return `""`.

### M2. Duplicated `jsonString` / `jsonStringFromEvent` functions

observe.go:663 and observe.go:994 -- identical logic with different names. Both unmarshal a string from a `map[string]json.RawMessage` and silently ignore errors.

**Fix:** Keep one, delete the other.

### M3. `json.Unmarshal` error silently ignored in JSON string helpers

observe.go:669 and observe.go:1001 -- `json.Unmarshal(raw, &s)` discards the error. Non-string JSON values would silently return `""`.

### M4. Duplicated XDG_CONFIG_HOME resolution logic

cron.go:104 and config_cmd.go:306 duplicate the same `os.Getenv("XDG_CONFIG_HOME")` fallback logic.

**Fix:** Extract to a shared helper in `pkg/config`.

### M5. `AppendSSHMount` polling loop doesn't respect context cancellation

docker/ssh.go:55-62 -- uses `time.Sleep(200ms)` in a loop without checking `ctx.Done()`. Contrast with `waitForDinD` (dind.go:78-88) which properly uses `select` with context.

**Fix:** Add `select { case <-ctx.Done(): return ctx.Err() case <-time.After(200*ms): }`.

### M6. `ResolveTarget` partial matching is overly permissive

docker/container.go:52-55 -- `strings.Contains(n, nameOrPartial)` means searching for `"a"` matches any container name containing "a". This could return unexpected containers.

**Fix:** Require prefix match (`strings.HasPrefix`) or minimum match length.

### M7. `ensureRunning` uses hardcoded 2-second sleep with no verification

observe.go:145 -- after `docker start`, sleeps 2s and hopes the container is running. No actual health check.

**Fix:** Poll `docker inspect --format '{{.State.Running}}'` in a loop.

### M8. Duplicate regex pattern between `validate` and `repourl` packages

validate/validate.go:9 and repourl/parse.go:9 both compile `^[A-Za-z0-9][A-Za-z0-9._\-]*$`.

**Fix:** Export from `validate` and import in `repourl`.

### M9. `PipelineManifest.Stages` dependency expansion is non-deterministic

fleet/manifest.go:166-170 -- iterates `stageNames` map with `for name := range stageNames`. Map iteration order is random, so expanded dependency ordering varies between runs.

**Fix:** Sort the expanded dependencies.

### M10. No validation that fleet/pipeline agents have required fields

fleet/manifest.go:84-101 and 106-196 -- `ParseFleet` and `ParsePipeline` don't validate that agents have required fields (`name`, `type`, `repo`). Invalid manifests parse successfully but fail at spawn time with unclear errors.

**Fix:** Add post-parse validation.

### M11. `tmux.HasSession` error return value is always nil

tmux/tmux.go:20-27 -- `HasSession` returns `(bool, error)` but the error is always `nil`. Every caller ignores it. The function signature promises an error it never delivers.

**Fix:** Either simplify to return only `bool`, or have it return real errors for non-"session not found" failures.

### M12. `repourl.ClonePath` has redundant validation checks

repourl/parse.go:39-47 -- explicit checks for empty/dot-prefix/dash-prefix on owner and repo names are already covered by the `namePattern` regex `^[A-Za-z0-9][A-Za-z0-9._\-]*$` (which requires first char to be alphanumeric).

**Fix:** Remove redundant checks or keep them only for distinct error messages.

### M13. Duplicate logic between `authDestination()` and `agentConfigDir()`

spawn.go:480-489 and observe.go:697-704 -- both functions map agent type to a path (`/home/agent/.claude` vs `/home/agent/.codex`) with identical logic.

**Fix:** Consolidate into a single function.

### M14. `runQuickStart` calls `config.DetectGitIdentity()` redundantly

spawn.go:140 -- `identity := config.DetectGitIdentity()` is called even though `executeSpawn` will also call it (line 194) if `opts.Identity` is empty. Redundant subprocess call.

### M15. `extractTar` has no size limit on entries

observe.go:860-868 -- no `io.LimitReader` on tar entries being extracted. A malicious tar could write arbitrarily large files despite the path traversal check being present and correct.

**Fix:** Add `io.LimitReader(tr, maxSize)` to bound extraction.

### M16. `parseSchedule` approximates cron expressions inaccurately

cron.go:380-405 -- standard cron expressions are reduced to simple interval approximations. `"0 9 * * 1"` (Monday at 9am) would match the default hourly fallback, not a weekly schedule.

**Fix:** Document the limitation, or use a proper cron parser library.

### M17. Shared mutable package-level flag state across tests

command_test.go (multiple locations) -- tests modify package-level variables like `listJSON`, `stopAll`, `diffStat` via `defer` to restore them. Fragile and not safe for `t.Parallel()`.

### M18. `Render()` does not escape all shell-sensitive characters

docker/runtime.go:107-118 -- quotes arguments containing ` \t"'$\` but not `;`, `|`, `&`, `(`, `)`, backtick, `!`, etc. If output is ever passed to a shell, this could cause issues.

**Fix:** Document that `Render()` output is for display only, or use comprehensive quoting.

### M19. `CheckBudget` function naming is misleading

events/budget.go:3 -- `CheckBudget(cost, budget float64) bool` returns `true` when budget is _exceeded_. The name does not convey the return value meaning.

**Fix:** Rename to `BudgetExceeded` or `IsOverBudget`.

### M20. `go.mod` has all dependencies marked `// indirect`

go.mod:6-18 -- packages directly imported (e.g., `gopkg.in/yaml.v3` in `pkg/fleet/manifest.go`, `cobra` in `cmd/`) are incorrectly marked as indirect.

**Fix:** Run `go mod tidy`.

### M21. `parseValue` doesn't reject single-character quoted values

config/defaults.go:133-155 -- a value of `"` (single double-quote) satisfies both `HasPrefix` and `HasSuffix` checks since the same character is both. This produces `inner = ""` which may not be intended.

**Fix:** Require `len(raw) >= 2` for quoted values.

### M22. `DockerRunCmd.AddFlag` naming is misleading

docker/runtime.go:36 -- `AddFlag(flags ...string)` is used both as `AddFlag("--cap-drop=ALL")` (single arg) and `AddFlag("--network", opts.Network)` (two args). It's really "AddArgs."

**Fix:** Rename to `AddArgs` or split into `AddFlag`/`AddFlagValue`.

### M23. Labels `Type` and `AgentType` naming ambiguity

labels/labels.go:5,25 -- having both `AgentType` and `Type` as constants with similar names is confusing without documentation.

**Fix:** Add comments distinguishing them (e.g., `AgentType` = engine like claude/codex, `Type` = container role like agent/sidecar).

---

## Summary

| Severity | Count | Key Themes |
|----------|-------|------------|
| Critical | 6 | Lost prompt on retry, cron race condition, non-deterministic test helper, infinite polling, swallowed cleanup errors, silent SSH setup failures |
| Important | 19 | Error/not-found conflation, escape ordering bug, silent error swallowing, bool merge limitation, unchecked Close, concurrent write safety, heredoc injection |
| Minor | 23 | Dead code, duplication, permissive matching, missing context cancellation, naming inconsistencies, missing validation |

**Priority recommendations:**
1. **C1** -- `retry` losing prompts is a user-facing data loss bug
2. **C4/C5/C6** -- silent failures make debugging impossible in production
3. **I3** -- escape ordering is a correctness bug in config parsing
4. **I2/I6/I16** -- swallowed errors hide real problems
5. **I9/I10** -- unchecked Close/Rel can produce silently corrupted output
6. **I12** -- bool merge limitation will surprise fleet users
