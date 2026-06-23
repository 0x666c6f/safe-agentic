---
name: agent-manifest-author
description: Author or update safe-agentic fleet and pipeline manifests. Use when the user wants reusable orchestration YAML for parallel workers, staged flows, nested pipelines, model fan-out, or dry-run validation before execution.
---

# Author Safe-Agentic Manifests

Use this skill for reusable orchestration, not one-off `spawn` commands.

## Choose manifest type

- `fleet.yaml`: parallel, mostly independent workers
- `pipeline.yaml`: stage ordering, retries, failure handlers, nested pipelines

## Workflow

1. map work units, repos, auth needs, and dependency edges
2. keep prompts task-specific; do not hide orchestration in one vague mega-prompt
3. encode shared defaults once, override only when needed
4. use `profile:` when a role already exists in `~/.safe-ag/agents/*.toml` or `.safe-ag/agents/*.toml`
5. dry-run first with `safe-ag fleet ... --dry-run` or `safe-ag pipeline ... --dry-run`
6. keep example manifests and real manifests small and reviewable

## Fleet starter

```yaml
agents:
  - name: api-review
    profile: reviewer
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    prompt: "Review current branch and write findings"

  - name: web-fix
    type: codex
    repo: git@github.com:org/web.git
    ssh: true
    prompt: "Fix the failing lint and typecheck jobs"
```

## Pipeline starter

```yaml
name: review-fix-pr
steps:
  - name: review
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    prompt: "Review the branch and summarize actionable findings"

  - name: fix
    type: codex
    repo: git@github.com:org/api.git
    ssh: true
    prompt: "Address the review findings and run tests"
    depends_on: review
```

Only use implemented pipeline controls. `depends_on` is supported. `retry`, `on_failure`, `when`, and `outputs` are rejected for now.

Supported agent fields include `profile`, `template`, `template_vars`, `instructions`, `instructions_file`, `ephemeral_auth`, `docker_socket`, `pids_limit`, `identity`, `max_cost`, `notify`, `on_exit`, `on_complete`, and `on_fail`. Manifest fields override profile fields.

## Validation

```bash
safe-ag fleet path/to/fleet.yaml --dry-run
safe-ag pipeline path/to/pipeline.yaml --dry-run
```

Check for:
- wrong field names
- missing `ssh` or auth flags
- prompts that depend on hidden human context
- dependency cycles or stages with unclear ownership
