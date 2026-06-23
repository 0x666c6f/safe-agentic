# CLI Reference

This page is the exhaustive command reference for `safe-ag` as it exists today.

Conventions used below:
- `<required>` means a required positional argument
- `[optional]` means an optional positional argument
- `--latest` means "target the most recently started container"

## Top-level

```bash
safe-ag [command]
safe-ag [command] --help
```

Global flags:

| Flag | Meaning |
|---|---|
| `-h`, `--help` | show help |
| `-v`, `--version` | print version |

Top-level commands:

| Command | Purpose |
|---|---|
| `action` | run configured project or user actions inside agents |
| `attach` | attach to an agent tmux session |
| `audit` | show audit log entries |
| `aws-refresh` | refresh AWS credentials in a running container |
| `browser` | capture browser verification artifacts |
| `checkpoint` | manage workspace snapshots |
| `cleanup` | remove containers, networks, and optional auth volumes |
| `config` | manage persistent defaults |
| `cost` | estimate API cost from session data |
| `cron` | manage scheduled jobs |
| `diagnose` | run environment health checks |
| `diff` | show git diff from an agent workspace |
| `fleet` | spawn agents from a fleet manifest |
| `handoff` | copy or locate an agent workspace for handoff |
| `inbox` | show events that may need attention |
| `list` | list agent containers |
| `logs` | show session conversation logs |
| `mcp-login` | authenticate an MCP service |
| `output` | show agent output or derived views |
| `peek` | show the latest visible output |
| `pipeline` | run staged pipelines |
| `pr` | create a GitHub PR from agent work |
| `pr-fix` | fix review feedback on the current or given PR |
| `pr-review` | run a one-shot PR review workflow |
| `profile` | run reusable agent profiles |
| `replay` | replay a session event log |
| `retry` | retry a failed agent with the same config |
| `review` | run an AI review over the diff |
| `review-comments` | store local file/line review comments |
| `run` | quick-start wrapper around `spawn` |
| `search` | search agent session logs |
| `server` | serve safe-agentic state over JSON protocol |
| `sessions` | export session data |
| `setup` | initialize VM and build the image |
| `spawn` | start a new agent container |
| `steer` | send a follow-up message into an agent tmux session |
| `stop` | stop agent containers |
| `summary` | show a compact agent summary |
| `template` | manage prompt templates |
| `timeline` | show recent events and audit entries |
| `todo` | manage merge-gate todos |
| `tui` | launch the terminal UI |
| `update` | rebuild the image |
| `vm` | manage the OrbStack VM |
| `workspace` | stage, unstage, or revert files in an agent workspace |
| `worktree` | manage host worktrees created by `--worktree` |

## Container-targeting convention

Many commands take one of these forms:

```bash
safe-ag <command> <name>
safe-ag <command> --latest
```

Commands in this family:
- `attach`
- `aws-refresh`
- `cost`
- `diff`
- `logs`
- `output`
- `peek`
- `pr`
- `replay`
- `retry`
- `review`
- `sessions`
- `stop`
- `summary`
- most `checkpoint` and `todo` subcommands

## `spawn`

Usage:

```bash
safe-ag spawn <claude|codex|shell> [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--auto-trust` | bool | skip the trust prompt |
| `--aws` | string | AWS profile for credential injection |
| `--background` | bool | run detached instead of attaching |
| `--allow-setup-scripts` | bool | allow repo-provided `safe-agentic.json` setup hooks |
| `--cpus` | string | CPU limit |
| `--docker` | bool | enable Docker-in-Docker |
| `--docker-socket` | bool | mount the VM Docker socket directly |
| `--dry-run` | bool | print the resolved launch command only; sensitive env and labels are redacted |
| `--ephemeral-auth` | bool | use a per-container auth volume |
| `--fleet-volume` | string | shared fleet volume name |
| `--identity` | string | git identity in `Name <email>` form |
| `--instructions` | string | task instructions |
| `--instructions-file` | string | read instructions from a file |
| `--max-cost` | string | kill if estimated cost exceeds this budget |
| `--memory` | string | memory limit, e.g. `8g` |
| `--name` | string | explicit container name suffix |
| `--no-docker` | bool | disable default Docker-in-Docker |
| `--no-docker-socket` | bool | disable default host Docker socket |
| `--no-reuse-auth` | bool | disable default shared auth volume |
| `--no-reuse-gh-auth` | bool | disable default GitHub CLI auth reuse |
| `--no-seed-auth` | bool | disable default host auth seeding |
| `--no-ssh` | bool | disable default SSH agent forwarding |
| `--network` | string | custom Docker network |
| `--notify` | string | notification target string |
| `--on-complete` | string | command to run on success |
| `--on-exit` | string | command to run on exit |
| `--on-fail` | string | command to run on failure |
| `--pids-limit` | int | PIDs limit, minimum 64 |
| `--prompt` | string | initial prompt |
| `--repo` | strings | repository URL to clone; repeatable |
| `--reuse-auth` | bool | reuse shared auth volume |
| `--reuse-gh-auth` | bool | reuse GitHub CLI auth |
| `--seed-auth` | bool | copy host Claude/Codex auth into this session |
| `--ssh` | bool | enable SSH agent forwarding |
| `--template` | string | prompt template name |
| `--var` | strings | template variable assignment `key=value`; repeatable |
| `--worktree` | bool | create and mount a managed host git worktree from the current checkout |
| `--worktree-branch` | string | branch name for `--worktree` |
| `--worktree-include` | string | include file for ignored local files; default `.safe-aginclude` |
| `--worktree-path` | string | destination path for `--worktree` |

Worktree mode:

```bash
safe-ag spawn claude --worktree --name auth-fix --prompt "Fix auth tests"
```

`--worktree` must run from inside a git checkout and cannot be combined with `--repo`. It creates a branch under `safe-ag/<container>` by default, bind-mounts that checkout at `/workspace`, and copies ignored local files listed in `.safe-aginclude`.

Spawn policy:

```toml
# ~/.safe-ag/rules.toml or .safe-ag/rules.toml
[allow]
docker_modes = ["off", "dind"]
networks = ["managed"]
aws_profiles = ["dev"]
ssh = false
reuse_auth = false
reuse_gh_auth = false
seed_auth = false
setup_scripts = false
```

Policy is enforced after config defaults are applied and before network/container creation. User and nearest project rules both apply; any deny blocks the spawn. Omitted keys are unrestricted.

## `run`

Usage:

```bash
safe-ag run <repo-url> [repo-url...] [prompt] [flags]
```

`run` is a convenience wrapper around `spawn`.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--allow-setup-scripts` | bool | allow repo-provided `safe-agentic.json` setup hooks |
| `--background` | bool | run detached |
| `--cpus` | string | CPU limit |
| `--dry-run` | bool | print the resolved launch command only; sensitive env and labels are redacted |
| `--instructions` | string | task instructions |
| `--max-cost` | string | cost budget |
| `--no-docker` | bool | disable default Docker-in-Docker |
| `--no-docker-socket` | bool | disable default host Docker socket |
| `--no-reuse-auth` | bool | disable default shared auth volume |
| `--no-reuse-gh-auth` | bool | disable default GitHub CLI auth reuse |
| `--no-seed-auth` | bool | disable default host auth seeding |
| `--no-ssh` | bool | disable default SSH agent forwarding |
| `--memory` | string | memory limit |
| `--name` | string | container name |
| `--network` | string | custom Docker network |
| `--seed-auth` | bool | copy host Claude/Codex auth into this session |
| `--template` | string | prompt template |
| `--var` | strings | template variable assignment `key=value`; repeatable |

## `list`

Usage:

```bash
safe-ag list [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--json` | bool | output raw JSON-like line format from Docker listing |

## `action`

Usage:

```bash
safe-ag action list
safe-ag action show <name>
safe-ag action run <name> [agent|--latest]
```

Actions are loaded from `~/.safe-ag/actions.toml`, then `.safe-ag/actions.toml` in the current directory. Project actions override user actions with the same name.

Schema:

```toml
[actions.test]
description = "Run unit tests"
command = "go test ./..."

[actions.lint]
command = "npm run lint"
cwd = "frontend"
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--file` | strings | additional actions.toml file; repeatable |
| `--latest` | bool | for `action run`, target the latest container |

## `profile`

Usage:

```bash
safe-ag profile list
safe-ag profile show <name>
safe-ag profile run <name> [prompt]
```

Profiles are loaded from `~/.safe-ag/agents/*.toml`, then `.safe-ag/agents/*.toml` in the current directory. Project profiles override user profiles with the same name.

Schema:

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

Useful fields mirror spawn flags: `template`, `template_vars`, `instructions`, `network`, `memory`, `cpus`, `pids_limit`, `aws`, `max_cost`, `docker`, `docker_socket`, `seed_auth`, `auto_trust`, and lifecycle callbacks.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--dir` | strings | additional profile directory; repeatable |
| `--dry-run` | bool | for `profile run`, show the resolved spawn command |

## `search`

Usage:

```bash
safe-ag search <query> [agent|--latest]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--case-sensitive` | bool | use case-sensitive matching |
| `--latest` | bool | target the latest container |
| `--lines` | int | session lines to scan per agent; default `500` |

## `server`

Usage:

```bash
safe-ag server --stdio
SAFE_AGENTIC_SERVER_TOKEN=secret safe-ag server --listen 127.0.0.1:8765
```

Reads newline-delimited JSON requests from stdin and writes JSON responses to stdout. With `--listen`, accepts authenticated `POST /rpc` requests with `Authorization: Bearer <token>`.

Methods: `schema`, `ping`, `timeline`, `inbox`, `agents.list`, `agent.logs`, `agent.diff`, `actions.list`, and `actions.run`.

Example:

```json
{"jsonrpc":"2.0","id":1,"method":"timeline","params":{"lines":20}}
```

## `browser`

Usage:

```bash
safe-ag browser capture <url> [--mode auto|http|chrome] [--annotation NOTE] [--out DIR] [--timeout 30s]
```

Captures browser artifacts under `~/.safe-ag/state/browser/<timestamp>` by default. `http` mode captures DOM and headers. `chrome` mode uses headless Chrome/CDP to capture DOM, screenshot, console, and network artifacts. `--annotation` writes notes into `annotations.json` for handoff to agents. `auto` tries Chrome when available, then falls back to HTTP. It does not mount or reuse host browser profiles or cookies.

## `attach`

Usage:

```bash
safe-ag attach <name|--latest> [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |

## `steer`

Usage:

```bash
safe-ag steer <name|--latest> "follow-up message"
```

If the target container is stopped, `steer` starts it first and waits for the tmux session.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |

## `peek`

Usage:

```bash
safe-ag peek [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |
| `--lines` | int | number of lines to show; default `30` |

## `logs`

Usage:

```bash
safe-ag logs [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--follow`, `-f` | bool | follow log output |
| `--latest` | bool | target the latest container |
| `--lines` | int | number of log entries; default `50` |

## `summary`

Usage:

```bash
safe-ag summary [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |

## `output`

Usage:

```bash
safe-ag output [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--commits` | bool | show git commit log |
| `--diff` | bool | show git diff |
| `--files` | bool | show changed files |
| `--json` | bool | emit JSON |
| `--latest` | bool | target the latest container |

## `diff`

Usage:

```bash
safe-ag diff [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--stat` | bool | show diffstat only |

## `review`

Usage:

```bash
safe-ag review [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--base` | string | base branch for diff; default `main` |

## `review-comments`

Usage:

```bash
safe-ag review-comments list [agent|--latest]
safe-ag review-comments add [agent|--latest] <file> <line> <body>
safe-ag review-comments resolve <id>
safe-ag review-comments clear <agent|--latest>
```

Comments are stored locally in `~/.safe-ag/state/review-comments.jsonl`. Use them to keep file/line review notes attached to an agent while you steer fixes or prepare handoff.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--all` | bool | for `list`, include resolved comments |
| `--file` | string | override the review-comments storage file |
| `--latest` | bool | target the latest container |

## `handoff`

Usage:

```bash
safe-ag handoff <agent|--latest> --to-local ./workspace-copy
safe-ag handoff <agent|--latest> --to-worktree
```

`--to-local` copies `/workspace` out of the container. `--to-worktree` prints the managed host worktree path for agents spawned with `--worktree`.

## `workspace`

Usage:

```bash
safe-ag workspace stage <agent|--latest> <path...>
safe-ag workspace unstage <agent|--latest> <path...>
safe-ag workspace revert <agent|--latest> <path...> --yes
safe-ag workspace stage-patch <agent|--latest> selected.patch
safe-ag workspace revert-patch <agent|--latest> selected.patch --yes
```

Paths must stay relative to the workspace. `revert` and `revert-patch` discard changes and require `--yes` when stdin is not interactive. Patch commands accept selected hunks from a normal unified diff and reject workspace-escaping paths.

## `worktree`

Usage:

```bash
safe-ag worktree list
safe-ag worktree snapshot <agent|--latest> [label]
safe-ag worktree restore <agent|--latest> <stash-ref>
safe-ag worktree cleanup [--dry-run] [--all]
```

`list` reads `~/.safe-ag/state/worktrees.jsonl`. `snapshot` and `restore` operate on the git worktree attached to an agent. `cleanup` drops missing registry entries by default; `--all` also removes registered worktrees with `git worktree remove --force`.

## `timeline`

Usage:

```bash
safe-ag timeline
```

Shows recent events from `~/.safe-ag/state/events.jsonl` and audit entries from `~/.safe-ag/state/audit.jsonl`.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--lines` | int | number of recent entries; default `50` |

## `inbox`

Usage:

```bash
safe-ag inbox
safe-ag inbox --all
```

Shows events likely to need attention, such as failed cron jobs or entries marked `needs-auth`, `stuck`, `failed-tests`, `ready-for-review`, or `ready-for-pr`.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--all` | bool | include informational entries |

## `pr`

Usage:

```bash
safe-ag pr [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--base` | string | base branch; default `main` |
| `--title` | string | PR title |

## `retry`

Usage:

```bash
safe-ag retry <name|--latest> [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--feedback` | string | additional guidance appended to the retry prompt |

## `replay`

Usage:

```bash
safe-ag replay [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |
| `--tools-only` | bool | show only tool calls |

## `sessions`

Usage:

```bash
safe-ag sessions [name|--latest] [dest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |

## `aws-refresh`

Usage:

```bash
safe-ag aws-refresh [name|--latest] [profile] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |

## `cost`

Usage:

```bash
safe-ag cost [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--history` | string | show historical costs, e.g. `7d`, `30d` |
| `--latest` | bool | target the latest container |

## `audit`

Usage:

```bash
safe-ag audit [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--lines` | int | number of entries to show; default `50` |

## `checkpoint`

Subcommands:

### `checkpoint create`

```bash
safe-ag checkpoint create <name|--latest> [label]
```

### `checkpoint list`

```bash
safe-ag checkpoint list <name|--latest>
```

### `checkpoint restore`

```bash
safe-ag checkpoint restore <name|--latest> <ref>
```

No additional flags beyond `--help`.

## `todo`

Subcommands:

### `todo add`

```bash
safe-ag todo add <name|--latest> <text>
```

### `todo list`

```bash
safe-ag todo list <name|--latest>
```

### `todo check`

```bash
safe-ag todo check <name|--latest> <index>
```

### `todo uncheck`

```bash
safe-ag todo uncheck <name|--latest> <index>
```

No additional flags beyond `--help`.

## `fleet`

Usage:

```bash
safe-ag fleet <manifest.yaml> [flags]
safe-ag fleet status
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--dry-run` | bool | print what would run without executing |
| `--repo` | strings | default repo URL for agents missing `repo` or `repos` |
| `--var` | strings | manifest variable assignment `key=value`; repeatable |

Subcommands:

### `fleet status`

```bash
safe-ag fleet status
```

## `pipeline`

Usage:

```bash
safe-ag pipeline <pipeline.yaml|name> [flags]
safe-ag pipeline list
safe-ag pipeline show <name>
safe-ag pipeline inspect <name>
safe-ag pipeline render <name>
safe-ag pipeline validate <name>
safe-ag pipeline create <name>
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--background` | bool | run the pipeline in the background and return immediately |
| `--dry-run` | bool | print the execution plan without running |
| `--repo` | strings | default repo URL for agents missing `repo` or `repos` |
| `--var` | strings | manifest variable assignment `key=value`; repeatable |

Saved user pipelines live in `~/.safe-ag/pipelines/`. Built-in review presets ship under the same catalog surface.

## `config`

Subcommands:

### `config show`

```bash
safe-ag config show
```

Reads `~/.safe-ag/config.toml`.
Set `SAFE_AGENTIC_CONFIG_HOME` to read from another safe-agentic config home
without changing the process `HOME`.

### `config get`

```bash
safe-ag config get <key>
```

Examples:

```bash
safe-ag config get defaults.memory
safe-ag config get SAFE_AGENTIC_DEFAULT_MEMORY
```

### `config set`

```bash
safe-ag config set <key> <value>
```

Examples:

```bash
safe-ag config set defaults.memory 16g
safe-ag config set defaults.identity "Your Name <you@example.com>"
```

### `config reset`

```bash
safe-ag config reset <key>
```

No additional flags beyond `--help`.

## `template`

Subcommands:

### `template list`

```bash
safe-ag template list
```

User templates live in `~/.safe-ag/templates/`.

### `template show`

```bash
safe-ag template show <name>
```

### `template render`

```bash
safe-ag template render <name>
```

### `template create`

```bash
safe-ag template create <name>
```

No additional flags beyond `--help`.

## `pipeline` saved catalog

Saved user pipelines live in `~/.safe-ag/pipelines/`.

## `pr-review`

Usage:

```bash
safe-ag pr-review [claude|codex|dual] [pr]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--dry-run` | bool | print the resolved review pipeline without running |
| `--repo` | strings | default repo URL; inferred from current checkout when omitted |
| `--var` | strings | workflow variable assignment `key=value`; repeatable |

Behavior:
- defaults to `dual`
- infers current PR via `gh pr view --json number` when omitted
- runs one-shot review presets without the watcher loop

## `pr-fix`

Usage:

```bash
safe-ag pr-fix [pr]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--dry-run` | bool | print the resolved fix pipeline without running |
| `--repo` | strings | default repo URL; inferred from current checkout when omitted |
| `--var` | strings | workflow variable assignment `key=value`; repeatable |

## `mcp-login`

Usage:

```bash
safe-ag mcp-login <service> [container]
```

No additional flags beyond `--help`.

## `cron`

Subcommands:

### `cron add`

```bash
safe-ag cron add <name> <schedule> <command...>
```

Accepted schedule styles:
- `every 1h`
- `every 6h`
- `every 30m`
- `daily 09:00`
- standard cron expressions like `0 */6 * * *`

### `cron list`

```bash
safe-ag cron list
```

### `cron remove`

```bash
safe-ag cron remove <name>
```

### `cron enable`

```bash
safe-ag cron enable <name>
```

### `cron disable`

```bash
safe-ag cron disable <name>
```

### `cron run`

```bash
safe-ag cron run <name>
```

### `cron daemon`

```bash
safe-ag cron daemon
```

No additional flags beyond `--help`.

## `tui`

Usage:

```bash
safe-ag tui
```

No command-specific flags beyond `--help`.

See [TUI Reference](tui.md) for keybindings, modes, and interaction model.

## `setup`

Usage:

```bash
safe-ag setup
```

No command-specific flags beyond `--help`.

## `update`

Usage:

```bash
safe-ag update [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--full` | bool | full rebuild without cache |
| `--quick` | bool | bust only the AI CLI layer |

## `diagnose`

Usage:

```bash
safe-ag diagnose
```

No command-specific flags beyond `--help`.

## `cleanup`

Usage:

```bash
safe-ag cleanup [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--auth` | bool | also remove shared and isolated auth volumes |

## `stop`

Usage:

```bash
safe-ag stop <name|--latest|--all> [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--all` | bool | stop and remove all agent containers |
| `--latest` | bool | target the latest container |

## `vm`

Subcommands:

### `vm start`

```bash
safe-ag vm start
```

### `vm stop`

```bash
safe-ag vm stop
```

### `vm ssh`

```bash
safe-ag vm ssh
```

No additional flags beyond `--help`.
