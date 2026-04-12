╭─ safe-agentic container
│  Agent: claude
│  Workspace: /workspace
╰─ Ready.
# Go Test Suite Review

**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`
**Date:** 2026-04-10
**Total test files:** 25
**Packages:** 13 (including `cmd/safe-ag`, `tui`, and 11 under `pkg/`)

---

## 1. Coverage Summary

```
Package                     Coverage   Status
────────────────────────────────────────────────
pkg/validate                100.0%     OK
pkg/cost                    100.0%     OK
pkg/orb                      98.1%     OK
pkg/audit                    97.1%     OK
pkg/docker                   97.4%     OK
pkg/repourl                  97.5%     OK
pkg/events                   96.3%     OK
pkg/config                   95.8%     OK
pkg/tmux                     94.1%     OK
pkg/fleet                    90.0%     OK
pkg/inject                   41.5%     FAILING
tui                           6.3%     FAILING
cmd/safe-ag                  BUILD ERR  FAILING
```

**Packages below 90%:**

| Package | Coverage | Root cause |
|---------|----------|------------|
| `pkg/inject` | 41.5% | `ReadClaudeSupportFiles()` (tar/gzip creation) is entirely untested. `ReadAWSCredentials` missing profile edge cases. Config permission-denied paths only partially covered. |
| `tui` | 6.3% | Only `actions_test.go` exists with 7 small tests. The TUI rendering, key handling, and state management are untested. |
| `cmd/safe-ag` | N/A | Build error: `observe.go` references undefined `addLatestFlag` and `targetFromArgs` (likely in a file not yet on this branch). Cannot measure coverage. |

---

## 2. Critical Findings

### P0 -- Tests That Cannot Fail / Don't Verify Behavior

**2.1 Integration tests silently skip on failure (cmd/safe-ag/integration_test.go)**

| Test | Pattern |
|------|---------|
| `TestE2E_DiffCommand` (line 687) | Uses `time.Sleep(5s)`, then logs error instead of failing. Comment: "Not fatal." This test **can never detect a regression**. |
| `TestE2E_PeekShowsOutput` (line 663) | `time.Sleep(5s)` then `t.Skipf` on failure. Output assertion: "even empty is fine." |
| `TestE2E_TodoWorkflow` (line 737) | `time.Sleep(3s)` then `t.Skipf` on failure. |

**2.2 Retry tests verify string concatenation, not retry logic (cmd/safe-ag/lifecycle_test.go)**

`TestRetryFeedbackAppended` (line 291) and `TestRetryFeedbackNoOriginalPrompt` (line 310) manually concatenate strings and assert the result. They do **not** call any function under test. These provide zero regression protection.

**2.3 Misleading test name (pkg/tmux/tmux_test.go)**

`TestAttach_ReturnsErrorFromExecutor` (line 164): The test comment admits "FakeExecutor doesn't simulate errors for RunInteractive, so just confirm the call is made." The test name claims error propagation; the test body doesn't test it.

**2.4 Nineteen discarded outputs (cmd/safe-ag/command_test.go)**

Tests capture `output` from command execution but assign to `_ = output`. They verify the right Docker commands were *sent* but never check what the *user sees*. Affected tests:

`TestListCommand`, `TestPeekCommand`, `TestDiffCommand`, `TestOutputCommand_Files`, `TestOutputCommand_Commits`, `TestReviewCommand`, `TestPRCommand`, `TestCheckpointRevert`, `TestDiagnoseCommand_AllOK`, `TestSetupCommand_DockerAvailable`, `TestSetupCommand_DockerNotAvailable`, `TestRunAuditCommand_WithEntries`, `TestConfigGet_NotSet`, and 6+ DryRun spawn tests.

**2.5 DryRun spawn tests are too weak (cmd/safe-ag/command_test.go)**

10+ tests (`TestSpawnWithEphemeralAuth`, `TestSpawnWithDockerAccess`, `TestSpawnWithDockerSocket`, `TestSpawnWithCallbacks`, `TestSpawnWithTemplate`, `TestSpawnWithInstructions`, `TestSpawnWithFleetVolume`, `TestSpawnWithGHAuth`, `TestSpawnWithPrompt`, `TestSpawnWithAWS`, etc.) all only check `strings.Contains(output, "Would execute")`. None verify the feature-specific flags/volumes/env-vars actually appear in the generated Docker command.

---

### P1 -- Missing Security-Relevant Tests

**2.6 No tar path traversal test (cmd/safe-ag/command_test.go)**

`extractTar` has no test for path traversal attacks (e.g., `../../etc/passwd` in tar headers). This is a security-critical gap for a security-focused project.

**2.7 AWS credential profile substring matching bug (pkg/inject/inject_test.go)**

`ReadAWSCredentials` uses `strings.Contains(content, "["+profile+"]")` which matches `[dev]` inside `[dev-staging]`. No test catches this potential credential leakage.

**2.8 No test for `--network host` / `--privileged` rejection (cmd/safe-ag/integration_test.go)**

No integration test spawns with `--network host` or `--privileged` to verify rejection. The validation logic exists in `pkg/validate` but the end-to-end path through `spawn` is untested.

**2.9 No `http://` URL scheme test (pkg/repourl/parse_test.go)**

`http://github.com/org/repo` would fall through to the scp-style parser branch (it contains `:` and `/`), extracting `"//github.com/org/repo"` as the clone path. This is likely a bug, and no test exposes it.

---

### P2 -- Flaky / Timing-Dependent Patterns

**2.10 `time.Sleep` in integration tests (cmd/safe-ag/integration_test.go)**

| Location | Sleep | Risk |
|----------|-------|------|
| `ensureSharedContainer` (line 93) | 5s | Waits for repo clone; fails on slow networks |
| `TestDet_MultipleRepos` (line 542) | 8s | Same |
| `TestE2E_StopRemovesContainer` (line 574) | 2s | Race if `docker rm` is slow |
| `TestE2E_CleanupRemovesAll` (line 618) | 2s | Same |
| `TestE2E_StopAll` (line 822) | 2s | Same |

**2.11 `time.Sleep` in production code exercised by tests (pkg/docker/ssh_test.go)**

`TestAppendSSHMount_FallbackDirect` exercises the relay retry loop which calls `time.Sleep(200ms)` x5 = 1s in production code. No way to inject a clock, making this test inherently slow.

**2.12 Global flag mutation without parallelism guards (cmd/safe-ag/command_test.go)**

Package-level variables (`listJSON`, `stopAll`, `diffStat`, `outputDiff`, `costHistory`, etc.) are mutated and restored with `defer`. Safe today (no `t.Parallel()`), but adding parallelism would immediately cause races.

---

### P2 -- Missing Edge Cases

**2.13 `pkg/inject` -- `ReadClaudeSupportFiles()` entirely untested**

This function handles tar/gzip archiving of `CLAUDE.md`, `hooks/`, `commands/`, and `statusline-command.sh`. Covers complex I/O (tar creation, directory walking, gzip compression) with zero test coverage. Primary contributor to the 41.5% coverage.

**2.14 `pkg/cost` -- Non-deterministic prefix matching**

`lookupPricing` iterates a map and checks `strings.HasPrefix`. If a model name matches multiple prefixes (e.g., `"claude-3-haiku-20240307"` matches both `"claude-3-haiku"` and `"claude-3"`), the result depends on Go map iteration order. No test exposes this.

**2.15 `pkg/config/defaults_test.go` -- Missing defaults**

`TestDefaults` checks CPUs, Memory, PIDsLimit but never verifies `ReuseAuth: "true"` and `ReuseGHAuth: "true"` which are set at `defaults.go:42-43`.

**2.16 `pkg/events/events_test.go` -- Weak event field assertions**

`TestEmitWritesJSONL` (line 11-44) parses 2 events and asserts count is 2, but never verifies `Type`, `Payload`, or `Timestamp` fields.

**2.17 `pkg/fleet/manifest_test.go` -- Ambiguous pipeline behavior untested**

No test for a pipeline YAML with both `steps` AND `stages` defined. The implementation silently ignores `steps` when `stages` is present, but this is undocumented and untested. Also missing: invalid YAML syntax test, duplicate names, agents missing required fields (`name`, `type`, `repo`).

**2.18 `pkg/tmux/tmux_test.go` -- WaitForSession timeout path untested**

The 300-iteration retry loop that returns `"tmux session not ready after 60s"` has zero test coverage. Only context cancellation is tested.

**2.19 `cmd/safe-ag/spawn_test.go` -- Thin coverage of core spawn logic**

Only 127 lines for security-critical spawn logic. `TestSpawnDryRunContainsSecurityFlags` builds a Docker command manually via the `docker` package API, not by calling `executeSpawn`. Cannot catch regressions where `executeSpawn` forgets to call `AppendRuntimeHardening`.

**2.20 `pkg/docker/runtime_test.go` -- Incomplete tmpfs assertion**

`TestAppendRuntimeHardening` (line 147) asserts `--tmpfs /home/agent/.config:rw,noexec,size=32m` but does NOT verify the `uid=1000,gid=1000` suffix that `AddTmpfsOwned` produces. The assertion is incomplete.

---

### P3 -- Weak Assertions / Missing Negative Tests

**2.21 Error messages never verified**

Tests across `pkg/config/identity_test.go`, `pkg/validate/validate_test.go`, `pkg/repourl/parse_test.go`, `pkg/inject/inject_test.go` only check `err != nil` without inspecting the error message. A regression that returns the wrong error would go unnoticed.

**2.22 `FakeExecutor` prefix matching creates false confidence (pkg/docker/ tests)**

`FakeExecutor.Run()` matches responses by prefix of joined args. `fake.SetResponse("docker inspect --format", "claude")` matches ANY `docker inspect --format ...` call regardless of the actual format template. Tests don't verify exact commands being constructed. A bug that passes the wrong `--format` template would go undetected.

**2.23 Error-swallowing functions tested as correct (pkg/docker/)**

`ContainerExists` and `VolumeExists` return `false, nil` on ANY error (daemon down, permission denied, etc.). Tests exercise this pattern without flagging it. `TestContainerExists_DaemonError` or `TestVolumeExists_PermissionDenied` would reveal this design gap.

**2.24 Missing negative tests across packages**

| Package | Missing test |
|---------|-------------|
| `cmd/safe-ag` | `runList` when docker returns error |
| `cmd/safe-ag` | `runAttach` for container in "created" state |
| `cmd/safe-ag` | `runStop` when docker stop fails |
| `cmd/safe-ag` | `runFleet` with malformed YAML |
| `cmd/safe-ag` | `runPipeline` (no tests at all, only `runFleet`) |
| `pkg/docker` | `StartDinDRuntime` when `CreateLabeledVolume` fails |
| `pkg/docker` | `StartDinDRuntime` when `docker run -d` fails |
| `pkg/docker` | `PrepareNetwork` with `customNetwork = "bridge"` or `"container:*"` |
| `pkg/docker` | `AppendSSHMount` where relay succeeds after N retries |
| `pkg/config` | `DetectGitIdentity` when only name or only email is set |
| `pkg/config` | Defaults file with CRLF line endings |
| `pkg/config` | Defaults file with duplicate keys |
| `pkg/events` | `Emit` to a read-only file |
| `pkg/events` | `Emit` with empty event type string |
| `pkg/fleet` | Pipeline with circular `depends_on` |
| `pkg/fleet` | Agents missing required fields |
| `pkg/orb` | `OrbExecutor.Run` with cancelled context |
| `pkg/orb` | `OrbExecutor.Run` stderr capture |
| `tui` | `parseSessionMeta` with malformed JSON |
| `tui` | `sessionSearchDirs` with path traversal in repo name |

---

## 3. Cross-Cutting Issues

| Issue | Scope | Impact |
|-------|-------|--------|
| Tests test the `docker` package helpers, not the actual spawn workflow | `cmd/safe-ag` | A regression in `executeSpawn` (forgetting hardening) would pass all tests |
| No concurrency tests for shared-state code | `pkg/audit`, `pkg/orb`, `pkg/events` | Race conditions in production would go undetected |
| Context cancellation under-tested | All packages accepting `context.Context` | Only `tmux.WaitForSession` and `docker.waitForDinD` have cancellation tests |
| No tests verify Docker command argument ordering | `pkg/docker` | Docker CLI is position-sensitive; `strings.Contains` doesn't verify ordering |
| `FakeExecutor` overlapping prefix matches are non-deterministic | `pkg/orb` | `TestFakeExecutor_MultiplePrefixMatches_ErrorTakesPriority` documents this but doesn't fix it |
| Integration tests require real OrbStack VM | `cmd/safe-ag` | All `TestE2E_*` and `TestDet_*` tests skip in standard CI |

---

## 4. Recommended Actions (Priority Order)

1. **Fix `cmd/safe-ag` build error** -- resolve missing `addLatestFlag`/`targetFromArgs` so coverage can be measured.
2. **Add assertions to discarded outputs** -- replace all 19 `_ = output` patterns with actual content checks.
3. **Strengthen DryRun spawn tests** -- parse the generated Docker command and assert feature-specific flags.
4. **Add `ReadClaudeSupportFiles` tests** -- cover the tar/gzip round-trip to bring `pkg/inject` above 90%.
5. **Add tar path traversal test** for `extractTar` -- security-critical.
6. **Fix retry tests** -- call the actual retry function, not manual string concatenation.
7. **Convert integration `time.Sleep` to polling loops** with timeouts to reduce flakiness.
8. **Add negative tests** for the top 10 missing cases listed in section 2.24.
9. **Fix `lookupPricing` map iteration non-determinism** -- sort keys or use a deterministic data structure.
10. **Add `tui` tests** -- current 6.3% coverage is effectively untested.
