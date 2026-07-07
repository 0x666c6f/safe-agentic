# Fleets & Pipelines

Multi-agent orchestration from YAML manifests. Use a **fleet** for parallel independent agents, a **pipeline** for staged execution with dependencies, and a **judge stage** to fan out candidates and crown a winner.

## Fleet — parallel fan-out

```bash
berth fleet fleet.yaml
berth fleet fleet.yaml --dry-run     # resolved spawn commands, no launch
berth fleet status                   # progress of the running fleet
```

Minimal fleet:

```yaml
agents:
  - name: api-tests
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    prompt: "Fix failing tests"

  - name: frontend-lint
    type: codex
    repo: git@github.com:org/frontend.git
    prompt: "Fix lint errors"
```

All agents spawn from one manifest, run in parallel, and share a fleet label plus an optional shared `/fleet` volume for cross-agent scratch.

## Pipeline — staged execution

```bash
berth pipeline pipeline.yaml
berth pipeline pipeline.yaml --dry-run       # print the stage-order plan
berth pipeline pipeline.yaml --background    # run detached
```

Minimal pipeline:

```yaml
name: test-and-fix
steps:
  - name: run-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Run the full test suite"

  - name: fix-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Fix the failures"
    depends_on: run-tests
```

Steps normalize into stages; a stage runs once its dependencies are satisfied. Pipelines also support `models: [...]` fan-out (duplicate a stage per agent engine) and nested sub-pipelines. Unsupported control fields (`retry`, `on_failure`, `when`, `outputs`) are rejected instead of silently ignored.

### Saved pipelines

```bash
berth pipeline list                  # saved pipelines in ~/.berth/pipelines/
berth pipeline show review
berth pipeline create review
berth pipeline review --repo git@github.com:org/repo.git --var topic=security
berth pipeline validate review       # check without running
berth pipeline render review         # fully resolved YAML
```

## Judge stages — best-of-N

A stage with a `judge` block picks the single best result among candidate runs instead of spawning its own agent: it collects each candidate's working-tree diff and final message, runs a one-shot Claude judge, and records a strict-JSON verdict.

```yaml
name: judge-fanout
defaults:
  repo: ${repo}
  ssh: true
  reuse_auth: true
  auto_trust: true
steps:
  - name: implement                # fan out across engines → 2 candidates
    models: [claude, codex]
    prompt: "${task}\n\nYou are candidate ${model}. Commit focused, tested changes."
  - name: pick-winner
    judge:
      criteria: "correctness and tests first, then minimal diff"
      auto_pr: true                # open a PR from the winner
      base: main
    depends_on: implement
```

Rules that matter:

- a judge must depend on stages producing **at least two** candidates (via `models: [...]` or multiple `agents:` in one stage)
- a judge stage carries no `prompt`/`repo`/`type`/etc. of its own
- `auto_pr` requires candidates spawned with `reuse_gh_auth: true` (the PR helper pushes over HTTPS)

Verdicts persist to `~/.berth/state/judge/`. Full field-by-field schema: [Manifests reference](../reference/manifests.md).

## Vars, profiles, defaults

- `${key}` placeholders resolve from `--var key=value`; `${repo}` falls back to the current checkout's `origin`
- `defaults:` applies to every agent/stage; per-agent fields override it
- `profile: <name>` pulls a [saved agent profile](configuration.md#agent-profiles); manifest fields override profile fields

The full manifest schema (every field, type, default) lives in the [Manifests reference](../reference/manifests.md).

## Choosing between them

| Shape | Reach for |
|---|---|
| independent tasks, wall-clock speed | fleet |
| later work depends on earlier output | pipeline |
| parallel analysis feeding a consolidation stage | pipeline with fan-in `depends_on` |
| multiple attempts, ship the best one | pipeline with a judge stage |

## Runnable examples

```bash
berth fleet examples/fleet-review-and-fix.yaml
berth pipeline examples/pipeline-consolidate-and-fix.yaml
berth pipeline examples/pipeline-double-review-reconcile.yaml
berth pipeline examples/pipeline-judge-fanout.yaml
berth pipeline examples/pipeline-display-nested.yaml --dry-run
```

See [`examples/README.md`](https://github.com/0x666c6f/berth/blob/main/examples/README.md) for walkthroughs. For one-shot PR review/fix without writing a manifest, use [`berth pr-review` / `berth pr-fix`](workflow.md#one-shot-pr-review-workflows).
