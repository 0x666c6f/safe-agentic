# CLI Reference

This page is the command-oriented reference for `safe-ag`.

For task-based guidance, use:
- [Quickstart](../quickstart.md)
- [Command Map](../usage.md)
- [Workflow](../guide/workflow.md)

## Top-level usage

```bash
safe-ag [command]
safe-ag [command] --help
```

Global flags:

| Flag | Meaning |
|---|---|
| `-h`, `--help` | show help |
| `-v`, `--version` | print version |

## Command list

| Command | Purpose |
|---|---|
| `attach` | attach to an agent tmux session |
| `audit` | show audit log entries |
| `aws-refresh` | refresh AWS credentials in a running container |
| `checkpoint` | create/list/revert workspace snapshots |
| `cleanup` | remove containers, networks, and optional auth volumes |
| `config` | manage persistent defaults |
| `cost` | estimate API cost from session data |
| `cron` | manage scheduled jobs |
| `dashboard` | start the web dashboard |
| `diagnose` | run environment health checks |
| `diff` | show git diff from an agent workspace |
| `fleet` | spawn agents from a fleet manifest |
| `list` | list agent containers |
| `logs` | show conversation/session logs |
| `mcp-login` | authenticate an MCP service |
| `output` | show agent output or derived views |
| `peek` | view the latest visible agent output |
| `pipeline` | run staged pipeline manifests |
| `pr` | create a GitHub PR from agent work |
| `replay` | replay a session event log |
| `retry` | retry a failed agent with the same config |
| `review` | run an AI review over the agent diff |
| `run` | quick-start wrapper around `spawn` |
| `sessions` | export session data |
| `setup` | initialize VM and build image |
| `spawn` | start a new agent container |
| `stop` | stop agent containers |
| `summary` | show a compact agent summary |
| `template` | manage prompt templates |
| `todo` | manage merge-gate todos |
| `tui` | launch the terminal dashboard |
| `update` | rebuild the Docker image |
| `vm` | manage the OrbStack VM |

## Common command patterns

Spawn:

```bash
safe-ag spawn claude --repo https://github.com/org/repo.git
safe-ag spawn claude --ssh --repo git@github.com:org/repo.git
safe-ag run git@github.com:org/repo.git "Fix the failing tests"
```

Inspect:

```bash
safe-ag list
safe-ag peek --latest
safe-ag logs --latest
safe-ag summary --latest
safe-ag output --latest
safe-ag diff --latest
safe-ag review --latest
```

Manage:

```bash
safe-ag attach --latest
safe-ag stop --latest
safe-ag cleanup
safe-ag cleanup --auth
safe-ag sessions --latest
```

Workflow:

```bash
safe-ag checkpoint create --latest "before refactor"
safe-ag todo add --latest "Run integration tests"
safe-ag retry --latest --feedback "Focus on tests only"
safe-ag pr --latest --title "fix: stabilize test suite"
```

Orchestration:

```bash
safe-ag fleet fleet.yaml
safe-ag fleet status
safe-ag pipeline pipeline.yaml
safe-ag pipeline pipeline.yaml --dry-run
```

## Help strategy

The binary already exposes subcommand help:

```bash
safe-ag spawn --help
safe-ag cleanup --help
safe-ag pipeline --help
```

Use this page for orientation, and use `--help` for exact flags on the command you are about to run.
