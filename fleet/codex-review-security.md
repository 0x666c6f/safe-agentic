# Security Review: Go Phase 1 Foundation

**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`  
**Reviewer:** Codex  
**Date:** 2026-04-11  
**Scope:** All Go code present on this branch (`tui/*.go`, `tui/actions_test.go`)

## Scope Notes

`CLAUDE.md` describes a larger Go rewrite with `cmd/safe-ag/` and multiple `pkg/` packages, but those Go paths do not exist on this branch. The actual Go implementation here is the TUI/dashboard under `tui/`, so this review is limited to that code.

## Executive Summary

I did **not** find a straightforward shell-injection bug from the reviewed `exec.Command` calls. Most process launches use argument vectors, and the one user-influenced shell hop in `tui/actions.go:148-157` shell-quotes `cwd` before building `bash -lc`.

I did find **3 actionable issues**:

1. `agent-tui --dashboard` exposes unauthenticated stop/log endpoints that trust raw URL path data as a Docker target.
2. The branch does not enforce the `CLAUDE.md` Go security model: the documented validation/orchestration packages are absent, and the TUI issues raw `orb ... docker ...` commands directly.
3. The file-copy UI accepts arbitrary container and VM paths with no confinement to managed workspaces.

## Findings

### 1. High: dashboard HTTP handlers allow unauthenticated container control and log access

**Evidence**

- `tui/main.go:21-34` starts the dashboard on a caller-supplied bind address.
- `tui/dashboard.go:53-60` routes `/agents/<name>/logs` directly into `handleAgentLogs`.
- `tui/dashboard.go:75-103` executes `docker exec <name> tmux capture-pane ...` using the path-derived `name`.
- `tui/dashboard.go:140-152` executes `docker stop <name>` using the path-derived `name`.
- Neither handler validates that `name` belongs to the current `safe-agentic` agent list, matches the managed `agent-` prefix, or carries any anti-CSRF/auth check.

**Impact**

Anyone who can reach the dashboard can target arbitrary VM containers by name, not just the containers surfaced by `fetchAgents()`. On a non-loopback bind this is remote by default; on the default loopback bind it is still reachable from the local browser with no CSRF protection on the stop endpoint. That is enough to stop agents and read live tmux output from any container whose name is known and which has tmux installed.

**Why this matters against `CLAUDE.md`**

`CLAUDE.md:45-47` says Go code should centralize Docker/VM execution and validate container names. These handlers bypass that entirely.

**Recommendation**

- Validate `name` against the current poller snapshot or a shared container-name validator before calling Docker.
- Reject non-`safe-agentic` containers explicitly.
- Add dashboard authentication or a per-process random token.
- Enforce `Origin`/CSRF checks for browser-initiated mutating routes.

### 2. Medium: the documented Go security model is not enforced by the code on this branch

**Evidence**

- `CLAUDE.md:44-48` claims Go-side enforcement through `pkg/orb/`, `pkg/docker/`, `pkg/validate/`, and `pkg/repourl/`.
- `CLAUDE.md:183-201` further claims unsafe Docker options are blocked and names/network modes are validated.
- The only Go code on this branch is `tui/`; there is no Go `cmd/safe-ag/` or `pkg/` tree to provide those controls.
- Instead, the TUI issues raw host-side commands in multiple places:
  - `tui/poller.go:190-199`
  - `tui/actions.go:48-67`
  - `tui/actions.go:86-128`
  - `tui/dashboard.go:93-150`
  - `tui/overlay.go:69`

**Impact**

This is an assurance gap with security consequences. Reviewers and users reading `CLAUDE.md` would expect centralized validation and policy enforcement in Go, but the current code has none of that. As a result, container names, Docker targets, and copy paths are trusted ad hoc at each call site. Finding 1 exists specifically because these shared validation layers are not present.

**Recommendation**

- Either narrow `CLAUDE.md` to describe the actual branch state, or port the documented validation/orchestration layers before claiming parity.
- Introduce a single helper for managed-container targeting and route all TUI/dashboard Docker calls through it.
- Add tests that prove non-managed names and unsafe targets are rejected.

### 3. Low: copy UI allows arbitrary path selection inside the container and the VM

**Evidence**

- `tui/overlay.go:48-77` takes free-form `containerPath` and `hostPath` input from the UI.
- The handler then runs `docker cp <container>:<containerPath> <hostPath>` with no validation, no path-cleaning, and no confinement to `/workspace` or a managed export directory.

**Impact**

A TUI operator can copy any file that Docker can read from the container filesystem, including session/auth material under `/home/agent`, to any writable path in the VM. This is not a shell injection bug, but it does bypass the "work only within managed workspaces" spirit of the project and makes it easy to exfiltrate or overwrite data outside the repo checkout.

**Why this matters against `CLAUDE.md`**

`CLAUDE.md` presents the system as safe-by-default and tightly scoped around isolated workspaces. This UI path is much broader than that model.

**Recommendation**

- Restrict source paths to approved prefixes such as `/workspace/` and explicit exportable session directories.
- Restrict destination paths to a dedicated export directory in the VM.
- Show the resolved source/destination paths before executing the copy.

## Areas Reviewed With No Findings

- `exec.Command` / `exec.CommandContext`: I did not find user-controlled shell interpolation through these calls. The reviewed code generally passes arguments as argv, not as shell strings.
- `tui/actions.go:148-157`: resume uses `shellQuoteArgs` for `cwd`, so the `bash -lc` hop is not a trivial injection sink.
- Path traversal via repo labels in log lookup: `tui/actions.go:359-366` replaces `/` with `-`, so the repo label does not directly become a traversable filesystem path.
- Secrets in env or labels: I did not find Go code on this branch writing secrets into environment variables or Docker labels. The TUI reads status labels only (`safe-agentic.agent-type`, `repo-display`, `ssh`, `auth`, `gh-auth`, `docker`, `network-mode`, `fleet`, `terminal`).

## Overall Assessment

The main security issue in the current Go foundation is not classic `exec.Command` injection. It is missing enforcement. The dashboard and TUI trust raw names and paths in places where `CLAUDE.md` says centralized validation should already exist. Until that gap is closed, the branch should not be described as enforcing the documented Go security model.
