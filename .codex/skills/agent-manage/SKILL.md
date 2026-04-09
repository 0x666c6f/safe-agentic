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
agent aws-refresh <name> perso

# Refresh latest container
agent aws-refresh --latest
```

Re-reads `~/.aws/credentials` from the host and writes into the running container. No restart needed.

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
