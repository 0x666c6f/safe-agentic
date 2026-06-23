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
safe-ag spawn claude --worktree --name task-name
safe-ag run <repo...> [prompt]
safe-ag attach <name>
safe-ag attach --latest
safe-ag steer --latest "continue, but keep the change smaller"
safe-ag profile run reviewer "focus security only"
```

Use `spawn` when you want explicit control. Use `run` for a quick single-task session.
Use `profile run` for repeatable roles from `~/.safe-ag/agents/*.toml` or `.safe-ag/agents/*.toml`.

## See what the agent is doing

```bash
safe-ag list
safe-ag peek <name>
safe-ag logs <name>
safe-ag search "error text"
safe-ag summary <name>
safe-ag output <name>
safe-ag diff <name>
safe-ag review <name>
safe-ag review-comments list <name>
safe-ag timeline
safe-ag inbox
safe-ag server --stdio
```

Typical meaning:
- `peek`: last visible pane output
- `logs`: conversation/session log
- `summary`: compact state snapshot
- `output`: last useful result
- `diff`: git diff from the workspace
- `review`: review the diff
- `review-comments`: local file/line notes for review handoff
- `timeline`: recent event/audit stream
- `inbox`: failures or status markers that need attention

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
safe-ag steer <name> "address the failing test only"
safe-ag handoff <name> --to-worktree
safe-ag handoff <name> --to-local ./workspace-copy
safe-ag worktree snapshot <name> "before cleanup"
safe-ag worktree restore <name> stash@{0}
safe-ag worktree cleanup --dry-run
safe-ag workspace stage <name> src/app.go
safe-ag workspace revert <name> src/app.go --yes
safe-ag workspace stage-patch <name> selected.patch
safe-ag workspace revert-patch <name> selected.patch --yes
safe-ag pr <name> [--title ... --base ...]
safe-ag browser capture http://localhost:3000 --mode auto --annotation "checkout flow"
```

## Auth and config

```bash
safe-ag config show
safe-ag config get <key>
safe-ag config set <key> <value>
safe-ag config reset <key>

# hard spawn guards
$EDITOR ~/.safe-ag/rules.toml

safe-ag action list
safe-ag action run test --latest

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
safe-ag pipeline inspect review
safe-ag pipeline render review
safe-ag pipeline validate review
safe-ag pr-review
safe-ag pr-fix
```

## TUI

```bash
safe-ag tui
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
