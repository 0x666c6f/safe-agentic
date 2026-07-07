---
name: agent-manage
description: List, attach to, steer, search, run actions for, stop, or clean up running berth containers. Use when the user asks to check agents, send follow-up instructions, run a configured action, find prior output, stop agents, attach to an agent, clean up, or manage running containers.
---

# Manage Safe Agents

List, attach to, steer, search, run actions for, stop, export sessions, or clean up agent containers.

## Commands

### List agents (running + stopped)

```bash
berth list
```

Shows all agent containers with name, type, repo, auth, network, and status.

### Attach to an agent

```bash
berth attach <name>
```

If the container is running, opens a shell into it. If stopped, restarts it.
The name can be the full container name (e.g., `agent-codex-cardinality-restrictions`) or the suffix (e.g., `cardinality-restrictions`).

### Steer an agent

```bash
berth steer <name> "focus on the failing test only"
berth steer --latest "continue, but keep the change narrow"
```

Sends follow-up text into a tmux-backed session without attaching. If the container is stopped, it starts it first.

### Search agent logs

```bash
berth search "error text"
berth search "Needle" <name> --case-sensitive
```

Searches rendered session logs across all agents by default, or one target with `<name>` / `--latest`.

### Timeline and inbox

```bash
berth timeline
berth inbox
berth inbox --all
```

Use `timeline` for recent event/audit history. Use `inbox` for classified statuses such as `failed`, `failed-tests`, `needs-auth`, `stuck`, `ready-for-review`, and `ready-for-pr`.

### Run configured actions

```bash
berth action list
berth action show test
berth action run test --latest
berth action run lint <name>
```

Loads actions from `~/.berth/actions.toml` then `.berth/actions.toml`. Project actions override user actions. Commands run inside the target agent workspace.

### Track review comments

```bash
berth review-comments add <name> cmd/main.go 42 "Handle empty input first"
berth review-comments list <name>
berth review-comments resolve rc-123
berth review-comments clear <name>
```

Stores local file/line review notes in `~/.berth/state/review-comments.jsonl`. Use with `berth steer` when an agent needs to address review feedback without losing context.

### Stage or revert workspace files

```bash
berth workspace stage <name> src/app.go
berth workspace unstage <name> src/app.go
berth workspace revert <name> src/app.go --yes
berth workspace stage-patch <name> selected.patch
berth workspace revert-patch <name> selected.patch --yes
```

Use `workspace revert` or `revert-patch` only when the user asked to discard those changes or the scope is clearly confirmed. Patch commands are for selected hunks from a unified diff.

### Capture browser verification artifacts

```bash
berth browser capture http://localhost:3000 --mode auto --annotation "login button visible"
berth browser capture https://example.com --mode chrome --out /tmp/browser-artifact
```

Captures DOM, headers, and artifact metadata. Chrome mode also captures screenshot, console, and network data without reusing host browser profiles/cookies.

### JSON protocol for clients

```bash
berth server --stdio
BERTH_SERVER_TOKEN=secret berth server --listen 127.0.0.1:8765
```

Use for client integrations that need `schema`, `ping`, `timeline`, `inbox`, `agents.list`, `agent.logs`, `agent.diff`, `actions.list`, or `actions.run` without scraping human CLI output. HTTP mode requires a bearer token.

### Handoff workspaces

```bash
berth handoff <name> --to-worktree
berth handoff <name> --to-local ./workspace-copy
berth worktree list
berth worktree cleanup --dry-run
```

Use `--to-worktree` for agents spawned with `--worktree`; it prints the managed host checkout path. Use `--to-local` to copy `/workspace` out of any agent container.

### Stop and remove agents

```bash
# Stop one agent
berth stop <name>

# Stop all agents
berth stop --all
```

Stops and removes the container, its DinD sidecar (if any), and managed network. Auth volumes persist until cleanup.

### Export session history

```bash
# Export to default path (./agent-sessions/<name>/)
berth sessions <name>

# Export to custom path
berth sessions --latest ~/sessions/
```

Copies session files, history, and session index from the container to the host.

### MCP OAuth login

```bash
# Standalone (uses default agent-codex-auth volume)
berth mcp-login linear

# For a specific container
berth mcp-login notion <container>
```

Runs OAuth login in a temporary container. Token persists in the auth volume for all agents using `--reuse-auth`.

### Refresh AWS credentials

```bash
# Refresh from container's original profile
berth aws-refresh <name>

# Refresh with explicit profile
berth aws-refresh <name> my-profile

# Refresh latest container
berth aws-refresh --latest
```

Re-reads `~/.aws/credentials` from the host and writes into the running container. No restart needed — AWS SDKs re-read the file automatically.

### Peek at agent output

```bash
berth peek <name>              # last 30 lines of tmux pane
berth peek --latest            # latest container
berth peek <name> --lines 50   # more lines
```

Shows what the agent is currently doing without attaching. Only works on running tmux containers.

### Output — extract agent results

```bash
berth output <name>            # last agent message
berth output --latest          # latest container
berth output --diff <name>     # git diff
berth output --files <name>    # list changed files
berth output --commits <name>  # git log
berth output --json <name>     # all of the above as JSON
```

Use `--json` to pipe results into scripts or `--on-exit` callbacks.

### Summary — one-screen overview

```bash
berth summary <name>
berth summary --latest
```

Compact overview: type, status, repo, branch, elapsed time, cost estimate, last agent message, changed files. Use before reviewing or creating a PR.

### Retry — re-run with original config

```bash
berth retry <name>
berth retry --latest
berth retry --latest --feedback "Focus only on the auth module"
```

Stops the container, respawns with the same flags, and optionally appends feedback to the original prompt.

### Config — manage defaults

```bash
berth config show                          # show all current defaults
berth config get memory                    # get one value
berth config set memory 16g                # set a value
berth config reset memory                  # reset to built-in default
berth config reset --all                   # reset everything
```

### Template — manage prompt templates

```bash
berth template list                        # list built-in + custom templates
berth template show security-audit         # preview a template
berth template create my-template          # create custom template ($EDITOR opens)
```

Built-in templates: `security-audit`, `code-review`, `test-coverage`, `dependency-update`, `bug-fix`, `docs-review`.

### Diff — review agent's changes

```bash
berth diff <name>              # full git diff
berth diff --latest --stat     # diffstat summary
```

Shows the git diff from the agent's working tree inside the container.

### Checkpoints — snapshot and revert

```bash
berth checkpoint create <name> "before refactor"
berth checkpoint list <name>
berth checkpoint revert <name> checkpoint-1712678400
```

Snapshots the working tree using git stash refs. Revert restores code without polluting branch history.

### Todos — track merge requirements

```bash
berth todo add <name> "Run tests"
berth todo list <name>
berth todo check <name> 1
berth todo uncheck <name> 1
```

JSON-based todo list inside the container. `berth pr` blocks if incomplete todos exist.

### PR creation

```bash
berth pr <name> --title "feat: add caching" --base dev
berth pr --latest
```

Commits, pushes, and creates a GitHub PR via `gh pr create`. Requires `--ssh` on the container. Blocked by incomplete todos.

### Code review

```bash
berth review <name>                # codex review --uncommitted (or git diff fallback)
berth review --latest --base main  # codex review --base main
```

Runs `codex review` inside the container if available, otherwise shows raw `git diff`.

### Cost estimation

```bash
berth cost <name>
berth cost --latest
```

Parses session JSONL for token usage and estimates API spend per model.

### Audit log

```bash
berth audit               # last 50 entries
berth audit --lines 100
```

Shows the append-only operation log (`~/.config/berth/audit.jsonl`). Every spawn, stop, and attach is recorded.

### Full cleanup

```bash
berth cleanup          # keeps auth volumes
berth cleanup --auth   # also removes auth volumes
```

This:
1. Stops all running agent containers
2. Removes all containers (running + stopped)
3. Removes managed per-container networks
4. Prunes dangling images
5. With `--auth`: also removes shared and isolated auth volumes

## TUI keybindings

`berth tui` provides a k9s-style interactive dashboard. Key keybindings:

| Key | Action |
|-----|--------|
| `a` / `Enter` | Attach to agent |
| `r` | Resume agent |
| `s` | Stop agent |
| `l` | View logs |
| `d` | Describe (docker inspect) |
| `f` | Diff overlay |
| `R` | Code review overlay |
| `t` | Todos overlay |
| `x` | Create checkpoint |
| `g` | Create PR |
| `$` | Cost estimation overlay |
| `A` | Audit log overlay |
| `p` | Toggle preview pane |
| `n` | Spawn new agent |
| `e` | Export sessions |
| `/` | Filter agents |
| `?` | Help overlay (all keybindings) |
| `:action <name>` | Run configured action in selected agent |
| `:comments` | Show review comments for selected agent |
| `:timeline` | Show recent events |
| `:inbox` | Show events needing attention |
| `:fleet <file>` | Spawn from manifest |
| `:pipeline <file>` | Run pipeline |

## Workflow

1. **Check agents** with `berth list`
2. **Peek/search** to understand current or prior output
3. **Steer** with a focused follow-up instead of attaching when possible
4. **Run actions** for repo-native checks inside the agent workspace
5. **Export sessions** before stopping if you want to keep history on host
6. **Stop** individual agents or all at once
7. **Cleanup** periodically to free resources

## Examples

```bash
# What agents exist?
berth list

# Reattach to a stopped codex agent
berth attach cardinality-restrictions

# Save sessions before cleanup
berth sessions cardinality-restrictions ~/my-sessions/

# Done for the day — stop everything
berth stop --all

# Full reset — remove all containers, auth, networks
berth cleanup --auth
```
