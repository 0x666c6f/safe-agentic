# Command Map

This page answers: "Which command do I run for the job in front of me?"

If you want the raw reference pages instead:
- [CLI Reference](reference/cli.md)
- [TUI Reference](reference/tui.md)

## Setup and maintenance

```bash
safe-ag setup
safe-ag update
safe-ag update --quick
safe-ag update --full
safe-ag diagnose
safe-ag vm start
safe-ag vm stop
safe-ag vm ssh
```

## Spawn and connect

```bash
safe-ag spawn <claude|codex|shell> ...
safe-ag run <repo...> [prompt]
safe-ag attach <name>
safe-ag attach --latest
```

Use `spawn` when you want explicit control. Use `run` for a quick single-task session.

## See what the agent is doing

```bash
safe-ag list
safe-ag peek <name>
safe-ag logs <name>
safe-ag summary <name>
safe-ag output <name>
safe-ag diff <name>
safe-ag review <name>
```

Typical meaning:
- `peek`: last visible pane output
- `logs`: conversation/session log
- `summary`: compact state snapshot
- `output`: last useful result
- `diff`: git diff from the workspace
- `review`: review the diff

## Manage containers

```bash
safe-ag stop <name>
safe-ag stop --latest
safe-ag stop --all
safe-ag cleanup
safe-ag cleanup --auth
safe-ag sessions <name> [dest]
```

## Workflow helpers

```bash
safe-ag checkpoint create <name> [label]
safe-ag checkpoint list <name>
safe-ag checkpoint restore <name> <ref>

safe-ag todo add <name> "text"
safe-ag todo list <name>
safe-ag todo check <name> <index>
safe-ag todo uncheck <name> <index>

safe-ag retry <name> [--feedback "..."]
safe-ag pr <name> [--title ... --base ...]
```

## Auth and config

```bash
safe-ag config show
safe-ag config get <key>
safe-ag config set <key> <value>
safe-ag config reset <key>

safe-ag template list
safe-ag template show <name>
safe-ag template create <name>

safe-ag mcp-login <service> [container]
safe-ag aws-refresh <name> [profile]
```

## Fleet and pipelines

```bash
safe-ag fleet manifest.yaml
safe-ag fleet manifest.yaml --dry-run
safe-ag fleet status

safe-ag pipeline pipeline.yaml
safe-ag pipeline pipeline.yaml --dry-run
```

## TUI and dashboard

```bash
safe-ag tui
safe-ag dashboard --bind localhost:8420
```

## High-signal examples

Private repo, reusable auth:

```bash
safe-ag spawn codex \
  --ssh \
  --reuse-auth \
  --reuse-gh-auth \
  --repo git@github.com:org/repo.git
```

Background run with a prompt:

```bash
safe-ag spawn claude \
  --background \
  --ssh \
  --repo git@github.com:org/repo.git \
  --prompt "Run the test suite and fix failures"
```

Infrastructure session:

```bash
safe-ag spawn claude \
  --ssh \
  --aws my-profile \
  --repo git@github.com:org/infra.git \
  --prompt "Review Terraform drift"
```

Parallel review:

```bash
safe-ag fleet examples/fleet-review-and-fix.yaml
safe-ag pipeline examples/pipeline-consolidate-and-fix.yaml
```
