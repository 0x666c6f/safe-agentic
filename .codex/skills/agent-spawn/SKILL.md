---
name: agent-spawn
description: Spawn a sandboxed Claude Code or Codex agent in a hardened container. Use when the user asks to start, launch, run, or spawn an agent on a repo — e.g. "run claude on this repo", "start codex on myrepo", "spawn an agent".
---

# Spawn a Safe Agent

Launch Claude Code or Codex inside an isolated, hardened Docker container.

## Workflow

1. **Determine the repo URL(s)** from the user's request. If they say "this repo", use the current git remote.
2. **Choose the agent type**: `claude` (default) or `codex`.
3. **Choose the shortcut or full command** based on what's needed.

## Quick aliases (use these when possible)

```bash
# Most common — auto-detects SSH from URL
agent-claude <repo-url> [<repo-url>...]
agent-codex <repo-url> [<repo-url>...]
```

These auto-enable `--ssh` when the URL starts with `git@` or `ssh://`.

## Full command (when you need options)

```bash
agent spawn <claude|codex> [options]
```

### Options

| Flag | Purpose | Default |
|------|---------|---------|
| `--repo URL` | Repo to clone (repeatable) | none |
| `--name NAME` | Human-readable container name | timestamp |
| `--prompt 'TASK'` | Initial task for the agent | none (interactive) |
| `--ssh` | Forward SSH agent for private repos | off |
| `--reuse-auth` | Keep OAuth token + config across sessions | ephemeral |
| `--reuse-gh-auth` | Keep GitHub CLI auth across sessions | ephemeral |
| `--network NAME` | Join existing Docker network | dedicated bridge |
| `--memory SIZE` | Memory limit | 8g |
| `--cpus N` | CPU limit | 4 |
| `--pids-limit N` | PID limit | 512 |

## Examples

```bash
# Public repo
agent-claude https://github.com/myorg/myrepo.git

# Private repo (SSH auto-detected)
agent-claude git@github.com:myorg/myrepo.git

# Named session with persistent auth
agent spawn claude --ssh --reuse-auth --name api-work --repo git@github.com:myorg/api.git

# With an initial prompt (agent starts working immediately)
agent spawn codex --ssh --reuse-auth --name fix-ci --repo git@github.com:myorg/api.git \
  --prompt 'Fix the failing CI tests'

# Multiple repos
agent-claude git@github.com:myorg/frontend.git git@github.com:myorg/backend.git

# Big repo with more resources
agent spawn claude --memory 16g --cpus 8 --repo https://github.com/large/monorepo.git

# Untrusted code (no SSH, no internet)
agent spawn claude --repo https://github.com/unknown/repo.git --network agent-isolated
```

## Getting the repo URL from the current directory

If the user says "this repo" or "current repo":
```bash
git remote get-url origin
```

Then pass that URL to the spawn command.

## MCP OAuth login (before or after spawning)

If the agent needs MCP servers (Linear, Notion, etc.), authenticate first:

```bash
# No container needed — uses default auth volume
agent mcp-login linear
agent mcp-login notion
```

The token persists in the auth volume for all agents using `--reuse-auth`.

## After spawning

The agent opens interactively. On first run, an OAuth URL appears — the user opens it in their browser. After auth, the agent is ready to use.

Containers persist after exit (stopped state). Use `agent attach <name>` to reattach, or `agent stop <name>` to remove. Use `--reuse-auth` to keep auth tokens and config across spawns.

## Host config injection

The host's `~/.codex/config.toml` or `~/.claude/settings.json` is automatically injected into new containers. This carries over MCP servers, model settings, features, and plugins. The config is only seeded once — edits inside the container are preserved.
