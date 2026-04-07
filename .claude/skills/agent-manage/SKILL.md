---
name: agent-manage
description: List, attach to, stop, or clean up running safe-agentic containers. Use when the user asks to check agents, stop agents, attach to an agent, clean up, or manage running containers.
---

# Manage Safe Agents

List, attach to, stop, or clean up sandboxed agent containers.

## Commands

### List running agents

```bash
agent list
```

Shows all running agent containers with name, status, creation time, and image.

### Attach to a running agent

Open a second shell into an already-running agent:

```bash
agent attach <name>
```

The name can be the full container name (e.g., `agent-claude-api-work`) or the suffix (e.g., `api-work`).

### Stop agents

```bash
# Stop one agent
agent stop <name>

# Stop all agents
agent stop --all
```

Stopping removes the container and its per-session network. Auth and workspace volumes persist until cleanup.

### Full cleanup

```bash
agent cleanup
```

This:
1. Stops all running agent containers
2. Removes stopped containers
3. Removes shared auth volumes (`--reuse-auth` tokens)
4. Removes managed per-container networks
5. Prunes dangling images

Run this when you want a fresh start or to revoke persistent auth tokens.

## Workflow

1. **Check what's running** with `agent list`
2. **Attach** if you need a second terminal into an agent
3. **Stop** individual agents or all at once
4. **Cleanup** periodically to free resources and revoke tokens

## Examples

```bash
# What's running?
agent list

# Attach to the api-work agent for a second shell
agent attach api-work

# Done for the day — stop everything
agent stop --all

# Full reset — remove all containers, auth, networks
agent cleanup
```
