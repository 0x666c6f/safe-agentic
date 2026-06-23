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
safe-ag-claude <repo-url> [<repo-url>...]
safe-ag-codex <repo-url> [<repo-url>...]
```

These auto-enable `--ssh` when the URL starts with `git@` or `ssh://`.

## Saved profiles

```bash
safe-ag profile list
safe-ag profile show reviewer
safe-ag profile run reviewer "focus on auth only"
```

Profiles live in `~/.safe-ag/agents/*.toml` and `.safe-ag/agents/*.toml`. Use them for repeatable roles before hand-writing a long `safe-ag spawn` command.

## Policy guards

Before suggesting a risky spawn mode, remember `~/.safe-ag/rules.toml` and nearest `.safe-ag/rules.toml` can deny it after defaults are applied:

```toml
[allow]
docker_modes = ["off", "dind"]  # off, dind, host-socket
networks = ["managed", "none"]
aws_profiles = ["dev"]
ssh = false
reuse_auth = false
reuse_gh_auth = false
seed_auth = false
setup_scripts = false
```

If a spawn fails with a policy denial, adjust the command to the allowed mode instead of bypassing the rule.

## Full command (when you need options)

```bash
safe-ag spawn <claude|codex> [options]
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
| `--aws PROFILE` | Inject AWS credentials from `~/.aws/credentials` | off |
| `--network NAME` | Join existing Docker network | dedicated bridge |
| `--memory SIZE` | Memory limit | 8g |
| `--cpus N` | CPU limit | 4 |
| `--pids-limit N` | PID limit | 512 |
| `--template NAME` | Use a built-in prompt template | none |
| `--instructions 'TEXT'` | Inject agent role context (prepended to prompt) | none |
| `--instructions-file PATH` | Load role context from a file | none |
| `--on-exit 'CMD'` | Run a host command when agent finishes | none |
| `--max-cost N` | Cost budget label | none |
| `--worktree` | Create and mount a managed host git worktree from current checkout | off |
| `--background` | Headless mode — detach immediately after spawn. Codex needs pre-seeded `--reuse-auth`. | off |
| `--auto-trust` | Skip the trust prompt on first run | off |

## Examples

```bash
# Public repo
safe-ag-claude https://github.com/myorg/myrepo.git

# Private repo (SSH auto-detected)
safe-ag-claude git@github.com:myorg/myrepo.git

# Named session with persistent auth
safe-ag spawn claude --ssh --reuse-auth --name api-work --repo git@github.com:myorg/api.git

# With an initial prompt (agent starts working immediately)
safe-ag spawn codex --ssh --reuse-auth --name fix-ci --repo git@github.com:myorg/api.git \
  --prompt 'Fix the failing CI tests'

# Multiple repos
safe-ag-claude git@github.com:myorg/frontend.git git@github.com:myorg/backend.git

# Big repo with more resources
safe-ag spawn claude --memory 16g --cpus 8 --repo https://github.com/large/monorepo.git

# With AWS credentials for infrastructure work
safe-ag spawn claude --ssh --aws my-aws-profile --repo git@github.com:myorg/infra.git

# Current checkout as isolated managed worktree
safe-ag spawn claude --worktree --name auth-fix --prompt 'Fix the auth tests'

# Untrusted code (no SSH, no internet)
safe-ag spawn claude --repo https://github.com/unknown/repo.git --network agent-isolated
```

## Worktree Mode

`--worktree` must be run from inside a git checkout and cannot be combined with `--repo`.
It creates `~/.safe-ag/worktrees/<container-name>` on branch `safe-ag/<container-name>`, bind-mounts it at `/workspace`, and copies ignored local files listed in `.safe-aginclude`.

```bash
safe-ag handoff auth-fix --to-worktree
safe-ag handoff auth-fix --to-local ./workspace-copy
safe-ag worktree snapshot auth-fix "before review fixes"
safe-ag worktree restore auth-fix stash@{0}
safe-ag worktree cleanup --dry-run
```

## Templates

```bash
# List available templates
safe-ag template list

# Preview a template
safe-ag template show security-audit

# Use a template (no --prompt needed)
safe-ag spawn claude --ssh --repo git@github.com:org/api.git --template security-audit

# Combine template with extra instructions
safe-ag spawn claude --ssh --repo git@github.com:org/api.git \
  --template code-review \
  --instructions "Focus on the auth module only."
```

Built-in templates: `security-audit`, `code-review`, `test-coverage`, `dependency-update`, `bug-fix`, `docs-review`.

## Background and callbacks

```bash
# Headless — spawn and return immediately
safe-ag spawn claude --background --auto-trust --ssh \
  --repo git@github.com:org/api.git \
  --prompt "Fix the failing tests" \
  --on-exit "safe-ag output --latest --json > /tmp/result.json"
```

For Codex, background mode only works after one interactive `--reuse-auth` run has created `/home/agent/.codex/auth.json` in `agent-codex-auth`.

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
safe-ag mcp-login linear
safe-ag mcp-login notion
```

The token persists in the auth volume for all agents using `--reuse-auth`.

## Fleet — spawn multiple agents from manifest

```bash
safe-ag fleet fleet.yaml
safe-ag fleet fleet.yaml --dry-run
```

YAML manifest format:
```yaml
agents:
  - name: api-worker
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    reuse_auth: true
    prompt: "Fix the failing tests"
  - name: frontend
    type: codex
    repo: https://github.com/org/frontend.git
```

Supported fields per agent: `name`, `type`, `repo`, `repos`, `ssh`, `reuse_auth`, `reuse_gh_auth`, `seed_auth`, `docker`, `allow_setup_scripts`, `prompt`, `aws`, `network`, `memory`, `cpus`, `background`, `auto_trust`.

## Pipeline — multi-step agent workflows

```bash
safe-ag pipeline pipeline.yaml
safe-ag pipeline pipeline.yaml --dry-run
```

Pipeline format:
```yaml
name: test-and-fix
steps:
  - name: run-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Run all tests and report results"
  - name: fix-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Fix the failing tests"
    depends_on: run-tests
  - name: create-pr
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Create a PR with the fixes"
    depends_on: fix-tests
```

Steps run sequentially. `depends_on` waits until dependencies complete successfully. `retry`, `on_failure`, `when`, and `outputs` are not implemented and are rejected instead of silently ignored.

## Lifecycle scripts (safe-agentic.json)

Repos can include a `safe-agentic.json` at root for optional setup:

```json
{
  "scripts": {
    "setup": "npm install && cp .env.example .env"
  }
}
```

The `setup` script runs after clone only when the session was spawned with `--allow-setup-scripts` or the manifest agent sets `allow_setup_scripts: true`.

## After spawning

The agent opens interactively. On first run, an OAuth URL appears — the user opens it in their browser. After auth, the agent is ready to use.

Containers persist after exit (stopped state). Use `safe-ag attach <name>` to reattach, or `safe-ag stop <name>` to remove. Use `--reuse-auth` to keep auth tokens and config across spawns.

## Host config injection

The host's `~/.codex/config.toml` or `~/.claude/settings.json` is automatically injected into new containers. This carries over MCP servers, model settings, features, and plugins. The config is only seeded once — edits inside the container are preserved.
