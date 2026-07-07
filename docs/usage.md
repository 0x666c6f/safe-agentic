# Command Map

This page answers: "Which command do I run for the job in front of me?"

If you want the raw reference pages instead:
- [CLI Reference](reference/cli.md)
- [TUI Reference](reference/tui.md)

## Setup and maintenance

```bash
berth setup
berth update
berth update --quick
berth update --full
berth diagnose
berth vm start
berth vm stop
berth vm ssh
```

## Spawn and connect

```bash
berth spawn <claude|codex|shell> ...
berth spawn claude --worktree --name task-name   # requires one-time: berth setup --enable-worktrees
berth run <repo...> [prompt]
berth attach <name>
berth attach --latest
berth steer --latest "continue, but keep the change smaller"
berth profile run reviewer "focus security only"
```

Use `spawn` when you want explicit control. Use `run` for a quick single-task session.
Use `profile run` for repeatable roles from `~/.berth/agents/*.toml` or `.berth/agents/*.toml`.

## See what the agent is doing

```bash
berth list
berth peek <name>
berth logs <name>
berth search "error text"
berth summary <name>
berth output <name>
berth diff <name>
berth review <name>
berth review-comments list <name>
berth timeline
berth inbox
berth server --stdio
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
berth stop <name>
berth stop --latest
berth stop --all
berth cleanup
berth cleanup --auth
berth sessions <name> [dest]
```

## Workflow helpers

```bash
berth checkpoint create <name> [label]
berth checkpoint list <name>
berth checkpoint restore <name> <ref>

berth todo add <name> "text"
berth todo list <name>
berth todo check <name> <index>
berth todo uncheck <name> <index>

berth retry <name> [--feedback "..."]
berth steer <name> "address the failing test only"
berth handoff <name> --to-worktree   # worktree commands need: berth setup --enable-worktrees
berth handoff <name> --to-local ./workspace-copy
berth worktree snapshot <name> "before cleanup"
berth worktree restore <name> stash@{0}
berth worktree cleanup --dry-run
berth workspace stage <name> src/app.go
berth workspace revert <name> src/app.go --yes
berth workspace stage-patch <name> selected.patch
berth workspace revert-patch <name> selected.patch --yes
berth pr <name> [--title ... --base ...]
berth browser capture http://localhost:3000 --mode auto --annotation "checkout flow"
```

## Auth and config

```bash
berth config show
berth config get <key>
berth config set <key> <value>
berth config reset <key>

# hard spawn guards
$EDITOR ~/.berth/rules.toml

berth action list
berth action run test --latest

berth template list
berth template show <name>
berth template create <name>

berth mcp-login <service> [container]
berth aws-refresh <name> [profile]
```

## Fleet and pipelines

```bash
berth fleet manifest.yaml
berth fleet manifest.yaml --dry-run
berth fleet status

berth pipeline pipeline.yaml
berth pipeline pipeline.yaml --dry-run
berth pipeline inspect review
berth pipeline render review
berth pipeline validate review
berth pr-review
berth pr-fix
```

## TUI

```bash
berth tui
```

## High-signal examples

Private repo, reusable auth:

```bash
berth spawn codex \
  --ssh \
  --reuse-auth \
  --reuse-gh-auth \
  --repo git@github.com:org/repo.git
```

Background run with a prompt:

```bash
berth spawn claude \
  --background \
  --ssh \
  --repo git@github.com:org/repo.git \
  --prompt "Run the test suite and fix failures"
```

Infrastructure session:

```bash
berth spawn claude \
  --ssh \
  --aws my-profile \
  --repo git@github.com:org/infra.git \
  --prompt "Review Terraform drift"
```

Parallel review:

```bash
berth fleet examples/fleet-review-and-fix.yaml
berth pipeline examples/pipeline-consolidate-and-fix.yaml
```
