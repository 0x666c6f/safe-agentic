# AGENTS.md

This file provides guidance to coding agents working in this repository.

## What This Is

An isolated environment for running AI coding agents (Claude Code, Codex) inside an OrbStack VM with per-agent Docker containers. Default stance: safe by default. Dangerous capabilities like SSH forwarding or auth reuse stay opt-in.

## Architecture

```text
macOS Host (safe-ag CLI)
  -> OrbStack VM "safe-agentic" (Ubuntu 24.04, hardened)
    -> Docker containers (ephemeral, per-agent)
       - read-only rootfs + tmpfs scratch
       - cap-drop ALL + no-new-privileges
       - per-session OAuth + ephemeral cache volumes
       - dedicated bridge network per container
       - SSH agent off unless --ssh
```

Isolation boundaries:

1. macOS host <-> OrbStack VM
2. VM <-> container
3. container <-> container

See `docs/architecture.md`.

## Key Files

- `bin/safe-ag`: compiled Go CLI.
- `bin/safe-ag-tui`: compiled Go TUI.
- `bin/agent-session.sh`: tmux session wrapper inside the container.
- `bin/repo-url.sh`: repo URL parsing and clone-path validation.
- `vm/setup.sh`: idempotent VM bootstrap + hardening; re-run on `safe-ag vm start`.
- `Dockerfile`: pinned downloads, checksum verification, non-root `agent` user, no sudo.
- `entrypoint.sh`: container init, git config injection, optional auth/config seeding, repo clone validation, agent launch.
- `config/bashrc`: shell defaults inside containers.
- `package.json`: Codex CLI pin for reproducible `npm ci`.
- `package-lock.json`: lockfile for Codex CLI install.
- `op-env.sh`: optional 1Password secret injection template.

## Project Structure

- `bin/`: built binaries plus retained runtime shell helpers
- `config/`: shell and prompt config copied into the container
- `docs/`: architecture, quickstart, usage, security docs
- `examples/`, `templates/`, `tui/`: example assets, templates, terminal UI
- `vm/`: OrbStack VM bootstrap and hardening
- `.claude/skills/`, `.codex/skills/`: repo-local skill bundles; keep pairs aligned

## Common Commands

```bash
# First-time setup
safe-ag setup

# Rebuild image
safe-ag update

# Spawn agents
safe-ag spawn claude --ssh --repo git@github.com:org/repo.git
safe-ag spawn codex --ssh --reuse-auth --repo git@github.com:org/repo.git --name my-task
safe-ag spawn codex --ssh --prompt 'Fix the CI tests' --repo git@github.com:org/repo.git

# Management
safe-ag list
safe-ag attach <name>
safe-ag stop <name|--all>
safe-ag cleanup
safe-ag tui

# VM
safe-ag vm start
safe-ag vm stop
safe-ag vm ssh
```

```bash
# Sessions, output, workflows
safe-ag peek <container>
safe-ag output <name>|--latest
safe-ag summary <name>|--latest
safe-ag diff <name>|--latest [--stat]
safe-ag checkpoint create <name> [label]
safe-ag checkpoint list <name>
safe-ag checkpoint revert <name> <ref>
safe-ag todo add <name> "text"
safe-ag todo list <name>
safe-ag retry <name>|--latest [--feedback "text"]
safe-ag pr <name> [--title T --base B]
safe-ag review <name> [--base B]
```

## Build And Verify

Minimum bar for changes:

- `bash -n bin/agent-session.sh bin/repo-url.sh entrypoint.sh vm/setup.sh`
- `make build-all`
- `go test ./...`
- if touching live integration harness, prefer:
  - `SAFE_AGENTIC_INTEGRATION=1 go test -tags integration ./cmd/safe-ag`
  - `SAFE_AGENTIC_DEEP_INTEGRATION=1 SAFE_AGENTIC_INTEGRATION=1 go test -tags integration ./cmd/safe-ag -run <focused-case>`
- prefer smoke tests for touched flows: `safe-ag setup`, `safe-ag spawn shell`, cleanup path
- validate touched repo-local skills
- update docs when behavior/flags/security posture changes

Useful commands:

```bash
# Build + test
make build-all
go test ./...

# Live integration
SAFE_AGENTIC_INTEGRATION=1 go test -tags integration ./cmd/safe-ag

# Heavier live cases
SAFE_AGENTIC_DEEP_INTEGRATION=1 SAFE_AGENTIC_INTEGRATION=1 go test -tags integration ./cmd/safe-ag -run <focused-case>

# Skill validation
codex skills validate .codex/skills/<skill-name>
```

If fixing a bug, add the smallest regression check that fits.

## Coding Conventions

- Go-first repo; retained shell runtime scripts use `set -euo pipefail`
- 2-space indentation
- quote expansions unless word splitting required
- prefer small helpers over long inline blocks
- use kebab-case filenames
- CLI subcommands live under `cmd/safe-ag`
- comment non-obvious trust boundaries, mounts, auth, isolation details

Implementation patterns:

- use `orb run -m "$VM_NAME"` for VM operations
- keep read-only rootfs pattern intact: baked configs copied into tmpfs at runtime
- validate repo clone paths via `repo_clone_path()`
- build context from tracked files only
- `SAFE_AGENTIC_VM_NAME` overrides the target VM; use it for isolated test VMs

## Security Model

| Default | Override |
| --- | --- |
| SSH agent off | `--ssh` |
| per-session auth | `--reuse-auth` |
| AWS credentials off | `--aws <profile>` |
| read-only rootfs | none |
| `cap-drop ALL` + `no-new-privileges` | none |
| dedicated bridge network | `--network <name>` |
| memory 8g / CPU 4 / PIDs 512 | `--memory`, `--cpus`, `--pids-limit` |
| baked GitHub SSH host keys + strict host checking | none |
| Docker `userns-remap` in VM | none |
| no sudo, no supplemental groups | none |

Unsafe Docker modes like `--privileged`, host networking, or raw passthrough remain blocked. Safer defaults beat documentation-only warnings.

When changing auth, network, mounts, isolation, or host-sharing behavior:

- update `README.md`
- call out security impact in commit/PR notes
- do not broaden SSH forwarding, auth volume sharing, or Docker privileges casually

## Documentation

- `README.md`: operational overview
- `docs/architecture.md`: diagrams, isolation boundaries, flow sequences
- `docs/quickstart.md`: getting started
- `docs/usage.md`: command reference
- `docs/security.md`: threat model, supply chain, filesystem layout

## Skills

Keep matching skill pairs in sync:

- `.claude/skills/*`
- `.codex/skills/*`

Repo-local skills currently cover:

- `agent-spawn`
- `agent-manage`
- `agent-setup`

## Known Limitations

- OrbStack VM hardening remains best-effort; per-VM file sharing disable still missing. Re-harden on VM restart with `safe-ag vm start`.
- Claude `--dangerously-skip-permissions` and Codex `--yolo` are acceptable here because the container is the sandbox; with `--ssh`, pushes stay possible.
- Build still trusts upstream signing roots for package ecosystems; direct downloads are pinned and checksum-verified.

## Commit Style & Releases

Use [Conventional Commits](https://www.conventionalcommits.org/). Commit prefixes drive automated semver releases:

| Prefix | Effect |
|--------|--------|
| `feat:` | minor bump |
| `fix:` | patch bump |
| `feat!:` or `BREAKING CHANGE` in body | major bump |
| `docs:`, `chore:`, `ci:`, `test:` | no release |

Scopes optional: `feat(tui):`, `fix(ci):`, etc.

Every push to `main` triggers `.github/workflows/release.yml` which runs CI, computes version, builds universal macOS `safe-ag` and `safe-ag-tui` binaries, creates a GitHub Release with changelog, and updates the Homebrew tap (`0x666c6f/homebrew-tap`).

`safe-ag --version` prints `safe-agentic vX.Y.Z`. The release workflow injects the version with Go ldflags.

Keep commits focused. Before handoff, prefer full gate over partial checks.
