# Configuration

Everything berth reads from disk: persistent defaults, hard policy rules, prompt templates, agent profiles, actions, and where all of it lives.

## Config home

```text
~/.berth/
├── config.toml      # persistent defaults (berth config)
├── rules.toml       # hard policy limits
├── templates/       # custom prompt templates
├── agents/          # agent profiles (*.toml)
├── actions.toml     # named workspace commands
├── pipelines/       # saved pipelines
├── cron.json        # scheduled jobs (berth cron)
└── state/           # audit log, events, judge verdicts, browser captures
```

Projects can carry their own `.berth/` directory (`rules.toml`, `actions.toml`, `agents/`) plus a `berth.json` with optional setup scripts and a `.berthinclude` list for [worktrees](worktrees.md).

Relocate the tree without changing `HOME`:

```bash
BERTH_CONFIG_HOME=/tmp/berth-home berth list   # config + state
BERTH_STATE_HOME=/tmp/berth-state              # state separately (defaults to config home)
```

Migrating from safe-agentic? See [Installation](../install.md#migrating-from-safe-agentic).

## Persistent defaults — `berth config`

```bash
berth config keys                     # every key with current + default value
berth config show
berth config get defaults.memory
berth config set defaults.memory 16g
berth config reset defaults.memory
```

`~/.berth/config.toml`:

```toml
version = 1

[defaults]
memory = "16g"
cpus = "8"
ssh = false
reuse_auth = false
reuse_gh_auth = false
seed_auth = false
docker = false
docker_socket = false
network = "my-net"
identity = "Your Name <you@example.com>"

[git]                                  # optional: overrides the detected host identity
author_name = "Your Name"
author_email = "you@example.com"
committer_name = "Your Name"
committer_email = "you@example.com"
```

These apply to `berth spawn` and `berth run`. Legacy env-style keys (`BERTH_DEFAULT_*`, `GIT_*`) work as aliases in `get`/`set`/`reset`. Risk-widening defaults keep per-session opt-outs:

```bash
berth spawn claude --no-ssh --no-reuse-auth --repo https://github.com/org/repo.git
berth spawn claude --ephemeral-auth --repo https://github.com/org/repo.git
```

## Policy rules

Hard spawn-time guards, checked after defaults are applied and **before** berth creates networks, worktrees, or containers. Unlike config defaults, no flag can override them.

Locations: `~/.berth/rules.toml` (user) and `.berth/rules.toml` (project). Both apply; project rules cannot weaken user rules.

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

## Templates

Reusable prompts with `${var}` placeholders:

```bash
berth template list                   # built-in + user templates
berth template show security-audit
berth template create backend-audit  # starter .md, opens $EDITOR
berth template render security-audit --var area=payments
```

Use at spawn time:

```bash
berth spawn claude --repo ... --template security-audit --var area=payments
```

`${repo}` is inferred from the current checkout's `origin` when `--repo` is omitted.

## Agent profiles

Reusable spawn presets for roles you run often. Locations: `~/.berth/agents/*.toml` and `.berth/agents/*.toml` (project wins on name collision).

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

```bash
berth profile list
berth profile show reviewer
berth profile run reviewer "focus auth code"
berth profile run reviewer --dry-run
```

Profiles also plug into fleet/pipeline manifests via `profile:` — see [Manifests](../reference/manifests.md).

## Actions

Named commands you can run inside an agent workspace (Codex-app-style local actions, executed in the container). Locations: `~/.berth/actions.toml` and `.berth/actions.toml`.

```toml
[actions.test]
description = "Run Go tests"
command = "go test ./..."

[actions.frontend-lint]
command = "npm run lint"
cwd = "frontend"
```

```bash
berth action list
berth action run test --latest
berth action run frontend-lint my-agent
```

## Scheduled jobs

`~/.berth/cron.json` is managed by `berth cron add/list/remove/...` — see [Automation](automation.md#scheduled-runs-berth-cron).

## Auth helpers

```bash
berth mcp-login linear                # MCP OAuth inside the newest agent
berth mcp-login notion <container>
berth aws-refresh --latest            # re-inject fresh AWS credentials
berth aws-refresh api-refactor my-profile
```

## VM and image maintenance

```bash
berth setup             # first run; safe to re-run
berth update            # rebuild image (cached) / --quick / --full
berth vm start          # start VM + re-apply hardening and NAT
berth vm stop
berth vm ssh            # debug shell in the VM
berth diagnose          # health-check VM, egress, image, worktree posture
```

## Environment variables

| Variable | Effect |
|---|---|
| `BERTH_VM_NAME` | point the CLI at a different Apple container machine |
| `BERTH_CONFIG_HOME` | relocate `~/.berth` |
| `BERTH_STATE_HOME` | relocate state files separately |
| `BERTH_SERVER_TOKEN` | bearer token for [`berth server --listen`](automation.md#machine-readable-state-berth-server) |

Useful for isolated testing, dedicated work VMs, and integration harnesses.
