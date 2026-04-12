# safe-agentic

[![CI](https://github.com/0x666c6f/safe-agentic/actions/workflows/ci.yml/badge.svg)](https://github.com/0x666c6f/safe-agentic/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.25.5-00ADD8?logo=go)](https://go.dev/)
[![Coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2F0x666c6f%2Fsafe-agentic%2Fbadges%2F.github%2Fbadges%2Fcoverage.json)](https://github.com/0x666c6f/safe-agentic/actions/workflows/coverage.yml)

Isolated environment for running AI coding agents (Claude Code, Codex) safely. Safe by default: SSH forwarding is opt-in, auth is ephemeral unless reused explicitly, containers run read-only with all Linux capabilities dropped, `no-new-privileges`, dedicated per-session networks get egress guardrails, images are local-only at launch, and resource limits apply.

## Features

**Isolation & Security**
- Three isolation boundaries: macOS ↔ VM (OrbStack + hardening), VM ↔ container (Docker + userns-remap), container ↔ container (separate volumes/networks/namespaces)
- Read-only rootfs, `cap-drop ALL`, `no-new-privileges`, seccomp profile
- Per-container dedicated bridge networks with egress guardrails (TCP 22/80/443 only)
- VM hardening: macOS filesystem mounts blocked, integration commands removed
- Docker userns-remap: container UIDs remapped to unprivileged host UIDs
- GitHub SSH host keys baked with `StrictHostKeyChecking yes` — no TOFU
- Resource limits: memory, CPU, PIDs (configurable per-container)
- Build context safety: only git-tracked files sent to VM

**Agent Lifecycle**
- `--prompt 'task'` — send an initial task so the agent starts working immediately
- Tmux-backed reattach — `safe-ag attach`/TUI resume reopen the live agent session; detach with `Ctrl-b d`
- Container persistence — containers survive exit, `safe-ag attach` restarts stopped ones
- `safe-ag list` — show running + stopped containers with metadata
- `safe-ag stop` / `safe-ag cleanup` — stop, remove, and clean up resources
- `safe-ag cp` — extract files, logs, or build artifacts from containers
- `safe-ag sessions` — export session history for archival
- `safe-ag diagnose` — health check for OrbStack, VM, Docker, image, SSH, defaults

**Developer Workflow**
- `safe-ag diff` — show git diff from an agent's working tree
- `safe-ag checkpoint` — create/list/revert working tree snapshots (git stash refs)
- `safe-ag todo` — track merge requirements; blocks PR creation until all checked off
- `safe-ag pr` — create a GitHub PR from the agent's branch (push + `gh pr create`)
- `safe-ag review` — AI code review via `codex review` or raw diff fallback
- `safe-agentic.json` — lifecycle scripts (`setup` runs after repo clone)

**Fleet & Orchestration**
- `safe-ag fleet manifest.yaml` — spawn multiple agents from a YAML manifest
- `safe-ag pipeline pipeline.yaml` — multi-step workflows with retry, dependencies, failure handlers

**Analytics**
- `safe-ag cost` — estimate API spend by parsing session token usage
- `safe-ag audit` — append-only JSONL log of all spawn/stop/attach operations
- `safe-ag dashboard` — browser dashboard backed by the same poller as the TUI

**Auth & Config**
- `--ssh` — SSH agent forwarding via socat relay (userns-remap compatible)
- `--reuse-auth` — persist Claude/Codex OAuth tokens across sessions
- `--reuse-gh-auth` — persist GitHub CLI auth across sessions
- `--aws <profile>` — inject AWS credentials from `~/.aws/credentials`; refresh with `safe-ag aws-refresh`
- `safe-ag mcp-login` — MCP OAuth login (Linear, Notion, etc.) with token persistence
- Host config auto-injection: `~/.codex/config.toml` and `~/.claude/settings.json` carry MCP servers, model settings, features, and plugins into containers
- `--identity` — explicit git author/committer attribution
- `defaults.sh` — persistent defaults for memory, CPU, network, Docker, auth, identity

**Docker & Networking**
- `--docker` — per-session Docker-in-Docker sidecar
- `--docker-socket` — direct VM Docker daemon access
- `--network` — join custom or isolated Docker networks
- Multi-repo support: clone multiple repos into a single container

**Tools Included**
- AI agents: Claude Code, Codex
- SRE/DevOps: terraform, kubectl, helm, aws-cli, vault, docker, compose
- Modern CLI: ripgrep, fd, bat, eza, zoxide, fzf, jq, yq, delta, gh, socat
- Runtimes: Node.js 22, pnpm, Bun, Python 3.12, Go 1.23

## Installation

**Homebrew (recommended):**

```bash
brew tap 0x666c6f/tap && brew install safe-agentic
```

This installs `safe-ag` and `safe-ag-tui`. Check the installed version with `safe-ag --version`.

**From source:** Clone the repo, run `make build-all`, then add `bin/` to your PATH.

## Quick Start

```bash
safe-ag setup
safe-ag spawn claude --ssh --repo git@github.com:myorg/myrepo.git
safe-ag diagnose
```

- add `--reuse-auth` to keep Claude/Codex OAuth between sessions
- add `--reuse-gh-auth` to keep `gh auth login` state between sessions
- add `--prompt 'task'` to send an initial task to the agent
- add `--aws <profile>` to inject AWS credentials; refresh with `safe-ag aws-refresh <name>`
- add `--docker` for Docker-in-Docker, or `--docker-socket` to mount the VM daemon directly
- add `--identity 'You <you@example.com>'` to avoid `Agent <agent@localhost>` commits
- host `~/.codex/config.toml` and `~/.claude/settings.json` are auto-injected into containers (MCP servers, model settings carry over)
- containers persist after exit — `safe-ag attach` restarts stopped ones; tmux sessions detach with `Ctrl-b d`
- put defaults in `~/.config/safe-agentic/defaults.sh` for memory, CPUs, network, Docker mode, shared auth, and git identity
  - format: simple `KEY=value` lines only; no shell snippets

## Architecture

```mermaid
graph TB
    subgraph mac["macOS Host"]
        cli["safe-ag CLI"]
        op["1Password SSH Agent"]
        orb["OrbStack"]
    end

    subgraph vm["OrbStack VM: safe-agentic (Ubuntu 24.04, hardened)"]
        hard["VM Hardening<br/>macOS mounts blocked · integration cmds removed"]
        dockerd["Docker daemon (userns-remap)"]

        subgraph net1["Dedicated bridge network"]
            c1["Ephemeral container<br/>Read-only · cap-drop ALL · no-new-privileges<br/>8g mem · 4 CPU · 512 PIDs<br/>GitHub host keys baked"]
        end
    end

    cli -->|"orb run docker ..."| dockerd
    op -.->|"SSH socket<br/>(only with --ssh, :ro)"| c1
    dockerd --> c1
    orb --> vm
```

> **[Full architecture docs](docs/architecture.md)** — system overview, component map, sequence diagrams for setup, spawn, SSH auth, OAuth, container lifecycle, and image build.

## Threat model

**Goal:** Protect your macOS host and repos from unintended agent side-effects. Safe by default — dangerous features require explicit opt-in.

**What this protects against:**
- Agents modifying files outside their cloned repo (read-only rootfs + per-agent workspace volume)
- Agents accessing your macOS filesystem (VM hardened: macOS mounts blocked)
- Agents interfering with each other (per-agent containers, networks, auth)
- Agents reaching VM/private-network services by default (managed bridges block local/private egress and only allow TCP 22/80/443)
- Credential exposure (SSH agent OFF by default, per-session OAuth tokens)
- SSH MITM (GitHub host keys baked into image, StrictHostKeyChecking yes)
- Container privilege escalation (capabilities dropped, no sudo)

**Opt-in flags that widen the attack surface:**
- `--ssh` — Forwards SSH agent into container. Required for `git@` repos. A compromised agent could use SSH keys for other operations.
- `--reuse-auth` — Shares OAuth token volume across sessions. Compromised container could steal the token.
- `--reuse-gh-auth` — Shares GitHub CLI auth volume across sessions. Compromised container could steal the token.
- `--docker` — Starts a privileged Docker-in-Docker sidecar for the session. Needed only when the agent must build or run containers itself.
- `--docker-socket` — Mounts the VM Docker socket directly. Broadest Docker access; the agent can control the VM daemon.
- `--aws <profile>` — Injects AWS credentials from `~/.aws/credentials` into the container. Credentials are written to a tmpfs; a compromised container could use them to access AWS resources for the session duration.
- `--network <name>` — Joins an existing Docker network in the VM and bypasses the default managed-network egress guardrails. Use only for deliberately shared or isolated networks you created.

**Known limitations:**
- **OrbStack hardening is best-effort.** OrbStack does not yet support per-VM file sharing disable ([#169](https://github.com/orbstack/orbstack/issues/169)). `vm/setup.sh` mounts tmpfs over macOS paths and removes mac commands, but OrbStack may re-enable sharing on VM restart. Re-run `safe-ag setup` after VM restarts, and disable file sharing in OrbStack UI (Settings > Linux) for defense-in-depth.
- **`--dangerously-skip-permissions` is broad.** Claude Code in this mode can execute any command inside the container. With `--ssh`, a malicious repo could push to other repos or exfiltrate data over the network.
- **Codex yolo mode is equally broad.** Codex runs with `--yolo`, so it can execute any command inside the container. With `--ssh`, a malicious repo could push to other repos or exfiltrate data over the network.
- **Build chain still trusts upstream signing roots and registries.** Direct-download binaries are pinned and checksum-verified; apt repos are signed; npm packages are lockfile-pinned. A compromised upstream signing chain could still affect builds.

**For untrusted repos:**
```bash
# Create an isolated Docker network with no internet access (one-time)
safe-ag vm ssh
docker network create --internal agent-isolated
exit

# Spawn without SSH, on isolated network
safe-ag spawn claude --repo <untrusted-repo> --network agent-isolated
```

## Prerequisites

1. **OrbStack**: `brew install orbstack`
2. **1Password Desktop App** (for SSH keys):
   - Settings → Developer → Enable "Use the SSH Agent"
   - SSH key for GitHub configured in 1Password
3. **CLI**: Install via Homebrew (see [Installation](#installation) above), or add `safe-agentic/bin` to your PATH for source installs

For isolated local runs or dedicated test VMs, `SAFE_AGENTIC_VM_NAME` overrides the target OrbStack VM:

```bash
SAFE_AGENTIC_VM_NAME=safe-agentic-alt safe-ag list
```

## Setup

```bash
safe-ag setup
```

Creates OrbStack VM, hardens it, installs Docker, builds the agent image. Progress now prints numbered phases during VM bootstrap and image build.

**After VM restarts:** Run `safe-ag vm start` (auto re-applies hardening).

## Usage

### Spawn an agent

```bash
# Claude Code with SSH (required for git@ repos)
safe-ag spawn claude --ssh --repo git@github.com:myorg/myrepo.git

# HTTPS repo without SSH forwarding
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git

# Codex with persistent auth (skip OAuth next time)
safe-ag spawn codex --ssh --reuse-auth --repo git@github.com:myorg/myrepo.git

# Reuse GitHub CLI auth too
safe-ag spawn codex --ssh --reuse-auth --reuse-gh-auth --repo git@github.com:myorg/myrepo.git

# Docker support: DinD by default, host socket only when you ask for it
safe-ag shell --docker --repo https://github.com/myorg/myrepo.git
safe-ag shell --docker-socket --repo https://github.com/myorg/myrepo.git

# Named session
safe-ag spawn claude --ssh --repo git@github.com:myorg/api.git --name api-refactor

# Multiple repos (cloned as org/repo to avoid name collisions)
safe-ag spawn claude --ssh --repo git@github.com:myorg/api.git --repo git@github.com:other/api.git

# Name + auth reuse
safe-ag spawn claude --ssh --name api-fix --reuse-auth --identity 'You <you@example.com>' --repo git@github.com:myorg/api.git

# Untrusted repo — no SSH, isolated network
safe-ag spawn claude --repo https://github.com/myorg/untrusted.git --network agent-isolated

# Tune limits explicitly when needed
safe-ag shell --repo https://github.com/myorg/myrepo.git --memory 12g --cpus 6
```

### Manage agents

```bash
safe-ag list                  # List running + stopped agents
safe-ag attach <name>         # Tmux attach (restarts stopped containers)
safe-ag attach --latest       # Attach to newest agent
safe-ag cp <name> <container-path> <host-path>  # Copy files out safely
safe-ag cp --latest <container-path> <host-path>
safe-ag stop <name>           # Stop and remove specific agent
safe-ag stop --latest         # Stop and remove newest agent
safe-ag stop --all            # Stop and remove all agents
safe-ag cleanup               # Stop all + keep shared auth + prune managed networks
safe-ag cleanup --auth        # Also remove shared auth volumes
safe-ag mcp-login <server>    # MCP OAuth login (persists in auth volume)
safe-ag sessions <name>       # Export session history from container
safe-ag peek <name>           # Show last 30 lines of agent's tmux pane
safe-ag peek --latest --lines 50
safe-ag aws-refresh <name>    # Refresh AWS credentials in running container
safe-ag diagnose              # Check orb/VM/docker/image/SSH/defaults
safe-ag dashboard --bind localhost:8420   # Web dashboard
```

Use `safe-ag cp` when you need logs, test output, or build artifacts on the host without adding bind mounts:

```bash
safe-ag cp api-refactor /workspace/tmp/test.log ./test.log
safe-ag cp --latest /workspace/dist ./dist
```

Older `safe-ag cleanup` removed shared auth volumes too. Full reset now needs `safe-ag cleanup --auth`.

Claude and Codex sessions run inside `tmux` in the container with a large scrollback buffer. Detach with `Ctrl-b d`; reattach later with `safe-ag attach` or the TUI.

### Interactive shell (no agent, no auth)

```bash
safe-ag shell --ssh --repo git@github.com:myorg/myrepo.git
```

### Maintenance

```bash
safe-ag update                # Rebuild image
safe-ag update --quick        # Rebuild AI CLI layer only (fast)
safe-ag update --full         # Full rebuild, no cache
```

### VM management

```bash
safe-ag vm ssh                # SSH into the VM for debugging
safe-ag vm stop               # Stop the VM
safe-ag vm start              # Start the VM (re-applies hardening)
```

## Tools included

### AI Agents
- Claude Code (`claude`)
- Codex (`codex`)

### SRE/DevOps
terraform, kubectl, helm, aws-cli, vault, docker, docker compose

### Modern CLI
ripgrep (`rg`), fd, bat, eza, zoxide (`z`), fzf, jq, yq, delta, gh

### Runtimes
Node.js 22, `pnpm`, Bun, Python 3.12, Go 1.23

## Security defaults

| Feature | Default | Override |
|---------|---------|----------|
| SSH agent | OFF | `--ssh` (socat relay for userns-remap compat) |
| Auth persistence | Ephemeral per-session volume | `--reuse-auth` |
| GitHub CLI auth | Ephemeral per-session volume | `--reuse-gh-auth` |
| AWS credentials | OFF | `--aws <profile>` (tmpfs-backed, refresh with `safe-ag aws-refresh`) |
| Docker access | OFF | `--docker` (DinD) / `--docker-socket` |
| Root filesystem | Read-only | — |
| Capabilities | Dropped (`ALL`) + `no-new-privileges` | — |
| Network | Dedicated per-container bridge with local/private egress blocked; TCP 22/80/443 only | `--network <name>` |
| Resource limits | `--memory 8g --cpus 4 --pids-limit 512` | explicit flags |
| GitHub host keys | Baked & pinned (StrictHostKeyChecking yes) | — |
| Workspace/auth/cache volumes | Ephemeral | `--reuse-auth` / `--reuse-gh-auth` for auth only |
| Sudo | Removed | — |

## How auth works

### Claude Code / Codex (OAuth)

On first `safe-ag spawn`, the CLI shows an OAuth URL. Open it in your macOS browser to authenticate with your subscription.

- **Default**: OAuth token is stored in an anonymous per-session volume. You log in each time. Container exit discards the token.
- **`--reuse-auth`**: Token persists in a shared volume (`safe-ag spawn claude --repo-auth` / `safe-ag spawn codex --repo-auth`). Log in once, reuse across sessions.

### GitHub CLI (`gh`)

`gh` is installed in the image.

- **Default**: `gh auth login` state lives in an anonymous per-session volume at `/home/agent/.config/gh`.
- **`--reuse-gh-auth`**: GitHub CLI auth persists in `agent-gh-auth`. Reuse across sessions; remove with `safe-ag cleanup --auth`.

### Git (SSH via 1Password)

Only available when `--ssh` is passed:
```
git clone/push inside container
  → SSH agent socket forwarded: container → socat relay (VM) → macOS → 1Password
  → Uses SSH keys managed by 1Password
  → socat relay bridges userns-remap UID permissions
```

GitHub host keys are baked into the image with `StrictHostKeyChecking yes` — no trust-on-first-use.

### Git identity

Containers default to neutral git identity (`Agent <agent@localhost>`). Your host git `user.name` / `user.email` are no longer copied in automatically.

If you want explicit attribution, export it before launch:

```bash
GIT_AUTHOR_NAME="Your Name" \
GIT_AUTHOR_EMAIL="you@example.com" \
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git
```

`GIT_COMMITTER_NAME` / `GIT_COMMITTER_EMAIL` are also honored if you set them explicitly.

Or use a one-off flag:

```bash
safe-ag spawn claude --identity "Your Name <you@example.com>" --repo https://github.com/myorg/myrepo.git
```

Or set persistent defaults in `${XDG_CONFIG_HOME:-~/.config}/safe-agentic/defaults.sh`:

```bash
SAFE_AGENTIC_DEFAULT_MEMORY=16g
SAFE_AGENTIC_DEFAULT_CPUS=8
SAFE_AGENTIC_DEFAULT_NETWORK=agent-isolated
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=true
SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH=true
SAFE_AGENTIC_DEFAULT_DOCKER=true
SAFE_AGENTIC_DEFAULT_IDENTITY="Your Name <you@example.com>"
```

Use simple `KEY=value` assignments only. `defaults.sh` is parsed as config, not executed as shell.

### Docker inside agents

`docker` and Compose are installed in the image, but daemon access stays off unless you opt in:

- `--docker`: starts a per-session DinD sidecar; safer default when the agent needs to build or run containers
- `--docker-socket`: mounts `/var/run/docker.sock` from the VM directly; use only when the agent must control the VM daemon itself

If you want Docker on by default, set `SAFE_AGENTIC_DEFAULT_DOCKER=true` in `defaults.sh`.

### Launch behavior

`safe-ag spawn` / `safe-ag shell` now require the VM to already have `safe-agentic:latest`. They will not auto-pull from a registry. If the image is missing, run `safe-ag update` or `safe-ag setup`.

### Host config injection

Your host `~/.codex/config.toml` and `~/.claude/settings.json` are automatically injected into containers on first launch. MCP server definitions, model settings, and feature flags carry over. If config already exists in the auth volume (from a prior run or `mcp-login`), the host config is not re-injected.

If no host config is found, safe-agentic writes minimal defaults:

- Codex: `~/.codex/config.toml` with `approval_policy = "never"` and `sandbox_mode = "danger-full-access"`
- Claude: `~/.claude/settings.json` with bypass-permissions mode

This keeps manual `codex` / `claude` runs from `safe-ag shell` aligned with the sandbox model.

### Build context safety

`safe-ag update` sends only git-tracked files that exist on disk to the VM. Untracked files (including `.env` or scratch files) are excluded from the build context.

## More docs

- **[Quickstart](docs/quickstart.md)** — from zero to a sandboxed agent in 5 minutes
- **[Architecture](docs/architecture.md)** — system diagrams, component map, sequence flows
- **[Usage guide](docs/usage.md)** — all commands, options, defaults, and troubleshooting
- **[Security model](docs/security.md)** — isolation boundaries, threat model, supply chain hardening
