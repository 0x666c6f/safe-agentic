# Codex Test Review

Command run:

```bash
PATH=/workspace/.cache/go-mod/golang.org/toolchain@v0.0.1-go1.25.5.linux-arm64/bin:/usr/local/go/bin:$PATH \
GOPATH=/workspace/.cache/go \
GOMODCACHE=/workspace/.cache/go-mod \
GOCACHE=/workspace/.cache/go-build \
GOTMPDIR=/workspace/.cache/go-tmp \
/usr/local/go/bin/go test ./... -cover
```

Packages below 90% coverage:

| Package | Coverage |
| --- | ---: |
| `safe-agentic/cmd/safe-ag` | 60.9% |
| `safe-agentic/pkg/fleet` | 58.2% |
| `safe-agentic/pkg/inject` | 41.5% |
| `safe-agentic/pkg/labels` | 0.0% |
| `safe-agentic/tui` | 6.1% |

`go tool cover -func` reports 46.0% statement coverage overall. The large `integration` suites in [`cmd/safe-ag/integration_test.go`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/integration_test.go) and [`cmd/safe-ag/deterministic_test.go`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/deterministic_test.go) are build-tagged and do not contribute to the default `go test ./... -cover` result.

## Findings

1. `pkg/labels` and most of `tui` are effectively untested.
[`pkg/labels/labels.go`](/workspace/0x666c6f/safe-agentic/pkg/labels/labels.go) has no `*_test.go` at all, so every label constant plus `ContainerFilter()` can regress silently. [`tui/actions_test.go`](/workspace/0x666c6f/safe-agentic/tui/actions_test.go) only covers a few helpers, while `go tool cover` shows nearly all interactive behavior in [`tui/actions.go`](/workspace/0x666c6f/safe-agentic/tui/actions.go), [`tui/app.go`](/workspace/0x666c6f/safe-agentic/tui/app.go), [`tui/poller.go`](/workspace/0x666c6f/safe-agentic/tui/poller.go), and related UI files at 0.0%. Core flows like attach/resume branches, footer warnings, poller parsing, overlay actions, and preview updates are not pinned down.

2. `pkg/fleet` tests only cover happy-path parsing and miss the logic that is most likely to break.
[`pkg/fleet/manifest_test.go`](/workspace/0x666c6f/safe-agentic/pkg/fleet/manifest_test.go) exercises simple manifest parsing, but there are no tests for `defaults`, `vars`, model expansion, or dependency rewriting. Coverage confirms the gap: `ParsePipeline` is 60.0%, `mergeDefaults` is 51.9%, and `interpolateVars` in [`pkg/fleet/manifest.go`](/workspace/0x666c6f/safe-agentic/pkg/fleet/manifest.go) is 0.0%. Missing negatives include malformed YAML, unknown dependency targets after model expansion, and precedence when agent values override defaults.

3. `pkg/inject` misses the highest-risk file packaging path entirely.
[`pkg/inject/inject_test.go`](/workspace/0x666c6f/safe-agentic/pkg/inject/inject_test.go) covers basic base64 helpers and single-file config reads, but it never exercises `ReadClaudeSupportFiles`, `tarFile`, or `tarDir` in [`pkg/inject/inject.go`](/workspace/0x666c6f/safe-agentic/pkg/inject/inject.go). That leaves support bundle creation at 0.0% coverage. Missing cases include nested directories, empty support sets, unreadable files, tar member names, and verifying that gzip+tar output can actually be decoded into the expected file tree.

4. Several tests in `cmd/safe-ag/command_test.go` add coverage without verifying behavior.
[`TestDiagnoseCommand_AllOK`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/command_test.go#L2061), [`TestSetupCommand_DockerAvailable`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/command_test.go#L2104), and [`TestSetupCommand_DockerNotAvailable`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/command_test.go#L2124) intentionally avoid meaningful assertions. The same pattern appears in the execute-spawn branch tests starting at [`TestSpawnWithEphemeralAuth`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/command_test.go#L2424): they only check for `"Would execute"`, so they do not verify that ephemeral auth, Docker modes, callbacks, templates, instructions, or AWS settings actually change the rendered command/env/labels. [`TestRunFleet_DryRun`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/command_test.go#L2949) similarly only checks for `"Fleet manifest"` rather than the spawned agent plan.

5. `pkg/orb` contains a test that explicitly accepts nondeterminism instead of locking behavior down.
[`TestFakeExecutor_MultiplePrefixMatches_ErrorTakesPriority`](/workspace/0x666c6f/safe-agentic/pkg/orb/orb_test.go#L185) documents that the result may be either success or error because `FakeExecutor.Run` iterates over Go maps in unspecified order. That means the test cannot catch regressions in prefix precedence, and it also hides a real nondeterminism in [`pkg/orb/orb.go`](/workspace/0x666c6f/safe-agentic/pkg/orb/orb.go). This should be replaced with deterministic longest-prefix matching plus an assertion that the exact error wins.

6. `pkg/tmux` has an acknowledged hole in its negative-path testing.
[`TestAttach_ReturnsErrorFromExecutor`](/workspace/0x666c6f/safe-agentic/pkg/tmux/tmux_test.go#L164) states that the fake cannot simulate `RunInteractive` failure and then just re-runs the happy path. The actual error branch in `Attach` is therefore untested. Either the fake needs an injectable interactive error, or the package should accept a narrower interface that can be stubbed for this case.

7. The integration-style suites rely on fixed sleeps and skip on transient failures, which makes them flaky and can mask regressions.
[`cmd/safe-ag/integration_test.go`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/integration_test.go) uses fixed sleeps after lifecycle operations at lines 115, 574, 617, 677, 701, 754, and 822 instead of waiting on specific readiness conditions. [`cmd/safe-ag/deterministic_test.go`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/deterministic_test.go) has the same pattern at lines 90-98 and 539-549. Both files also use `t.Skipf` for transient runtime issues, for example container readiness or missing injected configs, which converts real failures into skips.

8. Some tests depend on ambient machine state instead of controlled fixtures.
[`TestDetectGitIdentity_Format`](/workspace/0x666c6f/safe-agentic/pkg/config/identity_test.go#L8) passes or skips depending on the host Git config. [`TestOrbExecutor_Run_SuccessWithRealVM`](/workspace/0x666c6f/safe-agentic/pkg/orb/orb_test.go#L235) depends on a real Orb VM being available. These are better suited to explicit integration tests; the default unit test suite should use fully controlled inputs.

9. There is a broad weak-assertion pattern around rendered shell commands.
[`pkg/docker/runtime_test.go`](/workspace/0x666c6f/safe-agentic/pkg/docker/runtime_test.go), [`cmd/safe-ag/spawn_test.go`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/spawn_test.go), and parts of [`cmd/safe-ag/integration_test.go`](/workspace/0x666c6f/safe-agentic/cmd/safe-ag/integration_test.go) mostly join argv into a string and use `strings.Contains`. That misses duplicate flags, bad ordering, partial matches, and quoting bugs. For command-builders, exact slice assertions are a stronger default.

## Highest-value follow-ups

1. Add targeted unit tests for `pkg/labels`, `pkg/inject` support bundle creation, and `pkg/fleet` defaults/vars/model expansion.
2. Replace no-op coverage tests in `cmd/safe-ag/command_test.go` with assertions on exact commands, labels, env vars, and output.
3. Make `pkg/orb.FakeExecutor` deterministic so prefix-precedence tests can assert a single outcome.
4. Convert fixed sleeps in build-tagged integration tests into readiness polling with bounded timeouts and fail on missing required artifacts instead of skipping.
