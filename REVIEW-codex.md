# REVIEW-codex

## Findings

### [critical] `spawn` always mounts the shared Claude/Codex auth volume, even when `--reuse-auth` is not set

- Evidence: `cmd/safe-ag/spawn.go:330-352` always falls through to `safe-agentic-<agent>-auth` unless `--ephemeral-auth` is set. There is no `opts.ReuseAuth` gate in the spawn path.
- Evidence: the docs still describe shared auth as opt-in in `README.md:10-13`, `README.md:33-39`, `docs/security/defaults.md:5-18`, and `docs/architecture/isolation.md:27-35`.
- Impact: every default session shares persisted Claude/Codex auth state across containers, which breaks the documented isolation model and silently widens credential reuse to sessions that were supposed to be public-repo-only.
- Suggested fix: only mount the shared auth volume when `opts.ReuseAuth` is true; otherwise use an ephemeral volume or no auth volume at all. Add regression coverage for default `spawn`, `run`, and `shell` sessions.

### [high] VM hardening is not actually re-applied by `safe-ag setup` or `safe-ag vm start`, despite the command text and docs saying it is

- Evidence: `cmd/safe-ag/setup.go:76-103` only prints manual `orb push` and `orb run` instructions for `vm/setup.sh`; it does not execute them.
- Evidence: `cmd/safe-ag/setup.go:165-199` advertises `vm start` as "Start the VM (and re-harden)" but only runs `orb start` and prints a tip afterward.
- Evidence: the docs promise automatic hardening in `README.md:68`, `docs/guide/configuration.md:86-89`, and `AGENTS.md:36,193`.
- Impact: users can reasonably believe they are back inside a hardened VM boundary after `setup` or `vm start`, while the OrbStack file-sharing hardening may still be absent. That is a trust-boundary problem, not just a docs typo.
- Suggested fix: either push and run `vm/setup.sh` automatically from both paths, or stop claiming automatic re-hardening and make the manual step explicit everywhere.

### [medium] `safe-agentic.json` setup hooks are effectively dead for normal cloned repos

- Evidence: repos are cloned into `/workspace/<owner>/<repo>` in `entrypoint.sh:256-283`.
- Evidence: hook discovery only scans `/workspace/*/safe-agentic.json` in `entrypoint.sh:289-299`, which misses the extra repo directory level.
- Evidence: the feature is still documented as part of startup in `docs/guide/spawning.md:74-89` and `docs/architecture/container.md:12-18`.
- Impact: repository setup hooks silently do not run in the common case, so onboarding/bootstrap flows drift from the docs and from user expectations.
- Suggested fix: scan `/workspace/*/*/safe-agentic.json` or walk the cloned repo list directly. Add an integration test that clones into `/workspace/org/repo` and asserts the hook is found.

### [medium] `--template`, `--on-complete`, and `--on-fail` are advertised features, but the runtime never uses them

- Evidence: `cmd/safe-ag/spawn.go:412-449` stores template and callback data in env vars or labels.
- Evidence: the runtime only consumes `SAFE_AGENTIC_ON_EXIT_B64` in `entrypoint.sh:442-448`; there is no corresponding read path for `SAFE_AGENTIC_TEMPLATE_B64`, `safe-agentic.on-complete-b64`, or `safe-agentic.on-fail-b64`.
- Evidence: the flags are documented in `docs/guide/spawning.md:74-88` and `docs/reference/cli.md:117-124`.
- Impact: users can ask for a template or success/failure callbacks and get no effect, while the CLI makes it look like the feature worked.
- Suggested fix: either wire templates into prompt construction and execute completion/failure hooks in the entrypoint/session wrapper, or remove the flags until they are implemented.

### [medium] `retry` cannot faithfully reconstruct the original prompt from a real container

- Evidence: `cmd/safe-ag/lifecycle.go:430-437` reconstructs the prompt from `SAFE_AGENTIC_PROMPT_B64`.
- Evidence: `cmd/safe-ag/spawn.go:412-420` never writes `SAFE_AGENTIC_PROMPT_B64`; it only passes the prompt as CLI args and stores a truncated label.
- Evidence: `entrypoint.sh:376-378` already has support for `SAFE_AGENTIC_PROMPT_B64`, so the missing piece is in the spawn path, not the runtime.
- Impact: `safe-ag retry` respawns without the original prompt unless tests or callers inject a synthetic env var that production containers never receive.
- Suggested fix: persist the full prompt in an env var or another reconstructable field at spawn time, then add a regression test that retries a real spawned prompt instead of a fabricated inspect output.

### [medium] Fleet and pipeline defaults cannot be overridden back to `false`

- Evidence: `pkg/fleet/manifest.go:123-138` merges booleans via `mergeBoolDefault`.
- Evidence: `pkg/fleet/manifest.go:272-273` implements that merge as `value || fallback`.
- Impact: once a manifest default sets `ssh`, `reuse_auth`, `reuse_gh_auth`, `auto_trust`, `background`, or `docker` to `true`, an individual agent cannot explicitly turn it back off. That makes least-privilege fleet composition impossible.
- Suggested fix: represent manifest booleans as pointers or separate "is set" state so `false` can override a `true` default. Add tests for explicit false overrides.

### [medium] Multi-repo sessions are supported at spawn time, but workflow commands act on whichever repo `find ... | head -1` returns first

- Evidence: docs present repeated `--repo` usage in `docs/guide/spawning.md:54-60`.
- Evidence: `cmd/safe-ag/workflow.go:16-30` resolves the workspace by finding the first `.git` directory under `/workspace` and then reuses that helper for diff/review/PR flows (`cmd/safe-ag/workflow.go:471-538` and the commands above them).
- Impact: `diff`, `review`, `pr`, checkpoint helpers, and other workspace-based commands target an arbitrary repo in multi-repo containers instead of the repo the user intended.
- Suggested fix: require an explicit repo selector for multi-repo sessions, or persist the primary repo at spawn time and use that consistently. Add tests that cover two cloned repos.

### [medium] The config/defaults surface exposes boolean defaults that the spawn path never applies

- Evidence: `pkg/config/defaults.go:14-25` and `cmd/safe-ag/config_cmd.go:212-260` expose defaults for SSH, Docker, Docker socket, auth reuse, and GH auth reuse.
- Evidence: `cmd/safe-ag/spawn.go:233-259` only applies config defaults for memory, CPUs, PIDs, network, and identity. The boolean defaults are loaded but ignored by `executeSpawn`.
- Evidence: `docs/guide/configuration.md:19-27` documents these keys as part of the defaults file.
- Impact: `safe-ag config set reuse_auth true`, `... set ssh true`, and similar settings look supported but do not affect normal `spawn`/`run` behavior. That is especially confusing because the hardcoded defaults in `pkg/config/defaults.go:36-43` already claim `ReuseAuth` and `ReuseGHAuth` are `"true"`.
- Suggested fix: either apply those defaults during spawn option resolution or remove them from the supported config surface until they are real.

## Verification Notes

- `bash -n bin/agent-session.sh bin/repo-url.sh entrypoint.sh vm/setup.sh` succeeded.
- `go test ./...` could not be run in this environment because `go` was not present in `PATH` (`/bin/bash: line 1: go: command not found`).
