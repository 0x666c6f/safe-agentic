# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

An isolated environment for running AI coding agents (Claude Code, Codex) inside an OrbStack VM with per-agent Docker containers. The design philosophy is **safe by default** — dangerous features (SSH forwarding, auth persistence) require explicit opt-in flags.

## Architecture

```
macOS Host (bin/agent CLI)
  └── OrbStack VM "safe-agentic" (Ubuntu 24.04, hardened)
       └── Docker containers (ephemeral, per-agent)
            ├── Read-only rootfs + tmpfs scratch
            ├── Per-session OAuth + per-agent caches
            └── SSH agent OFF unless --ssh
```

Three isolation boundaries: macOS ↔ VM (OrbStack + hardening), VM ↔ container (Docker), container ↔ container (separate volumes/namespaces).

## Key Files & Relationships

- **`bin/agent`** — Host-side CLI dispatcher. All commands (`spawn`, `setup`, `update`, etc.) are `cmd_*` functions. Uses bash arrays for docker commands (prevents shell-splitting injection). Talks to VM via `orb run -m safe-agentic`.
- **`bin/agent-claude`, `bin/agent-codex`** — Quick aliases that add `--ssh` by default and delegate to `bin/agent spawn`.
- **`vm/setup.sh`** — Idempotent VM bootstrap. Hardens OrbStack (blocks macOS mounts with tmpfs, removes `open`/`osascript`/`code`), installs Docker CE. Re-run on every `agent vm start` because OrbStack may restore mounts.
- **`Dockerfile`** — 6 layers ordered by change frequency: system packages → runtimes (Node 22, Python 3.12, Go 1.23) → SRE tools → modern CLI → AI CLIs (cache-bust ARG) → user setup. No sudo. GitHub SSH host keys baked in. Configs stored in `.ssh.baked/` and `.config.baked/` (read-only rootfs; entrypoint copies to tmpfs).
- **`entrypoint.sh`** — Container init: copies baked configs to writable tmpfs, configures git, clones repos into `/workspace/<org>/<repo>`, launches agent or shell based on `AGENT_TYPE` env var.
- **`config/bashrc`, `config/starship.toml`** — Shell environment inside containers. Modern tool aliases (rg, fd, bat, eza).
- **`op-env.sh`** — Template for optional 1Password secret injection (AWS creds, etc.). Not used for base OAuth flow.

## Commands

```bash
# First-time setup (creates VM, installs Docker, builds image)
agent setup

# Spawn agents
agent spawn claude --ssh --repo git@github.com:org/repo.git
agent spawn codex --ssh --reuse-auth --repo git@github.com:org/repo.git --name my-task

# Quick aliases (include --ssh)
agent-claude git@github.com:org/repo.git
agent-codex git@github.com:org/repo.git

# Management
agent list
agent attach <name>
agent stop <name|--all>
agent cleanup                  # removes containers + per-agent volumes

# Image rebuild
agent update                   # cached build
agent update --quick           # bust only AI CLI layer
agent update --full            # no cache

# VM management
agent vm ssh                   # debug the VM
agent vm start                 # start + re-harden
agent vm stop
```

## Security Model

| Default | Override |
|---------|----------|
| SSH agent OFF | `--ssh` |
| Per-session auth (discarded on cleanup) | `--reuse-auth` |
| Read-only rootfs | `-- --read-write` |
| Capabilities dropped (ALL except SETUID/SETGID) | `-- --cap-add=...` |
| GitHub SSH host keys baked + StrictHostKeyChecking yes | — |
| Per-agent cache volumes | — |
| No sudo | — |

Extra docker flags go after `--` and are inserted **before** the image name (they're docker run flags, not entrypoint args).

## Conventions

- All bash scripts use `set -euo pipefail`.
- Docker commands built as bash arrays (`local -a docker_cmd=(...)`) to prevent injection.
- VM operations go through `vm_exec()` / `orb run -m "$VM_NAME"`.
- Dockerfile layers are architecture-aware (`dpkg --print-architecture` + conditionals for arm64/x86_64).
- Read-only rootfs pattern: bake configs into `.foo.baked/`, entrypoint copies to tmpfs-mounted `.foo/` at runtime.
- Idempotent operations: check state before applying (e.g., `mountpoint -q`, `command -v`, `groups | grep -q`).
- Repo clone paths use `org/repo` (extracted from git URL via sed) to avoid basename collisions.
- Build context uses `git ls-files -c` filtered by `test -e` — only tracked files that exist on disk. No untracked files, no deleted-but-tracked files.

## Known Limitations

- OrbStack VM hardening is best-effort — no per-VM file sharing disable yet ([#169](https://github.com/orbstack/orbstack/issues/169)). Re-harden on VM restart.
- `--dangerously-skip-permissions` lets Claude execute anything inside the container. With `--ssh`, this includes pushing to other repos.
- Some build dependencies use mutable upstream sources (`curl | bash`, GitHub `/latest`).
