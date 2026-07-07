# Fleet and Pipelines

Use a fleet when you want parallel agents. Use a pipeline when you want staged execution.

## Fleet

```bash
berth fleet fleet.yaml
berth fleet fleet.yaml --dry-run
berth fleet status
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

Fleet behavior:
- all agents are spawned from one manifest
- they share a fleet label/volume context
- they run in parallel
- `--dry-run` shows the resolved spawn commands without launching anything

## Pipeline

```bash
berth pipeline pipeline.yaml
berth pipeline pipeline.yaml --dry-run
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

Pipeline behavior:
- steps are normalized into stages
- stages with satisfied dependencies run
- model-expanded stages are supported
- sub-pipelines are supported

## Common manifest fields

| Field | Meaning |
|---|---|
| `name` | human-readable agent/stage name |
| `profile` | profile name from `~/.berth/agents/*.toml` or `.berth/agents/*.toml` |
| `type` | `claude` or `codex` |
| `repo` / `repos` | one or many repos |
| `prompt` | task |
| `template`, `template_vars` | prompt template and variables |
| `instructions`, `instructions_file` | extra agent instructions |
| `ssh` | SSH forwarding |
| `reuse_auth` | shared Claude/Codex auth |
| `reuse_gh_auth` | shared `gh` auth |
| `seed_auth` | copy host Claude/Codex auth into this session |
| `ephemeral_auth` | force per-session auth |
| `docker` | DinD access |
| `docker_socket` | direct VM Docker socket access |
| `allow_setup_scripts` | allow repo-provided `berth.json` setup hooks |
| `aws` | AWS profile |
| `network` | custom Docker network |
| `memory`, `cpus`, `pids_limit` | resource limits |
| `max_cost`, `notify`, `on_exit`, `on_complete`, `on_fail` | metadata and callbacks |
| `background` | spawn without attach |
| `auto_trust` | skip trust prompt |

Profiles can be used in `defaults` or per agent. Manifest fields override profile fields.

Pipeline-only fields:

| Field | Meaning |
|---|---|
| `depends_on` | dependency edge |
| `models` | duplicate a stage across models |
| `pipeline` | nested pipeline file |

Unsupported control fields such as `retry`, `on_failure`, `when`, and `outputs` are rejected for now instead of being silently ignored.

## Real examples

```bash
berth fleet examples/fleet-review-and-fix.yaml
berth pipeline examples/pipeline-consolidate-and-fix.yaml
berth pipeline examples/pipeline-double-review-reconcile.yaml
berth pipeline examples/pipeline-display-nested.yaml --dry-run
```

## Choosing between them

Use a fleet when:
- tasks are independent
- you want wall-clock speed from parallelism

Use a pipeline when:
- later work depends on earlier output
- you need stage ordering

Use both when:
- parallel review or analysis feeds a later consolidation/fix stage

The `examples/pipeline-double-review-reconcile.yaml` example shows a richer version:
- category-specific review branches
- both Claude and Codex contributing reports to the same branch per category
- a final Codex reconciliation/fix/PR stage
- the reconciled report goes into the PR description, not a committed `REVIEW-RECONCILED.md` file
