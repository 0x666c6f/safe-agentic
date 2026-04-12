# Configuration

## Defaults file

`~/.config/safe-agentic/defaults.sh` — loaded on every `safe-ag` command.

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

`safe-ag config` manages defaults from the command line instead of editing the defaults file directly.

```bash
# Show all current defaults
safe-ag config show

# Get a single value
safe-ag config get memory

# Set a value
safe-ag config set memory 16g
safe-ag config set cpus 8
safe-ag config set reuse_auth true
safe-ag config set identity "Your Name <you@example.com>"

# Reset to built-in defaults
safe-ag config reset memory
safe-ag config reset --all
```

Changes write to `~/.config/safe-agentic/defaults.sh` and take effect on the next `safe-ag` command.

## Template command

`safe-ag template` manages built-in and custom prompt templates.

```bash
# List all templates (built-in + custom)
safe-ag template list

# Preview a template's prompt
safe-ag template show security-audit
safe-ag template show my-custom-template

# Create a new custom template (opens $EDITOR)
safe-ag template create backend-engineer
```

Custom templates are stored in `~/.config/safe-agentic/templates/`. Built-in templates (`security-audit`, `code-review`, `test-coverage`, `dependency-update`, `bug-fix`, `docs-review`) are read-only and cannot be overwritten.

Use templates at spawn time:

```bash
safe-ag spawn claude --ssh --repo git@github.com:org/api.git --template security-audit
```

## MCP OAuth login

```bash
safe-ag mcp-login linear       # uses default codex auth volume
safe-ag mcp-login notion
safe-ag mcp-login linear <name>  # target a specific container
```

Tokens persist in the auth volume. One-time setup per MCP server.

## AWS credentials

```bash
# Inject at spawn time
safe-ag spawn claude --ssh --aws my-aws-profile --repo ...

# Refresh in a running container
safe-ag aws-refresh api-refactor
safe-ag aws-refresh --latest my-profile
```

Credentials stored on tmpfs. AWS SDKs re-read automatically — no restart needed.

## Docker access

```bash
# Safer: per-session Docker-in-Docker sidecar
safe-ag spawn claude --docker --repo ...

# Broader: mount VM Docker socket directly
safe-ag spawn claude --docker-socket --repo ...
```

## Setup and maintenance

```bash
safe-ag setup               # first-time VM + image creation
safe-ag update               # rebuild image (cached)
safe-ag update --quick       # rebuild AI CLI layer only
safe-ag update --full        # full rebuild, no cache
safe-ag vm start             # start VM + re-harden
safe-ag vm stop              # stop VM
safe-ag vm ssh               # debug the VM
safe-ag diagnose             # health check
```

## Advanced environment variables

These are mainly useful for isolated local runs, CI, or integration tests.

```bash
SAFE_AGENTIC_VM_NAME=safe-agentic-alt
```

- Overrides the OrbStack VM targeted by `safe-ag`.
- Useful when you want the CLI to operate against a dedicated test VM instead of the default `safe-agentic`.

```bash
SAFE_AGENTIC_INTEGRATION_VM=safe-agentic-alt
SAFE_AGENTIC_DEEP_INTEGRATION=1
```

- `SAFE_AGENTIC_INTEGRATION_VM` tells the integration harness which VM to target.
- `SAFE_AGENTIC_DEEP_INTEGRATION=1` enables the heaviest live cases:
  - dashboard HTTP
  - config/auth injection
  - AWS injection
  - Docker mode validation
  - live fleet/pipeline execution
  - Claude/Codex startup probes

## Troubleshooting

### `No SSH_AUTH_SOCK in VM`

```bash
safe-ag diagnose
safe-ag vm start
```

Verify 1Password SSH agent is enabled on macOS.

### `Image not found`

```bash
safe-ag update
```

### OAuth hangs

Wait for the device code prompt. Use `--reuse-auth` to persist the session.
