---
name: agent-orchestrate
description: Orchestrate multi-agent work with berth. Use when the user wants one agent to supervise parallel or staged berth runs, split work across repos, monitor progress, retry failures, consolidate outputs, or drive review-to-PR loops.
---

# Orchestrate Safe Agents

Use this skill when one agent should act as the conductor for other `berth` sessions.

## Pick the shape

- independent tasks: use `berth fleet`
- ordered stages: use `berth pipeline`
- one-off delegation with tight feedback: use `berth spawn ... --background`

## Control loop

1. define task split and success signal before spawning
2. prefer `--background` for worker sessions
3. watch with `berth list`, `berth peek`, `berth summary`
4. pull results with `berth output`, `berth diff`, `berth review`
5. if a worker drifts, use `berth retry --feedback "..."` instead of ad-hoc hand edits
6. checkpoint before consolidation or risky merge work
7. stop or cleanup finished workers

## Common commands

```bash
berth spawn claude --background --name api-tests --repo git@github.com:org/api.git \
  --prompt "Run tests, fix failures, then summarize remaining risk"

berth list
berth peek api-tests
berth summary api-tests
berth output api-tests
berth diff api-tests
berth review api-tests
berth retry api-tests --feedback "Reduce scope to auth/timeouts only"
berth stop api-tests
```

## Multi-agent review/fix pattern

```bash
berth fleet examples/fleet-review-and-fix.yaml --dry-run
berth fleet examples/fleet-review-and-fix.yaml
```

- use fleet for parallel review, triage, or per-repo work
- use pipeline when a later fixer depends on earlier review output

## Guardrails

- always name workers clearly; names become the control surface
- prefer `--reuse-auth` only when workers need the same MCP or CLI auth
- use `berth review` before `berth pr`
- use `berth todo` to enforce merge gates on worker containers
- use `berth cleanup --auth` after high-risk or shared-machine runs
