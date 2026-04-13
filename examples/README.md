# Example Manifests

## Review → Fix → PR

Use the fleet manifest first, then run the consolidation pipeline:

```bash
safe-ag fleet examples/fleet-review-and-fix.yaml
safe-ag tui
safe-ag pipeline examples/pipeline-consolidate-and-fix.yaml
```

This:
1. Spawns 4 parallel review agents (code, security, tests, docs)
2. Each agent pushes findings to a `review/*` branch
3. Lets you monitor progress in the TUI
4. Runs the follow-up pipeline: consolidate → fix critical/high → create PR
5. Cleans up review branches

## Manual two-phase workflow

```bash
# Phase 1: spawn reviewers
safe-ag fleet examples/fleet-review-and-fix.yaml
safe-ag tui  # monitor

# Phase 2: after all finish
safe-ag pipeline examples/pipeline-consolidate-and-fix.yaml
```

## Standalone fleet (no follow-up)

```bash
safe-ag fleet examples/fleet-self-review.yaml
```

## Standalone pipeline

```bash
safe-ag pipeline examples/pipeline-security-hardening.yaml
```

## Minimal nested pipeline for display checks

Use this when you only want to validate pipeline tree rendering.

```bash
safe-ag pipeline examples/pipeline-display-nested.yaml --dry-run
```

If you run it without `--dry-run`, each nesting level spawns exactly one tiny leaf agent with prompt `Reply OK.` and `auto_trust: true`, so the run can progress without waiting at the Codex trust prompt.
