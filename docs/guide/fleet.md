# Fleet & Orchestration

Run multiple agents in parallel or chain them into workflows.

## Fleet — spawn from manifest

```bash
agent fleet fleet.yaml
agent fleet fleet.yaml --dry-run    # preview without spawning
```

### Manifest format

```yaml
agents:
  - name: api-tests
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    reuse_auth: true
    prompt: "Fix failing tests"

  - name: frontend-lint
    type: codex
    repo: git@github.com:org/frontend.git
    prompt: "Fix all linting errors"

  - name: docs-update
    type: claude
    repo: git@github.com:org/docs.git
    prompt: "Update API documentation"
```

### Supported fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Container name suffix |
| `type` | string | `claude` or `codex` (required) |
| `repo` | string | Repository URL |
| `ssh` | bool | Enable SSH forwarding |
| `reuse_auth` | bool | Persist OAuth tokens |
| `reuse_gh_auth` | bool | Persist GitHub CLI auth |
| `docker` | bool | Enable Docker-in-Docker |
| `prompt` | string | Initial task |
| `aws` | string | AWS profile name |
| `network` | string | Docker network name |
| `memory` | string | Memory limit (e.g., `16g`) |
| `cpus` | string | CPU limit |

Agents spawn in parallel. Monitor them with `agent tui`.

## Pipeline — multi-step workflows

```bash
agent pipeline pipeline.yaml
agent pipeline pipeline.yaml --dry-run
```

### Pipeline format

```yaml
name: test-and-fix
steps:
  - name: run-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Run all tests and report results"
    on_failure: fix-tests

  - name: fix-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Fix the failing tests"
    retry: 2

  - name: create-pr
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Create a PR with the fixes"
    depends_on: fix-tests
```

### Step fields

All fleet fields plus:

| Field | Type | Description |
|-------|------|-------------|
| `depends_on` | string | Only run after this step completes |
| `retry` | int | Re-attempt up to N times (5s delay) |
| `on_failure` | string | Trigger this step on failure |

### Execution model

Steps run **sequentially**. Each step spawns an agent, waits for it to finish, then proceeds.

- **`depends_on`**: Skip if the named step hasn't completed successfully
- **`retry`**: On failure, wait 5 seconds and re-run (up to N times)
- **`on_failure`**: When a step fails (after retries), jump to the named handler step

## Fleet + Pipeline: artifact passing

Fleet agents run in separate containers and can't share files directly. The pattern for coordination is:

1. **Fleet agents write findings to files and push to branches**
2. **A pipeline agent fetches those branches, consolidates, and acts**

```bash
# Phase 1: parallel review (each agent pushes to a review/* branch)
agent fleet examples/fleet-review-and-fix.yaml

# Monitor with TUI — wait for all agents to finish
agent tui

# Phase 2: consolidate findings → fix critical issues → create PR
agent pipeline examples/pipeline-consolidate-and-fix.yaml
```

This gives you parallel review speed with coordinated follow-up action.

## Real-world examples

### Example 1: Parallel dependency updates across repos

```yaml
# fix-all-deps.yaml
agents:
  - name: api-deps
    type: claude
    repo: git@github.com:myorg/api.git
    ssh: true
    reuse_auth: true
    prompt: "Update all npm dependencies to latest stable versions, fix any breaking changes, ensure tests pass"

  - name: web-deps
    type: claude
    repo: git@github.com:myorg/web.git
    ssh: true
    reuse_auth: true
    prompt: "Update all npm dependencies to latest stable versions, fix any breaking changes, ensure tests pass"

  - name: docs-deps
    type: codex
    repo: git@github.com:myorg/docs.git
    prompt: "Update Python dependencies in requirements.txt to latest versions"
```

```bash
agent fleet fix-all-deps.yaml
agent tui  # monitor all 3 agents
```

### Example 2: Security audit fleet

```yaml
# security-audit.yaml
agents:
  - name: audit-api
    type: claude
    repo: git@github.com:myorg/api.git
    ssh: true
    prompt: "Perform a security audit: check for SQL injection, XSS, CSRF, auth bypass, and secrets in code"

  - name: audit-infra
    type: claude
    repo: git@github.com:myorg/infra.git
    ssh: true
    aws: my-aws-profile
    prompt: "Audit Terraform configs for: overly permissive IAM, public S3 buckets, missing encryption, open security groups"
```

```bash
agent fleet security-audit.yaml
agent tui  # watch both audits run in parallel
```

### Example 3: CI-like pipeline

```yaml
# ci-pipeline.yaml
name: ci-fix-deploy
steps:
  - name: run-tests
    type: claude
    repo: git@github.com:myorg/api.git
    ssh: true
    prompt: "Run the full test suite. Report which tests pass and which fail."

  - name: fix-failures
    type: claude
    repo: git@github.com:myorg/api.git
    ssh: true
    prompt: "Fix all failing tests. Do not skip or delete tests."
    retry: 3
    depends_on: run-tests

  - name: create-pr
    type: claude
    repo: git@github.com:myorg/api.git
    ssh: true
    reuse_auth: true
    prompt: "Commit all changes, push to a new branch, create a PR titled 'fix: resolve test failures'"
    depends_on: fix-failures
```

```bash
# Dry run first to see the plan
agent pipeline ci-pipeline.yaml --dry-run

# Run it
agent pipeline ci-pipeline.yaml
```
