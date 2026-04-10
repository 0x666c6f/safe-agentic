# Design Document: safe-agentic v2 Improvements

**Date:** 2026-04-10
**Status:** Draft
**Author:** Market study synthesis (40+ tools analyzed across 6 categories)

---

## Context

The agentic orchestration market has matured rapidly in Q1 2026. Key inflection points:

- **Claude Managed Agents** launched April 8, 2026 ($0.08/hr + API costs, cloud-hosted)
- **Cursor** shipped self-hosted cloud agents with browser access and video recording
- **E2B/Daytona** offer sub-100ms microVM sandboxes with SDK-first APIs
- **Codex CLI** ships sandbox-on by default; Cursor GA'd OS-level sandboxes
- **A2A protocol** (Linux Foundation) has 150+ organizations; MCP is ubiquitous
- Enterprise orgs went from 15-30 agents (mid-2025) to 50-200+ (early 2026)

safe-agentic's triple isolation boundary (VM + container + network) remains genuinely unique — Anthropic's own Auto Mode has a 17% false negative rate on dangerous actions, and 32% of devs using `--dangerously-skip-permissions` experienced unintended file modifications. Container-level isolation beats classifier-based permissions.

But the UX gap is widening. Competitors offer SDK APIs, web dashboards, sub-100ms startup, snapshot/fork, and webhook-driven async workflows. This document proposes improvements to close those gaps while preserving safe-agentic's security-first identity.

---

## Guiding Principles

1. **Security stays default-on.** New features must not weaken the isolation model.
2. **Local-first, open-source.** Cloud is optional, never required.
3. **Progressive disclosure.** Simple things simple, complex things possible.
4. **Multi-vendor.** Claude + Codex today, extensible to others.
5. **Composable.** Each feature works standalone; features compose for power users.

---

## Phase 1: UX Simplification (Low effort, High impact)

### 1.1 One-Command Quick Start

**Problem:** The common case requires too many flags:
```bash
# Today (7 tokens to parse)
agent spawn claude --ssh --repo git@github.com:org/repo.git --prompt "Fix tests"

# Competitor (Devin): assign a ticket, done
# Competitor (GitHub Copilot Agent): add emoji to issue, done
```

**Proposal:** Add `safe-ag run` as a high-level command with smart defaults:
```bash
# Minimal — auto-detect agent type, auto-enable SSH for git@ URLs
safe-ag run git@github.com:org/repo.git "Fix the failing tests"

# With explicit agent type
safe-ag run --codex git@github.com:org/repo.git "Fix the failing tests"

# Multiple repos
safe-ag run git@github.com:org/api.git git@github.com:org/shared.git "Update shared types"
```

**Behavior:**
- Default agent type: `claude` (configurable via `agent config set default_agent claude`)
- Auto-enable `--ssh` when URL starts with `git@` or `ssh://` (already done in agent-claude/agent-codex)
- Auto-generate name from repo + timestamp (already done)
- Auto-enable `--reuse-auth` if a shared auth volume exists (new default, see 1.2)
- Prompt is the last positional argument (no `--prompt` flag needed)

**Implementation:** New `cmd_run()` function in `bin/agent` that delegates to `cmd_spawn()` with inferred flags. ~50 lines.

**Command hierarchy after this change:**
```
safe-ag run        # Simple mode (smart defaults, minimal flags)
safe-ag spawn      # Power mode (all flags, full control)
safe-ag fleet      # Orchestration mode (YAML manifests)
safe-ag pipeline   # Sequential orchestration (YAML pipelines)
```

### 1.2 Auth Default Flip

**Problem:** Per-session ephemeral auth is secure but creates friction for the 90% case where users re-authenticate repeatedly. Competitors (Cursor, Devin) persist auth by default.

**Proposal:** Make `--reuse-auth` the default. Add `--ephemeral-auth` for the paranoid case.

**Behavior:**
- First spawn: auth flow runs, token stored in `agent-claude-auth` / `agent-codex-auth` volume
- Subsequent spawns: reuse volume automatically
- `safe-ag run --ephemeral-auth` for one-off isolated sessions
- `safe-ag cleanup --auth` to revoke (unchanged)
- `SAFE_AGENTIC_DEFAULT_REUSE_AUTH=true` becomes the default in new installs

**Security note:** This trades convenience for a slightly larger blast radius if a container is compromised (attacker gets a reusable token). Acceptable because: (a) the token is already inside a hardened VM with userns-remap, (b) competitors all do this, (c) `--ephemeral-auth` remains available.

### 1.3 Git Identity Auto-Detection

**Problem:** Default identity is `Agent <agent@localhost>` — commits look anonymous unless the user remembers `--identity`. Aider's auto-commit with proper attribution is the gold standard.

**Proposal:** Auto-detect from host git config:
```bash
# In cmd_spawn / cmd_run, before container creation:
if [[ -z "$identity" ]]; then
    local git_name git_email
    git_name=$(git config --global user.name 2>/dev/null || echo "")
    git_email=$(git config --global user.email 2>/dev/null || echo "")
    if [[ -n "$git_name" && -n "$git_email" ]]; then
        identity="$git_name <$git_email>"
    fi
fi
```

**Implementation:** ~10 lines in `bin/agent`. No breaking changes.

---

## Phase 2: Async Agent Lifecycle (Medium effort, High impact)

### 2.1 Event System & Webhooks

**Problem:** `--on-exit` is the only lifecycle callback. Competitors offer rich event systems:
- Claude Code: 24 hook events
- Cursor: real-time agent status in Agents Window
- Devin: Slack notifications on completion
- GitHub Copilot Agent: GitHub Actions integration

**Proposal:** Add an event bus with pluggable sinks.

**Events:**
| Event | Fires when | Payload |
|-------|-----------|---------|
| `agent.spawned` | Container starts | name, type, repo, config |
| `agent.ready` | Agent CLI initialized | name, model |
| `agent.idle` | No CPU activity for 30s | name, duration |
| `agent.active` | CPU activity resumes | name |
| `agent.completed` | Agent exits cleanly (exit 0) | name, duration, cost_estimate, changed_files |
| `agent.failed` | Agent exits non-zero | name, exit_code, last_output |
| `agent.checkpoint` | Checkpoint created | name, ref, label |
| `agent.budget_exceeded` | Cost estimate exceeds --max-cost | name, estimated_cost, budget |

**Sinks:**
```bash
# CLI flags (simple)
safe-ag run --on-complete "slack-notify #dev" --on-fail "pagerduty trigger"

# Config file (advanced)
# ~/.config/safe-agentic/events.yaml
sinks:
  - type: command
    events: [agent.completed, agent.failed]
    command: "notify-send 'Agent {{.Name}}' '{{.Event}}'"

  - type: webhook
    events: [agent.completed]
    url: https://hooks.slack.com/services/xxx
    method: POST
    body: |
      {"text": "Agent {{.Name}} finished on {{.Repo}}. Cost: ~${{.CostEstimate}}"}

  - type: file
    events: ["*"]
    path: ~/.config/safe-agentic/events.jsonl
```

**Implementation approach:**
- Events emitted as JSONL to a file inside the container at `/workspace/.safe-agentic/events.jsonl`
- Host-side event watcher (background process started by `cmd_spawn`) polls this file via `docker exec` every 5 seconds and dispatches to configured sinks
- Reuse audit log infrastructure — events are a superset of audit entries
- `--on-complete` and `--on-fail` are sugar for the most common event-sink pair

**Backward compatibility:** `--on-exit` continues to work, mapped to both `agent.completed` and `agent.failed`.

### 2.2 Budget Enforcement

**Problem:** `--max-cost` exists as a label but enforcement is unclear. A single misconfigured looping agent can burn thousands before anyone notices. Industry consensus is: stop agents on budget breach, don't just alert.

**Proposal:** Active cost monitoring with kill switch.

**Mechanism:**
1. Agent session writes token counts to `/workspace/.safe-agentic/session.jsonl` (already exists)
2. Host-side monitor (same watcher as 2.1) polls this file every 10 seconds
3. Computes running cost estimate using model pricing table
4. If estimate exceeds `--max-cost`, sends SIGTERM to agent process, waits 5s, then SIGKILL
5. Emits `agent.budget_exceeded` event before killing
6. Container preserved (not removed) so user can inspect state

```bash
safe-ag run --max-cost 5.00 git@github.com:org/repo.git "Refactor auth module"
# Agent killed if estimated spend exceeds $5.00
```

**Pricing table:** Already exists in `cmd_cost()` — extract to shared config:
```bash
# ~/.config/safe-agentic/pricing.sh
PRICING_CLAUDE_OPUS_INPUT=15.00    # per 1M tokens
PRICING_CLAUDE_OPUS_OUTPUT=75.00
PRICING_CLAUDE_SONNET_INPUT=3.00
PRICING_CLAUDE_SONNET_OUTPUT=15.00
PRICING_CODEX_INPUT=2.00
PRICING_CODEX_OUTPUT=8.00
```

**Fleet integration:** `max_cost` field in fleet manifests applies per-agent.

### 2.3 Notification Integration

**Problem:** After spawning a background agent, the only way to know it finished is `agent list` or `agent peek`.

**Proposal:** Built-in notification sinks for common targets.

```bash
# macOS native notification (default for foreground terminal)
safe-ag run --notify terminal ...

# Slack webhook
safe-ag config set notify_slack_webhook https://hooks.slack.com/services/xxx
safe-ag run --notify slack ...

# Custom command
safe-ag run --notify "my-notify-script" ...

# Multiple
safe-ag run --notify terminal,slack ...
```

**Implementation:** Thin wrapper around the event system (2.1). `--notify X` is sugar for subscribing to `agent.completed` and `agent.failed` with the named sink.

---

## Phase 3: State Management (Medium effort, High impact)

### 3.1 Container State Snapshots

**Problem:** Current checkpoints use git stash refs — they capture code state but not container state (installed packages, environment, running processes). Competitors offer full environment snapshots:
- Daytona: pause/fork/snapshot/resume
- Fly.io Sprites: 300ms checkpoint/restore
- E2B: filesystem + memory snapshots

**Proposal:** Docker commit-based snapshots that capture the full container filesystem.

```bash
# Save full container state (git state + installed packages + config)
safe-ag checkpoint save my-agent "after-dependency-install"

# List checkpoints (git refs + container snapshots)
safe-ag checkpoint list my-agent

# Restore to a checkpoint (resets container to snapshot, restores git state)
safe-ag checkpoint restore my-agent cp-20260410-143022

# Fork: create a new agent from a checkpoint (explore alternate approach)
safe-ag checkpoint fork my-agent new-approach "try-redis-instead"
```

**Implementation:**
```bash
cmd_checkpoint_save() {
    local name="$1" label="$2"
    local container_id timestamp snapshot_tag

    # 1. Pause the agent process (SIGSTOP on tmux server, which pauses the agent)
    vm_exec docker exec "$name" tmux send-keys -t agent "" # no-op to verify session
    vm_exec docker exec "$name" kill -STOP "$(vm_exec docker exec "$name" tmux list-panes -t agent -F '#{pane_pid}')"

    # 2. Git checkpoint (existing)
    vm_exec docker exec "$name" git -C /workspace/... stash create

    # 3. Docker commit (new)
    timestamp=$(date +%Y%m%d-%H%M%S)
    snapshot_tag="safe-agentic-checkpoint:${name}-${timestamp}"
    vm_exec docker commit "$name" "$snapshot_tag"

    # 4. Store metadata
    vm_exec docker exec "$name" cat >> /workspace/.safe-agentic/checkpoints.jsonl <<EOF
    {"timestamp":"$timestamp","label":"$label","image":"$snapshot_tag","git_ref":"$git_ref"}
    EOF

    # 5. Resume agent (SIGCONT)
    vm_exec docker exec "$name" kill -CONT 1
}
```

**Fork implementation:** `docker run` from the snapshot image with a new container name and network. The forked agent starts from the exact state of the parent.

**Storage:** Snapshots stored as Docker images in the VM's local registry. `safe-ag cleanup --snapshots` removes old ones.

**Limitations:**
- Docker commit captures filesystem but not running processes — agent restarts from entrypoint on restore
- Snapshot size depends on container filesystem changes (typically 100MB-1GB)
- Not as fast as Fly.io Sprites (seconds vs 300ms) but adequate for the use case

### 3.2 Session Event Log (Anthropic-Style)

**Problem:** Current session persistence relies on tmux session state — if the container crashes, context is lost. Anthropic's Managed Agents use an append-only event log as the source of truth: if a harness crashes, a new one reboots and restores from the log.

**Proposal:** Structured event log as the canonical session record.

**Format:** `/workspace/.safe-agentic/session-events.jsonl`
```jsonl
{"ts":"2026-04-10T14:30:00Z","type":"session.start","agent":"claude","model":"opus-4-6","repo":"org/repo"}
{"ts":"2026-04-10T14:30:05Z","type":"tool.call","tool":"Read","args":{"file":"src/auth.py"},"tokens":{"in":150,"out":0}}
{"ts":"2026-04-10T14:30:10Z","type":"tool.call","tool":"Edit","args":{"file":"src/auth.py"},"tokens":{"in":200,"out":500}}
{"ts":"2026-04-10T14:30:15Z","type":"git.commit","sha":"abc1234","message":"fix: handle null token"}
{"ts":"2026-04-10T14:31:00Z","type":"agent.message","content":"I've fixed the auth bug...","tokens":{"in":0,"out":150}}
{"ts":"2026-04-10T14:35:00Z","type":"session.end","exit_code":0,"total_tokens":{"in":5000,"out":2000},"cost_estimate":0.23}
```

**Benefits:**
- Cost tracking becomes exact (not estimated from polling)
- `agent output` reads structured data instead of parsing tmux pane
- `agent sessions` exports a clean, machine-readable record
- Session replay becomes possible (see Phase 5)
- Container crashes don't lose session history

**Implementation:** Claude Code already writes to `~/.claude/projects/*/` JSONL files. Codex writes similar logs. The entrypoint wraps the agent launch to tee/transform these into the unified format.

---

## Phase 4: Fleet & Communication (Medium effort, High impact)

### 4.1 Agent-to-Agent Communication

**Problem:** Fleet agents currently work in complete isolation. The only coordination is through git (push to branches, let another agent pull). Competitors offer:
- Claude Agent Teams: shared task list + peer-to-peer messaging
- CrewAI: role-based delegation
- A2A protocol: standardized agent discovery and task management

**Proposal:** Shared task list via a mounted volume, inspired by Claude Agent Teams.

**Architecture:**
```
┌─────────────────────────────────────────────────┐
│  Shared Volume: /fleet/<fleet-id>/              │
│  ├── tasks.jsonl        (append-only task list) │
│  ├── messages.jsonl     (agent-to-agent msgs)   │
│  └── agents.json        (registered agents)     │
├─────────────────────────────────────────────────┤
│  Container A            Container B             │
│  (backend agent)        (frontend agent)        │
│  mounts /fleet/...      mounts /fleet/...       │
│  reads/writes tasks     reads/writes tasks      │
└─────────────────────────────────────────────────┘
```

**Task format:**
```jsonl
{"id":"t1","status":"pending","assignee":null,"title":"Fix API auth","depends_on":[],"created_by":"lead"}
{"id":"t2","status":"in_progress","assignee":"backend-agent","title":"Update user model","depends_on":["t1"],"created_by":"lead"}
{"id":"t3","status":"completed","assignee":"frontend-agent","title":"Update login form","depends_on":[],"created_by":"lead"}
```

**Fleet manifest integration:**
```yaml
# fleet-manifest.yaml
name: auth-refactor
shared_tasks: true          # Enable shared task volume
agents:
  - name: backend
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    prompt: |
      You are the backend agent. Check /fleet/tasks.jsonl for your tasks.
      Mark tasks as in_progress when you start, completed when done.
      Write findings to /fleet/messages.jsonl for other agents.

  - name: frontend
    type: claude
    repo: git@github.com:org/web.git
    ssh: true
    prompt: |
      You are the frontend agent. Check /fleet/tasks.jsonl for your tasks.
      Read /fleet/messages.jsonl for context from other agents.

  - name: tests
    type: codex
    repo: git@github.com:org/api.git
    ssh: true
    prompt: |
      You write tests. Wait for backend tasks to complete before starting.
```

**Implementation:**
- Create a Docker volume `fleet-<id>` mounted read-write at `/fleet/` in all fleet containers
- Inject task list context into agent preamble (security-preamble.md already has `{{PLACEHOLDER}}` tokens)
- File locking via `flock` for concurrent writes to tasks.jsonl
- The lead agent (first in manifest, or explicit `lead: true`) creates initial tasks
- `safe-ag fleet status <manifest>` shows task progress across all agents
- Shared volume lifecycle: created on `safe-ag fleet`, persists until all fleet agents are stopped, then cleaned up by `safe-ag cleanup` or `safe-ag fleet stop <manifest>`

**Future (A2A):** The shared volume is a stepping stone. A proper A2A implementation would use HTTP/SSE between containers on the same Docker network, enabling real-time messaging. The task list format is designed to be compatible with A2A's task management primitives.

### 4.2 Pipeline Conditional Branching

**Problem:** Current pipeline `on_failure` only supports jumping to a named step. No conditional branching based on agent output.

**Proposal:** Add output-based routing.

```yaml
name: test-and-fix
steps:
  - name: run-tests
    type: claude
    repo: ...
    prompt: "Run the test suite. Write results to /workspace/test-results.json"
    outputs:
      test_passed: "jq -r '.passed' /workspace/test-results.json"

  - name: celebrate
    depends_on: run-tests
    when: "{{ steps.run-tests.outputs.test_passed == 'true' }}"
    type: shell
    command: "echo 'All tests pass!'"

  - name: fix-tests
    depends_on: run-tests
    when: "{{ steps.run-tests.outputs.test_passed == 'false' }}"
    type: claude
    repo: ...
    prompt: "Fix the failing tests. Results: {{ steps.run-tests.outputs }}"
    retry: 2
```

**Implementation:**
- After each step completes, evaluate `outputs` expressions by exec'ing commands inside the container
- Store outputs in a step context map
- Evaluate `when` conditions using simple string comparison (no full expression engine)
- Pass outputs to subsequent step prompts via template substitution

---

## Phase 5: Observability (Medium effort, Medium-High impact)

### 5.1 Web Dashboard

**Problem:** CLI-only management breaks down at 10+ agents. The TUI helps but isn't shareable, can't be bookmarked, and doesn't support remote access. Every major competitor offers a web view:
- Devin: full web IDE with terminal + browser
- Cursor: Agents Window in IDE
- GitHub Copilot: native GitHub web UI
- CrewAI: Studio dashboard

**Proposal:** Lightweight web dashboard served by a Go binary (reuse TUI's Go codebase).

**Architecture:**
```
┌─────────────────────────────┐
│  safe-ag dashboard          │
│  (Go binary, port 8420)     │
│                             │
│  ├── GET /                  │  → Agent grid (status, cost, duration)
│  ├── GET /agents/:name      │  → Agent detail (logs, diff, output)
│  ├── GET /agents/:name/logs │  → SSE stream of agent output
│  ├── GET /fleet/:id         │  → Fleet view (task progress)
│  ├── GET /events            │  → SSE stream of all events
│  └── POST /agents/:name/stop│  → Stop agent
└─────────────────────────────┘
```

**Tech stack:**
- Go (share code with `tui/` binary — Docker client, stat collection, agent management)
- HTML + SSE (server-sent events for live streaming — no JS framework needed)
- Single binary, zero dependencies, no build step
- `safe-ag dashboard` starts on `localhost:8420`
- Optional `--bind 0.0.0.0` for remote access (with basic auth token)

**Views:**

**Agent Grid** (home page):
```
┌──────────────────┬──────────┬───────┬──────────┬──────────┐
│ Name             │ Status   │ Type  │ Duration │ Est Cost │
├──────────────────┼──────────┼───────┼──────────┼──────────┤
│ backend-refactor │ ● Active │ claude│ 12m      │ $0.45    │
│ test-writer      │ ◐ Idle   │ codex │ 8m       │ $0.12    │
│ frontend-fix     │ ✓ Done   │ claude│ 23m      │ $1.20    │
│ docs-update      │ ✗ Failed │ claude│ 5m       │ $0.08    │
└──────────────────┴──────────┴───────┴──────────┴──────────┘
```

**Agent Detail** (click into agent):
- Live terminal output (SSE-streamed from `agent peek`)
- Git diff viewer (from `agent diff`)
- Changed files list
- Cost breakdown (per-model token usage)
- Actions: Stop, Retry, Checkpoint, Create PR

**Fleet View** (for fleet/pipeline runs):
- Task progress table (pending/in-progress/completed)
- Agent assignment visualization
- Pipeline step dependency graph (simple DAG)

**Implementation phases:**
1. Static HTML served by Go HTTP server with Docker client for agent data (1-2 days)
2. SSE streaming for live logs (1 day)
3. Action endpoints (stop, retry) (1 day)
4. Fleet/pipeline views (2-3 days)

### 5.2 Session Replay

**Problem:** When an agent fails or produces unexpected results, the only diagnostic is `agent output` (last message) or `agent peek` (last N terminal lines). AgentOps offers time-travel debugging — frame-by-frame replay of agent sessions.

**Proposal:** Replay agent sessions from the event log (3.2).

```bash
# Replay a session step by step
safe-ag replay my-agent

# Show only tool calls
safe-ag replay my-agent --tools-only

# Show cost accumulation over time
safe-ag replay my-agent --cost-timeline

# Export as HTML for sharing
safe-ag replay my-agent --html > session-report.html
```

**TUI replay mode:**
```
┌─ Session Replay: backend-refactor ──────────────────────┐
│ [14:30:00] Session started (claude opus-4-6)            │
│ [14:30:05] Read src/auth.py (150 tokens)                │
│ [14:30:10] Edit src/auth.py (+12/-3 lines, 700 tokens)  │
│ [14:30:15] Bash: npm test (exit 0)                      │
│ [14:30:20] Git commit: "fix: handle null token"         │
│ [14:31:00] Agent message: "I've fixed the auth bug..."  │
│ [14:35:00] Session ended (exit 0)                       │
├─────────────────────────────────────────────────────────┤
│ Total: 5,000 in / 2,000 out tokens │ Cost: ~$0.23      │
│ Duration: 5m │ Files changed: 1 │ Commits: 1            │
│ ← → navigate │ q quit │ d show diff │ f show full msg   │
└─────────────────────────────────────────────────────────┘
```

**Web dashboard integration:** The HTML export becomes a page in the web dashboard at `/agents/:name/replay`.

**Depends on:** Phase 3.2 (Session Event Log).

### 5.3 Integrated Cost Dashboard

**Problem:** `agent cost` shows per-agent cost but there's no fleet-level or historical view. No trend analysis, no budget tracking across sessions.

**Proposal:** Cost tracking in the audit log + dashboard visualization.

**Data model:**
```jsonl
# Appended to audit.jsonl on agent completion
{"timestamp":"...","action":"cost","container":"backend-refactor","cost":0.23,"tokens":{"in":5000,"out":2000},"model":"opus-4-6","duration_s":300}
```

**CLI:**
```bash
# Per-agent (existing, enhanced)
safe-ag cost my-agent

# Fleet total
safe-ag cost --fleet auth-refactor

# Historical (last 7 days)
safe-ag cost --history 7d

# Budget report (spend vs budget across all agents)
safe-ag cost --budget-report
```

**Dashboard widget:** Cost-over-time chart on the web dashboard home page. Per-agent cost bars in the agent grid.

---

## Phase 6: SDK & API Layer (High effort, High impact)

### 6.1 Go CLI Rewrite

**Problem:** `bin/agent` is 4,391 lines of bash. This limits:
- Proper argument parsing with generated help
- Parallel execution without subshell hacks
- Type safety and testability
- Plugin/extension architecture
- The web dashboard shares no code with the CLI

**Proposal:** Rewrite the CLI in Go, sharing code with the TUI and web dashboard.

**Architecture:**
```
cmd/
  agent/          # CLI binary (cobra commands)
  tui/            # TUI binary (existing, refactored)
  dashboard/      # Web dashboard binary

pkg/
  agent/          # Core agent lifecycle (spawn, stop, attach, etc.)
  docker/         # Docker client wrapper
  network/        # Network management
  validation/     # Input validation
  config/         # Config/defaults management
  events/         # Event bus (2.1)
  cost/           # Cost tracking
  fleet/          # Fleet/pipeline orchestration
```

**Migration strategy:**
1. Extract `bin/agent-lib.sh` functions into Go `pkg/` packages
2. Implement `cmd_run` and `cmd_spawn` in Go first (most-used commands)
3. Shell out to existing bash for unported commands (transitional)
4. Port remaining commands one by one
5. Keep `bin/agent` as a thin wrapper that delegates to Go binary during migration

**Why Go:**
- TUI is already Go (tview/tcell) — code sharing
- Single binary distribution (already done for TUI via Homebrew)
- Docker client library is Go-native
- cobra/viper for CLI argument parsing with auto-generated help
- goroutines for parallel agent management

### 6.2 Python/TypeScript SDK

**Problem:** No programmatic API for CI/CD integration, custom dashboards, or workflow automation. Competitors offer:
- Claude Agent SDK: 5-10 line agent creation
- E2B: `pip install e2b` + `from e2b import Sandbox`
- OpenHands: composable Python SDK
- Daytona: SDKs in Python, TypeScript, Ruby, Go

**Proposal:** Python and TypeScript SDKs wrapping the Go CLI via subprocess or Unix socket.

**Python SDK:**
```python
from safe_agentic import Agent, Fleet

# Simple spawn
agent = Agent.run(
    repo="git@github.com:org/repo.git",
    prompt="Fix the failing tests",
    max_cost=5.00,
)

# Wait for completion
result = agent.wait()
print(result.cost_estimate)    # $0.23
print(result.changed_files)    # ["src/auth.py"]
print(result.diff)             # unified diff string

# Event callbacks
agent = Agent.run(
    repo="git@github.com:org/repo.git",
    prompt="Refactor auth module",
    on_complete=lambda r: create_github_pr(r),
    on_fail=lambda r: notify_slack(r),
)

# Fleet
fleet = Fleet.from_manifest("fleet.yaml")
fleet.run()
for agent in fleet.agents:
    print(f"{agent.name}: {agent.status} (${agent.cost_estimate})")

# Programmatic fleet
fleet = Fleet(
    agents=[
        Agent(type="claude", repo="...", prompt="Fix backend"),
        Agent(type="codex", repo="...", prompt="Write tests"),
    ],
    shared_tasks=True,
)
fleet.run()
```

**Implementation:** The SDK talks to the Go CLI binary via:
1. **Subprocess** (simplest): `subprocess.run(["safe-ag", "spawn", "--json", ...])` and parse JSON output
2. **Unix socket** (richer): Go binary exposes a local socket API for real-time events and control
3. **HTTP API** (if dashboard exists): reuse dashboard endpoints

Phase 1 uses subprocess. Phase 2 adds socket for streaming events. The HTTP API comes free with the dashboard.

**Depends on:** Phase 6.1 (Go CLI with `--json` output on all commands).

---

## Phase Summary

| Phase | Scope | Effort | Impact | Dependencies |
|-------|-------|--------|--------|-------------|
| **1** | UX Simplification | 1-2 days | High | None |
| **2** | Async Lifecycle | 1 week | High | None |
| **3** | State Management | 1-2 weeks | High | Phase 2 (events) |
| **4** | Fleet Communication | 1-2 weeks | High | None |
| **5** | Observability | 2-3 weeks | Medium-High | Phase 2 (events), Phase 3 (event log) |
| **6** | SDK & API | 4-8 weeks | High | Phase 5 (dashboard) |

**Recommended execution order:** 1 → 2 → 4 → 3 → 5 → 6

Phase 1 is a quick win that improves daily UX immediately. Phase 2 (events) is the foundation for everything that follows. Phase 4 (fleet communication) is independent and can overlap with Phase 3. Phase 5 depends on the event system. Phase 6 is the long pole but has the highest platform impact.

---

## Competitive Positioning After Implementation

| Capability | Claude Managed Agents | Cursor Cloud | E2B / Daytona | safe-agentic v2 |
|-----------|----------------------|--------------|---------------|-----------------|
| Isolation | Managed containers | Cloud VMs | MicroVMs | VM + Container + Network |
| Code leaves machine | Yes (cloud) | Yes (cloud) | Yes (cloud) | **No (local)** |
| Multi-vendor | No (Claude only) | No (Cursor models) | Model-agnostic | **Claude + Codex + extensible** |
| Cost | $0.08/hr + API | $20-200/mo | $0.05/hr + $150/mo | **Free (OSS)** |
| SDK/API | Full SDK | IDE-native | Full SDK | **Full SDK (Phase 6)** |
| Web dashboard | Console | IDE Agents Window | Dashboard | **Dashboard (Phase 5)** |
| Snapshots | Checkpointing | N/A | Snapshots | **Full snapshots (Phase 3)** |
| Fleet orchestration | Agent Teams (preview) | 8 parallel | N/A | **Fleet + Pipeline + Tasks (Phase 4)** |
| Event system | 24 hooks | IDE events | Webhooks | **Event bus + webhooks (Phase 2)** |
| Async notifications | N/A | In-IDE | Webhooks | **Slack, terminal, webhooks (Phase 2)** |
| Cost enforcement | N/A | Plan-based | N/A | **Budget kill switch (Phase 2)** |

**Key message:** safe-agentic v2 is the only tool that combines enterprise-grade isolation, multi-vendor agent support, full lifecycle orchestration, and zero platform fees — all running locally on your machine.

---

## Open Questions

1. **Go rewrite scope:** Full rewrite or hybrid (Go for new features, bash for existing)? The hybrid approach ships faster but accumulates tech debt.

2. **Web dashboard auth:** Basic token auth is fine for localhost. For remote access, should we support OAuth / SSO, or punt to a reverse proxy?

3. **A2A protocol:** The shared-volume task list (4.1) is a pragmatic first step. When should we implement proper A2A over HTTP/SSE between containers? Is there enough demand?

4. **Cloud offload:** Should safe-agentic ever offer optional cloud execution (e.g., spawn on a remote VM instead of OrbStack)? This would compete directly with E2B/Daytona but expand the addressable market.

5. **Plugin system:** Multiple features (event sinks, notification targets, agent types) would benefit from a plugin architecture. Design now or bolt on later?

6. **Pricing table updates:** Hardcoded model pricing becomes stale. Fetch from an API, or let users configure?
