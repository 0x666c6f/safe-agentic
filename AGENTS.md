# Repository Guidelines

## Project Structure & Module Organization

`bin/` holds the host-side CLI entrypoints: `agent`, `agent-claude`, `agent-codex`.
`vm/setup.sh` bootstraps and hardens the OrbStack VM.
`entrypoint.sh` is the container entrypoint: git setup, repo cloning, agent launch.
`Dockerfile` defines the agent image and bundled tools.
`config/` contains shell and prompt config copied into the container.
`.claude/skills/` and `.codex/skills/` contain repo-local Claude/Codex skill bundles; keep equivalent skills aligned when adding or changing them.
Root docs live in `README.md`; keep operational/security behavior documented there when changing isolation or auth flows.

## Build, Test, and Development Commands

Run from repo root:

```bash
agent setup              # create VM, apply hardening, build image
agent update             # rebuild image inside VM
agent update --quick     # rebuild AI CLI layer only
agent vm start           # start VM and re-apply hardening
agent spawn codex --repo git@github.com:org/repo.git
```

Verification commands:

```bash
bash -n bin/agent entrypoint.sh vm/setup.sh   # shell syntax check
agent shell --repo git@github.com:org/repo.git
agent cleanup
codex skills validate .codex/skills/<skill-name>
```

## Coding Style & Naming Conventions

Bash-first repo. Keep scripts POSIX-leaning Bash with `set -euo pipefail`.
Use 2-space indentation. Quote expansions unless word splitting is required.
Prefer small helpers (`cmd_setup`, `cmd_spawn`) over long inline blocks.
Use kebab-case for filenames and `cmd_*` for CLI subcommand functions.
Comment trust boundaries, mounts, auth, and other non-obvious security behavior.

## Testing Guidelines

No formal test framework yet. Minimum bar:

- run `bash -n` on changed scripts
- smoke-test affected flows (`agent setup`, `agent spawn`, `agent shell`, cleanup path)
- validate touched `.codex/skills/*` with `quick_validate.py`; keep matching `.claude/skills/*/SKILL.md` and `.codex/skills/*/SKILL.md` in sync
- verify README/usage text when CLI flags or behavior change

If fixing a bug, add the smallest regression check that fits; for shell changes, that can be a reproducible command sequence documented in the PR.

## Commit & Pull Request Guidelines

Use Conventional Commits, matching current history: `feat:`, `fix:`, `docs:`, `chore:`.
Keep commits focused; one behavior change per commit when practical.
PRs should include:

- short problem/solution summary
- exact verification commands run
- security impact note for auth, network, mount, or isolation changes
- linked issue if one exists

Screenshots usually not needed; use terminal output snippets instead.

## Security & Configuration Tips

Default posture matters here. Prefer safer defaults over documentation-only warnings.
Do not broaden SSH agent forwarding, auth volume sharing, or Docker privileges without updating `README.md` and explaining the tradeoff in the PR.
