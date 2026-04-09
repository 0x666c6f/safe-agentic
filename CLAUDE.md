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
            ├── cap-drop ALL + no-new-privileges
            ├── Per-session OAuth + ephemeral cache volumes
            ├── Dedicated bridge network per container
            └── SSH agent OFF unless --ssh
```

Three isolation boundaries: macOS ↔ VM (OrbStack + hardening + userns-remap), VM ↔ container (Docker), container ↔ container (separate volumes/networks/namespaces).

Full diagrams in `docs/architecture.md`.

## Key Files & Relationships

- **`bin/agent`** — Host-side CLI dispatcher. All commands (`spawn`, `setup`, `update`, etc.) are `cmd_*` functions. Sources `bin/agent-lib.sh` for container/network helpers. Talks to VM via `orb run -m safe-agentic`.
- **`bin/agent-lib.sh`** — Shared functions: input validation (`validate_name_component`, `validate_network_name`), network lifecycle (`create_managed_network`, `remove_managed_network`), container runtime construction (`build_container_runtime`, `append_runtime_hardening`), volume helpers. Docker commands built as bash arrays to prevent injection.
- **`bin/agent-claude`, `bin/agent-codex`** — Quick aliases that auto-detect SSH URLs (`git@`, `ssh://`) and delegate to `bin/agent spawn`.
- **`vm/setup.sh`** — Idempotent VM bootstrap. Hardens OrbStack (blocks macOS mounts with tmpfs, removes `open`/`osascript`/`code`, masks OrbStack integration dirs), installs Docker CE with `userns-remap`, installs socat for SSH relay and MCP bridging. Re-run on every `agent vm start`.
- **`Dockerfile`** — All binary downloads pinned with SHA256 checksums (or GPG for AWS CLI). Uses `SHELL ["/bin/bash", "-o", "pipefail", "-c"]`. Codex CLI installed via `npm ci` with lockfile; Claude Code installed via official installer (version-pinned, verified by `claude --version`). Non-root `agent` user, no sudo, no supplemental groups.
- **`entrypoint.sh`** — Container init: copies baked SSH config to tmpfs, writes git config from host env vars (`GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`), injects host config (`~/.codex/config.toml`, `~/.claude/settings.json`) if not already present in the auth volume, writes AWS credentials if injected, validates and clones repos via `repo_clone_path()` (rejects traversal/injection), launches agent inside tmux or starts interactive shell.
- **`bin/agent-session.sh`** — Agent session wrapper launched inside tmux. Handles Claude (`--dangerously-skip-permissions` via `script` PTY), Codex (`--yolo`), and shell modes. Detects resume vs fresh start via state file.
- **`config/bashrc`** — Shell environment inside containers. Modern tool aliases (rg, fd, bat, eza).
- **`package.json`, `package-lock.json`** — Pins Codex CLI version for reproducible `npm ci` installs.
- **`op-env.sh`** — Template for optional 1Password secret injection. Not used for base OAuth flow.

## Commands

```bash
# First-time setup (creates VM, installs Docker, builds image)
agent setup

# Spawn agents
agent spawn claude --ssh --repo git@github.com:org/repo.git
agent spawn codex --ssh --reuse-auth --repo git@github.com:org/repo.git --name my-task
agent spawn codex --ssh --prompt 'Fix the CI tests' --repo git@github.com:org/repo.git

# Quick aliases (auto-detect SSH from URL)
agent-claude git@github.com:org/repo.git
agent-codex https://github.com/org/repo.git

# Management
agent list                     # shows running + stopped containers
agent tui                      # k9s-style interactive dashboard (build: make -C tui)
agent attach <name>            # reattach (restarts stopped containers)
agent stop <name|--all>        # stop + remove
agent cleanup                  # removes containers + managed networks (keeps auth)

# MCP OAuth login (token persists in auth volume)
agent mcp-login linear
agent mcp-login <container> notion

# Export session history
agent sessions <container>
agent sessions --latest ~/sessions/

# Peek at agent output without attaching
agent peek <container>                 # last 30 lines of tmux pane
agent peek --latest --lines 50         # more lines

# AWS credentials
agent spawn claude --ssh --aws morpho-infra-terraform-k8s --repo git@github.com:org/repo.git
agent aws-refresh <container>              # refresh expired credentials
agent aws-refresh --latest perso           # refresh with different profile

# Image rebuild
agent update                   # cached build
agent update --quick           # bust only AI CLI layer
agent update --full            # no cache

# VM management
agent vm ssh                   # debug the VM
agent vm start                 # start + re-harden
agent vm stop

# Resource tuning
agent spawn claude --memory 16g --cpus 8 --pids-limit 1024 --repo ...

# Untrusted repos (no SSH, no internet)
agent spawn claude --repo https://... --network agent-isolated
```

## Security Model

| Default | Override |
|---------|----------|
| SSH agent OFF | `--ssh` (uses socat relay in VM for userns-remap compat) |
| Per-session auth (ephemeral volume) | `--reuse-auth` |
| AWS credentials OFF | `--aws <profile>` (tmpfs-backed, refresh with `agent aws-refresh`) |
| Host config auto-injected (seeds only, no overwrite) | — |
| Read-only rootfs | — |
| cap-drop ALL + no-new-privileges | — |
| Dedicated bridge network per container | `--network <name>` |
| Memory 8g, CPU 4, PIDs 512 | `--memory`, `--cpus`, `--pids-limit` |
| GitHub SSH host keys baked + StrictHostKeyChecking yes | — |
| Docker userns-remap in VM | — |
| No sudo, no supplemental groups | — |
| Git identity from host env vars | — |

Unsafe Docker flags (`--privileged`, `host` network, `--` passthrough) are blocked. The `--network` flag validates against `host`, `bridge`, and `container:*` modes.

## Testing

```bash
# Run all tests (14 suites, 200+ assertions)
bash tests/run-all.sh

# Run a single suite
bash tests/test-docker-cmd.sh

# Syntax check only
bash tests/test-syntax.sh
```

Test files in `tests/`:
- `test-syntax.sh` — `bash -n` on all scripts
- `test-validation.sh` — name and network input validation
- `test-repo-clone-path.sh` — URL parsing, traversal, injection
- `test-docker-cmd.sh` — security flags, volumes, SSH, git identity
- `test-dockerfile.sh` — checksums, no curl|bash, non-root user
- `test-entrypoint.sh` — git config, clone validation, agent launch
- `test-vm-setup.sh` — mount blocking, userns-remap, fstab
- `test-cli-dispatch.sh` — help, errors, aliases, multi-repo
- `test-agent-lifecycle.sh` — attach, stop, cleanup, custom networks
- `test-update.sh` — tracked-only build context, --quick/--full
- `test-live-integration.sh` — real VM/Docker smoke tests (optional, skip-aware)
- `agent-cli-security.sh` — end-to-end security regressions

Tests use a fake `orb` binary to capture docker commands without a real VM.

## Conventions

- All bash scripts use `set -euo pipefail`.
- Docker commands built as bash arrays (`local -a docker_cmd=(...)`) to prevent injection.
- VM operations go through `vm_exec()` / `orb run -m "$VM_NAME"`.
- Dockerfile uses `SHELL ["/bin/bash", "-o", "pipefail", "-c"]` and verifies all downloads (SHA256 or GPG).
- Read-only rootfs pattern: bake configs into `.ssh.baked/`, entrypoint copies to tmpfs at runtime.
- Repo clone paths validated by `repo_clone_path()`: rejects traversal, dot-prefixed names, special characters; only `https://` and `ssh://` URL schemes plus scp-style `git@host:org/repo`.
- Build context uses `git ls-files -c` filtered by `test -e` — only tracked files that exist on disk.
- Input validation: container names via `validate_name_component`, network names via `validate_network_name` (blocks `host`, `bridge`, `container:*`).

## Documentation

- `docs/architecture.md` — Mermaid diagrams: system overview, isolation boundaries, component map, sequence flows (setup, spawn, SSH auth, OAuth, lifecycle, build)
- `docs/quickstart.md` — 5-step getting started
- `docs/usage.md` — full command reference with workflows
- `docs/security.md` — threat model, supply chain, filesystem layout

## Skills

Agent skills in `.claude/skills/` and `.codex/skills/`:
- `agent-spawn` — spawn a sandboxed agent
- `agent-manage` — list/attach/stop/cleanup
- `agent-setup` — first-time setup, rebuild, troubleshooting

## Known Limitations

- OrbStack VM hardening is best-effort — no per-VM file sharing disable yet ([#169](https://github.com/orbstack/orbstack/issues/169)). Re-harden on VM restart with `agent vm start`.
- `--dangerously-skip-permissions` lets Claude execute anything inside the container. With `--ssh`, this includes pushing to other repos.
- Codex runs in yolo mode (`--yolo`) for the same reason: the container is the sandbox.
- Build trusts upstream signing roots (apt GPG keys, npm registry). Direct-download binaries are pinned and checksum-verified.
