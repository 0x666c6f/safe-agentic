# Workflow

This page covers the "agent did work, now what?" loop.

## Status and output

```bash
berth summary --latest
berth output --latest
berth diff --latest
berth review --latest
```

Typical order:

1. `summary` for quick state
2. `output` for the last meaningful message
3. `diff` for the code change
4. `review` for an extra review pass

## Retry with feedback

```bash
berth retry --latest
berth retry --latest --feedback "Focus only on src/ and add tests"
```

`retry` reconstructs the original spawn options and starts a fresh container with the same session shape.

## Steer a running agent

```bash
berth steer --latest "Focus only on the failing test and avoid unrelated refactors"
berth steer api-refactor "Run the narrower test first"
```

Use `steer` when the agent is active and you want to add guidance without attaching to the tmux session. If the container is stopped, `steer` starts it and waits for tmux.

## Review comments

```bash
berth review-comments add --latest cmd/main.go 42 "Handle empty input before parsing"
berth review-comments list --latest
berth steer --latest "Address the open review comments, then run tests"
berth review-comments resolve rc-123
```

Use review comments for local file/line notes that should survive between `review`, `diff`, `steer`, and PR handoff. They are stored on the host under `~/.berth/state/`.

## Checkpoints

```bash
berth checkpoint create --latest "before big refactor"
berth checkpoint list --latest
berth checkpoint restore --latest <ref>
```

Use checkpoints when you want a reversible snapshot before the agent makes a risky change.

## Todos

```bash
berth todo add --latest "Run integration tests"
berth todo add --latest "Update changelog"
berth todo list --latest
berth todo check --latest 1
```

This is the built-in merge checklist.

## PR creation

```bash
berth pr --latest --title "fix: stabilize flaky tests"
berth pr --latest --base main
```

Before `berth pr`, you usually want:
- SSH enabled at spawn time
- GitHub auth available
- todos completed

## Typical end-to-end loop

```bash
berth spawn claude --ssh --reuse-auth \
  --repo git@github.com:org/api.git \
  --prompt "Fix the failing CI tests"

berth peek --latest
berth steer --latest "Keep the fix narrow and add one regression test"
berth summary --latest
berth diff --latest
berth review --latest
berth review-comments add --latest src/api.go 37 "Add a regression test for this branch"
berth todo add --latest "Verify locally"
berth todo check --latest 1
berth pr --latest --title "fix: resolve CI failures"
```

## Hooks and post-run automation

You can use lifecycle hooks to automate follow-up work:

```bash
berth spawn claude \
  --background \
  --repo https://github.com/org/repo.git \
  --prompt "Write a migration plan" \
  --on-exit "berth output --latest > /tmp/plan.txt"
```

## What belongs here vs elsewhere

- this page: review, retry, checkpoint, todo, PR
- [Managing Agents](managing.md): attach/logs/cleanup/session export
- [Fleet and Pipelines](fleet.md): multi-agent orchestration
