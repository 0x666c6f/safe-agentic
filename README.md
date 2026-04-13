# safe-agentic

[![CI](https://github.com/0x666c6f/safe-agentic/actions/workflows/ci.yml/badge.svg)](https://github.com/0x666c6f/safe-agentic/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.25.5-00ADD8?logo=go)](https://go.dev/)
[![Go Report](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2F0x666c6f%2Fsafe-agentic%2Fbadges%2F.github%2Fbadges%2Fgoreport.json)](https://github.com/0x666c6f/safe-agentic/actions/workflows/coverage.yml)
[![Coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2F0x666c6f%2Fsafe-agentic%2Fbadges%2F.github%2Fbadges%2Fcoverage.json)](https://github.com/0x666c6f/safe-agentic/actions/workflows/coverage.yml)

`safe-agentic` runs Claude Code and Codex inside Docker containers inside a hardened OrbStack VM.

Primary CLI: `safe-ag`.
Agent-facing shortcuts also ship: `safe-ag-claude`, `safe-ag-codex`.

The goal is simple:
- let the agent operate freely inside its own sandbox
- keep host access, shared auth, SSH, Docker daemon access, and private-network access opt-in
- make daily agent workflows practical, not theoretical

## What you get

- per-agent containers with read-only rootfs, `cap-drop ALL`, `no-new-privileges`, and resource limits
- a hardened OrbStack VM that acts as a host boundary between macOS and agent containers
- dedicated managed Docker networks by default
- tmux-backed sessions that you can reattach to later
- CLI + TUI + dashboard for spawning, monitoring, reviewing, and shipping work
- fleet and pipeline manifests for parallel and staged agent runs

## Core model

```text
macOS host
  -> OrbStack VM (safe-agentic)
    -> Docker daemon
      -> one container per agent
```

Default stance:
- no SSH forwarding
- no shared auth
- no AWS credentials
- no Docker daemon access
- read-only container rootfs
- dedicated managed network

## Install

Homebrew:

```bash
brew install orbstack
brew tap 0x666c6f/tap
brew install safe-agentic
```

From source:

```bash
brew install orbstack
git clone git@github.com:0x666c6f/safe-agentic.git
cd safe-agentic
make build-all
export PATH="$PWD/bin:$PATH"
```

## First run

```bash
safe-ag setup
safe-ag diagnose
```

`safe-ag setup` creates the VM, reapplies hardening, and builds the local image.

## First agent

Public repo:

```bash
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git
```

Private repo:

```bash
safe-ag spawn claude --ssh --repo git@github.com:myorg/myrepo.git
```

With an immediate task:

```bash
safe-ag spawn claude \
  --ssh \
  --repo git@github.com:myorg/myrepo.git \
  --prompt "Fix the failing CI tests"
```

## Daily commands

```bash
safe-ag list
safe-ag attach --latest
safe-ag peek --latest
safe-ag logs --latest
safe-ag diff --latest
safe-ag output --latest
safe-ag review --latest
safe-ag stop --latest
safe-ag cleanup --auth
safe-ag tui
```

## Common workflows

Single-agent loop:

```bash
safe-ag spawn claude --ssh --reuse-auth --repo git@github.com:org/api.git \
  --prompt "Fix the flaky test suite"

safe-ag peek --latest
safe-ag diff --latest
safe-ag review --latest
safe-ag pr --latest --title "fix: stabilize test suite"
```

Parallel fleet:

```bash
safe-ag fleet fleet.yaml
safe-ag tui
```

Staged pipeline:

```bash
safe-ag pipeline pipeline.yaml
safe-ag pipeline pipeline.yaml --dry-run
```

## Safety model

Three boundaries matter:

1. macOS host -> OrbStack VM
2. OrbStack VM -> container
3. container -> container

Important opt-in flags:

| Flag | Why you would use it | What it widens |
|---|---|---|
| `--ssh` | private repos, pushes | repo access through your SSH agent |
| `--reuse-auth` | avoid re-auth | shared agent auth volume |
| `--reuse-gh-auth` | `gh` inside containers | shared GitHub auth volume |
| `--aws <profile>` | infra work | AWS API access |
| `--docker` | build/test containers | DinD sidecar |
| `--docker-socket` | full Docker control | direct VM daemon access |
| `--network <name>` | custom connectivity | bypass managed network policy |

If you only need a public repo and a prompt, do not add flags you do not need.

## Docs map

- [Quickstart](docs/quickstart.md): install, setup, first session
- [Usage guide](docs/usage.md): command map by job
- [Spawning](docs/guide/spawning.md): repo/auth/network/runtime options
- [Managing](docs/guide/managing.md): attach, logs, copy, cleanup
- [Workflow](docs/guide/workflow.md): diff, retry, review, PRs
- [Fleet and pipelines](docs/guide/fleet.md): manifests and orchestration
- [Configuration](docs/guide/configuration.md): defaults, templates, VM/image maintenance
- [Architecture](docs/architecture.md): component map and reference pages
- [Security model](docs/security.md): defaults, threat surface, limitations

## Notes

- containers persist after the agent exits; `safe-ag attach` will restart stopped containers when needed
- `safe-ag cleanup` keeps shared auth by default; use `safe-ag cleanup --auth` for full reset
- `SAFE_AGENTIC_VM_NAME` lets you point the CLI at a different OrbStack VM
- `safe-ag-tui` is a separate binary; `safe-ag tui` is the normal entrypoint
