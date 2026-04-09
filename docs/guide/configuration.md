# Configuration

## Defaults file

`~/.config/safe-agentic/defaults.sh` — loaded on every `agent` command.

```bash
SAFE_AGENTIC_DEFAULT_MEMORY=16g
SAFE_AGENTIC_DEFAULT_CPUS=8
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=true
SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH=true
SAFE_AGENTIC_DEFAULT_SSH=false
SAFE_AGENTIC_DEFAULT_DOCKER=false
SAFE_AGENTIC_DEFAULT_NETWORK=agent-isolated
SAFE_AGENTIC_DEFAULT_IDENTITY="Your Name <you@example.com>"
```

Simple `KEY=value` lines only. Not sourced as shell.

## Config command

`agent config` manages defaults from the command line instead of editing the defaults file directly.

```bash
# Show all current defaults
agent config show

# Get a single value
agent config get memory

# Set a value
agent config set memory 16g
agent config set cpus 8
agent config set reuse_auth true
agent config set identity "Your Name <you@example.com>"

# Reset to built-in defaults
agent config reset memory
agent config reset --all
```

Changes write to `~/.config/safe-agentic/defaults.sh` and take effect on the next `agent` command.

## Template command

`agent template` manages built-in and custom prompt templates.

```bash
# List all templates (built-in + custom)
agent template list

# Preview a template's prompt
agent template show security-audit
agent template show my-custom-template

# Create a new custom template (opens $EDITOR)
agent template create backend-engineer
```

Custom templates are stored in `~/.config/safe-agentic/templates/`. Built-in templates (`security-audit`, `code-review`, `test-coverage`, `dependency-update`, `bug-fix`, `docs-review`) are read-only and cannot be overwritten.

Use templates at spawn time:

```bash
agent spawn claude --ssh --repo git@github.com:org/api.git --template security-audit
```

## MCP OAuth login

```bash
agent mcp-login linear       # uses default codex auth volume
agent mcp-login notion
agent mcp-login <name> linear  # target a specific container
```

Tokens persist in the auth volume. One-time setup per MCP server.

## AWS credentials

```bash
# Inject at spawn time
agent spawn claude --ssh --aws my-aws-profile --repo ...

# Refresh in a running container
agent aws-refresh api-refactor
agent aws-refresh --latest my-profile
```

Credentials stored on tmpfs. AWS SDKs re-read automatically — no restart needed.

## Docker access

```bash
# Safer: per-session Docker-in-Docker sidecar
agent spawn claude --docker --repo ...

# Broader: mount VM Docker socket directly
agent spawn claude --docker-socket --repo ...
```

## Setup and maintenance

```bash
agent setup               # first-time VM + image creation
agent update               # rebuild image (cached)
agent update --quick       # rebuild AI CLI layer only
agent update --full        # full rebuild, no cache
agent vm start             # start VM + re-harden
agent vm stop              # stop VM
agent vm ssh               # debug the VM
agent diagnose             # health check
```

## Troubleshooting

### `No SSH_AUTH_SOCK in VM`

```bash
agent diagnose
agent vm start
```

Verify 1Password SSH agent is enabled on macOS.

### `Image not found`

```bash
agent update
```

### OAuth hangs

Wait for the device code prompt. Use `--reuse-auth` to persist the session.
