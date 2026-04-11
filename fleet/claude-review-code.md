# Code Quality Review: Go Rewrite (Phase 1)

Reviewer: Claude Opus 4.6
Date: 2026-04-11
Scope: `cmd/safe-ag/` and `pkg/` — all `.go` files on `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`

---

## Critical

### C1. `retry` command loses the original prompt

`reconstructSpawnOpts` reads `SAFE_AGENTIC_PROMPT_B64` from the container environment (lifecycle.go:430), but `executeSpawn` never sets that env var — the prompt is passed as CMD args (spawn.go:348-353). Every `retry` silently drops the prompt.

**Fix:** Either store the prompt as an env var during spawn (like instructions/template), or reconstruct it from the container's CMD args.

### C2. Cron daemon has race condition on config file

`runCronDaemon` (cron.go:281-315) reads and writes the cron config file in a tight loop without file locking. If two daemon instances run simultaneously, or a user runs `cron add` while the daemon is writing, the config file can be corrupted or updates lost.

**Fix:** Use `flock(2)` via `syscall.Flock` or a lockfile before reading/writing the config.

### C3. `FakeExecutor.Run` has non-deterministic prefix matching

`FakeExecutor.Run` (orb/orb.go:71-87) iterates over both `errors` and `responses` maps with `range`. When a command matches prefixes in both maps, the result depends on Go's random map iteration order. The test at orb_test.go:185 acknowledges this with a comment.

**Fix:** Check errors first with sorted longest-prefix-first matching, or require that the same prefix not appear in both maps.

---

## Important

### I1. `runQuickStart` silently discards all but the last non-URL argument as the prompt

spawn.go:120-129 — the loop overwrites `prompt` on each non-URL arg instead of joining them. Running `safe-ag run "fix the bug" repo-url "and add tests"` silently drops `"fix the bug"`.

**Fix:** Join non-URL args with spaces: `prompt = strings.Join(nonURLArgs, " ")`.

### I2. `ContainerExists` / `VolumeExists` conflate "error" with "not found"

docker/container.go:12-18, docker/volume.go:10-16 — any error from `docker inspect` returns `(false, nil)`. A Docker daemon crash, permission error, or network failure would be silently interpreted as "does not exist", potentially leading to duplicate container creation.

**Fix:** Parse the error/exit code to distinguish "not found" from other failures. Return `(false, err)` for unexpected errors.

### I3. `RemoveDinDRuntime` and `CleanupAllDinD` silently swallow all errors

docker/dind.go:91-97 and 99-123 — all `exec.Run` errors are discarded. Both functions always return `nil`. A failing cleanup leaves orphaned containers/volumes with no indication.

**Fix:** Collect errors and return a combined error (e.g., `errors.Join`), or at minimum log them.

### I4. `parseValue` escape processing order is wrong for `\\"`

config/defaults.go:140-141 — processes `\"` → `"` before `\\` → `\`. For input `\\"`, step 1 turns `\"` into `"` (consuming the second backslash), then step 2 has nothing to replace. Result: `"` instead of correct `\"`.

**Fix:** Swap the two `ReplaceAll` lines — process `\\` → `\` first, then `\"` → `"`.

### I5. `mustReadFile` misnamed — doesn't panic, returns nil on error

config_cmd.go:582-586 — Go convention is that `must*` functions panic on error. This one silently returns nil, which causes empty credentials to be written to the container.

**Fix:** Rename to `readFileOrNil`, or make it actually panic, or return the error.

### I6. `runCleanup` silently ignores all errors

lifecycle.go:276-328 — errors from `docker.ContainerExists`, `docker.RemoveNetwork`, etc. are all swallowed. A total Docker daemon failure is indistinguishable from a clean run.

**Fix:** Accumulate errors and report them at the end.

### I7. `audit.Logger.Log` is not safe for concurrent use

pkg/audit/audit.go:32-80 — `Log` opens, writes, and closes the file without synchronization. Concurrent goroutines calling `Log` can interleave writes or corrupt data on non-Linux platforms where `O_APPEND` atomicity isn't guaranteed.

**Fix:** Add a `sync.Mutex` to `Logger`, or document that concurrent use is not supported.

### I8. `inject.ReadClaudeSupportFiles` doesn't check `tw.Close()` / `gw.Close()` errors

inject/inject.go:99-100 — on the success path, the tar writer and gzip writer are closed without checking return values. A gzip finalization error would produce a silently corrupted archive.

**Fix:** Check both return values and propagate errors.

### I9. `inject.tarDir` ignores error from `filepath.Rel`

inject/inject.go:128 — `rel, _ := filepath.Rel(baseDir, path)` discards the error. If `Rel` fails, `rel` contains the full path, producing incorrect tar entries.

**Fix:** Check and return the error.

### I10. `cost.lookupPricing` prefix matching is non-deterministic

cost/pricing.go:56-67 — iterates over a map, so for model IDs that match multiple prefixes (e.g., `claude-3-sonnet-20241022` could match both `claude-3-sonnet` and a hypothetical `claude-3`), the result depends on random map iteration order. Currently safe with the existing table but fragile as models are added.

**Fix:** Sort prefixes by length (longest first) and return the first match.

### I11. `fleet.mergeDefaults` cannot override boolean defaults back to `false`

fleet/manifest.go:199-240 — uses `if !agent.SSH && defaults.SSH { agent.SSH = true }`. YAML unmarshals a missing `ssh:` field as `false`, so an agent can't explicitly set `ssh: false` to override `defaults.ssh: true`.

**Fix:** Use `*bool` for optional boolean fields where `nil` means "not specified".

### I12. Multiple `os.UserHomeDir()` errors silently ignored

spawn.go:300,305,331; cron.go:107; config_cmd.go:308; audit/audit.go:25; config/defaults.go:52; events/events.go:41 — all use `home, _ := os.UserHomeDir()`. If `$HOME` is unset, `home` is `""`, producing paths like `"/.config/safe-agentic/..."` that would write to the filesystem root.

**Fix:** Return errors from functions that call `UserHomeDir`, or fail fast with a clear error message.

### I13. `events.Emit` returns unwrapped errors

events/events.go:17-37 — four error returns (lines 19, 28, 31, 36) for `os.MkdirAll`, `json.Marshal`, `os.OpenFile`, and `fmt.Fprintf` are returned without wrapping. Callers can't determine which operation failed.

**Fix:** Wrap each with `fmt.Errorf("emit event: <op>: %w", err)`.

---

## Minor

### M1. Dead code: `tool_use` case always returns empty string

observe.go:219-224 — the `"tool_use"` subtype branch returns `""` regardless of the `hookInfos` check. The if/else paths both return `""`.

### M2. Duplicated `jsonString` / `jsonStringFromEvent` functions

observe.go:663 and observe.go:994 — identical logic with different names. Both unmarshal a string from a `map[string]json.RawMessage` and silently ignore errors.

**Fix:** Keep one, delete the other.

### M3. `json.Unmarshal` error silently ignored in JSON string helpers

observe.go:669 and observe.go:1001 — `json.Unmarshal(raw, &s)` discards the error. Non-string JSON values would silently return `""`.

### M4. Duplicated XDG_CONFIG_HOME resolution logic

cron.go:104 and config_cmd.go:306 duplicate the same `os.Getenv("XDG_CONFIG_HOME")` fallback logic.

**Fix:** Extract to a shared helper in `pkg/config`.

### M5. `AppendSSHMount` polling loop doesn't respect context cancellation

docker/ssh.go:55-62 — uses `time.Sleep(200ms)` in a loop without checking `ctx.Done()`. Contrast with `waitForDinD` (dind.go:78-88) which properly uses `select` with context.

**Fix:** Add `select { case <-ctx.Done(): return ctx.Err() case <-time.After(200*ms): }`.

### M6. `ResolveTarget` partial matching is overly permissive

docker/container.go:52-55 — `strings.Contains(n, nameOrPartial)` means searching for `"a"` matches any container name containing "a". This could return unexpected containers.

**Fix:** Require prefix match (`strings.HasPrefix`) or minimum match length.

### M7. `ensureRunning` uses hardcoded 2-second sleep with no verification

observe.go:145 — after `docker start`, sleeps 2s and hopes the container is running. No actual health check.

**Fix:** Poll `docker inspect --format '{{.State.Running}}'` in a loop.

### M8. `parsePeriod` silently ignores trailing characters

observe.go:681 — `fmt.Sscanf(period, "%d%s", &n, &unit)` would accept `"7days"` and parse `unit` as `"days"`, falling through to the default error case. Not a bug per se, but `"7da"` would also be rejected rather than partially matched.

### M9. Duplicate regex pattern between `validate` and `repourl` packages

validate/validate.go:9 and repourl/parse.go:9 both compile `^[A-Za-z0-9][A-Za-z0-9._\-]*$`.

**Fix:** Export from `validate` and import in `repourl`.

### M10. `PipelineManifest.Stages` dependency expansion is non-deterministic

fleet/manifest.go:166-170 — iterates `stageNames` map with `for name := range stageNames`. Map iteration order is random, so `newDeps` ordering varies between runs.

**Fix:** Sort the expanded dependencies.

### M11. No validation that fleet/pipeline agents have required fields

fleet/manifest.go:84-101 and 106-196 — `ParseFleet` and `ParsePipeline` don't validate that agents have required fields (`name`, `type`, `repo`). Invalid manifests parse successfully but fail at spawn time with unclear errors.

**Fix:** Add post-parse validation.

### M12. `tmux.HasSession` and `WaitForSession` conflate errors with "not found"

tmux/tmux.go:20-27 and 31 — any error from the tmux check (including Docker failures) is treated as "session not found". The error from `HasSession` is discarded in `WaitForSession`.

---

## Summary

| Severity | Count | Key Themes |
|----------|-------|------------|
| Critical | 3 | Lost prompt on retry, cron race condition, non-deterministic test helper |
| Important | 13 | Swallowed errors, error/not-found conflation, escape ordering bug, bool merge limitation, unchecked Close |
| Minor | 12 | Dead code, duplication, permissive matching, missing context cancellation |

**Priority recommendations:**
1. **C1** — `retry` losing prompts is a user-facing data loss bug
2. **I4** — escape ordering is a correctness bug in config parsing
3. **I2/I3/I6** — swallowed errors make debugging impossible
4. **I8/I9** — unchecked Close/Rel can produce silently corrupted output
5. **I11** — bool merge limitation will surprise fleet users
