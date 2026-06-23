---
name: agent-manage
description: List, attach to, steer, search, run actions for, stop, or clean up running safe-agentic containers. Use when the user asks to check agents, send follow-up instructions, run a configured action, find prior output, stop agents, attach to an agent, clean up, or manage running containers.
---

# Manage Safe Agents

List, attach to, steer, search, run actions for, stop, export sessions, or clean up agent containers.

## Commands

### List agents (running + stopped)

```bash
safe-ag list
```

Shows all agent containers with name, type, repo, auth, network, and status.

### Attach to an agent

```bash
safe-ag attach <name>
```

If the container is running, opens a shell into it. If stopped, restarts it.
The name can be the full container name (e.g., `agent-codex-cardinality-restrictions`) or the suffix (e.g., `cardinality-restrictions`).

### Steer an agent

```bash
safe-ag steer <name> "focus on the failing test only"
safe-ag steer --latest "continue, but keep the change narrow"
```

Sends follow-up text into a tmux-backed session without attaching. If the container is stopped, it starts it first.

### Search agent logs

```bash
safe-ag search "error text"
safe-ag search "Needle" <name> --case-sensitive
```

Searches rendered session logs across all agents by default, or one target with `<name>` / `--latest`.

### Timeline and inbox

```bash
safe-ag timeline
safe-ag inbox
safe-ag inbox --all
```

Use `timeline` for recent event/audit history. Use `inbox` for classified statuses such as `failed`, `failed-tests`, `needs-auth`, `stuck`, `ready-for-review`, and `ready-for-pr`.

### Run configured actions

```bash
safe-ag action list
safe-ag action show test
safe-ag action run test --latest
safe-ag action run lint <name>
```

Loads actions from `~/.safe-ag/actions.toml` then `.safe-ag/actions.toml`. Project actions override user actions. Commands run inside the target agent workspace.

### Track review comments

```bash
safe-ag review-comments add <name> cmd/main.go 42 "Handle empty input first"
safe-ag review-comments list <name>
safe-ag review-comments resolve rc-123
safe-ag review-comments clear <name>
```

Stores local file/line review notes in `~/.safe-ag/state/review-comments.jsonl`. Use with `safe-ag steer` when an agent needs to address review feedback without losing context.

### Stage or revert workspace files

```bash
safe-ag workspace stage <name> src/app.go
safe-ag workspace unstage <name> src/app.go
safe-ag workspace revert <name> src/app.go --yes
safe-ag workspace stage-patch <name> selected.patch
safe-ag workspace revert-patch <name> selected.patch --yes
```

Use `workspace revert` or `revert-patch` only when the user asked to discard those changes or the scope is clearly confirmed. Patch commands are for selected hunks from a unified diff.

### Capture browser verification artifacts

```bash
safe-ag browser capture http://localhost:3000 --mode auto --annotation "login button visible"
safe-ag browser capture https://example.com --mode chrome --out /tmp/browser-artifact
```

Captures DOM, headers, and artifact metadata. Chrome mode also captures screenshot, console, and network data without reusing host browser profiles/cookies.

### JSON protocol for clients

```bash
safe-ag server --stdio
SAFE_AGENTIC_SERVER_TOKEN=secret safe-ag server --listen 127.0.0.1:8765
```

Use for client integrations that need `schema`, `ping`, `timeline`, `inbox`, `agents.list`, `agent.logs`, `agent.diff`, `actions.list`, or `actions.run` without scraping human CLI output. HTTP mode requires a bearer token.

### Handoff workspaces

```bash
safe-ag handoff <name> --to-worktree
safe-ag handoff <name> --to-local ./workspace-copy
safe-ag worktree list
safe-ag worktree cleanup --dry-run
```

Use `--to-worktree` for agents spawned with `--worktree`; it prints the managed host checkout path. Use `--to-local` to copy `/workspace` out of any agent container.

### Stop and remove agents

```bash
# Stop one agent
safe-ag stop <name>

# Stop all agents
safe-ag stop --all
```

Stops and removes the container, its DinD sidecar (if any), and managed network. Auth volumes persist until cleanup.

### Export session history

```bash
# Export to default path (./agent-sessions/<name>/)
safe-ag sessions <name>

# Export to custom path
safe-ag sessions --latest ~/sessions/
```

Copies session files, history, and session index from the container to the host.

### MCP OAuth login

```bash
# Standalone (uses default agent-codex-auth volume)
safe-ag mcp-login linear

# For a specific container
safe-ag mcp-login notion <container>
```

Runs OAuth login in a temporary container. Token persists in the auth volume for all agents using `--reuse-auth`.

### Refresh AWS credentials

```bash
# Refresh from container's original profile
safe-ag aws-refresh <name>

# Refresh with explicit profile
safe-ag aws-refresh <name> my-profile

# Refresh latest container
safe-ag aws-refresh --latest
```

Re-reads `~/.aws/credentials` from the host and writes into the running container. No restart needed — AWS SDKs re-read the file automatically.

### Peek at agent output

```bash
safe-ag peek <name>              # last 30 lines of tmux pane
safe-ag peek --latest            # latest container
safe-ag peek <name> --lines 50   # more lines
```

Shows what the agent is currently doing without attaching. Only works on running tmux containers.

### Output — extract agent results

```bash
safe-ag output <name>            # last agent message
safe-ag output --latest          # latest container
safe-ag output --diff <name>     # git diff
safe-ag output --files <name>    # list changed files
safe-ag output --commits <name>  # git log
safe-ag output --json <name>     # all of the above as JSON
```

Use `--json` to pipe results into scripts or `--on-exit` callbacks.

### Summary — one-screen overview

```bash
safe-ag summary <name>
safe-ag summary --latest
```

Compact overview: type, status, repo, branch, elapsed time, cost estimate, last agent message, changed files. Use before reviewing or creating a PR.

### Retry — re-run with original config

```bash
safe-ag retry <name>
safe-ag retry --latest
safe-ag retry --latest --feedback "Focus only on the auth module"
```

Stops the container, respawns with the same flags, and optionally appends feedback to the original prompt.

### Config — manage defaults

```bash
safe-ag config show                          # show all current defaults
safe-ag config get memory                    # get one value
safe-ag config set memory 16g                # set a value
safe-ag config reset memory                  # reset to built-in default
safe-ag config reset --all                   # reset everything
```

### Template — manage prompt templates

```bash
safe-ag template list                        # list built-in + custom templates
safe-ag template show security-audit         # preview a template
safe-ag template create my-template          # create custom template ($EDITOR opens)
```

Built-in templates: `security-audit`, `code-review`, `test-coverage`, `dependency-update`, `bug-fix`, `docs-review`.

### Diff — review agent's changes

```bash
safe-ag diff <name>              # full git diff
safe-ag diff --latest --stat     # diffstat summary
```

Shows the git diff from the agent's working tree inside the container.

### Checkpoints — snapshot and revert

```bash
safe-ag checkpoint create <name> "before refactor"
safe-ag checkpoint list <name>
safe-ag checkpoint revert <name> checkpoint-1712678400
```

Snapshots the working tree using git stash refs. Revert restores code without polluting branch history.

### Todos — track merge requirements

```bash
safe-ag todo add <name> "Run tests"
safe-ag todo list <name>
safe-ag todo check <name> 1
safe-ag todo uncheck <name> 1
```

JSON-based todo list inside the container. `safe-ag pr` blocks if incomplete todos exist.

### PR creation

```bash
safe-ag pr <name> --title "feat: add caching" --base dev
safe-ag pr --latest
```

Commits, pushes, and creates a GitHub PR via `gh pr create`. Requires `--ssh` on the container. Blocked by incomplete todos.

### Code review

```bash
safe-ag review <name>                # codex review --uncommitted (or git diff fallback)
safe-ag review --latest --base main  # codex review --base main
```

Runs `codex review` inside the container if available, otherwise shows raw `git diff`.

### Cost estimation

```bash
safe-ag cost <name>
safe-ag cost --latest
```

Parses session JSONL for token usage and estimates API spend per model.

### Audit log

```bash
safe-ag audit               # last 50 entries
safe-ag audit --lines 100
```

Shows the append-only operation log (`~/.config/safe-agentic/audit.jsonl`). Every spawn, stop, and attach is recorded.

### Full cleanup

```bash
safe-ag cleanup          # keeps auth volumes
safe-ag cleanup --auth   # also removes auth volumes
```

This:
1. Stops all running agent containers
2. Removes all containers (running + stopped)
3. Removes managed per-container networks
4. Prunes dangling images
5. With `--auth`: also removes shared and isolated auth volumes

## TUI keybindings

`safe-ag tui` provides a k9s-style interactive dashboard. Key keybindings:

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

1. **Check agents** with `safe-ag list`
2. **Peek/search** to understand current or prior output
3. **Steer** with a focused follow-up instead of attaching when possible
4. **Run actions** for repo-native checks inside the agent workspace
5. **Export sessions** before stopping if you want to keep history on host
6. **Stop** individual agents or all at once
7. **Cleanup** periodically to free resources

## Examples

```bash
# What agents exist?
safe-ag list

# Reattach to a stopped codex agent
safe-ag attach cardinality-restrictions

# Save sessions before cleanup
safe-ag sessions cardinality-restrictions ~/my-sessions/

# Done for the day — stop everything
safe-ag stop --all

# Full reset — remove all containers, auth, networks
safe-ag cleanup --auth
```
