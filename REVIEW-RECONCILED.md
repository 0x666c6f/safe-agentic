# Reconciled Self-Review

## Shared findings

### [high] Shared auth reuse is enabled by default instead of being opt-in

Both reports agree that the spawn path mounts the shared Claude/Codex auth volume whenever `--ephemeral-auth` is not set, without consulting `--reuse-auth`. The shared analysis points to `cmd/safe-ag/spawn.go:330-353` / `cmd/safe-ag/spawn.go:330-352`, where the default path selects `safe-agentic-<agent>-auth` and labels the session as shared. That contradicts the documented security posture in `README.md`, `docs/security.md`, and related spawning/defaults docs that describe auth reuse as disabled unless `--reuse-auth` is explicitly enabled.

Impact: default sessions can read and mutate auth state from prior sessions, widening the default trust boundary.

### [high] VM egress guardrails do not apply to the managed Docker bridges actually created

Both reports agree that `vm/setup.sh:151-175` installs iptables rules matching interfaces named `sa+`, while managed networks are created by `pkg/docker/network.go:15-24` as ordinary Docker bridge networks without an explicit bridge name. The likely runtime interface names are `br-<id>`, so the RFC1918/localhost egress restrictions the docs describe for the managed default path are unlikely to match the agent bridges in practice.

Impact: the default managed-network path appears broader than documented, weakening the claim that private-network reachability is opt-in.

### [medium] Retry does not faithfully reconstruct the original spawn configuration

Both reports agree that retry logic depends on state that normal spawn does not persist correctly. The shared overlap is prompt reconstruction: `cmd/safe-ag/lifecycle.go:430-437` expects `SAFE_AGENTIC_PROMPT_B64`, but spawn passes prompts as command arguments and does not export that env var (`cmd/safe-ag/spawn.go:412-421` / `417-421`).

Impact: `safe-ag retry` can rerun without the original prompt context. Codex A additionally reports a repo-list round-trip bug from serializing repos as comma-joined `REPOS` and reading them back with `strings.Fields`.

### [medium] Documented success/failure exit hooks are inert

Both reports agree that `--on-complete` and `--on-fail` are accepted and recorded in spawn metadata (`cmd/safe-ag/spawn.go:443-448` / nearby lines), but the runtime only executes `SAFE_AGENTIC_ON_EXIT_B64` in `entrypoint.sh:439-449`. Neither report found a code path that consumes the success/failure hook metadata.

Impact: users can configure documented hooks that silently never run.

### [medium] `safe-agentic.json` setup hooks are searched at the wrong path depth

Both reports agree that repos are cloned into `/workspace/<owner>/<repo>` but hook discovery scans only `/workspace/*/safe-agentic.json` in `entrypoint.sh:293-299`. That misses the repo-local config file for the normal clone layout created earlier in the same script.

Impact: documented repo setup automation does not run for standard cloned repositories.

## Codex-a-only findings

### [medium] `--template` appears to be accepted but never applied

Codex A reports that spawn writes `SAFE_AGENTIC_TEMPLATE_B64` (`cmd/safe-ag/spawn.go:433-449`), but did not find any startup/runtime path that consumes it. That review groups `--template` together with the inert `--on-complete` / `--on-fail` flags as documented behavior that currently appears to be a no-op.

### [low] The `tui/` test suite is not currently hermetic

Codex A reports that `go test ./...` passed outside `tui/` but failed in `tui/` because the fake `orb` fixtures and command-text expectations have drifted (`tui/main_test.go`, `tui/poller_test.go`, `tui/app_test.go`, `tui/actions_helpers_test.go`). Codex B did not validate this area because its local Go test attempt was blocked by toolchain/cache issues.

## Codex-b-only findings

### [low] `aws-refresh` has a shell quoting bug around `AWS_PROFILE`

Codex B reports that `cmd/safe-ag/config_cmd.go:572-576` appends `AWS_PROFILE` into `~/.bashrc` through a shell-constructed `bash -c` string without escaping the profile name, while `pkg/inject/inject.go:168-187` only does a loose profile existence check. That can break or inject shell syntax for unusual profile names containing quotes, spaces, or metacharacters.

## Conflicts/uncertainties

- No direct factual conflicts emerged between the two reports. The overlapping high-severity findings are materially the same.
- The retry issue is shared, but only Codex A identified the additional multi-repo reconstruction bug (`REPOS` serialization vs. `strings.Fields`). That portion should be re-verified when fixing retry.
- The inert-hook issue is shared for `--on-complete` / `--on-fail`; only Codex A extends it to `--template`. That extension is plausible from the cited code, but it was not independently corroborated by Codex B.
- The TUI test-suite failure was only observed by Codex A. Codex B's environment prevented a comparable `go test ./...` run, so this remains a single-source validation note rather than a reconciled code finding.
