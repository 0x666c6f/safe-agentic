# Codex App Parity Roadmap

This roadmap compares safe-agentic with Codex app features and turns the gaps into implementation tracks.

Reference inputs checked 2026-06-23:
- Codex app feature overview: https://developers.openai.com/codex/app/features
- Codex browser-use changelog: https://developers.openai.com/codex/changelog
- Codex web setup and GitHub PR creation: https://developers.openai.com/codex/cloud
- Codex GitHub code review: https://developers.openai.com/codex/integrations/github
- Codex app PR review pane: https://developers.openai.com/codex/app/review
- Codex CLI local review presets: https://developers.openai.com/codex/cli/features
- Codex subagents: https://developers.openai.com/codex/subagents
- Codex app settings: https://developers.openai.com/codex/app/settings

## Product Position

safe-agentic should not clone the Codex app. Its job is to keep the stronger local security model while borrowing the Codex app UX primitives that make agents easier to steer.

| Capability | Codex app | safe-agentic target |
|---|---|---|
| Thread history | searchable threads | searchable agent session logs and exports |
| Review pane | inline comments, stage/revert | TUI/CLI review inbox, file/hunk actions |
| Worktrees | managed worktrees and handoff | VM-managed worktrees plus local handoff |
| Automations | scheduled runs and triage inbox | cron-backed runs plus action inbox |
| Local actions | setup/actions buttons | `.safe-ag/actions.toml` commands |
| Browser | in-app preview, comments, browser use | container/VM browser companion |
| Subagents | built-in subagent orchestration | fleet/pipeline roles with read/write policies |
| Settings | app settings UI | config commands plus TUI settings |
| Cloud delegation | background cloud tasks | background containers, fleets, pipelines, profiles |
| Client integrations | app/IDE/cloud surfaces | stdio/HTTP JSON protocol |

## Phase 1: Daily Control Surface

Goal: reduce log digging and command memorization.

- Add `safe-ag action` to list/show/run project or user actions from `actions.toml`.
- Add `safe-ag search` to find prior agent output across live and stopped containers.
- Add TUI command `:action <name>` to run an action in the selected agent.
- Document action schema and search workflow.

Status: implemented in this branch.

## Phase 2: Review Experience

Goal: match the useful parts of the Codex review pane without weakening isolation.

- Add `safe-ag review-comments` storage for file/line comments.
- Add `safe-ag steer <agent> <message>` to send follow-up text into tmux-backed sessions.
- Add TUI review overlay with files and comment entry.
- Add stage/revert file operations with confirmation prompts.
- Add patch-based hunk stage/revert for selected diff hunks.

Status: implemented in this branch (`safe-ag steer`, `safe-ag review-comments`, TUI `:comments`, `safe-ag workspace stage|unstage|revert`, and patch-based `stage-patch|revert-patch` landed).

Validation:
- unit tests for comment persistence and shell quoting
- integration smoke against a disposable git repo in a container
- TUI tests for overlay navigation and confirmation gates

## Phase 3: Managed Worktrees

Goal: isolate code changes like Codex app Worktree mode while preserving VM/container boundaries.

- Add `safe-ag spawn --worktree` to create a host or VM worktree before container launch.
- Add `.safe-aginclude` for ignored local files that must be copied into the worktree.
- Add `safe-ag handoff <agent> --to-local|--to-worktree`.
- Add cleanup and snapshot restore for managed worktrees.

Status: implemented in this branch (`safe-ag spawn --worktree`, `.safe-aginclude` copying, bind-mounted `/workspace`, `safe-ag handoff --to-local|--to-worktree`, `safe-ag worktree cleanup`, and `safe-ag worktree snapshot/restore` landed).

Validation:
- worktree branch conflict tests
- ignored-file copy tests with symlink protections
- cleanup keeps pinned/in-progress worktrees

## Phase 4: Inbox And Timeline

Goal: make long-running and scheduled agent work observable.

- Add structured timeline events for spawn, setup, prompt, command, file changes, tests, PRs, callbacks.
- Add `safe-ag inbox` for cron/pipeline findings.
- Add statuses: stuck, needs-auth, failed-tests, ready-for-review, ready-for-pr.
- Add TUI inbox pane.

Status: implemented in this branch (`safe-ag timeline`, `safe-ag inbox`, TUI `:timeline`, TUI `:inbox`, cron/stop/cleanup event emission, and status classification landed).

Validation:
- event schema tests
- classifier fixture tests
- cron/inbox integration tests

## Phase 5: Browser Companion

Goal: bring visual app verification into the safe-agentic boundary.

- Add browser sidecar or headless browser runner inside VM/container network.
- Add screenshot, console, network, DOM snapshot capture.
- Add annotations stored as artifacts and feedable to agents.
- Keep signed-in host browser profiles out of scope by default.

Status: implemented in this branch (`safe-ag browser capture --mode auto|http|chrome` writes DOM, headers, screenshot, console, network, annotations, and artifact metadata when Chrome/CDP is available, with HTTP fallback and no host browser profile/cookies).

Validation:
- local static page screenshot test
- dev-server smoke test
- no host cookie/profile mount by default

## Phase 6: Agent Profiles And Policy

Goal: make repeated roles one command and keep dangerous capabilities controlled.

- Add `~/.safe-ag/agents/*.toml` custom profiles.
- Add read-only reviewer and write-enabled fixer policies.
- Add `~/.safe-ag/rules.toml` for allowed commands, networks, mounts, AWS profiles, and Docker modes.
- Add fleet role defaults that map to profiles.

Status: implemented in this branch (`safe-ag profile list/show/run`, TUI `:profile`, spawn-time `rules.toml` policy, and manifest `profile:` inheritance landed).

Validation:
- profile merge tests
- policy deny/allow tests
- fleet profile inheritance tests

## Phase 7: Rich Client Protocol

Goal: allow future desktop/IDE clients without coupling them to shell parsing.

- Add `safe-ag server` JSON-RPC over stdio or localhost socket.
- Expose agent events, table state, logs, diffs, and action results.
- Keep auth local; require capability token for non-stdio transports.

Status: implemented in this branch (`safe-ag server --stdio` and token-gated `--listen` landed for `schema`, `ping`, `timeline`, `inbox`, `actions.list`, `actions.run`, `agents.list`, `agent.logs`, and `agent.diff`).

Validation:
- JSON schema generation
- protocol conformance tests
- auth handshake tests for socket mode
