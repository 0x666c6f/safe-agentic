# Test Review Findings

**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`
**Date:** 2026-04-11
**Files reviewed:** 25 `*_test.go` files

## Coverage

`go test ./... -cover` passed.

Packages below 90%:

| Package | Coverage |
| --- | ---: |
| `cmd/safe-ag` | 61.6% |
| `pkg/fleet` | 58.2% |
| `pkg/inject` | 41.5% |
| `pkg/labels` | 0.0% |
| `tui` | 6.2% |

## Findings

### 1. Large command surfaces are effectively untested in `cmd/safe-ag`

The package is below threshold mostly because entire features have no meaningful tests at all: `runPipeline`, `runPipelineManifest`, `waitForContainers`, and `printPipelineTree` in [cmd/safe-ag/fleet.go](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/fleet.go#L187); the full cron surface in [cmd/safe-ag/cron.go](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/cron.go#L104); log/session rendering in [cmd/safe-ag/observe.go](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/observe.go#L62); and VM/TUI lifecycle helpers in [cmd/safe-ag/setup.go](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/setup.go#L181). The only fleet execution tests in [cmd/safe-ag/command_test.go](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/command_test.go#L2949) cover a dry-run banner and an empty manifest, but do not verify the real spawn path, error propagation from volume creation or `executeSpawn`, pipeline dependency deadlocks, sub-pipeline failures, duplicate cron names, invalid schedules, or cron state persistence.

### 2. Fleet manifest tests miss the parser branches that are most likely to regress

The current tests in [pkg/fleet/manifest_test.go](/workspace/0x666c6f/safe-agentic/pkg/fleet/manifest_test.go#L21) only cover basic happy paths plus missing files. They do not exercise defaults merging, `${var}` interpolation, `models` expansion, rewritten `depends_on` fan-out, or invalid YAML. Those are the exact branches implemented in [pkg/fleet/manifest.go](/workspace/0x666c6f/safe-agentic/pkg/fleet/manifest.go#L84), including `mergeDefaults` and `interpolateVars`, and they explain why `ParsePipeline` is still at 60.0% and `interpolateVars` remains uncovered. Missing negative tests here leave the manifest normalization logic largely unchecked.

### 3. `pkg/inject` leaves the tar/gzip support-file path completely uncovered

The tests in [pkg/inject/inject_test.go](/workspace/0x666c6f/safe-agentic/pkg/inject/inject_test.go#L10) cover base64 helpers, config reads, and AWS credentials, but there is no test coverage for `ReadClaudeSupportFiles`, `tarFile`, or `tarDir` in [pkg/inject/inject.go](/workspace/0x666c6f/safe-agentic/pkg/inject/inject.go#L49). That means there are no assertions for nested directory entries, relative tar paths, mixed file-and-directory bundles, empty bundle behavior, or error cases like unreadable files encountered during the walk. For a package at 41.5%, this is the largest gap.

### 4. TUI tests only cover pure helpers and barely touch user-visible behavior

The package is at 6.2%, and the single test file [tui/actions_test.go](/workspace/0x666c6f/safe-agentic/tui/actions_test.go#L9) only exercises helper functions like `buildAttachArgs`, `parseSessionMeta`, and one log-rendering path. None of the behavior-heavy methods in [tui/actions.go](/workspace/0x666c6f/safe-agentic/tui/actions.go#L39) are tested: `Attach`, `Resume`, `StopAgent`, `Logs`, `Checkpoint`, `Diff`, `Todo`, `Fleet`, `Pipeline`, `CreatePR`, and their error branches. Even the existing render test only checks for two substrings, so it would miss regressions in ordering, truncation, formatting, and ignored event types.

### 5. Several `cmd/safe-ag` tests are intentionally weak and do not verify behavior

There are multiple cases where tests explicitly capture output and then discard it, or only assert that a command completed. Examples include [cmd/safe-ag/command_test.go](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/command_test.go#L2061), [cmd/safe-ag/command_test.go](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/command_test.go#L2104), and [cmd/safe-ag/command_test.go](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/command_test.go#L2124), where `runDiagnose` and `runSetup` are exercised but the tests deliberately avoid asserting any concrete behavior. That makes the tests non-hermetic and low value: changes to user-facing output, branching, or error handling can slip through while the tests still pass.

### 6. `pkg/config` contains a flaky, environment-dependent test with weak assertions

[pkg/config/identity_test.go](/workspace/0x666c6f/safe-agentic/pkg/config/identity_test.go#L8) calls `DetectGitIdentity()` against the real machine config, skips when no global identity is configured, and only checks for the presence of `<` and `>`. That is both flaky across environments and too weak to catch malformed results like extra text around the address. A stable test should stub the git calls or split the logic so the returned value is validated deterministically.

### 7. `pkg/tmux` has a misleading test that does not exercise the claimed failure path

[pkg/tmux/tmux_test.go](/workspace/0x666c6f/safe-agentic/pkg/tmux/tmux_test.go#L164) is named `TestAttach_ReturnsErrorFromExecutor`, but the body uses `orb.NewFake()` and even documents that the fake cannot return an error from `RunInteractive`. The test therefore proves only the happy path while claiming to cover error propagation. It should use a stub executor that actually fails, otherwise the negative path in `Attach` is still untested.

### 8. `pkg/labels` has no tests at all

[pkg/labels/labels.go](/workspace/0x666c6f/safe-agentic/pkg/labels/labels.go#L1) is simple, but its package coverage is 0.0%. There is no assertion on `ContainerFilter()` or on the label keys shared across the CLI and container inspection flows. A trivial package can still break list/stop/cleanup targeting if the filter or labels drift, and today there is no regression signal at all.

## Notes

I reviewed all `*_test.go` files. The highest-value follow-up would be:

1. Add real command tests for `pipeline`, `cron`, and `logs`.
2. Expand `pkg/fleet` parser tests to cover defaults, vars, model expansion, and invalid manifests.
3. Add archive-content tests for `ReadClaudeSupportFiles`.
4. Replace environment-dependent or no-op tests in `pkg/config` and `pkg/tmux` with deterministic stubs.
