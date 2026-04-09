---
name: agent-manage
description: List, attach to, stop, or clean up running safe-agentic containers. Use when the user asks to check agents, stop agents, attach to an agent, clean up, or manage running containers.
---

# Manage Safe Agents

List, attach to, stop, export sessions, or clean up agent containers.

## Commands

### List agents (running + stopped)

```bash
agent list
```

Shows all agent containers with name, type, repo, auth, network, and status.

### Attach to an agent

```bash
agent attach <name>
```

If the container is running, opens a shell into it. If stopped, restarts it.
The name can be the full container name (e.g., `agent-codex-cardinality-restrictions`) or the suffix (e.g., `cardinality-restrictions`).

### Stop and remove agents

```bash
# Stop one agent
agent stop <name>

# Stop all agents
agent stop --all
```

Stops and removes the container, its DinD sidecar (if any), and managed network. Auth volumes persist until cleanup.

### Export session history

```bash
# Export to default path (./agent-sessions/<name>/)
agent sessions <name>

# Export to custom path
agent sessions --latest ~/sessions/
```

Copies session files, history, and session index from the container to the host.

### MCP OAuth login

```bash
# Standalone (uses default agent-codex-auth volume)
agent mcp-login linear

# For a specific container
agent mcp-login <container> notion
```

Runs OAuth login in a temporary container. Token persists in the auth volume for all agents using `--reuse-auth`.

### Refresh AWS credentials

```bash
# Refresh from container's original profile
agent aws-refresh <name>

# Refresh with explicit profile
agent aws-refresh <name> my-profile

# Refresh latest container
agent aws-refresh --latest
```

Re-reads `~/.aws/credentials` from the host and writes into the running container. No restart needed — AWS SDKs re-read the file automatically.

### Peek at agent output

```bash
agent peek <name>              # last 30 lines of tmux pane
agent peek --latest            # latest container
agent peek <name> --lines 50   # more lines
```

Shows what the agent is currently doing without attaching. Only works on running tmux containers.

### Output — extract agent results

```bash
agent output <name>            # last agent message
agent output --latest          # latest container
agent output --diff <name>     # git diff
agent output --files <name>    # list changed files
agent output --commits <name>  # git log
agent output --json <name>     # all of the above as JSON
```

Use `--json` to pipe results into scripts or `--on-exit` callbacks.

### Summary — one-screen overview

```bash
agent summary <name>
agent summary --latest
```

Compact overview: type, status, repo, branch, elapsed time, cost estimate, last agent message, changed files. Use before reviewing or creating a PR.

### Retry — re-run with original config

```bash
agent retry <name>
agent retry --latest
agent retry --latest --feedback "Focus only on the auth module"
```

Stops the container, respawns with the same flags, and optionally appends feedback to the original prompt.

### Config — manage defaults

```bash
agent config show                          # show all current defaults
agent config get memory                    # get one value
agent config set memory 16g                # set a value
agent config reset memory                  # reset to built-in default
agent config reset --all                   # reset everything
```

### Template — manage prompt templates

```bash
agent template list                        # list built-in + custom templates
agent template show security-audit         # preview a template
agent template create my-template          # create custom template ($EDITOR opens)
```

Built-in templates: `security-audit`, `code-review`, `test-coverage`, `dependency-update`, `bug-fix`, `docs-review`.

### Diff — review agent's changes

```bash
agent diff <name>              # full git diff
agent diff --latest --stat     # diffstat summary
```

Shows the git diff from the agent's working tree inside the container.

### Checkpoints — snapshot and revert

```bash
agent checkpoint create <name> "before refactor"
agent checkpoint list <name>
agent checkpoint revert <name> checkpoint-1712678400
```

Snapshots the working tree using git stash refs. Revert restores code without polluting branch history.

### Todos — track merge requirements

```bash
agent todo add <name> "Run tests"
agent todo list <name>
agent todo check <name> 1
agent todo uncheck <name> 1
```

JSON-based todo list inside the container. `agent pr` blocks if incomplete todos exist.

### PR creation

```bash
agent pr <name> --title "feat: add caching" --base dev
agent pr --latest
```

Commits, pushes, and creates a GitHub PR via `gh pr create`. Requires `--ssh` on the container. Blocked by incomplete todos.

### Code review

```bash
agent review <name>                # codex review --uncommitted (or git diff fallback)
agent review --latest --base main  # codex review --base main
```

Runs `codex review` inside the container if available, otherwise shows raw `git diff`.

### Cost estimation

```bash
agent cost <name>
agent cost --latest
```

Parses session JSONL for token usage and estimates API spend per model.

### Audit log

```bash
agent audit               # last 50 entries
agent audit --lines 100
```

Shows the append-only operation log (`~/.config/safe-agentic/audit.jsonl`). Every spawn, stop, and attach is recorded.

### Full cleanup

```bash
agent cleanup          # keeps shared auth volumes
agent cleanup --auth   # also removes auth volumes
```

This:
1. Stops all running agent containers
2. Removes all containers (running + stopped)
3. Removes managed per-container networks
4. Prunes dangling images
5. With `--auth`: also removes shared auth volumes

## TUI keybindings

`agent tui` provides a k9s-style interactive dashboard. Key keybindings:

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
| `:fleet <file>` | Spawn from manifest |
| `:pipeline <file>` | Run pipeline |

## Workflow

1. **Check agents** with `agent list`
2. **Attach** to resume a stopped agent or get a second shell
3. **Export sessions** before stopping if you want to keep history on host
4. **Stop** individual agents or all at once
5. **Cleanup** periodically to free resources

## Examples

```bash
# What agents exist?
agent list

# Reattach to a stopped codex agent
agent attach cardinality-restrictions

# Save sessions before cleanup
agent sessions cardinality-restrictions ~/my-sessions/

# Done for the day — stop everything
agent stop --all

# Full reset — remove all containers, auth, networks
agent cleanup --auth
```
