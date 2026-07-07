# AGENTS.md

This file provides guidance to coding agents working in this repository.

## What This Is

An isolated environment for running AI coding agents (Claude Code, Codex) inside an Apple container machine with per-agent Docker containers. Default stance: safe by default. Dangerous capabilities like SSH forwarding or auth reuse stay opt-in.

## Architecture

```text
macOS Host (berth CLI)
  -> Apple container machine "berth" (Alpine 3.22, hardened)
    -> Docker containers (ephemeral, per-agent)
       - read-only rootfs + tmpfs scratch
       - cap-drop ALL + no-new-privileges
       - per-session OAuth + ephemeral cache volumes
       - dedicated bridge network per container
       - SSH agent off unless --ssh
```

Isolation boundaries:

1. macOS host <-> Apple container machine
2. VM <-> container
3. container <-> container

See `docs/architecture.md`.

## Key Files

- `bin/berth`: compiled Go CLI.
- `bin/berth-tui`: compiled Go TUI.
- `bin/agent-session.sh`: tmux session wrapper inside the container.
- `bin/repo-url.sh`: repo URL parsing and clone-path validation.
- `vm/setup.sh`: idempotent VM bootstrap + hardening; re-run on `berth vm start`.
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
- `vm/`: Apple container machine bootstrap and hardening
- `.claude/skills/`, `.codex/skills/`: repo-local skill bundles; keep pairs aligned

## Common Commands

```bash
# First-time setup
berth setup

# Rebuild image
berth update

# Spawn agents
berth spawn claude --ssh --repo git@github.com:org/repo.git
berth spawn codex --ssh --reuse-auth --repo git@github.com:org/repo.git --name my-task
berth spawn codex --ssh --prompt 'Fix the CI tests' --repo git@github.com:org/repo.git

# Management
berth list
berth attach <name>
berth stop <name|--all>
berth cleanup
berth tui

# VM
berth vm start
berth vm stop
berth vm ssh
```

```bash
# Sessions, output, workflows
berth peek <container>
berth output <name>|--latest
berth summary <name>|--latest
berth diff <name>|--latest [--stat]
berth checkpoint create <name> [label]
berth checkpoint list <name>
berth checkpoint revert <name> <ref>
berth todo add <name> "text"
berth todo list <name>
berth retry <name>|--latest [--feedback "text"]
berth pr <name> [--title T --base B]
berth review <name> [--base B]
```

## Build And Verify

Minimum bar for changes:

- `bash -n bin/agent-session.sh bin/repo-url.sh entrypoint.sh vm/setup.sh`
- `make build-all`
- `go test ./...`
- if touching live integration harness, prefer:
  - `BERTH_INTEGRATION=1 go test -tags integration ./cmd/berth`
  - `BERTH_DEEP_INTEGRATION=1 BERTH_INTEGRATION=1 go test -tags integration ./cmd/berth -run <focused-case>`
- prefer smoke tests for touched flows: `berth setup`, `berth spawn shell`, cleanup path
- validate touched repo-local skills
- update docs when behavior/flags/security posture changes

Useful commands:

```bash
# Build + test
make build-all
go test ./...

# Live integration
BERTH_INTEGRATION=1 go test -tags integration ./cmd/berth

# Heavier live cases
BERTH_DEEP_INTEGRATION=1 BERTH_INTEGRATION=1 go test -tags integration ./cmd/berth -run <focused-case>

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
- CLI subcommands live under `cmd/berth`
- comment non-obvious trust boundaries, mounts, auth, isolation details

Implementation patterns:

- use `container machine run -n "$VM_NAME" -u root -- ...` for VM operations
- keep read-only rootfs pattern intact: baked configs copied into tmpfs at runtime
- validate repo clone paths via `repo_clone_path()`
- build context from tracked files only
- `BERTH_VM_NAME` overrides the target VM; use it for isolated test VMs
- `berth setup` configures Apple vmnet egress via host IP forwarding and PF anchor `com.apple/berth`; macOS admin approval may be required.

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
- `docs/install.md` + `docs/quickstart.md`: getting started
- `docs/guide/*.md`: task-oriented guides (spawning, managing, workflow, worktrees, fleet, automation, tui, app, configuration)
- `docs/reference/cli.md`: exhaustive command reference
- `docs/architecture.md`: isolation boundaries, networking, container internals
- `docs/security.md`: defaults, wideners, threat model, supply chain

## Skills

Keep matching skill pairs in sync:

- `.claude/skills/*`
- `.codex/skills/*`

Repo-local skills currently cover:

- `agent-spawn`
- `agent-manage`
- `agent-setup`
- `agent-orchestrate`
- `agent-manifest-author`

## Known Limitations

- Default posture is `home-mount=none` — the host home is never shared with the VM. Apple `container` cannot mount a single host directory into a machine, so `--worktree` is opt-in: `berth setup --enable-worktrees` switches the machine to `home-mount=rw`, and `vm/setup.sh` binds only the worktrees root to `/worktrees`, detaches the rest of the home share, then masks `/Users`, `/Volumes`, `/private`, `/mnt/mac`. Enabling this weakens the VM boundary (rw home share); keep secrets out of the worktrees root. `berth diagnose` reports the posture; `berth setup`/`berth vm start` reconcile in either direction. Worktree paths outside the root are rejected. See the threat model in `docs/security.md`.
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

Every push to `main` triggers `.github/workflows/release.yml` which runs CI, computes version, builds universal macOS `berth` and `berth-tui` binaries, creates a GitHub Release with changelog, and updates the Homebrew tap (`0x666c6f/homebrew-tap`).

`berth --version` prints `berth vX.Y.Z`. The release workflow injects the version with Go ldflags.

Keep commits focused. Before handoff, prefer full gate over partial checks.
