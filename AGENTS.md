# AGENTS.md

This file provides guidance to coding agents working in this repository.

## What This Is

An isolated environment for running AI coding agents (Claude Code, Codex) inside an OrbStack VM with per-agent Docker containers. Default stance: safe by default. Dangerous capabilities like SSH forwarding or auth reuse stay opt-in.

## Architecture

```text
macOS Host (bin/agent CLI)
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

See [docs/architecture.md](/Users/florian/perso/safe-agentic/docs/architecture.md).

## Key Files

- [bin/agent](/Users/florian/perso/safe-agentic/bin/agent): host-side CLI dispatcher; `cmd_*` functions; calls into VM via `orb run -m safe-agentic`.
- [bin/agent-lib.sh](/Users/florian/perso/safe-agentic/bin/agent-lib.sh): validation, Docker runtime construction, network lifecycle, volume helpers; Docker commands built as bash arrays.
- [bin/agent-claude](/Users/florian/perso/safe-agentic/bin/agent-claude): quick Claude alias; auto-detects SSH URLs.
- [bin/agent-codex](/Users/florian/perso/safe-agentic/bin/agent-codex): quick Codex alias; auto-detects SSH URLs.
- [bin/agent-session.sh](/Users/florian/perso/safe-agentic/bin/agent-session.sh): tmux session wrapper; fresh vs resume handling.
- [vm/setup.sh](/Users/florian/perso/safe-agentic/vm/setup.sh): idempotent VM bootstrap + hardening; re-run on `agent vm start`.
- [Dockerfile](/Users/florian/perso/safe-agentic/Dockerfile): pinned downloads, checksum verification, non-root `agent` user, no sudo.
- [entrypoint.sh](/Users/florian/perso/safe-agentic/entrypoint.sh): container init, git config injection, optional auth/config seeding, repo clone validation, agent launch.
- [config/bashrc](/Users/florian/perso/safe-agentic/config/bashrc): shell defaults inside containers.
- [package.json](/Users/florian/perso/safe-agentic/package.json): Codex CLI pin for reproducible `npm ci`.
- [package-lock.json](/Users/florian/perso/safe-agentic/package-lock.json): lockfile for Codex CLI install.
- [op-env.sh](/Users/florian/perso/safe-agentic/op-env.sh): optional 1Password secret injection template.

## Project Structure

- `bin/`: host-side CLI entrypoints and shared shell helpers
- `config/`: shell and prompt config copied into the container
- `docs/`: architecture, quickstart, usage, security docs
- `examples/`, `templates/`, `tui/`: example assets, templates, terminal UI
- `tests/`: shell test suites and smoke checks
- `vm/`: OrbStack VM bootstrap and hardening
- `.claude/skills/`, `.codex/skills/`: repo-local skill bundles; keep pairs aligned

## Common Commands

```bash
# First-time setup
agent setup

# Rebuild image
agent update
agent update --quick
agent update --full

# Spawn agents
agent spawn claude --ssh --repo git@github.com:org/repo.git
agent spawn codex --ssh --reuse-auth --repo git@github.com:org/repo.git --name my-task
agent spawn codex --ssh --prompt 'Fix the CI tests' --repo git@github.com:org/repo.git

# Quick aliases
agent-claude git@github.com:org/repo.git
agent-codex https://github.com/org/repo.git

# Management
agent list
agent attach <name>
agent stop <name|--all>
agent cleanup
agent tui

# VM
agent vm start
agent vm stop
agent vm ssh
```

```bash
# Sessions, output, workflows
agent peek <container>
agent output <name>|--latest
agent summary <name>|--latest
agent diff <name>|--latest [--stat]
agent checkpoint create <name> [label]
agent checkpoint list <name>
agent checkpoint revert <name> <ref>
agent todo add <name> "text"
agent todo list <name>
agent retry <name>|--latest [--feedback "text"]
agent pr <name> [--title T --base B]
agent review <name> [--base B]
```

## Build And Verify

Minimum bar for changes:

- `bash -n bin/agent entrypoint.sh vm/setup.sh`
- run affected test suites in `tests/`
- prefer smoke tests for touched flows: `agent setup`, `agent spawn`, `agent shell`, cleanup path
- validate touched repo-local skills
- update docs when behavior/flags/security posture changes

Useful commands:

```bash
# Full test run
bash tests/run-all.sh

# Single suite
bash tests/test-docker-cmd.sh

# Syntax-only
bash tests/test-syntax.sh

# Skill validation
codex skills validate .codex/skills/<skill-name>
```

If fixing a bug, add the smallest regression check that fits.

## Testing Inventory

- `tests/test-syntax.sh`: `bash -n` on scripts
- `tests/test-validation.sh`: name + network validation
- `tests/test-repo-clone-path.sh`: URL parsing, traversal, injection
- `tests/test-docker-cmd.sh`: security flags, volumes, SSH, git identity
- `tests/test-dockerfile.sh`: checksums, non-root user, no `curl | bash`
- `tests/test-entrypoint.sh`: git config, clone validation, agent launch
- `tests/test-entrypoint-runtime.sh`: runtime entrypoint behavior
- `tests/test-vm-setup.sh`: mount blocking, `userns-remap`, `fstab`
- `tests/test-cli-dispatch.sh`: help, errors, aliases, multi-repo
- `tests/test-agent-lifecycle.sh`: attach, stop, cleanup, custom networks
- `tests/test-management-commands.sh`: management command coverage
- `tests/test-update.sh`: tracked-only build context, quick/full rebuild
- `tests/test-audit.sh`: audit logging
- `tests/test-checkpoint.sh`: checkpoint create/list/revert
- `tests/test-copy-command.sh`: file copy commands
- `tests/test-cost.sh`: cost estimation
- `tests/test-diagnose.sh`: diagnostic commands
- `tests/test-diff.sh`: diff display
- `tests/test-fleet.sh`: fleet manifest parsing and spawning
- `tests/test-lifecycle-scripts.sh`: `safe-agentic.json` lifecycle scripts
- `tests/test-pipeline.sh`: pipeline YAML parsing and execution
- `tests/test-pr.sh`: PR creation
- `tests/test-review.sh`: review command
- `tests/test-todo.sh`: todo management
- `tests/test-live-integration.sh`: real VM/Docker smoke tests; optional
- `tests/test-live-agent-clis.sh`: live agent CLI tests; optional
- `tests/agent-cli-security.sh`: end-to-end security regressions

Tests use a fake `orb` binary to capture Docker commands without requiring a real VM.

## Coding Conventions

- Bash-first repo; scripts use `set -euo pipefail`
- 2-space indentation
- quote expansions unless word splitting required
- prefer small helpers over long inline blocks
- use kebab-case filenames
- CLI subcommands live in `cmd_*` functions
- comment non-obvious trust boundaries, mounts, auth, isolation details

Implementation patterns:

- build Docker commands as bash arrays
- use `vm_exec()` / `orb run -m "$VM_NAME"` for VM operations
- keep read-only rootfs pattern intact: baked configs copied into tmpfs at runtime
- validate repo clone paths via `repo_clone_path()`
- build context from tracked files only

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

- update [README.md](/Users/florian/perso/safe-agentic/README.md)
- call out security impact in commit/PR notes
- do not broaden SSH forwarding, auth volume sharing, or Docker privileges casually

## Documentation

- [README.md](/Users/florian/perso/safe-agentic/README.md): operational overview
- [docs/architecture.md](/Users/florian/perso/safe-agentic/docs/architecture.md): diagrams, isolation boundaries, flow sequences
- [docs/quickstart.md](/Users/florian/perso/safe-agentic/docs/quickstart.md): getting started
- [docs/usage.md](/Users/florian/perso/safe-agentic/docs/usage.md): command reference
- [docs/security.md](/Users/florian/perso/safe-agentic/docs/security.md): threat model, supply chain, filesystem layout

## Skills

Keep matching skill pairs in sync:

- `.claude/skills/*`
- `.codex/skills/*`

Repo-local skills currently cover:

- `agent-spawn`
- `agent-manage`
- `agent-setup`

## Known Limitations

- OrbStack VM hardening remains best-effort; per-VM file sharing disable still missing. Re-harden on VM restart with `agent vm start`.
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

Every push to `main` triggers `.github/workflows/release.yml` which runs CI, computes version, builds a universal macOS TUI binary, creates a GitHub Release with changelog, and updates the Homebrew tap (`0x666c6f/homebrew-tap`).

`bin/agent` has `VERSION="dev"` at line 5. The release workflow injects the real version. `agent --version` prints `safe-agentic vX.Y.Z`.

Keep commits focused. Before handoff, prefer full gate over partial checks.
