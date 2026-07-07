# Spawning Agents

Use `berth spawn` when you want explicit control over the agent session.

## Base form

```bash
berth spawn <claude|codex|shell> [flags]
```

Agent-first shortcuts:

```bash
berth-claude <repo-url> [repo-url...]
berth-codex <repo-url> [repo-url...]
```

`berth-claude` and `berth-codex` expand to `spawn ... --repo ...` and auto-enable `--ssh` for `git@` and `ssh://` remotes.

## Most common cases

Public repo:

```bash
berth spawn claude --repo https://github.com/myorg/myrepo.git
```

Private repo:

```bash
berth spawn claude --ssh --repo git@github.com:myorg/myrepo.git
```

Immediate task:

```bash
berth spawn codex \
  --ssh \
  --repo git@github.com:org/repo.git \
  --prompt "Fix the failing tests"
```

Background run:

```bash
berth spawn claude \
  --background \
  --ssh \
  --repo git@github.com:org/repo.git \
  --prompt "Review the latest changes"
```

## Choose the agent

| Type | Use it for |
|---|---|
| `claude` | general coding, review, analysis |
| `codex` | coding sessions where you want the Codex CLI |
| `shell` | an interactive shell in the same sandbox model |

## Repo and auth choices

`--repo` is repeatable.

```bash
berth spawn claude \
  --repo git@github.com:org/frontend.git \
  --repo git@github.com:org/backend.git
```

Use:
- `https://...` when you do not need SSH
- `git@...` with `--ssh` for private repos or pushes

Auth-related flags:

| Flag | Effect |
|---|---|
| `--ssh` | forward your SSH agent into the container |
| `--no-ssh` | override `ssh = true` from config for one session |
| `--reuse-auth` | opt into shared Claude/Codex auth |
| `--no-reuse-auth` / `--ephemeral-auth` | override `reuse_auth = true` from config for one session |
| `--reuse-gh-auth` | persist GitHub CLI auth |
| `--no-reuse-gh-auth` | override `reuse_gh_auth = true` from config for one session |
| `--seed-auth` | copy host Claude/Codex auth into this session's auth mount |
| `--no-seed-auth` | override `seed_auth = true` from config for one session |
| `--allow-setup-scripts` | allow repo-provided `berth.json` setup hooks to run |

By default, Claude/Codex auth starts empty in a per-container mount. Use `--seed-auth` only when you intentionally want to copy host agent auth into that session. Use `--reuse-auth` only when you intentionally want shared state across sessions.

## Prompt, instructions, template

Three ways to shape the agent session:

```bash
berth spawn claude --repo ... --prompt "Fix the flaky tests"
berth spawn claude --repo ... --instructions "Only touch docs and tests"
berth spawn claude --repo ... --instructions-file ./role.md
berth spawn claude --repo ... --template security-audit
berth spawn claude --repo ... --template security-audit --var area=payments
```

Use:
- `--prompt` for the task
- `--instructions` for role/constraints
- `--template` for a reusable built-in prompt
- `--var key=value` for template placeholders

Templates can reference `${repo}`. If `--repo` is omitted, `berth` tries to infer it from the current checkout's `origin` remote.

## Managed worktrees

Use `--worktree` when you want a Codex-app-style isolated checkout for the task while keeping the container sandbox.

```bash
berth spawn claude --worktree --name auth-fix --prompt "Fix the auth tests"
berth spawn codex --worktree --worktree-branch berth/api-review --background
```

Rules:
- run from inside the source git checkout
- do not pass `--repo`; the current checkout is the source
- default path is `~/.berth/worktrees/<container-name>`
- default branch is `berth/<container-name>`
- ignored local files listed in `.berthinclude` are copied into the worktree

`--worktree` is opt-in (off by default):
- enable it once with `berth setup --enable-worktrees` (or `berth config set defaults.worktrees_mount true` then `berth setup`); disable with `berth setup --disable-worktrees`
- when enabled, the machine runs `home-mount=rw` and `vm/setup.sh` binds *only* the worktrees root (`~/.berth/worktrees`, or `defaults.worktrees_dir`) to a stable `/worktrees`, detaches the rest of the home share, then masks `/Users`, `/Volumes`, `/private`, and `/mnt/mac`
- on spawn, the host worktree path is translated to its in-VM `/worktrees/...` path for the container bind
- a `--worktree-path` outside the worktrees root is rejected before launch; the root must live under your home directory
- `berth setup`/`berth vm start` reconcile the machine to match the config; `berth diagnose` reports the posture

Security trade-off:
- the default (`home-mount=none`) shares no host data with the VM; enabling the worktree mount switches to `home-mount=rw`, which shares your whole home at the virtiofs level
- berth detaches and masks everything except the worktrees root, but this is a **weaker boundary** than the default — a VM-root compromise or Docker escape could re-reach host home
- keep secrets and unrelated projects out of the worktrees root; see the [threat model](../security/threat-model.md)

Handoff:

```bash
berth handoff auth-fix --to-worktree
berth handoff auth-fix --to-local ./workspace-copy
```

Snapshots and cleanup:

```bash
berth worktree snapshot auth-fix "before review fixes"
berth worktree restore auth-fix stash@{0}
berth worktree list
berth worktree cleanup --dry-run
```

## Runtime and network options

Docker:

```bash
berth spawn claude --docker --repo ...
berth spawn claude --docker-socket --repo ...
berth spawn claude --no-docker --repo ...
```

Use `--no-docker` or `--no-docker-socket` to override Docker defaults from `~/.berth/config.toml`.

AWS:

```bash
berth spawn claude --aws my-profile --repo ...
```

Custom network:

```bash
berth spawn claude --network my-net --repo ...
```

Resource tuning:

```bash
berth spawn claude --memory 12g --cpus 6 --pids-limit 1024 --repo ...
```

## Naming and behavior flags

```bash
berth spawn claude --name api-fix --repo ...
berth spawn claude --background --repo ...
berth spawn claude --auto-trust --repo ...
berth spawn claude --dry-run --repo ...
```

Meaning:
- `--name`: stable human-readable suffix
- `--background`: do not attach immediately
- `--auto-trust`: skip the first trust prompt for the agent CLI
- `--dry-run`: print the resolved launch command only; sensitive env and labels are redacted

## What happens after spawn

1. berth resolves defaults and validates flags
2. it creates or joins the Docker network
3. it starts the container with the hardened runtime flags
4. it clones repos into `/workspace`
5. if `--allow-setup-scripts` is set, it runs `berth.json` setup hooks
6. it launches the agent inside tmux
7. it attaches unless you used `--background`

## Related commands

```bash
berth list
berth attach --latest
berth peek --latest
berth diff --latest
berth output --latest
```
