---
name: agent-manage
description: List, attach to, stop, or clean up running safe-agentic containers. Use when the user asks to check agents, stop agents, attach to an agent, clean up, or manage running containers.
---

# Manage Safe Agents

List, attach to, stop, export sessions, or clean up agent containers.

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
safe-ag cleanup          # keeps shared auth volumes
safe-ag cleanup --auth   # also removes auth volumes
```

This:
1. Stops all running agent containers
2. Removes all containers (running + stopped)
3. Removes managed per-container networks
4. Prunes dangling images
5. With `--auth`: also removes shared auth volumes

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
| `:fleet <file>` | Spawn from manifest |
| `:pipeline <file>` | Run pipeline |

## Workflow

1. **Check agents** with `safe-ag list`
2. **Attach** to resume a stopped agent or get a second shell
3. **Export sessions** before stopping if you want to keep history on host
4. **Stop** individual agents or all at once
5. **Cleanup** periodically to free resources

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
