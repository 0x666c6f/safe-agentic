<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/brand/berth-readme-header-dark-800x200.png">
    <img src="docs/assets/brand/berth-readme-header-light-800x200.png" alt="berth" width="400">
  </picture>
</p>

[![CI](https://github.com/0x666c6f/berth/actions/workflows/ci.yml/badge.svg)](https://github.com/0x666c6f/berth/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.25.5-00ADD8?logo=go)](https://go.dev/)
[![Go Report](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2F0x666c6f%2Fberth%2Fbadges%2F.github%2Fbadges%2Fgoreport.json)](https://github.com/0x666c6f/berth/actions/workflows/coverage.yml)
[![Coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2F0x666c6f%2Fberth%2Fbadges%2F.github%2Fbadges%2Fcoverage.json)](https://github.com/0x666c6f/berth/actions/workflows/coverage.yml)

`berth` runs Claude Code and Codex inside Docker containers inside a hardened Apple container machine.

Primary CLI: `berth`.
Agent-facing shortcuts also ship: `berth-claude`, `berth-codex`.

The goal is simple:
- let the agent operate freely inside its own sandbox
- keep host access, shared auth, SSH, Docker daemon access, and private-network access opt-in
- make daily agent workflows practical, not theoretical

## What you get

- per-agent containers with read-only rootfs, `cap-drop ALL`, `no-new-privileges`, and resource limits
- a hardened Apple container machine that acts as a host boundary between macOS and agent containers
- dedicated managed Docker networks by default
- tmux-backed sessions that you can reattach to later
- CLI + TUI for spawning, steering, monitoring, reviewing, and shipping work
- managed worktrees for isolated host checkouts with handoff/snapshot helpers (opt-in: `berth setup --enable-worktrees`, which deliberately widens the VM boundary — see the threat model)
- saved profiles, project/user actions, timeline, inbox, browser capture, workspace file ops, JSON stdio server, and log search for daily loops
- fleet and pipeline manifests for parallel and staged agent runs

## Core model

```text
macOS host
  -> Apple container machine (berth)
    -> Docker daemon
      -> one container per agent
```

Default stance:
- no SSH forwarding
- no shared auth
- no host Claude/Codex auth seeding
- no AWS credentials
- no Docker daemon access
- read-only container rootfs
- dedicated managed network

## Install

Install Apple container from the signed pkg on GitHub Releases, then install berth with Homebrew:

```bash
open https://github.com/apple/container/releases
brew tap 0x666c6f/tap
brew install berth
```

From source:

```bash
open https://github.com/apple/container/releases
git clone git@github.com:0x666c6f/berth.git
cd berth
make build-all
export PATH="$PWD/bin:$PATH"
```

## First run

```bash
berth setup
berth diagnose
```

`berth setup` starts Apple container, creates the machine, reapplies hardening, and builds the local image.
It may ask for macOS administrator approval to enable IP forwarding and load a `com.apple/berth` PF NAT anchor for Apple vmnet and nested Docker egress.

## First agent

Public repo:

```bash
berth spawn claude --repo https://github.com/myorg/myrepo.git
```

Private repo:

```bash
berth spawn claude --ssh --repo git@github.com:myorg/myrepo.git
```

With an immediate task:

```bash
berth spawn claude \
  --ssh \
  --repo git@github.com:myorg/myrepo.git \
  --prompt "Fix the failing CI tests"
```

## Daily commands

```bash
berth list
berth attach --latest
berth peek --latest
berth logs --latest
berth diff --latest
berth output --latest
berth review --latest
berth pr-review
berth pr-fix
berth stop --latest
berth cleanup --auth
berth tui
```

## Common workflows

Single-agent loop:

```bash
berth spawn claude --ssh --reuse-auth --repo git@github.com:org/api.git \
  --prompt "Fix the flaky test suite"

berth peek --latest
berth diff --latest
berth review --latest
berth pr --latest --title "fix: stabilize test suite"
```

Parallel fleet:

```bash
berth fleet fleet.yaml
berth tui
```

Staged pipeline:

```bash
berth pipeline pipeline.yaml
berth pipeline pipeline.yaml --dry-run
```

## Safety model

Three boundaries matter:

1. macOS host -> Apple container machine
2. Apple container machine -> container
3. container -> container

Important opt-in flags:

| Flag | Why you would use it | What it widens |
|---|---|---|
| `--ssh` | private repos, pushes | repo access through your SSH agent |
| `--reuse-auth` | avoid re-auth | shared agent auth volume |
| `--reuse-gh-auth` | `gh` inside containers | shared GitHub auth volume |
| `--seed-auth` | skip first login for this session | one-shot copy of host Claude/Codex auth |
| `--aws <profile>` | infra work | AWS API access |
| `--docker` | build/test containers | DinD sidecar |
| `--docker-socket` | full Docker control | direct VM daemon access |
| `--network <name>` | custom connectivity | bypass managed network policy |

If you only need a public repo and a prompt, do not add flags you do not need.

Hard local policy can deny risky spawn modes before any network, worktree, or container is created:

```toml
# ~/.berth/rules.toml or .berth/rules.toml
[allow]
docker_modes = ["off"]
networks = ["managed"]
ssh = false
reuse_auth = false
seed_auth = false
```

## Docs map

- [Quickstart](docs/quickstart.md): install, setup, first session
- [Usage guide](docs/usage.md): command map by job
- [Spawning](docs/guide/spawning.md): repo/auth/network/runtime options
- [Managing](docs/guide/managing.md): attach, logs, sessions, cleanup
- [Workflow](docs/guide/workflow.md): diff, retry, review, PRs
- [Fleet and pipelines](docs/guide/fleet.md): manifests and orchestration
- [Configuration](docs/guide/configuration.md): defaults, policy rules, profiles, actions, templates, VM/image maintenance
- [Codex App Parity Roadmap](docs/roadmap/codex-app-parity.md): UX roadmap and implementation tracks
- [Architecture](docs/architecture.md): component map and reference pages
- [Security model](docs/security.md): defaults, threat surface, limitations

## Notes

- containers persist after the agent exits; `berth attach` will restart stopped containers when needed
- `berth cleanup` keeps auth volumes by default; use `berth cleanup --auth` for full reset
- `BERTH_VM_NAME` lets you point the CLI at a different Apple container machine
- `BERTH_CONFIG_HOME` / `BERTH_STATE_HOME` relocate berth files without changing `HOME`
- `berth-tui` is a separate binary; `berth tui` is the normal entrypoint
