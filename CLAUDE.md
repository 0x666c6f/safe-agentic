# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

An isolated environment for running AI coding agents (Claude Code, Codex) inside an OrbStack VM with per-agent Docker containers. The design philosophy is **safe by default** — dangerous features (SSH forwarding, auth persistence) require explicit opt-in flags.

## Installation

**Homebrew (recommended):**

```bash
brew tap 0x666c6f/tap && brew install safe-agentic
```

This installs the CLI as `safe-ag`, `safe-ag-claude`, and `safe-ag-codex`. Check the installed version with `safe-ag --version`.

**From source:** Clone the repo, run `make build-all`, and add `bin/` to your PATH. The binaries are `safe-ag`, `safe-ag-claude`, and `safe-ag-codex`.

All documentation below uses `safe-ag` as the command name.

## Architecture

```
macOS Host (safe-ag Go CLI)
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

### Go CLI (runs on macOS host)

- **`cmd/safe-ag/`** — Cobra CLI binary. `main.go` (root command), `spawn.go` (spawn/run), `lifecycle.go` (list/attach/stop/cleanup/retry), `observe.go` (peek/output/summary/cost/audit/sessions/replay), `workflow.go` (diff/checkpoint/todo/pr/review), `fleet.go` (fleet/pipeline), `setup.go` (setup/update/vm/diagnose), `config_cmd.go` (config/template/mcp-login/aws-refresh).
- **`pkg/orb/`** — `Executor` interface wrapping `orb run -m safe-agentic`. All Docker/VM commands go through this. `FakeExecutor` for testing.
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
- **`pkg/labels/`** — All `safe-agentic.*` Docker label constants.

### Container-side (runs inside Docker)

- **`vm/setup.sh`** — Idempotent VM bootstrap. Hardens OrbStack, installs Docker CE with `userns-remap`, installs socat for SSH relay.
- **`Dockerfile`** — All binary downloads pinned with SHA256 checksums (or GPG for AWS CLI). Uses `SHELL ["/bin/bash", "-o", "pipefail", "-c"]`. Non-root `agent` user, no sudo.
- **`entrypoint.sh`** — Container init: SSH config, git config, host config injection, security preamble, repo cloning, agent launch.
- **`bin/agent-session.sh`** — Agent session wrapper inside tmux. Handles Claude/Codex/shell modes.
- **`config/bashrc`** — Shell environment inside containers.
- **`config/security-preamble.md`** — Template for container security context.
- **`package.json`, `package-lock.json`** — Pins Codex CLI version.

## Commands

```bash
# First-time setup (creates VM, installs Docker, builds image)
safe-ag setup

# Spawn agents
safe-ag spawn claude --ssh --repo git@github.com:org/repo.git
safe-ag spawn codex --ssh --reuse-auth --repo git@github.com:org/repo.git --name my-task
safe-ag spawn codex --ssh --prompt 'Fix the CI tests' --repo git@github.com:org/repo.git
safe-ag spawn claude --ssh --template security-audit --repo git@github.com:org/repo.git
safe-ag spawn claude --ssh --instructions 'Focus on the auth module' --prompt 'Refactor auth' --repo ...
safe-ag spawn claude --background --auto-trust --on-exit 'safe-ag output --latest --json > out.json' --repo ...

# Quick start with smart defaults
safe-ag run git@github.com:org/repo.git "Fix the CI tests"
safe-ag run https://github.com/org/repo.git "Add unit tests"

# Agent-facing shortcuts (auto-detect SSH from URL)
safe-ag-claude git@github.com:org/repo.git
safe-ag-codex https://github.com/org/repo.git --dry-run

# Management
safe-ag list                     # shows running + stopped containers
safe-ag tui                      # k9s-style interactive dashboard (build: make -C tui)
safe-ag dashboard [--bind host:port]  # start web dashboard
safe-ag attach <name>            # reattach (restarts stopped containers)
safe-ag stop <name|--all>        # stop + remove
safe-ag cleanup                  # removes containers + managed networks (keeps auth)

# MCP OAuth login (token persists in auth volume)
safe-ag mcp-login linear
safe-ag mcp-login notion <container>

# Export session history
safe-ag sessions <container>
safe-ag sessions --latest ~/sessions/

# Peek at agent output without attaching
safe-ag peek <container>                 # last 30 lines of tmux pane
safe-ag peek --latest --lines 50         # more lines

# AWS credentials
safe-ag spawn claude --ssh --aws my-aws-profile --repo git@github.com:org/repo.git
safe-ag aws-refresh <container>              # refresh expired credentials
safe-ag aws-refresh --latest my-profile      # refresh with different profile

# Image rebuild
safe-ag update                   # cached build
safe-ag update --quick           # bust only AI CLI layer
safe-ag update --full            # no cache

# VM management
safe-ag vm ssh                   # debug the VM
safe-ag vm start                 # start + re-harden
safe-ag vm stop

# Resource tuning
safe-ag spawn claude --memory 16g --cpus 8 --pids-limit 1024 --repo ...

# Untrusted repos (no SSH, no internet)
safe-ag spawn claude --repo https://... --network agent-isolated
```

```bash
# Spawn flags (new in v2)
#   --on-complete "cmd"     Run command on agent success
#   --on-fail "cmd"         Run command on agent failure
#   --notify targets        Notifications (terminal,slack,command:script)
#   --ephemeral-auth        One-off session, don't reuse auth volume
#   --max-cost N.NN         Kill agent if estimated cost exceeds budget

# Workflow
safe-ag diff <name>|--latest [--stat]       # show git diff from agent working tree
safe-ag checkpoint create <name> [label]     # snapshot working tree
safe-ag checkpoint list <name>               # list snapshots
safe-ag checkpoint revert <name> <ref>       # revert to snapshot
safe-ag checkpoint fork <name> <new-name> [label]  # fork from checkpoint snapshot
safe-ag todo add <name> "text"               # add merge requirement
safe-ag todo list <name>                     # show todos
safe-ag todo check <name> <index>            # mark done
safe-ag pr <name> [--title T --base B]       # create GitHub PR
safe-ag review <name> [--base B]             # AI code review

# Output & inspection
safe-ag output <name>|--latest              # last agent message
safe-ag output --diff <name>                # git diff
safe-ag output --files <name>               # list changed files
safe-ag output --commits <name>             # git log
safe-ag output --json <name>                # all as JSON
safe-ag summary <name>|--latest             # one-screen overview
safe-ag replay <name>|--latest              # replay session from event log

# Retry
safe-ag retry <name>|--latest [--feedback "text"]  # re-run with same config

# Templates
safe-ag template list                       # list built-in + custom templates
safe-ag template show <name>                # print template prompt
safe-ag template create <name>              # create custom template

# Config
safe-ag config set|get|show|reset           # manage defaults

# Fleet & Pipelines
safe-ag fleet manifest.yaml [--dry-run]      # spawn agents from manifest
safe-ag fleet status                         # show running fleet progress
safe-ag pipeline pipeline.yaml [--dry-run]   # run multi-step pipeline

# Analytics
safe-ag cost <name>                          # estimate API spend
safe-ag cost --history [7d]                  # historical cost from audit log
safe-ag audit [--lines N]                    # show operation log
```

## Security Model

| Default | Override |
|---------|----------|
| SSH agent OFF | `--ssh` (uses socat relay in VM for userns-remap compat) |
| Shared auth volume (default) | `--ephemeral-auth` for one-off sessions |
| AWS credentials OFF | `--aws <profile>` (tmpfs-backed, refresh with `safe-ag aws-refresh`) |
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
# Run Go tests (13 packages, 156+ tests)
make test

# Run all tests (Go + legacy bash)
make test && bash tests/run-all.sh

# Run a single Go package
go test ./pkg/docker/ -v

# Run a single bash suite
bash tests/test-docker-cmd.sh
```

Go test packages in `pkg/` and `cmd/safe-ag/`:
- `pkg/validate` — name, network, PIDs validation
- `pkg/orb` — Executor interface and FakeExecutor
- `pkg/repourl` — URL parsing, traversal prevention
- `pkg/config` — defaults loading, identity parsing
- `pkg/inject` — base64, config injection
- `pkg/audit` — JSONL audit log
- `pkg/docker` — DockerRunCmd, container, volume, network, SSH, DinD
- `pkg/tmux` — session management
- `pkg/events` — event emission, notifications, budget
- `pkg/cost` — model pricing, cost computation
- `pkg/fleet` — YAML manifest parsing
- `cmd/safe-ag` — spawn parity, container name resolution, retry reconstruction

Legacy bash tests in `tests/` (for entrypoint, Dockerfile, VM setup — things that still run in bash):
- `test-dockerfile.sh` — checksums, no curl|bash, non-root user
- `test-entrypoint.sh` — git config, clone validation, agent launch
- `test-vm-setup.sh` — mount blocking, userns-remap, fstab
- `test-live-integration.sh` — real VM/Docker smoke tests (optional)

## Conventions

- Go CLI uses cobra for commands, `pkg/orb.Executor` interface for all VM/Docker interaction.
- `DockerRunCmd` builder replaces bash `docker_cmd=()` arrays with type-safe methods.
- All container/network names validated via `pkg/validate` before use.
- Repo clone paths validated by `pkg/repourl.ClonePath()`: rejects traversal, dot-prefixed names, special characters.
- Tests use `orb.FakeExecutor` — no real OrbStack/Docker needed.
- Container-side scripts (entrypoint, agent-session) remain bash — they run inside Docker where Go isn't installed.
- Dockerfile uses `SHELL ["/bin/bash", "-o", "pipefail", "-c"]` and verifies all downloads (SHA256 or GPG).
- Build context uses `git ls-files -c` filtered by `test -e` — only tracked files that exist on disk.

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
- `agent-orchestrate` — supervise multi-agent safe-ag workflows
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

**Version injection:** `cmd/safe-ag/main.go` has `var Version = "dev"`. The release workflow injects the real version via `-ldflags "-X main.Version=X.Y.Z"`. `safe-ag --version` prints `safe-agentic vX.Y.Z`.

**No release?** If all commits since last tag are `docs:`, `chore:`, `ci:`, or `test:`, no release is created.

## Known Limitations

- OrbStack VM hardening is best-effort — no per-VM file sharing disable yet ([#169](https://github.com/orbstack/orbstack/issues/169)). Re-harden on VM restart with `safe-ag vm start`.
- `--dangerously-skip-permissions` lets Claude execute anything inside the container. With `--ssh`, this includes pushing to other repos.
- Codex runs in yolo mode (`--yolo`) for the same reason: the container is the sandbox.
- Build trusts upstream signing roots (apt GPG keys, npm registry). Direct-download binaries are pinned and checksum-verified.
