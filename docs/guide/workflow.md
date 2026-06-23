# Workflow

This page covers the "agent did work, now what?" loop.

## Status and output

```bash
safe-ag summary --latest
safe-ag output --latest
safe-ag diff --latest
safe-ag review --latest
```

Typical order:

1. `summary` for quick state
2. `output` for the last meaningful message
3. `diff` for the code change
4. `review` for an extra review pass

## Retry with feedback

```bash
safe-ag retry --latest
safe-ag retry --latest --feedback "Focus only on src/ and add tests"
```

`retry` reconstructs the original spawn options and starts a fresh container with the same session shape.

## Steer a running agent

```bash
safe-ag steer --latest "Focus only on the failing test and avoid unrelated refactors"
safe-ag steer api-refactor "Run the narrower test first"
```

Use `steer` when the agent is active and you want to add guidance without attaching to the tmux session. If the container is stopped, `steer` starts it and waits for tmux.

## Review comments

```bash
safe-ag review-comments add --latest cmd/main.go 42 "Handle empty input before parsing"
safe-ag review-comments list --latest
safe-ag steer --latest "Address the open review comments, then run tests"
safe-ag review-comments resolve rc-123
```

Use review comments for local file/line notes that should survive between `review`, `diff`, `steer`, and PR handoff. They are stored on the host under `~/.safe-ag/state/`.

## Checkpoints

```bash
safe-ag checkpoint create --latest "before big refactor"
safe-ag checkpoint list --latest
safe-ag checkpoint restore --latest <ref>
```

Use checkpoints when you want a reversible snapshot before the agent makes a risky change.

## Todos

```bash
safe-ag todo add --latest "Run integration tests"
safe-ag todo add --latest "Update changelog"
safe-ag todo list --latest
safe-ag todo check --latest 1
```

This is the built-in merge checklist.

## PR creation

```bash
safe-ag pr --latest --title "fix: stabilize flaky tests"
safe-ag pr --latest --base main
```

Before `safe-ag pr`, you usually want:
- SSH enabled at spawn time
- GitHub auth available
- todos completed

## Typical end-to-end loop

```bash
safe-ag spawn claude --ssh --reuse-auth \
  --repo git@github.com:org/api.git \
  --prompt "Fix the failing CI tests"

safe-ag peek --latest
safe-ag steer --latest "Keep the fix narrow and add one regression test"
safe-ag summary --latest
safe-ag diff --latest
safe-ag review --latest
safe-ag review-comments add --latest src/api.go 37 "Add a regression test for this branch"
safe-ag todo add --latest "Verify locally"
safe-ag todo check --latest 1
safe-ag pr --latest --title "fix: resolve CI failures"
```

## Hooks and post-run automation

You can use lifecycle hooks to automate follow-up work:

```bash
safe-ag spawn claude \
  --background \
  --repo https://github.com/org/repo.git \
  --prompt "Write a migration plan" \
  --on-exit "safe-ag output --latest > /tmp/plan.txt"
```

## What belongs here vs elsewhere

- this page: review, retry, checkpoint, todo, PR
- [Managing Agents](managing.md): attach/logs/cleanup/session export
- [Fleet and Pipelines](fleet.md): multi-agent orchestration
