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
~/.berth/
```

Layout:

```bash
~/.berth/config.toml
~/.berth/rules.toml
~/.berth/templates/
~/.berth/actions.toml
~/.berth/pipelines/
~/.berth/cron.json
~/.berth/state/
```

For isolated harnesses, set `BERTH_CONFIG_HOME` to relocate this tree
without changing `HOME` for host tools:

```bash
BERTH_CONFIG_HOME=/tmp/berth-home berth list
```

State files can be relocated separately with `BERTH_STATE_HOME`.
When unset, state stays under the config home.

## Preferences file

Path:

```bash
~/.berth/config.toml
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

These defaults apply to `berth spawn` and `berth run`.

Risk-widening defaults still have per-command opt-outs:

```bash
berth spawn claude --no-ssh --no-reuse-auth --repo https://github.com/org/repo.git
berth spawn claude --no-docker --no-reuse-gh-auth --no-seed-auth --repo https://github.com/org/repo.git
berth spawn claude --ephemeral-auth --repo https://github.com/org/repo.git
```

Use `--ephemeral-auth` or `--no-reuse-auth` to override `reuse_auth = true` for one session. Use `--no-seed-auth` to override `seed_auth = true`.

## `berth config`

```bash
berth config show
berth config get defaults.memory
berth config set defaults.memory 16g
berth config set defaults.identity "Your Name <you@example.com>"
berth config reset defaults.memory
```

Legacy env-style keys still work as aliases for `get`, `set`, and `reset`.

## Policy rules

Policy rules are hard spawn-time guards. They are checked after defaults are applied and before berth creates networks, worktrees, or containers.

Locations:

```bash
~/.berth/rules.toml
.berth/rules.toml
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
berth template list
berth template show security-audit
berth template create backend-audit
```

Use templates at spawn time:

```bash
berth spawn claude --repo ... --template security-audit
berth spawn claude --repo ... --template security-audit --var area=payments
```

If `--repo` is omitted, `berth` tries to infer `${repo}` from the current checkout's `origin` remote.

## Agent profiles

Profiles are reusable spawn presets for roles you run often.

Locations:

```bash
~/.berth/agents/*.toml
.berth/agents/*.toml
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
berth profile list
berth profile show reviewer
berth profile run reviewer "focus auth code"
berth profile run reviewer --dry-run
```

## Actions

Actions are named commands you can run inside an agent workspace. They mirror Codex app local actions while keeping execution inside the berth container.

Locations:

```bash
~/.berth/actions.toml
.berth/actions.toml
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
berth action list
berth action run test --latest
berth action run frontend-lint my-agent
```

## Pipelines

```bash
berth pipeline list
berth pipeline show review
berth pipeline create review
berth pipeline review --repo git@github.com:org/repo.git
berth pipeline review --repo git@github.com:org/repo.git --var topic=security
```

## Auth helpers

MCP auth:

```bash
berth mcp-login linear
berth mcp-login notion
berth mcp-login linear <container>
```

AWS refresh:

```bash
berth aws-refresh --latest
berth aws-refresh api-refactor my-profile
```

## VM and image maintenance

```bash
berth setup
berth update
berth update --quick
berth update --full
berth vm start
berth vm stop
berth vm ssh
berth diagnose
```

Use:
- `setup` for first run
- `update` after Dockerfile/image changes
- `vm start` if the VM was stopped and you want hardening re-applied
- `diagnose` to check VM/image readiness and spot risky spawn defaults before you launch agents

## Advanced environment variables

```bash
BERTH_VM_NAME=berth-alt
BERTH_CONFIG_HOME=/tmp/berth-home
BERTH_STATE_HOME=/tmp/berth-state
```

`BERTH_VM_NAME` points the CLI at a different Apple container machine.
`BERTH_CONFIG_HOME` and `BERTH_STATE_HOME` relocate berth
files while keeping the process `HOME` intact for host tools.

Useful for:
- isolated testing
- dedicated work VMs
- integration harnesses
