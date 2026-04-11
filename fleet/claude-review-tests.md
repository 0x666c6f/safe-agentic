# Test Quality Review — Go Rewrite (Phase 1)

**Date:** 2026-04-11
**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`
**Reviewer:** Claude (automated)
**Test runner:** `go test ./... -cover`

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
| pkg/labels | N/A (build tool issue) | UNKNOWN |

**4 packages below 90%. 1 package could not report coverage.**

---

## Critical Findings

### 1. tui/ — 6.2% coverage (CRITICAL)

Only `tui/actions_test.go` (105 lines, 8 tests) exists. The following source files have **zero test coverage**:

| File | Lines | Untested Functions |
|------|-------|--------------------|
| app.go | 382 | `NewApp`, `Run`, `handleInput` (155-line switch), `handleCommand`, `rebuildLayout`, `updatePreview` |
| model.go | 180 | `SortAgents`, `fieldByColumn` (14 cases), `FilterAgents`, `VisibleColumns` |
| table.go | 265 | `Update`, `SetSort`, `refresh` (107-line rendering), `restoreSelection` |
| poller.go | 293 | `Start`, `Stop`, `poll`, `fetchAgents`, `probeProcessActivity`, `parsePSOutput`, `mergeStats` |
| footer.go | 248 | `ShowFilter`, `ShowCommand`, `ShowConfirm`, `HandleConfirmKey` |
| overlay.go | 278 | `ShowOverlay`, `ShowCopyForm`, `ShowSpawnForm`, `shellQuoteArgs` |
| preview.go | 85 | `Toggle`, `Update`, `SetUnavailable` |
| dashboard.go | 154 | `Start` (HTTP server), `handleIndex`, `handleAgentDetail`, `handleAgentLogs` (SSE), `handleAPIAgents` |
| main.go | 98 | `main`, `preflight` |

**Existing tests are weak:**
- `TestBuildAttachArgs` / `TestBuildTmuxAttachArgs`: Only happy path, no edge cases for empty/special-char names.
- `TestParseSessionMeta`: No test for malformed JSON, missing fields, or empty payloads.
- `TestRenderSessionLogCurrentCodexFormat`: Only `strings.Contains` checks, doesn't validate structure.
- No negative tests in the entire package.

### 2. pkg/inject — 41.5% coverage (HIGH)

**Three functions completely untested:**

| Function | Lines | Description |
|----------|-------|-------------|
| `ReadClaudeSupportFiles` | 52-103 | Tar/gzip archive creation from support files |
| `tarFile` | 105-120 | Single-file tar header writing |
| `tarDir` | 122-152 | Directory traversal and tar archiving |

**Existing tests have gaps:**
- `TestReadClaudeConfigWithSettingsJSON`: No test for malformed JSON, empty file, or missing fields.
- `TestReadAWSCredentialsProfileFound`: No test for malformed credentials file, empty profile name, or special characters.
- `TestReadClaudeConfig_PermissionDenied` / `TestReadCodexConfig_PermissionDenied`: Skips if root — environment-dependent.
- No test for `EncodeFileB64` with symlinks, large files, or permission-denied paths.

### 3. pkg/fleet — 58.2% coverage (HIGH)

**Two functions entirely untested:**

| Function | Lines | Description |
|----------|-------|-------------|
| `mergeDefaults` | 199-240 | Merges manifest defaults into 14 agent fields |
| `interpolateVars` | 243-248 | Variable interpolation in YAML values |

**Complex logic untested:**
- Model expansion in `ParsePipeline` (lines 129-150): Expands `models: [a, b]` into multiple agents.
- `depends_on` reference fixing with model expansion (lines 154-179).
- Vars interpolation in `ParseFleet` (line 96-98 path).

**Missing negative tests:**
- No test for malformed YAML (syntax errors, not just missing file).
- No test for circular `depends_on` references.
- No test for invalid field values (negative memory/cpus, unknown agent type).
- No test for duplicate stage names in pipelines.

### 4. cmd/safe-ag — 61.9% coverage (HIGH)

**Major untested source files / functions:**

| File | Function | Issue |
|------|----------|-------|
| spawn.go | `executeSpawn` (lines 151-458) | Only dry-run tested; no unit tests for template loading, SSH key setup, auth injection, identity parsing |
| lifecycle.go | `runRetry` (lines 347-378) | Zero tests for retry command logic |
| observe.go | `renderLogEntry`, `extractTokenUsage`, `runLogs`, `runPeek`, `runOutput`, `runCost` | Minimal or no coverage for session log parsing, token extraction |
| config_cmd.go | `runTemplateList`, `runTemplateShow`, `runTemplateCreate` | No tests for template operations |
| fleet.go | `specToSpawnOpts`, `depsmet`, `waitForContainers` | No tests for fleet orchestration helpers |
| workflow.go | Todo, checkpoint, PR review operations | Minimal unit tests, no persistence verification |

---

## Quality Issues by Category

### A. Weak Assertions

| File | Test | Issue |
|------|------|-------|
| cmd/safe-ag/command_test.go | `TestListCommandJSON` | Checks `{{json .}}` in command string, never validates JSON output |
| cmd/safe-ag/command_test.go | `TestStopCommand_All` | Asserts stopCmds exist but never validates container names match |
| cmd/safe-ag/command_test.go | `TestCheckpointCreate` | Only checks "Checkpoint created" string, never validates SHA |
| cmd/safe-ag/command_test.go | All `TestOutput*` tests | Never validate actual output content, only that commands were invoked |
| pkg/docker/container_test.go | `TestResolveTarget_NotFound` | Only `strings.Contains(err, "not found")`, not exact error |
| pkg/docker/volume_test.go | `TestCreateLabeledVolume_VerifyLabels` | Checks label presence, not exact format |
| pkg/docker/network_test.go | `TestCreateManagedNetwork` | `strings.Contains` allows false positives |
| pkg/docker/runtime_test.go | `TestAppendRuntimeHardening_NoOptional` | Fragile trailing-space matching |
| pkg/events/events_test.go | `TestEmitTypeField` | Timestamp only checked for empty string, not RFC3339 format |
| pkg/events/notify_test.go | All tests | Check field count only, not field values |
| tui/actions_test.go | `TestRenderSessionLogCurrentCodexFormat` | Only `strings.Contains` checks |

### B. Missing Negative Tests

| Package | Missing |
|---------|---------|
| cmd/safe-ag | No tests for: docker ps failure, docker stop failure, docker rm failure, spawn with invalid repo URL, network unreachable, image pull failure, stop on already-stopped container |
| pkg/fleet | No tests for: malformed YAML, circular deps, invalid field values, duplicate names |
| pkg/inject | No tests for: malformed JSON/TOML, empty credentials file, tar write errors |
| pkg/docker | No tests for: multiple partial matches in `ResolveTarget`, `waitForDinD` timeout, DinD start failure (all retries exhausted) |
| pkg/events | No tests for: concurrent `Emit` calls (race condition), extremely large payloads |
| pkg/repourl | No tests for: IPv6 addresses, URLs with auth, query parameters, fragments |
| pkg/validate | No tests for: Unicode/emoji names, max-length names |

### C. Flaky Patterns

| File | Pattern | Risk |
|------|---------|------|
| cmd/safe-ag/deterministic_test.go | Shared container via `sync.Once` + hardcoded `time.Sleep(5s)` + 40-iteration polling | Fails on slow CI |
| cmd/safe-ag/deterministic_test.go | `time.Now().Unix()` in test filenames | Collision if two tests run same second |
| cmd/safe-ag/integration_test.go | `waitForContainer` polls 30s with 500ms sleep | May not be enough on CI |
| cmd/safe-ag/integration_test.go | Hardcoded `testPrefix = "agent-e2e-test"` | Cross-contamination with concurrent suites |
| cmd/safe-ag/command_test.go | Global flag state (`listJSON`, `outputDiff`) mutated without isolation | Parallel test failure risk |
| pkg/docker/dind_test.go | `waitForDinD` has 40×500ms polling loop but test mocks bypass it | Real timeout behavior untested |
| pkg/docker/ssh_test.go | SSH relay wait loop (5×200ms) is mocked away | Actual wait/timeout behavior untested |
| pkg/config/defaults_test.go | `TestLoadDefaults_OpenError` assumes non-root for permission test | Fails as root |
| pkg/inject/inject_test.go | `PermissionDenied` tests skip if root | Environment-dependent |
| pkg/orb/orb_test.go | VM name `"safe-agentic-test-nonexistent-vm-12345"` assumed nonexistent | Could exist in some environments |

### D. Tests That Don't Verify Behavior

| File | Test | Issue |
|------|------|-------|
| cmd/safe-ag/command_test.go | `TestRetryFeedbackAppended` | String concatenation, not actual retry logic |
| cmd/safe-ag/command_test.go | `TestContainerEnvVar` | Tests env var parsing helper, never verifies propagation to containers |
| cmd/safe-ag/spawn_test.go | `TestSpawnDryRunContainsSecurityFlags` | Constructs command manually, doesn't test actual dry-run output |
| cmd/safe-ag/spawn_test.go | `TestCoalesce` | Tautological: `coalesce("a", "b")` always returns "a" |
| cmd/safe-ag/integration_test.go | `TestE2E_DiffCommand` | Never creates file changes before running diff |
| cmd/safe-ag/integration_test.go | `TestE2E_TodoWorkflow` | Adds todo, marks done, but never retrieves to confirm state |
| pkg/docker/* | All tests using `fake.SetResponse` | Substring-based mock matching allows false positives |

---

## Recommendations

### Priority 1 — Coverage gaps (target: all packages >= 90%)

1. **tui/**: Add tests for `SortAgents`, `FilterAgents`, `VisibleColumns`, `parsePSOutput`, `mergeStats`. These are pure functions testable without a terminal. Defer UI interaction tests.
2. **pkg/inject**: Add tests for `ReadClaudeSupportFiles`, `tarFile`, `tarDir`. Create temp directories with known files and verify tar output.
3. **pkg/fleet**: Add tests for `mergeDefaults` (all 14 fields), `interpolateVars`, and model expansion logic.
4. **cmd/safe-ag**: Add unit tests for `renderLogEntry`, `extractTokenUsage`, `parsePeriod` edge cases, template operations.

### Priority 2 — Negative tests

5. Add error-path tests for all docker operations (stop failure, rm failure, network create failure).
6. Add malformed-input tests for YAML/JSON/TOML parsers in fleet, inject, config.
7. Add boundary tests for `truncate` (maxLen=0, negative, Unicode), `coalesce` (both non-empty), all name validators (Unicode, max-length).

### Priority 3 — Flaky pattern fixes

8. Replace hardcoded `time.Sleep` in deterministic_test.go with polling + deadline.
9. Isolate global flag state in command_test.go (reset in `t.Cleanup`).
10. Use unique per-test prefixes in integration_test.go instead of shared `testPrefix`.
11. Replace `time.Now().Unix()` filename generation with `t.Name()` or UUID.

### Priority 4 — Assertion quality

12. Replace `strings.Contains(err.Error(), "not found")` with exact error type checks or `errors.Is`.
13. Add content validation to all output/list/diff tests (not just "command was called").
14. Validate timestamp fields against RFC3339 format, not just non-empty.
15. Replace substring-based mock matching in docker fake with exact command matching.
