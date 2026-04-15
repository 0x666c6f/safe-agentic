# Configuration

Configuration falls into four buckets:
- persistent defaults
- templates
- pipelines
- auth helpers
- VM/image maintenance

## Config home

Path:

```bash
~/.safe-ag/
```

Layout:

```bash
~/.safe-ag/config.toml
~/.safe-ag/templates/
~/.safe-ag/pipelines/
~/.safe-ag/cron.json
~/.safe-ag/state/
```

Runtime state currently lives under:

```bash
~/.safe-ag/state/audit.jsonl
~/.safe-ag/state/events.jsonl
~/.safe-ag/state/pipelines/
```

## Preferences file

Path:

```bash
~/.safe-ag/config.toml
```

Format:

```toml
version = 1

[defaults]
memory = "16g"
cpus = "8"
reuse_auth = false
reuse_gh_auth = false
ssh = false
network = "my-net"
identity = "Your Name <you@example.com>"
```

## `safe-ag config`

```bash
safe-ag config show
safe-ag config get defaults.memory
safe-ag config set defaults.memory 16g
safe-ag config set defaults.identity "Your Name <you@example.com>"
safe-ag config reset defaults.memory
```

Legacy env-style keys still work as aliases for `get`, `set`, and `reset`.

## Templates

```bash
safe-ag template list
safe-ag template show security-audit
safe-ag template render security-audit
safe-ag template create backend-audit
```

Use templates at spawn time:

```bash
safe-ag spawn claude --repo ... --template security-audit
safe-ag spawn claude --repo ... --template security-audit --var area=payments
```

If `--repo` is omitted, `safe-ag` tries to infer `${repo}` from the current checkout's `origin` remote.

Templates can also declare explicit metadata in YAML front matter:
- description
- inputs
- examples
- tags

## Pipelines

```bash
safe-ag pipeline list
safe-ag pipeline show review
safe-ag pipeline inspect review
safe-ag pipeline render review
safe-ag pipeline validate review
safe-ag pipeline create review
safe-ag pipeline review --repo git@github.com:org/repo.git
safe-ag pipeline review --repo git@github.com:org/repo.git --var topic=security
```

Pipelines can declare:
- description
- inputs
- examples
- tags

Review presets ship as built-ins under `reviews/` and can be overridden in your own config home.

```bash
safe-ag pr-review
safe-ag pr-review claude
safe-ag pr-review codex
safe-ag pr-fix
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
