# Manifests

YAML schema for `berth fleet` and `berth pipeline`. Task-oriented walkthroughs live in the [Fleets & Pipelines guide](../guide/fleet.md); runnable manifests in [`examples/`](https://github.com/0x666c6f/berth/tree/main/examples).

## Agent fields

Every fleet agent, pipeline step, and stage agent accepts the same fields. Each maps to the matching [`berth spawn`](cli.md#spawn) flag:

| Field | Type | Meaning |
|---|---|---|
| `name` | string | agent/step name (container name derives from it) |
| `profile` | string | saved [agent profile](../guide/configuration.md#agent-profiles); manifest fields override profile fields |
| `type` | string | **required** — `claude`, `codex`, or `shell` |
| `repo` | string | repository URL |
| `repos` | list | multiple repository URLs |
| `prompt` | string | the task; supports `${var}` interpolation |
| `template` | string | prompt template name |
| `template_vars` | list | `key=value` template variables |
| `instructions` | string | standing instructions |
| `instructions_file` | string | read instructions from a file |
| `ssh` | bool | SSH agent forwarding |
| `reuse_auth` | bool | shared Claude/Codex auth volume |
| `ephemeral_auth` | bool | throwaway per-session auth |
| `reuse_gh_auth` | bool | shared GitHub CLI auth |
| `seed_auth` | bool | copy host Claude/Codex auth into the session |
| `auto_trust` | bool | auto-accept the agent's trust prompt |
| `background` | bool | spawn detached |
| `docker` | bool | Docker-in-Docker sidecar |
| `docker_socket` | bool | direct VM Docker socket |
| `allow_setup_scripts` | bool | run repo-provided `berth.json` setup hooks |
| `network` | string | custom Docker network |
| `memory` / `cpus` / `pids_limit` | string / string / int | resource limits |
| `identity` | string | git identity `Name <email>` |
| `aws` | string | AWS profile |
| `max_cost` | string | advisory USD budget |
| `notify` | string | notification targets (comma-separated) |
| `on_exit` / `on_complete` / `on_fail` | string | lifecycle hook commands |

Boolean fields are tri-state: unset inherits from `profile` / `defaults`; an explicit `false` overrides an inherited `true`.

Pipeline-only fields on a step: `depends_on` (string), `models` (list), `judge` (block). Unsupported control fields — `on_failure`, `retry`, `when`, `outputs` — are **rejected at parse time** instead of silently ignored.

## Shared top-level fields

Both manifest kinds accept:

| Field | Type | Meaning |
|---|---|---|
| `name` | string | manifest name |
| `description` | string | shown by `pipeline show`/`inspect` |
| `inputs` | list | declared inputs (below) |
| `examples` | list | example invocations |
| `tags` | list | catalog tags |
| `defaults` | agent fields | inherited by every agent; per-agent fields override |
| `vars` | map | `${key}` values interpolated in prompts |

### Inputs

```yaml
inputs:
  - name: repo
    description: Repository URL or current checkout repo.
    required: false
    default: ""
    infer: repo        # infer from the current checkout's origin
  - name: pr
    description: Pull request number when relevant.
    infer: pr          # infer from the current branch's open PR
```

Values resolve in order: `--var key=value` → `infer` → `default`. `${repo}` also falls back to `--repo` / the current checkout.

## Fleet manifest

All agents spawn in parallel and share a fleet volume mounted at `/fleet` for cross-agent scratch.

```yaml
name: review-and-fix
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

With defaults and vars:

```yaml
name: multi-review
description: Parallel reviews across two repos
vars:
  focus: "error handling"
defaults:
  type: claude
  ssh: true
  reuse_auth: true
  background: true
agents:
  - name: api-review
    repo: git@github.com:org/api.git
    prompt: "Review ${focus} in this repo"
  - name: web-review
    repo: git@github.com:org/web.git
    prompt: "Review ${focus} in this repo"
    type: codex          # per-agent override wins over defaults
```

## Pipeline manifest

Two layouts. **Flat steps** — each step becomes its own single-agent stage, `depends_on` is a single stage name:

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

**Stages** — each stage holds one or more agents that run in parallel; stages order by `depends_on` (list):

```yaml
name: fan-out-review
defaults:
  repo: ${repo}
  ssh: true
stages:
  - name: reviews
    agents:
      - name: sec-review
        type: claude
        prompt: "Security review"
      - name: perf-review
        type: codex
        prompt: "Performance review"

  - name: consolidate
    depends_on: [reviews]
    agents:
      - name: fixer
        type: codex
        prompt: "Read both reviews from /fleet and fix the top findings"

  - name: nested
    depends_on: [consolidate]
    pipeline: ./sub-pipeline.yaml     # sub-pipeline instead of inline agents
```

Stage fields:

| Field | Type | Meaning |
|---|---|---|
| `name` | string | stage name |
| `depends_on` | list | stages that must finish first; naming a `models`-expanded stage expands to its per-model children |
| `agents` | list | agents that run in parallel within the stage |
| `models` | list | duplicate the stage's agents once per entry; each entry becomes the agent `type` (`claude`, `codex`), available as `${model}` in prompts |
| `pipeline` | string | path to a sub-pipeline YAML (mutually exclusive with `agents`) |
| `judge` | block | best-of-N judge (mutually exclusive with `agents`/`pipeline`) |

## Judge block

A judge stage picks the best result among candidate runs: it collects each candidate's working-tree diff and final message, runs a one-shot Claude judge, and persists a JSON verdict to `~/.berth/state/judge/`.

```yaml
steps:
  - name: implement
    models: [claude, codex]
    prompt: "${task}\n\nYou are candidate ${model}."
  - name: pick-winner
    judge:
      criteria: "correctness and tests first, then minimal diff"
      auto_pr: true
      base: main
    depends_on: implement
```

| Field | Type | Default | Meaning |
|---|---|---|---|
| `criteria` | string | quality-first default | free-text ranking guidance |
| `auto_pr` | bool | `false` | open a GitHub PR from the winning container |
| `base` | string | `main` | PR base branch for `auto_pr` |
| `max_diff` | int | `12000` | per-candidate diff byte cap in the judge prompt |

Validation rules:

- a judge must `depends_on` stages producing **≥ 2 candidates** (`models: [...]` fan-out or multiple `agents:` in one stage)
- a judge stage carries no `prompt`, `template`, `repo`, `type`, `instructions`, `profile`, `models`, or `agents` of its own
- a judge cannot depend on a sub-pipeline stage or another judge

`auto_pr` pushes over HTTPS from a helper container using the winner's volumes — candidates need `reuse_gh_auth: true`.

## Review presets

[`berth pr-review` / `berth pr-fix`](../guide/workflow.md#one-shot-pr-review-workflows) run ordinary pipeline manifests resolved by name: user overrides in `~/.berth/pipelines/reviews/<name>.yaml` first, then the built-ins `claude`, `codex`, `dual` (the `pr-review` default — parallel Claude+Codex review feeding a Codex reconcile-fix), and `fix`. All declare required `repo` and `pr` inputs with `infer: repo` / `infer: pr`.

## Interpolation and precedence

- `${key}` in prompts resolves from `vars:` and `--var key=value` (CLI wins)
- `${repo}` falls back to `--repo`, then the current checkout's `origin`
- `${model}` is set during `models:` expansion
- field precedence: per-agent > `profile` > `defaults`
