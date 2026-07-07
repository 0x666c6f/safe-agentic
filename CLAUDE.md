# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

An isolated environment for running AI coding agents (Claude Code, Codex) inside an Apple container machine with per-agent Docker containers. The design philosophy is **safe by default** — dangerous features (SSH forwarding, auth persistence) require explicit opt-in flags.

## Installation

**Homebrew (recommended):**

```bash
brew tap 0x666c6f/tap && brew install berth
```

This installs the CLI as `berth`. Check the installed version with `berth --version`.

**From source:** Clone the repo, run `make build-all`, and add `bin/` to your PATH. The binaries are `berth`, `berth-claude`, and `berth-codex`.

All documentation below uses `berth` as the command name.

## Architecture

```
macOS Host (berth Go CLI)
  └── Apple container machine "berth" (Alpine 3.22, hardened)
       └── Docker containers (ephemeral, per-agent)
            ├── Read-only rootfs + tmpfs scratch
            ├── cap-drop ALL + no-new-privileges
            ├── Per-session OAuth + ephemeral cache volumes
            ├── Dedicated bridge network per container
            └── SSH agent OFF unless --ssh
```

Three isolation boundaries: macOS ↔ VM (Apple container + hardening + userns-remap), VM ↔ container (Docker), container ↔ container (separate volumes/networks/namespaces).

Full diagrams in `docs/architecture.md`.

## Key Files & Relationships

### Go CLI (runs on macOS host)

- **`cmd/berth/`** — Cobra CLI binary. `main.go` (root command), `spawn.go` (spawn/run), `lifecycle.go` (list/attach/stop/cleanup/retry), `observe.go` (peek/output/summary/cost/audit/sessions/replay), `workflow.go` (diff/checkpoint/todo/pr/review), `fleet.go` (fleet/pipeline), `setup.go` (setup/update/vm/diagnose), `config_cmd.go` (config/template/mcp-login/aws-refresh).
- **`pkg/vmexec/`** — `Executor` interface wrapping `container machine run -n berth`. All Docker/VM commands go through this. `FakeExecutor` for testing.
- **`pkg/docker/`** — `DockerRunCmd` builder (type-safe replacement for bash arrays), container/volume/network/SSH/DinD management.
- **`pkg/validate/`** — Input validation: container names, network names, PIDs limits.
- **`pkg/repourl/`** — URL parsing with traversal prevention.
- **`pkg/config/`** — `defaults.sh` loading, git identity detection/parsing.
- **`pkg/inject/`** — Base64 encoding, Claude/Codex/AWS config injection.
- **`pkg/audit/`** — JSONL append-only audit log.
- **`pkg/tmux/`** — Tmux session management, pane capture.
- **`pkg/events/`** — Event emission, notification targets, budget monitoring.
- **`pkg/cost/`** — API cost computation with model pricing table.
- **`pkg/fleet/`** — YAML manifest parsing for fleet/pipeline orchestration.
- **`pkg/labels/`** — All `berth.*` Docker label constants.

### Container-side (runs inside Docker)

- **`vm/setup.sh`** — Idempotent VM bootstrap. Hardens Apple container machine, installs Docker with `userns-remap`, installs socat for SSH relay.
- **`Dockerfile`** — Pinned Codex/npm install plus checksum or GPG verification for several direct downloads. Uses `SHELL ["/bin/bash", "-o", "pipefail", "-c"]`. Non-root `agent` user, no sudo.
- **`entrypoint.sh`** — Container init: SSH config, git config, host config injection, security preamble, repo cloning, agent launch.
- **`bin/agent-session.sh`** — Agent session wrapper inside tmux. Handles Claude/Codex/shell modes.
- **`config/bashrc`** — Shell environment inside containers.
- **`config/security-preamble.md`** — Template for container security context.
- **`package.json`, `package-lock.json`** — Pins Codex CLI version.

## Commands

```bash
# First-time setup (creates VM, installs Docker, builds image)
berth setup

# Spawn agents
berth spawn claude --ssh --repo git@github.com:org/repo.git
berth spawn codex --ssh --reuse-auth --repo git@github.com:org/repo.git --name my-task
berth spawn codex --ssh --prompt 'Fix the CI tests' --repo git@github.com:org/repo.git
berth spawn claude --ssh --template security-audit --repo git@github.com:org/repo.git
berth spawn claude --ssh --instructions 'Focus on the auth module' --prompt 'Refactor auth' --repo ...
berth spawn claude --background --auto-trust --on-exit 'berth output --latest --json > out.json' --repo ...

# Quick start with smart defaults (auto-enables `--ssh` for SSH URLs)
berth run git@github.com:org/repo.git "Fix the CI tests"
berth run https://github.com/org/repo.git "Add unit tests"

# Agent-facing shortcuts (auto-detect SSH from URL)
berth-claude git@github.com:org/repo.git
berth-codex https://github.com/org/repo.git --dry-run

# Management
berth list                     # shows running + stopped containers
berth tui                      # k9s-style interactive dashboard (build: make -C tui)
# macOS desktop app (Wails v3): make -C app dev|build — see app/README.md
berth attach <name>            # reattach (restarts stopped containers)
berth stop <name|--all>        # stop + remove
berth cleanup                  # removes containers + managed networks (keeps auth)

# MCP OAuth login
berth mcp-login linear
berth mcp-login notion <container>

# Export session history
berth sessions <container>
berth sessions --latest ~/sessions/

# Peek at agent output without attaching
berth peek <container>                 # last 30 lines of tmux pane
berth peek --latest --lines 50         # more lines

# AWS credentials
berth spawn claude --ssh --aws my-aws-profile --repo git@github.com:org/repo.git
berth aws-refresh <container>              # refresh expired credentials
berth aws-refresh --latest my-profile      # refresh with different profile

# Image rebuild
berth update                   # cached build
berth update --quick           # bust only AI CLI layer
berth update --full            # no cache

# VM management
berth vm ssh                   # debug the VM
berth vm start                 # start + re-harden
berth vm stop

# Resource tuning
berth spawn claude --memory 16g --cpus 8 --pids-limit 1024 --repo ...

# Untrusted repos (no SSH, no internet)
berth spawn claude --repo https://... --network agent-isolated
```

```bash
# Spawn flags (new in v2)
#   --on-complete "cmd"     Run command on agent success
#   --on-fail "cmd"         Run command on agent failure
#   --notify targets        Notifications (terminal,slack,command:script)
#   --ephemeral-auth        One-off session, don't reuse auth volume
#   --max-cost N.NN         Kill agent if estimated cost exceeds budget

# Workflow
berth diff <name>|--latest [--stat]       # show git diff from agent working tree
berth checkpoint create <name> [label]     # snapshot working tree
berth checkpoint list <name>               # list snapshots
berth checkpoint revert <name> <ref>       # revert to snapshot
berth todo add <name> "text"               # add merge requirement
berth todo list <name>                     # show todos
berth todo check <name> <index>            # mark done
berth pr <name> [--title T --base B]       # create GitHub PR
berth review <name> [--base B]             # AI code review

# Output & inspection
berth output <name>|--latest              # last agent message
berth output --diff <name>                # git diff
berth output --files <name>               # list changed files
berth output --commits <name>             # git log
berth output --json <name>                # all as JSON
berth summary <name>|--latest             # one-screen overview
berth replay <name>|--latest              # replay session from event log

# Retry
berth retry <name>|--latest [--feedback "text"]  # re-run with same config

# Templates
berth template list                       # list built-in + custom templates
berth template show <name>                # print template prompt
berth template create <name>              # create custom template

# Config
berth config set|get|show|reset           # manage defaults

# Fleet & Pipelines
berth fleet manifest.yaml [--dry-run]      # spawn agents from manifest
berth fleet status                         # show running fleet progress
berth pipeline pipeline.yaml [--dry-run]   # run multi-step pipeline

# Analytics
berth cost <name>                          # estimate API spend
berth cost --history [7d]                  # historical cost from audit log
berth audit [--lines N]                    # show operation log
```

## Security Model

| Default | Override |
|---------|----------|
| SSH agent OFF | `--ssh` (uses socat relay in VM for userns-remap compat) |
| Per-container auth volume | `--reuse-auth` to opt into shared Claude/Codex auth |
| AWS credentials OFF | `--aws <profile>` (tmpfs-backed, refresh with `berth aws-refresh`) |
| Host config auto-injected (seeds only, no overwrite) | — |
| Security preamble injected into CLAUDE.md / AGENTS.md | — |
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
# Build binaries
make build-all

# Run the Go test suite
go test ./...

# Run a single Go package
go test ./pkg/docker/ -v
```

Go test packages in `pkg/` and `cmd/berth/`:
- `pkg/validate` — name, network, PIDs validation
- `pkg/vmexec` — Executor interface and FakeExecutor
- `pkg/repourl` — URL parsing, traversal prevention
- `pkg/config` — defaults loading, identity parsing
- `pkg/inject` — base64, config injection
- `pkg/audit` — JSONL audit log
- `pkg/docker` — DockerRunCmd, container, volume, network, SSH, DinD
- `pkg/tmux` — session management
- `pkg/events` — event emission, notifications, budget
- `pkg/cost` — model pricing, cost computation
- `pkg/fleet` — YAML manifest parsing
- `cmd/berth` — spawn parity, container name resolution, retry reconstruction

Shell runtime verification lives in focused smoke or integration tests rather than a standalone `tests/` bash suite.

## Conventions

- Go CLI uses cobra for commands, `pkg/vmexec.Executor` interface for all VM/Docker interaction.
- `DockerRunCmd` builder replaces bash `docker_cmd=()` arrays with type-safe methods.
- All container/network names validated via `pkg/validate` before use.
- Repo clone paths validated by `pkg/repourl.ClonePath()`: rejects traversal, dot-prefixed names, special characters.
- Tests use `vmexec.FakeExecutor` — no real Apple container/Docker needed.
- Container-side scripts (entrypoint, agent-session) remain bash — they run inside Docker where Go isn't installed.
- Dockerfile uses `SHELL ["/bin/bash", "-o", "pipefail", "-c"]` and verifies all downloads (SHA256 or GPG).
- Build context uses `git ls-files -c` filtered by `test -e` — only tracked files that exist on disk.

## Documentation

- `docs/install.md` — installation, VM setup, safe-agentic migration
- `docs/quickstart.md` — first agent in five minutes
- `docs/guide/` — task-oriented guides: spawning, managing, workflow, worktrees, fleet, automation, tui, app, configuration
- `docs/reference/cli.md` — exhaustive command reference; `docs/reference/tui.md`, `docs/reference/manifests.md`
- `docs/architecture.md` — isolation boundaries, networking, container internals
- `docs/security.md` — defaults, wideners, threat model, supply chain

## Skills

Agent skills in `.claude/skills/` and `.codex/skills/`:
- `agent-spawn` — spawn a sandboxed agent
- `agent-manage` — list/attach/stop/cleanup
- `agent-setup` — first-time setup, rebuild, troubleshooting
- `agent-orchestrate` — supervise multi-agent berth workflows
- `agent-manifest-author` — author fleet and pipeline manifests

## Commit Style & Releases

This project uses [Conventional Commits](https://www.conventionalcommits.org/). Commit prefixes drive automated releases:

| Prefix | Effect | Example |
|--------|--------|---------|
| `feat:` | minor bump | `v0.1.0` → `v0.2.0` |
| `fix:` | patch bump | `v0.1.0` → `v0.1.1` |
| `feat!:` or `BREAKING CHANGE` in body | major bump | `v0.1.0` → `v1.0.0` |
| `docs:`, `chore:`, `ci:`, `test:` | no release | — |

Scopes are optional: `feat(tui):`, `fix(ci):`, etc.

**Automated pipeline:** Every push to `main` triggers `.github/workflows/release.yml`:
1. Runs full CI suite
2. Computes semver from commit prefixes since last tag
3. Builds universal macOS TUI binary (`lipo` amd64 + arm64)
4. Packages tarball, creates GitHub Release with changelog
5. Updates Homebrew tap (`0x666c6f/homebrew-tap`) with new formula

**Version injection:** `cmd/berth/main.go` has `var Version = "dev"`. The release workflow injects the real version via `-ldflags "-X main.Version=X.Y.Z"`. `berth --version` prints `berth vX.Y.Z`.

**No release?** If all commits since last tag are `docs:`, `chore:`, `ci:`, or `test:`, no release is created.

## Known Limitations

- **Default posture: `--home-mount none`** — the host home is never shared with the VM (strongest isolation). Apple's `container` has no way to mount a single host directory into a machine (only `--home-mount ro|rw|none`), so `--worktree` (which needs a host directory bind-mounted into the agent) is **opt-in**: `berth setup --enable-worktrees` (or `berth config set defaults.worktrees_mount true`) switches the machine to `home-mount=rw` and, via `vm/setup.sh`, binds only the worktrees root (`~/.berth/worktrees`, or `defaults.worktrees_dir`) to a stable `/worktrees`, then **detaches** the rest of the home share and tmpfs-masks `/Users`, `/Volumes`, `/private`, `/mnt/mac`. Agent containers only bind-mount a per-agent subdir of `/worktrees`, so the only host path reachable is the worktrees root.
- **Enabling worktrees is a deliberate weakening of the VM boundary.** `home-mount=rw` shares the whole home with the machine at the virtiofs level; berth detaches/masks everything except the worktrees root, but a VM-root compromise or Docker escape could re-reach host home (default `home-mount=none` shares nothing, so it can't). Keep secrets and unrelated projects out of the worktrees root. `berth diagnose` reports the posture; `berth setup`/`berth vm start` reconcile the machine in either direction and re-assert the masks. A `--worktree-path` outside the worktrees root is rejected before launch. See the threat model in `docs/security.md`.
- VM internet egress relies on host pf NAT plus `net.inet.ip.forwarding=1`, applied during `berth setup`. A macOS reboot resets forwarding and flushes the pf anchor, so the VM loses egress (clones time out, agents die on startup). `berth vm start` now re-applies NAT, and `berth diagnose` flags the missing egress.
- `--dangerously-skip-permissions` lets Claude execute anything inside the container. With `--ssh`, this includes pushing to other repos.
- Codex runs in yolo mode (`--yolo`) for the same reason: the container is the sandbox.
- Build trusts upstream signing roots (apt GPG keys, npm registry). Direct-download binaries are pinned and checksum-verified.
