# Configuration

Configuration falls into four buckets:
- persistent defaults
- templates
- policy rules
- actions
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
~/.safe-ag/rules.toml
~/.safe-ag/templates/
~/.safe-ag/actions.toml
~/.safe-ag/pipelines/
~/.safe-ag/cron.json
~/.safe-ag/state/
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
seed_auth = false
ssh = false
docker = false
docker_socket = false
network = "my-net"
identity = "Your Name <you@example.com>"
```

These defaults apply to `safe-ag spawn` and `safe-ag run`.

Risk-widening defaults still have per-command opt-outs:

```bash
safe-ag spawn claude --no-ssh --no-reuse-auth --repo https://github.com/org/repo.git
safe-ag spawn claude --no-docker --no-reuse-gh-auth --no-seed-auth --repo https://github.com/org/repo.git
safe-ag spawn claude --ephemeral-auth --repo https://github.com/org/repo.git
```

Use `--ephemeral-auth` or `--no-reuse-auth` to override `reuse_auth = true` for one session. Use `--no-seed-auth` to override `seed_auth = true`.

## `safe-ag config`

```bash
safe-ag config show
safe-ag config get defaults.memory
safe-ag config set defaults.memory 16g
safe-ag config set defaults.identity "Your Name <you@example.com>"
safe-ag config reset defaults.memory
```

Legacy env-style keys still work as aliases for `get`, `set`, and `reset`.

## Policy rules

Policy rules are hard spawn-time guards. They are checked after defaults are applied and before safe-agentic creates networks, worktrees, or containers.

Locations:

```bash
~/.safe-ag/rules.toml
.safe-ag/rules.toml
```

User rules and the nearest project rules both apply. Project rules cannot weaken user rules.

Example:

```toml
[allow]
docker_modes = ["off", "dind"]       # off, dind, host-socket
networks = ["managed", "none"]       # managed, none, or a custom network name
aws_profiles = ["dev"]
ssh = false
reuse_auth = false
reuse_gh_auth = false
seed_auth = false
setup_scripts = false
```

Absent keys mean no extra restriction. Boolean `false` denies that capability when a spawn would enable it.

Common use:

```toml
[allow]
docker_modes = ["off"]
networks = ["managed"]
ssh = false
reuse_auth = false
seed_auth = false
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
safe-ag spawn claude --repo ... --template security-audit --var area=payments
```

If `--repo` is omitted, `safe-ag` tries to infer `${repo}` from the current checkout's `origin` remote.

## Agent profiles

Profiles are reusable spawn presets for roles you run often.

Locations:

```bash
~/.safe-ag/agents/*.toml
.safe-ag/agents/*.toml
```

Project profiles override user profiles with the same filename or `name`.

Example:

```toml
agent_type = "codex"
repo = ["git@github.com:org/repo.git"]
container_name = "reviewer"
prompt = "Review this repo and report actionable issues"
ssh = true
reuse_auth = true
reuse_gh_auth = true
background = true
```

Run:

```bash
safe-ag profile list
safe-ag profile show reviewer
safe-ag profile run reviewer "focus auth code"
safe-ag profile run reviewer --dry-run
```

## Actions

Actions are named commands you can run inside an agent workspace. They mirror Codex app local actions while keeping execution inside the safe-agentic container.

Locations:

```bash
~/.safe-ag/actions.toml
.safe-ag/actions.toml
```

Project actions override user actions with the same name.

Example:

```toml
[actions.test]
description = "Run Go tests"
command = "go test ./..."

[actions.frontend-lint]
command = "npm run lint"
cwd = "frontend"
```

Run:

```bash
safe-ag action list
safe-ag action run test --latest
safe-ag action run frontend-lint my-agent
```

## Pipelines

```bash
safe-ag pipeline list
safe-ag pipeline show review
safe-ag pipeline create review
safe-ag pipeline review --repo git@github.com:org/repo.git
safe-ag pipeline review --repo git@github.com:org/repo.git --var topic=security
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
