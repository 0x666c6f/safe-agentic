---
name: agent-orchestrate
description: Orchestrate multi-agent work with safe-agentic. Use when the user wants one agent to supervise parallel or staged safe-ag runs, split work across repos, monitor progress, retry failures, consolidate outputs, or drive review-to-PR loops.
---

# Orchestrate Safe Agents

Use this skill when one agent should act as the conductor for other `safe-ag` sessions.

## Pick the shape

- independent tasks: use `safe-ag fleet`
- ordered stages: use `safe-ag pipeline`
- one-off delegation with tight feedback: use `safe-ag spawn ... --background`

## Control loop

1. define task split and success signal before spawning
2. prefer `--background` for worker sessions
3. watch with `safe-ag list`, `safe-ag peek`, `safe-ag summary`
4. pull results with `safe-ag output`, `safe-ag diff`, `safe-ag review`
5. if a worker drifts, use `safe-ag retry --feedback "..."` instead of ad-hoc hand edits
6. checkpoint before consolidation or risky merge work
7. stop or cleanup finished workers

## Common commands

```bash
safe-ag spawn claude --background --name api-tests --repo git@github.com:org/api.git \
  --prompt "Run tests, fix failures, then summarize remaining risk"

safe-ag list
safe-ag peek api-tests
safe-ag summary api-tests
safe-ag output api-tests
safe-ag diff api-tests
safe-ag review api-tests
safe-ag retry api-tests --feedback "Reduce scope to auth/timeouts only"
safe-ag stop api-tests
```

## Multi-agent review/fix pattern

```bash
safe-ag fleet examples/fleet-review-and-fix.yaml --dry-run
safe-ag fleet examples/fleet-review-and-fix.yaml
```

- use fleet for parallel review, triage, or per-repo work
- use pipeline when a later fixer depends on earlier review output

## Guardrails

- always name workers clearly; names become the control surface
- prefer `--reuse-auth` only when workers need the same MCP or CLI auth
- use `safe-ag review` before `safe-ag pr`
- use `safe-ag todo` to enforce merge gates on worker containers
- use `safe-ag cleanup --auth` after high-risk or shared-machine runs
