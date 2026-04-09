# Example Manifests

## Full orchestrated review → fix → PR

One command to run parallel reviews, wait for completion, consolidate findings, fix issues, and create a PR:

```bash
agent orchestrate examples/fleet-review-and-fix.yaml examples/pipeline-consolidate-and-fix.yaml
```

This:
1. Spawns 4 parallel review agents (code, security, tests, docs)
2. Each agent pushes findings to a `review/*` branch
3. Waits for all agents to finish (polls every 30s)
4. Runs a 3-step pipeline: consolidate → fix critical/high → create PR
5. Cleans up review branches

## Manual two-phase workflow

```bash
# Phase 1: spawn reviewers
agent fleet examples/fleet-review-and-fix.yaml
agent tui  # monitor

# Phase 2: after all finish
agent pipeline examples/pipeline-consolidate-and-fix.yaml
```

## Standalone fleet (no follow-up)

```bash
agent fleet examples/fleet-self-review.yaml
```

## Standalone pipeline

```bash
agent pipeline examples/pipeline-security-hardening.yaml
```
