# Configuration

Configuration falls into four buckets:
- persistent defaults
- templates
- auth helpers
- VM/image maintenance

## Defaults file

Path:

```bash
${XDG_CONFIG_HOME:-~/.config}/safe-agentic/defaults.sh
```

Format:

```bash
SAFE_AGENTIC_DEFAULT_MEMORY=16g
SAFE_AGENTIC_DEFAULT_CPUS=8
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=false
SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH=false
SAFE_AGENTIC_DEFAULT_SSH=false
SAFE_AGENTIC_DEFAULT_NETWORK=my-net
SAFE_AGENTIC_DEFAULT_IDENTITY="Your Name <you@example.com>"
```

This is parsed as `KEY=value`, not sourced as a shell script.

## `safe-ag config`

```bash
safe-ag config show
safe-ag config get SAFE_AGENTIC_DEFAULT_MEMORY
safe-ag config set SAFE_AGENTIC_DEFAULT_MEMORY 16g
safe-ag config set SAFE_AGENTIC_DEFAULT_IDENTITY "Your Name <you@example.com>"
safe-ag config reset SAFE_AGENTIC_DEFAULT_MEMORY
```

## Templates

```bash
safe-ag template list
safe-ag template show security-audit
safe-ag template create backend-audit
```

Use templates at spawn time:

```bash
safe-ag spawn claude --repo ... --template security-audit
```

## Auth helpers

MCP auth:

```bash
safe-ag mcp-login linear
safe-ag mcp-login notion
safe-ag mcp-login linear <container>
```

AWS refresh:

```bash
safe-ag aws-refresh --latest
safe-ag aws-refresh api-refactor my-profile
```

## VM and image maintenance

```bash
safe-ag setup
safe-ag update
safe-ag update --quick
safe-ag update --full
safe-ag vm start
safe-ag vm stop
safe-ag vm ssh
safe-ag diagnose
```

Use:
- `setup` for first run
- `update` after Dockerfile/image changes
- `vm start` if the VM was stopped and you want hardening re-applied

## Advanced environment variables

```bash
SAFE_AGENTIC_VM_NAME=safe-agentic-alt
```

This points the CLI at a different OrbStack VM.

Useful for:
- isolated testing
- dedicated work VMs
- integration harnesses
