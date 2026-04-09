# Spawning Agents

## Quick aliases (recommended)

```bash
agent-claude git@github.com:myorg/myrepo.git    # SSH auto-detected
agent-codex https://github.com/myorg/myrepo.git  # HTTPS, no SSH
```

These auto-enable `--ssh` when the URL starts with `git@` or `ssh://`.

## Full command

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
| `--reuse-auth` | Keep OAuth token across sessions | ephemeral |
| `--reuse-gh-auth` | Keep GitHub CLI auth across sessions | ephemeral |
| `--aws PROFILE` | Inject AWS credentials | off |
| `--docker` | Docker-in-Docker sidecar | off |
| `--docker-socket` | Mount VM Docker socket (dangerous) | off |
| `--network NAME` | Join existing Docker network | dedicated bridge |
| `--identity 'Name <email>'` | Git author attribution | Agent \<agent@localhost\> |
| `--memory SIZE` | Memory limit | 8g |
| `--cpus N` | CPU limit | 4 |
| `--pids-limit N` | PID limit | 512 |
| `--dry-run` | Show docker command without running | -- |

## Examples

### Private repo with a task

```bash
agent spawn claude --ssh --repo git@github.com:myorg/api.git \
  --prompt 'Fix the failing CI tests'
```

### Named session with persistent auth

```bash
agent spawn claude --ssh --reuse-auth --name api-refactor \
  --repo git@github.com:myorg/api.git
```

### Multiple repos

```bash
agent-claude git@github.com:myorg/frontend.git git@github.com:myorg/backend.git
```

Each cloned to `/workspace/org/repo` inside the container.

### With AWS credentials

```bash
agent spawn claude --ssh --aws morpho-infra --repo git@github.com:myorg/infra.git
```

Credentials injected to tmpfs. Refresh without restarting: `agent aws-refresh <name>`.

### Big repo with more resources

```bash
agent spawn claude --memory 16g --cpus 8 --repo https://github.com/large/monorepo.git
```

### Untrusted repo (no SSH, no internet)

```bash
agent spawn claude --repo https://github.com/unknown/repo.git --network agent-isolated
```

## What happens after spawn

1. Container starts with read-only rootfs, dropped capabilities, dedicated network
2. Repo is cloned to `/workspace/`
3. If `safe-agentic.json` exists in the repo, its `setup` script runs
4. Agent opens interactively (Claude Code or Codex)
5. On first run, an OAuth URL appears — open it in your browser
6. Container persists after exit — reattach with `agent attach`

## Host config injection

Your `~/.codex/config.toml` and `~/.claude/settings.json` are auto-injected on first launch. MCP servers, model settings, and feature flags carry over. Existing config in the auth volume is preserved.

## Git identity

Containers default to `Agent <agent@localhost>`. Override with:

```bash
agent spawn claude --identity "Your Name <you@example.com>" --repo ...
```

Or set in defaults file (`~/.config/safe-agentic/defaults.sh`):

```bash
SAFE_AGENTIC_DEFAULT_IDENTITY="Your Name <you@example.com>"
```
