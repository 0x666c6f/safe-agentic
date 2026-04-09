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
| `--template NAME` | Use a built-in prompt template | none |
| `--instructions 'TEXT'` | Inject agent role context (prepended to prompt) | none |
| `--instructions-file PATH` | Load role context from a file | none |
| `--on-exit 'CMD'` | Run a command on the host when agent finishes | none |
| `--max-cost N` | Cost budget label (informational) | none |
| `--background` | Headless mode — detach immediately after spawn | off |
| `--auto-trust` | Skip the trust prompt on first run | off |
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
agent spawn claude --ssh --aws my-aws-profile --repo git@github.com:myorg/infra.git
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

## More examples

### Parallel development sessions

```bash
# Work on 3 features simultaneously
agent spawn claude --ssh --reuse-auth --name feature-auth --repo git@github.com:myorg/api.git \
  --prompt "Implement OAuth2 PKCE flow for the mobile app"

agent spawn claude --ssh --reuse-auth --name feature-search --repo git@github.com:myorg/api.git \
  --prompt "Add full-text search to the /api/products endpoint using PostgreSQL tsvector"

agent spawn codex --ssh --reuse-auth --name fix-tests --repo git@github.com:myorg/api.git \
  --prompt "Fix all failing tests in the test suite"

# Monitor all 3
agent tui
```

### Untrusted code review

```bash
# Review a suspicious PR without giving it any access
agent spawn claude \
  --repo https://github.com/unknown-contributor/their-fork.git \
  --network agent-isolated \
  --prompt "Review this codebase for malicious code, backdoors, or security issues. Do not run any scripts."
```

## Templates

Built-in prompt templates let you run standard tasks without writing prompts.

```bash
# List available templates
agent template list

# Preview a template
agent template show security-audit

# Use a template
agent spawn claude --ssh --repo git@github.com:org/api.git --template security-audit

# Create a custom template
agent template create my-template
```

### Built-in templates

| Name | Purpose |
|------|---------|
| `security-audit` | OWASP top-10, secrets, SQL injection, IAM |
| `code-review` | Best practices, bugs, test coverage gaps |
| `test-coverage` | Identify untested paths, write tests |
| `dependency-update` | Update deps, fix breaking changes |
| `bug-fix` | Investigate and fix a reported bug |
| `docs-review` | Audit docs for accuracy and completeness |

### Template examples

```bash
# Security audit on infrastructure repo
agent spawn claude --ssh --aws my-profile \
  --repo git@github.com:org/infra.git \
  --template security-audit

# Test coverage pass on a feature branch
agent spawn codex --ssh --reuse-auth \
  --repo git@github.com:org/api.git \
  --template test-coverage \
  --name coverage-run

# Combine template with extra instructions
agent spawn claude --ssh --repo git@github.com:org/api.git \
  --template code-review \
  --instructions "Focus on the auth module only. Skip generated files."
```

## Instructions

Inject role context that prepends to the prompt (or replaces the prompt when used without `--prompt`).

```bash
# Inline instructions
agent spawn claude --ssh --repo git@github.com:org/api.git \
  --instructions "You are a senior Go engineer. Follow the project's error handling conventions."

# From a file
agent spawn claude --ssh --repo git@github.com:org/api.git \
  --instructions-file ./prompts/backend-engineer.md \
  --prompt "Refactor the auth middleware"
```

## Background mode and on-exit callbacks

```bash
# Headless — spawn and return immediately (no interactive attach)
agent spawn claude --background --ssh --repo git@github.com:org/api.git \
  --prompt "Update all dependencies and create a PR"

# Run a script when the agent finishes
agent spawn claude --background --ssh --repo git@github.com:org/api.git \
  --prompt "Fix the failing tests" \
  --on-exit "agent output --latest --json >> /tmp/results.json"

# Cost budget label (pipeline tooling can check this)
agent spawn claude --ssh --repo git@github.com:org/api.git \
  --max-cost 5 \
  --prompt "Refactor the payment module"
```

`--auto-trust` skips the interactive trust prompt — useful in CI or fleet manifests where the agent must start without user input.
