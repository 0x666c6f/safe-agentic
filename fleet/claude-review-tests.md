# Test Quality Review — Go Rewrite (Phase 1)

**Date:** 2026-04-11
**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`
**Reviewer:** Claude Opus 4.6 (deep review, 25 test files)
**Test runner:** `go test ./... -cover` (could not run — Go 1.25.5 required, only 1.23.4 available in container; coverage numbers below from prior run)

---

## Coverage Summary

| Package | Coverage | Status |
|---------|----------|--------|
| pkg/validate | 100.0% | PASS |
| pkg/cost | 100.0% | PASS |
| pkg/orb | 98.1% | PASS |
| pkg/repourl | 97.5% | PASS |
| pkg/docker | 97.4% | PASS |
| pkg/audit | 97.1% | PASS |
| pkg/events | 96.3% | PASS |
| pkg/config | 95.8% | PASS |
| pkg/tmux | 94.1% | PASS |
| **cmd/safe-ag** | **61.9%** | **FAIL** |
| **pkg/fleet** | **58.2%** | **FAIL** |
| **pkg/inject** | **41.5%** | **FAIL** |
| **tui** | **6.2%** | **FAIL** |
| pkg/labels | N/A (no test file) | **FAIL** |

**4 packages below 90%. 1 package has no tests at all.**

---

## Critical Findings

### 1. tui/ — 6.2% coverage (CRITICAL)

Only `tui/actions_test.go` (105 lines, 8 tests) exists. The TUI has ~2000 lines of source across 10 files.

**Security-critical untested functions:**

| Function | File | Risk | Why |
|----------|------|------|-----|
| `shellQuoteArgs` | overlay.go | **CRITICAL** | Constructs shell commands run inside containers. Zero tests for adversarial inputs (single quotes, backticks, `$()`, newlines, null bytes). A quoting bug = command injection. |
| `parsePSOutput` | poller.go | **HIGH** | Parses tab-separated Docker output feeding the entire TUI. Malformed output from a compromised VM could inject fake agent data. |
| `handleAPIStop` | dashboard.go | **HIGH** | Accepts agent name from URL path, passes to `docker stop`. Name extracted with `strings.TrimPrefix` — no input validation. |
| `parseLabels` | poller.go | **MEDIUM** | Splits on `,` — breaks when label values contain commas. |

**Untested pure functions (easily testable):**

| Function | File | Notes |
|----------|------|-------|
| `SortAgents` | model.go | Complex fleet grouping logic |
| `FilterAgents` / `matchesFilter` | model.go | Case-insensitive matching; omits `Activity` and `Fleet` fields (possible bug) |
| `VisibleColumns` | model.go | Priority-based column dropping — edge cases like width=0 untested |
| `extractText` | actions.go | Parses untrusted JSON from session files |
| `renderSessionLog` | actions.go | Only Codex format partially tested; Claude format completely untested |
| `resumeCLIArgs` | actions.go | Returns `--dangerously-skip-permissions` / `--yolo` flags; error path for unknown types untested |

**Existing test weaknesses:**
- `TestBuildResumeExecArgs`: Only tests `"codex"` and `"claude"` types, never tests error path for unknown types.
- `TestParseSessionMeta`: No test for malformed JSON, missing fields, empty input, or multiple `session_meta` entries.
- `TestRenderSessionLogCurrentCodexFormat`: Only `strings.Contains` checks — function could return garbage and test would pass.
- No negative tests in the entire package.

### 2. pkg/inject — 41.5% coverage (HIGH)

**Completely untested functions:**

| Function | Lines | Description |
|----------|-------|-------------|
| `ReadClaudeSupportFiles` | 52-103 | Complex tar/gzip archive creation from support files — a bug could inject wrong files or produce corrupt archives |
| `tarFile` | 105-120 | Single-file tar header writing |
| `tarDir` | 122-152 | Directory traversal and tar archiving |

**Existing test gaps:**
- `TestReadClaudeConfigWithSettingsJSON`: No test for malformed JSON, empty file, or missing fields.
- `TestReadAWSCredentialsProfileFound`: No test for malformed credentials file or special characters in profile names.
- Permission-denied tests skip if root — environment-dependent.
- No test for `EncodeFileB64` with symlinks, large files, or permission-denied paths.

### 3. pkg/fleet — 58.2% coverage (HIGH)

**Completely untested features:**

| Feature | Source Lines | Complexity |
|---------|-------------|------------|
| `mergeDefaults` | 199-240 | Merges 14 agent fields from manifest defaults |
| `interpolateVars` | 243-248 | Variable interpolation — security concern if user-controlled vars injected into prompts |
| Model expansion | 131-152 | Expands `models: [a, b]` into multiple agents |
| `depends_on` fixing | 157-181 | Adjusts references after model expansion (iterates map — non-deterministic order) |

**Missing negative tests:**
- No test for malformed YAML syntax
- No test for circular `depends_on` references
- No test for invalid field values (negative memory/cpus, unknown agent type)
- No test for duplicate stage names in pipelines

### 4. cmd/safe-ag — 61.9% coverage (HIGH)

**Entire files/features with zero unit test coverage:**

| File/Feature | Functions | Notes |
|---|---|---|
| `cron.go` (300+ lines, 14 functions) | `runCronAdd`, `runCronList`, `runCronRemove`, `runCronEnable`, `runCronDisable`, `runCronRun`, `runCronDaemon`, `setCronEnabled`, `executeCronJob`, `shouldRun`, `parseSchedule`, `loadCronConfig`, `saveCronConfig`, `cronConfigPath` | **Largest untested file in cmd/safe-ag** |
| `observe.go` | `runLogs`, `renderLogEntry`, `extractAssistantText`, `sessionSearchDirs`, `ensureRunning`, `jsonString` | Session log parsing/rendering |
| `fleet.go` pipeline | `runPipeline`, `runPipelineManifest`, `waitForContainers`, `printPipelineTree`, `printStageAgents` | Pipeline orchestration |
| `setup.go` | `runVMStart`, `runVMStop`, `findTUIBinary`, `runTUI`, `runDashboard` | VM management |
| `workflow.go` | `readTodos`, `writeTodos`, `workspaceFindCmd`, `workspaceExec` | Workflow helpers |

### 5. pkg/labels — No tests at all

The `ContainerFilter()` function returns a hardcoded label string used for container filtering. While trivial, a single test would prevent accidental breakage.

---

## Quality Issues by Category

### A. Weak Assertions (16+ `_ = output` patterns)

| File | Test | Issue |
|------|------|-------|
| cmd/safe-ag/command_test.go | `TestListCommand`, `TestPeekCommand`, `TestDiffCommand`, `TestOutputCommand_Files`, `TestConfigGet_NotSet`, `TestCheckpointRevert`, `TestPRCommand`, `TestReviewCommand` + 8 others | Output captured but **never asserted** (`_ = output`). Function could print garbage and test passes. |
| cmd/safe-ag/command_test.go | `TestDiagnoseCommand_AllOK` | Comment: "We just verify it completes without panic" — zero assertions on content. |
| cmd/safe-ag/command_test.go | `TestSetupCommand_*` | Explicitly "just want to exercise the code path" — not real tests. |
| cmd/safe-ag/command_test.go | `TestRunRetry_WithFeedback` | `_ = err` — ignores whether function succeeded or failed. |
| cmd/safe-ag/command_test.go | `TestVMSSHCommand` | No verification that `RunInteractive` was called. |
| cmd/safe-ag/deterministic_test.go | `TestDet_NodeVersion` | `strings.HasPrefix(out, "v")` — string "vanity" would also pass. |
| cmd/safe-ag/deterministic_test.go | `TestDet_SafeAgOutputFiles` | Captures output, only logs it (`t.Logf`) — **zero assertions**. |
| cmd/safe-ag/deterministic_test.go | `TestDet_SafeAgPeek` | Accepts both success and failure as valid (`if err == nil { return }`) — **can never fail**. |
| cmd/safe-ag/deterministic_test.go | `TestDet_SafeAgCost` | Same: accepts errors as "expected" — **can never fail**. |
| cmd/safe-ag/integration_test.go | `TestE2E_DiffCommand` | Logs error as non-fatal and returns early — test passes even when diff fails. |
| cmd/safe-ag/integration_test.go | `TestE2E_PeekShowsOutput` | `t.Skipf` on failure — **can never fail**. |
| pkg/docker/container_test.go | `TestInspectLabel` | FakeExecutor prefix matching means any `docker inspect --format` command matches — doesn't verify the correct label was queried. |
| pkg/docker/dind_test.go | `TestStartDinDRuntime` | Checks `docker run -d` was called but doesn't verify network name, volume mounts, labels, or image args. DinD runs `--privileged` — full command verification is security-critical. |
| pkg/docker/dind_test.go | `TestCleanupAllDinD` | Doesn't verify correct filter labels are being used — implementation that skips label filter would still pass. |
| pkg/orb/orb_test.go | `TestFakeExecutor_MultiplePrefixMatches_ErrorTakesPriority` | **Literally cannot fail** — success case uses `t.Log` instead of assertion. |
| tui/actions_test.go | `TestRenderSessionLogCurrentCodexFormat` | Only `strings.Contains` — doesn't validate structure or completeness. |

### B. Missing Negative Tests

| Package | Missing |
|---------|---------|
| cmd/safe-ag | `runList` when docker ps errors; `runAttach` with empty container name; `runCleanup` with `--auth` flag; `runPR`/`runReview` with docker exec errors; `runFleet` with invalid YAML; `runMCPLogin` with 0 args; `runAWSRefresh` entirely; spawn with invalid agent type; double-spawn same name |
| pkg/docker | Multiple partial matches in `ResolveTarget` (first match wins silently); `waitForDinD` timeout (40-iteration exhaustion); DinD start failure; `RemoveDinDRuntime`/`CleanupAllDinD` error swallowing; `PrepareNetwork` with `"bridge"` and `"container:*"` modes |
| pkg/fleet | Malformed YAML; circular deps; invalid field values; duplicate names |
| pkg/inject | Malformed JSON/TOML; empty credentials; tar write errors; symlinks in tarDir |
| pkg/config | CRLF line endings; empty-quoted values; duplicate keys; line `export =value` |
| pkg/repourl | `http://` URLs; URLs with ports, auth tokens, query params, fragments; null bytes |
| pkg/events | Concurrent `Emit` calls; extremely large payloads; symlink paths |
| pkg/validate | Unicode/emoji names; null bytes; max-length strings; `"container:"` with empty suffix |
| pkg/cost | Empty model string; negative token counts; case sensitivity inconsistency |
| tui | All of the above plus: `resumeCLIArgs` unknown types; `parseSessionMeta` malformed input |

### C. Flaky Patterns

| File | Pattern | Risk |
|------|---------|------|
| cmd/safe-ag/deterministic_test.go | Shared container via `sync.Once` + `time.Sleep(5s)` + 40-iteration polling | Fails on slow CI |
| cmd/safe-ag/deterministic_test.go | Tests modify shared container state (files, branches) without cleanup | Order-dependent failures |
| cmd/safe-ag/integration_test.go | `waitForContainer` polls 30s × 500ms; other tests use `time.Sleep(2-5s)` | Timing-dependent on slow machines |
| cmd/safe-ag/integration_test.go | Hardcoded `testPrefix = "agent-e2e-test"` | Cross-contamination if concurrent |
| cmd/safe-ag/command_test.go | 20+ global flag variables mutated with `defer` reset | Parallel test failure; leaked state if panic |
| cmd/safe-ag/command_test.go | `captureOutput` redirects `os.Stdout` globally | Not goroutine-safe — any `t.Parallel()` corrupts output |
| pkg/config/identity_test.go | `TestDetectGitIdentity_Format` calls real `git config` | Environment-dependent; skips in CI |
| pkg/cost/pricing.go | `lookupPricing` iterates map for prefix matching | Non-deterministic: `"gpt-4o-mini-2024"` could match `"gpt-4o"` or `"gpt-4o-mini"` depending on iteration order — **latent bug** |
| pkg/docker/dind.go | `RemoveDinDRuntime` and `CleanupAllDinD` return nil unconditionally | Silent failure to clean up privileged DinD containers |
| pkg/docker/container.go + volume.go | `ContainerExists` and `VolumeExists` treat any error as "not found" | Docker daemon down → silently returns false |

### D. Tests That Don't Verify Behavior

| File | Test | Issue |
|------|------|-------|
| cmd/safe-ag/command_test.go | `TestSetupCommand_DockerAvailable` / `_NotAvailable` | Explicitly: "just want to exercise the code path" — zero assertions. |
| cmd/safe-ag/command_test.go | `TestRunAuditCommand_WithEntries` | `_ = output` — no assertion on what audit printed. |
| cmd/safe-ag/lifecycle_test.go | `TestRetryFeedbackAppended` / `_NoOriginalPrompt` | Tests manual string concatenation, **not** the actual `runRetry` function. Would pass even if real implementation changed. |
| cmd/safe-ag/spawn_test.go | `TestSpawnDryRunContainsSecurityFlags` | Constructs command manually — doesn't test actual dry-run output. |
| All pkg/docker/* tests | FakeExecutor prefix matching | `fake.SetResponse("docker inspect --format", ...)` matches **any** `docker inspect --format` command regardless of label or container. Tests appear to verify behavior but actually only verify a command with the right prefix was called. |

### E. Systemic Issues

1. **FakeExecutor prefix matching is overly permissive.** `strings.HasPrefix` means `fake.SetResponse("docker inspect", "...")` matches both `docker inspect` and `docker inspect --format '{{.State.Running}}'`. Multiple tests appear to verify correct behavior but actually only verify that *some* docker command with the right prefix was called. For a security-focused project, verifying exact command arguments is critical.

2. **No concurrency tests anywhere.** For a tool managing Docker containers and networks, there are no tests for concurrent spawn, concurrent stop, or race conditions in shared auth volume access.

3. **No context timeout tests.** The source uses `context.Context` throughout, but only one test (`TestWaitForDinD_ContextCancel`) tests cancellation — and it cancels immediately rather than testing a deadline.

4. **Error swallowing in production code uncovered by tests.** `ContainerExists`, `VolumeExists`, `RemoveDinDRuntime`, and `CleanupAllDinD` all silently swallow errors. No test documents or asserts this behavior.

5. **Global mutable state in cmd/safe-ag.** 20+ package-level variables for CLI flags are mutated in tests with `defer` reset. This makes parallel test execution impossible and risks state leaks on panic.

---

## Recommendations

### Priority 1 — Security-critical coverage gaps

1. **`shellQuoteArgs` (tui/overlay.go)** — Add adversarial input tests: single quotes, backticks, `$()`, newlines, null bytes, empty args. This is the #1 security risk in the test suite.
2. **`parsePSOutput` (tui/poller.go)** — Add tests for malformed tab-separated output, missing fields, extra tabs, empty lines.
3. **`handleAPIStop` (tui/dashboard.go)** — Add input validation tests for agent names from URL paths.
4. **`ReadClaudeSupportFiles` (pkg/inject)** — Add tests with known temp files, verify tar output contents.
5. **`interpolateVars` (pkg/fleet)** — Add tests ensuring user-controlled vars can't inject shell commands into prompts.

### Priority 2 — Coverage targets (all packages >= 90%)

6. **tui/**: Test `SortAgents`, `FilterAgents`, `VisibleColumns`, `extractText`, `renderSessionLog` (Claude format). All pure functions.
7. **pkg/inject**: Test `ReadClaudeSupportFiles`, `tarFile`, `tarDir`.
8. **pkg/fleet**: Test `mergeDefaults` (all 14 fields), model expansion, `depends_on` fixing.
9. **cmd/safe-ag**: Test `cron.go` (14 functions, 300+ lines, zero coverage), `renderLogEntry`, `extractTokenUsage`, template operations.

### Priority 3 — Fix latent bugs found during review

10. **`lookupPricing` map iteration (pkg/cost)** — Non-deterministic prefix matching. Fix: sort keys by length descending (longest match first) or use a trie.
11. **`matchesFilter` missing fields (tui/model.go)** — `Activity` and `Fleet` fields omitted from filter check. Likely a bug.
12. **`RemoveDinDRuntime`/`CleanupAllDinD` silent error swallowing (pkg/docker/dind.go)** — At minimum, log errors. For security, consider returning them.

### Priority 4 — Test infrastructure improvements

13. Replace FakeExecutor's `strings.HasPrefix` matching with exact command matching (or at least longest-prefix matching).
14. Replace hardcoded `time.Sleep` in integration/deterministic tests with polling + deadline.
15. Isolate global flag state in command_test.go using `t.Cleanup` or test-local structs.
16. Replace `captureOutput` (os.Stdout redirect) with `bytes.Buffer` injection for goroutine safety.
17. Add `t.Setenv` instead of `os.Setenv`/`os.Unsetenv` for environment variable mutation.

### Priority 5 — Missing negative test categories

18. Docker operation failures (stop, rm, network create, image pull).
19. Malformed input (YAML, JSON, TOML, base64, credentials files).
20. Boundary values (empty strings, nil slices, zero/negative numbers, unicode, max-length strings).
21. Context cancellation and timeout paths.
22. Concurrent access to shared resources.
