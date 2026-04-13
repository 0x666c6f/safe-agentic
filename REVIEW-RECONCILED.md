# Reconciled Review Findings

Date: 2026-04-13

Sources reviewed:
- `REVIEW-CODE-CLAUDE.md`
- `REVIEW-CODE-CODEX.md`
- `REVIEW-SECURITY-CLAUDE.md`
- `REVIEW-SECURITY-CODEX.md`
- `REVIEW-DOCS-CLAUDE.md`
- `REVIEW-DOCS-CODEX.md`
- `REVIEW-TESTS-CLAUDE.md`
- `REVIEW-TESTS-CODEX.md`
- `REVIEW-DESIGN-TYPES-CLAUDE.md`
- `REVIEW-DESIGN-TYPES-CODEX.md`

This document reconciles overlapping findings against the current `main` tree. Shared findings are listed first in each category. Recommendation conflicts are called out explicitly at the end.

## Security And Isolation

### Shared findings

1. High: auth reuse is not actually opt-in at spawn time.
   Sources: Code/Codex, Security/Claude, Security/Codex, Docs/Codex, Docs/Claude.
   Current behavior mounts the shared Claude/Codex auth volume unless `--ephemeral-auth` is set. That contradicts the project’s "safe by default" posture and multiple docs that describe shared auth as opt-in.

2. High: AWS credential handling is broader and less safe than intended.
   Sources: Code/Claude, Security/Claude, Security/Codex.
   Two problems overlap:
   - `pkg/inject.ReadAWSCredentials` injects the entire `~/.aws/credentials` file instead of scoping to the selected profile.
   - `aws-refresh` rewrites credentials in-container using an interpolated heredoc and appends an unquoted profile export to `.bashrc`.

3. Critical/High: repo-controlled setup hooks cross a trust boundary too early.
   Sources: Security/Claude, Code/Codex, Docs/Codex.
   `entrypoint.sh` auto-discovers and executes `safe-agentic.json` setup scripts from cloned repos. That is convenient, but it means arbitrary repo content runs automatically before the agent session starts.

4. High: SSH relay setup is too permissive and not race-safe.
   Sources: Security/Codex, Security/Claude, Code/Claude.
   The relay socket is shared globally in `/tmp`, is intentionally world-accessible, and is created through a check-then-start flow that can race between concurrent spawns.

### Additional findings

1. Critical: PR creation still shells a user-controlled title through `bash -c`.
   Sources: Code/Claude, Security/Claude.

2. High: security preamble injection uses `sed` substitutions with unescaped runtime values.
   Source: Code/Claude.

3. High: `trust_workspace` interpolates filesystem paths directly into embedded Python.
   Source: Code/Claude.

4. High: Claude support-file packaging can follow symlinks and include unintended files.
   Source: Security/Codex; also raised as a test gap by Tests/Claude and Tests/Codex.

5. High but not directly fixed here: Claude CLI install is still `curl | bash` without repository-pinned verification.
   Sources: Code/Claude, Security/Claude.

6. Critical/High but architectural: DinD still relies on `--privileged`, and `--docker-socket` remains a powerful escape hatch into the VM Docker daemon.
   Sources: Security/Claude.

## Runtime, Manifest, And Pipeline Semantics

### Shared findings

1. High: fleet/manifest booleans cannot represent "explicit false".
   Sources: Code/Claude, Design-Types/Claude, Design-Types/Codex, Tests/Claude.
   `mergeBoolDefault` collapses "unset" and `false`, so defaults like `reuse_auth: true` cannot be overridden to `false` in YAML.

2. High: pipeline manifests advertise failure-control behavior that the runtime does not enforce.
   Sources: Code/Codex, Design-Types/Codex, Code/Claude, Tests/Codex.
   The manifest types and docs accept `on_failure`, `retry`, `when`, and `outputs`, but the executor only waits for containers to exit and treats any exit as completion.

3. High: spawn mode and defaults are represented as loose booleans/strings with incomplete validation.
   Sources: Design-Types/Claude, Design-Types/Codex, Code/Codex.
   Illegal combinations such as conflicting auth modes or multiple docker modes remain representable, and some defaults parsed by `config` are not consumed consistently.

### Additional findings

1. High: `--template` currently stores only the template name in env instead of resolving and injecting the template content.
   Source: Code/Claude.

2. High: on-exit callbacks are unreachable for the normal tmux-based agent flow because the callback block runs after control paths that never return.
   Source: Code/Claude.

3. Medium but clearly correct: retry reconstruction splits `REPOS` by whitespace even though spawn serializes them as comma-delimited.
   Sources: Code/Claude, Code/Codex.

## Documentation Drift

### Shared findings

1. High: auth-related docs describe shared auth as opt-in, but runtime behavior has been shared-by-default.
   Sources: Docs/Codex, Docs/Claude, Security/Codex.

2. High: setup and VM-start docs over-promise automatic hardening/image behavior.
   Source: Docs/Codex.

3. High: multiple command examples are stale or nonexistent.
   Sources: Docs/Claude, Docs/Codex.
   Repeated examples include nonexistent aliases, nonexistent commands, unsupported flags, and stale operational examples in `CLAUDE.md`.

### Additional findings

1. Critical docs issue: `AGENTS.md` contains author-specific absolute links instead of repo-relative links.
   Source: Docs/Claude.

2. Medium: docs imply setup hooks apply generically to cloned repos, while the current entrypoint only searches a narrow subset of workspace layouts.
   Sources: Docs/Codex, Code/Codex.

## Tests And Validation

### Shared findings

1. Critical/High: pipeline execution behavior is under-tested relative to the risk of the feature.
   Sources: Tests/Codex, Tests/Claude.

2. High: shell-driven entrypoint and setup behavior is still mostly protected by manual testing rather than focused regression tests.
   Sources: Tests/Codex, Tests/Claude.

3. High: security-sensitive support paths need coverage.
   Sources: Tests/Claude, Tests/Codex.
   The repeated hotspots are:
   - Claude support-file packaging
   - AWS refresh / AWS credential injection
   - VM hardening and network guardrails

### Additional findings

1. High: the TUI test harness is reported red in `go test ./...` and remains timing-sensitive.
   Source: Tests/Codex.

2. High: cron scheduling behavior has little effective regression coverage.
   Source: Tests/Codex.

## Recommendation Conflicts

1. Auth defaults:
   - Some docs and config examples imply shared auth is the expected default.
   - Security and architecture reviews expect shared auth to be opt-in.
   Reconciled direction: make auth ephemeral-by-default for standard spawns and require explicit `--reuse-auth` for shared state.

2. Repo setup hooks:
   - Product/docs reviews treat setup hooks as a normal convenience feature.
   - Security review treats automatic execution from cloned repos as a trust-boundary violation.
   Reconciled direction: keep the feature only behind explicit opt-in for the current session/container.

3. Pipeline control fields:
   - Design and code reviews disagree on whether to implement the advertised fields now or reject them until semantics exist.
   Reconciled direction: at minimum enforce real exit-status failure semantics immediately; do not continue advertising unsupported control fields silently.

## Already Addressed On Current Tree

1. The `observe.go` tail command is already shell-quoted on `main`, so the older unquoted path finding does not reproduce on the current tree.
