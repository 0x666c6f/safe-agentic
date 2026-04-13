# Fleet and Pipelines

Use a fleet when you want parallel agents. Use a pipeline when you want staged execution.

## Fleet

```bash
safe-ag fleet fleet.yaml
safe-ag fleet fleet.yaml --dry-run
safe-ag fleet status
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
safe-ag pipeline pipeline.yaml
safe-ag pipeline pipeline.yaml --dry-run
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
| `type` | `claude` or `codex` |
| `repo` / `repos` | one or many repos |
| `prompt` | task |
| `ssh` | SSH forwarding |
| `reuse_auth` | shared Claude/Codex auth |
| `reuse_gh_auth` | shared `gh` auth |
| `docker` | DinD access |
| `aws` | AWS profile |
| `network` | custom Docker network |
| `memory`, `cpus` | resource limits |
| `background` | spawn without attach |
| `auto_trust` | skip trust prompt |

Pipeline-only fields:

| Field | Meaning |
|---|---|
| `depends_on` | dependency edge |
| `retry` | retry count |
| `on_failure` | failure handler |
| `when` | conditional marker for downstream logic |
| `outputs` | command/output convention for later stages |
| `models` | duplicate a stage across models |
| `pipeline` | nested pipeline file |

## Real examples

```bash
safe-ag fleet examples/fleet-review-and-fix.yaml
safe-ag pipeline examples/pipeline-consolidate-and-fix.yaml
safe-ag pipeline examples/pipeline-display-nested.yaml --dry-run
```

## Choosing between them

Use a fleet when:
- tasks are independent
- you want wall-clock speed from parallelism

Use a pipeline when:
- later work depends on earlier output
- you need retries or stage ordering

Use both when:
- parallel review or analysis feeds a later consolidation/fix stage
