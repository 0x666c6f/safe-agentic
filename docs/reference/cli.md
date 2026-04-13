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
| `attach` | attach to an agent tmux session |
| `audit` | show audit log entries |
| `aws-refresh` | refresh AWS credentials in a running container |
| `checkpoint` | manage workspace snapshots |
| `cleanup` | remove containers, networks, and optional auth volumes |
| `config` | manage persistent defaults |
| `cost` | estimate API cost from session data |
| `cron` | manage scheduled jobs |
| `dashboard` | start the web dashboard |
| `diagnose` | run environment health checks |
| `diff` | show git diff from an agent workspace |
| `fleet` | spawn agents from a fleet manifest |
| `list` | list agent containers |
| `logs` | show session conversation logs |
| `mcp-login` | authenticate an MCP service |
| `output` | show agent output or derived views |
| `peek` | show the latest visible output |
| `pipeline` | run staged pipelines |
| `pr` | create a GitHub PR from agent work |
| `replay` | replay a session event log |
| `retry` | retry a failed agent with the same config |
| `review` | run an AI review over the diff |
| `run` | quick-start wrapper around `spawn` |
| `sessions` | export session data |
| `setup` | initialize VM and build the image |
| `spawn` | start a new agent container |
| `stop` | stop agent containers |
| `summary` | show a compact agent summary |
| `template` | manage prompt templates |
| `todo` | manage merge-gate todos |
| `tui` | launch the terminal dashboard |
| `update` | rebuild the image |
| `vm` | manage the OrbStack VM |

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
| `--cpus` | string | CPU limit |
| `--docker` | bool | enable Docker-in-Docker |
| `--docker-socket` | bool | mount the VM Docker socket directly |
| `--dry-run` | bool | print the resolved launch command only |
| `--ephemeral-auth` | bool | use a per-container auth volume |
| `--fleet-volume` | string | shared fleet volume name |
| `--identity` | string | git identity in `Name <email>` form |
| `--instructions` | string | task instructions |
| `--instructions-file` | string | read instructions from a file |
| `--max-cost` | string | kill if estimated cost exceeds this budget |
| `--memory` | string | memory limit, e.g. `8g` |
| `--name` | string | explicit container name suffix |
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
| `--ssh` | bool | enable SSH agent forwarding |
| `--template` | string | prompt template name |

## `run`

Usage:

```bash
safe-ag run <repo-url> [repo-url...] [prompt] [flags]
```

`run` is a convenience wrapper around `spawn`.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--background` | bool | run detached |
| `--cpus` | string | CPU limit |
| `--dry-run` | bool | dry run |
| `--instructions` | string | task instructions |
| `--max-cost` | string | cost budget |
| `--memory` | string | memory limit |
| `--name` | string | container name |
| `--network` | string | custom Docker network |
| `--template` | string | prompt template |

## `list`

Usage:

```bash
safe-ag list [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--json` | bool | output raw JSON-like line format from Docker listing |

## `attach`

Usage:

```bash
safe-ag attach <name|--latest> [flags]
```

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

### `checkpoint revert`

```bash
safe-ag checkpoint revert <name|--latest> <ref>
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

Subcommands:

### `fleet status`

```bash
safe-ag fleet status
```

## `pipeline`

Usage:

```bash
safe-ag pipeline <pipeline.yaml> [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--dry-run` | bool | print the execution plan without running |

## `config`

Subcommands:

### `config show`

```bash
safe-ag config show
```

### `config get`

```bash
safe-ag config get <key>
```

### `config set`

```bash
safe-ag config set <key> <value>
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

### `template show`

```bash
safe-ag template show <name>
```

### `template create`

```bash
safe-ag template create <name>
```

No additional flags beyond `--help`.

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

## `dashboard`

Usage:

```bash
safe-ag dashboard [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--bind` | string | bind address; default `localhost:8420` |

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
| `--auth` | bool | also remove shared auth volumes |

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
