# berth-app — macOS Desktop App for berth (Design)

Date: 2026-07-03
Status: Approved design, pre-implementation
Owner: florian (personal power tool)

## 1. Context

berth has a deep engine (isolated per-agent containers in an Apple VM, fleet/pipeline
orchestration, cost/budget kill, audit, checkpoints, replay) but only a terminal TUI as frontend.
Supacode (v0.10.5, benchmarked 2026-07-03 from the installed app + public docs/changelog)
proves the desktop UX pattern for agent orchestration: embedded terminals, presence badges,
"needs-you" state, rich notifications carrying the agent's last message, command palette.
Supacode is free and open source (github.com/supabitapp/supacode, native Swift + libghostty)
— its source is a usable reference for UX/implementation patterns (agent presence via
OSC 3008, zmx session persistence, hook auto-install) even though our stack differs.

Gap analysis conclusion: Supacode is a thin UX layer over host-run (unisolated) agents;
berth is a deep engine with no GUI. This app puts a Supacode-grade front on the
berth engine. Not a competitor product — a personal tool.

### Supacode features adopted

- Sidebar of live agents with status dots and grouping
- Embedded terminal per agent, splits, session persistence across app restarts
- "Needs-you" as a distinct attention state (not just busy/idle)
- macOS notifications whose body is the agent's last message; click focuses the agent
- Menubar (systray) status counts with per-agent dropdown
- Command palette (⌘K)
- Spawn form with dry-run preview

### Supacode features intentionally NOT adopted (v1)

- Host git-worktree management UI (berth manages worktrees; engine-side)
- Deeplink URL scheme, onboarding cards, themes gallery, auto-update, telemetry
- Remote/SSH hosts, multi-repo layout persistence
- Ghostty embedding (infeasible in a WKWebView; xterm.js chosen)

### berth advantages surfaced in the app (Supacode has none of these)

- Fleet/pipeline DAG view with judge verdicts and per-stage retry
- Cost dashboard and per-agent live cost; budget kill already engine-side
- Diff/checkpoint/stage/revert review workflow; retry-with-feedback; steer
- Audit log, timeline/inbox classification, templates/profiles

## 2. Requirements

- Personal power tool. macOS only. Single user, loopback only, no auth model.
- Scope: berth containers only (no host sessions, no other backends).
- Four pillars: embedded terminals, fleet/pipeline dashboard, diff/review UI,
  menubar + native notifications.
- Must not duplicate engine logic: berth CLI remains the single source of truth
  for mutations.

## 3. Architecture

Stack: **Wails v3 (alpha)** — Go backend + WKWebView frontend, single binary.
v3 chosen over v2 for built-in systray (menubar pillar) and multi-window later.
Risk accepted: v3 is alpha; API "reasonably stable", production apps exist.

Location: `app/` directory in the berth repo, mirroring `tui/`:
own `package main`, wired via `go.work`, own Makefile (`make -C app build`).

```
┌────────────────────────── Wails v3 app (single Go binary) ──────────────────────────┐
│  Go backend                                    │  Web frontend (WKWebView)          │
│  ├─ Poller        docker ps labels+stats, 2s   │  React + TS + Vite + Tailwind      │
│  │                (port of tui/poller.go)      │  ├─ Sidebar: agents by status      │
│  ├─ EventWatcher  fsnotify events.jsonl/audit  │  ├─ Agent tabs: Terminal(xterm.js) │
│  │                → classifier → notify        │  │   Diff / Output / Info          │
│  ├─ TermManager   PTY: container machine run   │  ├─ Fleet/pipeline DAG view        │
│  │                … docker exec tmux attach    │  ├─ Spawn form, Timeline, Cost     │
│  ├─ CLIRunner     shell berth (mutations)    │  └─ Command palette (⌘K)           │
│  ├─ StateReader   import pkg/{audit,events,    │                                    │
│  │                fleet,cost,labels,config}    │  transport: Wails bindings (calls) │
│  └─ Systray + Notifier (Wails v3 native)       │  + Wails events (push) + PTY chan  │
└─────────────────────────────────────────────────────────────────────────────────────┘
          │ reads                     │ mutations                 │ terminals
    ~/.berth/state/*.jsonl     berth CLI (JSON out)     PTY → tmux in container
    docker labels via vmexec     spawn/stop/steer/retry/…   (persists across restarts)
```

Key decisions:

- **Hybrid data access.** Reads: import `pkg/{audit,events,fleet,cost,labels,config}`
  directly + Docker labels via `pkg/vmexec`. Writes: shell out to `berth`
  (JSON output where available). No engine logic duplicated in the app.
- **No dependency on `berth server`** in v1. It is read-only and unused by the
  TUI today. Extending it (write methods + SSE) is future work, not a blocker.
- **Push to UI, poll the engine.** Backend polls Docker every 2s (as the TUI does)
  and watches state files; the frontend receives diffed Wails events only.
- **tmux is the persistence layer** (Supacode's zmx equivalent, already in every
  container). App restart ⇒ reattach; nothing is lost.
- Terminal: **xterm.js only** (WebGL renderer addon). Ghostty cannot render inside
  a WKWebView and libghostty has no embeddable renderer API (only libghostty-vt,
  alpha). Decision: no external-terminal escape hatch in v1.

## 4. Components

### Backend (Go)

| Component | Responsibility | Source of truth |
|---|---|---|
| Poller | 2s snapshot: `docker ps -a` (berth labels), `docker stats`, CPU probe → Working/Idle; diff vs last snapshot → events | port of `tui/poller.go` |
| EventWatcher | fsnotify on `~/.berth/state/events.jsonl`, `state/pipelines/`; classify via `pkg/events` classifier; dispatch notifications + UI events | `pkg/events` |
| TermManager | one goroutine per open terminal: `creack/pty` running `container machine run berth docker exec -it <c> tmux attach`; output → per-terminal Wails event stream (base64, ~16ms coalescing); input/resize ← bound methods; close = detach | tmux in container |
| CLIRunner | exec `berth <cmd>`; parse JSON (`list --json`, `output --json`) or exit code; every invocation logged with full argv | berth CLI |
| StateReader | direct reads: audit slice, cost compute, fleet/pipeline manifests + state, templates/profiles/actions catalogs, config.toml | `pkg/*` |
| Systray/Notifier | menubar icon + counts; macOS notifications (body = last agent message; click focuses agent) | Wails v3 systray + notifications |

### Frontend (React + TS + Vite + Tailwind, Zustand store)

- **Sidebar** — groups: running fleets/pipelines → standalone agents → stopped.
  Status dots: working / idle / needs-you (`needs-auth`/`stuck`) / ready-for-review / failed.
  Live cost under each name. Context menu: stop, retry, checkpoint, PR, export.
- **Agent workspace** (tabs per agent):
  1. *Terminal* — xterm.js (+fit, webgl, search addons). Side-by-side split, 2 panes max.
  2. *Diff* — diff2html rendering of `berth diff`; file tree; stage/revert via
     `workspace stage|revert`; checkpoint create/restore.
  3. *Output* — from `output --json`: last message, changed files, commits.
     Actions: PR, review, retry-with-feedback (textarea → `retry --feedback`),
     steer (input → `steer`).
  4. *Info* — labels, resources, audit slice, cost breakdown, todos, review-comments.
- **Fleet/pipeline view** — static DAG (boxes + arrows, no graph library), per-stage
  agent chips with status, judge verdicts, retry-failed-stage button.
- **Spawn form** — agent type, repo(s), prompt, template picker, profile picker,
  toggles (`--ssh`, `--reuse-auth`, `--worktree`, `--network`), resources;
  dry-run preview shows the generated command.
- **Command palette (⌘K)** — fuzzy over agents, `action list` actions, and verbs
  (spawn, stop-all, cleanup, vm start, diagnose).
- **Timeline/Inbox** — feed from `timeline`/`inbox` data.
- **Cost dashboard** — `cost --history`, per-model breakdown.

## 5. Data flow

- **Poll loop:** 2s ticker → label snapshot via `pkg/vmexec` → diff → emit
  `agent.added|updated|removed` Wails events only on change. Frontend store is
  event-fed; zero frontend polling.
- **File watch:** fsnotify on events.jsonl + pipelines state → classifier state
  changes → notification + store update. Fallback: 5s mtime poll (fsnotify
  unreliability on some volumes).
- **Mutations:** UI → bound Go method → `berth` exec → parse → optimistic store
  update + forced re-poll.
- **Terminal:** PTY output streamed per-terminal; keystrokes and resize via bound
  methods; closing the tab detaches (agent keeps running under tmux).

## 6. Error handling

- VM unreachable → full-width banner + one-click `berth vm start` streamed into a
  modal; UI degrades to last snapshot, greyed.
- CLI failure → toast with stderr tail, expandable to full output. Exit code ≠ 0 is
  always surfaced, never swallowed.
- PTY death → "disconnected — reattach" button; tmux makes reattach lossless.
- Missing/corrupt state files → treated as empty + "state unavailable" badge; no crash.

## 7. Testing

- Go: Poller and CLIRunner tested against `vmexec.FakeExecutor` (existing repo
  pattern); golden-file tests for label parsing and `output --json` parsing;
  classifier→notification table tests.
- Frontend: Zustand store reducer unit tests only.
- Smoke: `make -C app build` in CI; `wails doctor` documented in app README.

## 8. v1 cut line (explicit non-goals)

No multi-window. No remote hosts. No auth. No settings UI beyond reading
config.toml. Terminal split capped at 2 panes. DAG view static (no zoom/pan).
No auto-update, no telemetry, no packaging/notarization pipeline (run via
`wails dev` / local build).

## 9. Risks

- **Wails v3 alpha** — API churn possible; pin a nightly/tag, upgrade deliberately.
- **PTY through `container machine run`** — extra process layer between app and
  tmux; must verify resize + UTF-8 + control sequences survive the relay early
  (spike task #1 of the implementation plan).
- **fsnotify on `~/.berth/state`** — JSONL appends may coalesce; mtime fallback
  covers it.
- **Cost table dated** (`pkg/cost/pricing.go`) — engine-side fix, out of scope here;
  dashboard displays whatever the engine computes.

## 10. Future work (post-v1, noted not planned)

- Extend `berth server` (write methods + SSE) and migrate app + TUI onto it.
- Fix `logs --follow` engine-side; host-side session/replay cache.
- Menubar-only mini mode; multi-window; packaging/notarization.
