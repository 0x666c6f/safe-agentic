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

## Checkpoints

```bash
safe-ag checkpoint create --latest "before big refactor"
safe-ag checkpoint list --latest
safe-ag checkpoint revert --latest <ref>
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
safe-ag summary --latest
safe-ag diff --latest
safe-ag review --latest
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
