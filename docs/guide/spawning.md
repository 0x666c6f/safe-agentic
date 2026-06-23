# Spawning Agents

Use `safe-ag spawn` when you want explicit control over the agent session.

## Base form

```bash
safe-ag spawn <claude|codex|shell> [flags]
```

Agent-first shortcuts:

```bash
safe-ag-claude <repo-url> [repo-url...]
safe-ag-codex <repo-url> [repo-url...]
```

`safe-ag-claude` and `safe-ag-codex` expand to `spawn ... --repo ...` and auto-enable `--ssh` for `git@` and `ssh://` remotes.

## Most common cases

Public repo:

```bash
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git
```

Private repo:

```bash
safe-ag spawn claude --ssh --repo git@github.com:myorg/myrepo.git
```

Immediate task:

```bash
safe-ag spawn codex \
  --ssh \
  --repo git@github.com:org/repo.git \
  --prompt "Fix the failing tests"
```

Background run:

```bash
safe-ag spawn claude \
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
safe-ag spawn claude \
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
| `--allow-setup-scripts` | allow repo-provided `safe-agentic.json` setup hooks to run |

By default, Claude/Codex auth starts empty in a per-container mount. Use `--seed-auth` only when you intentionally want to copy host agent auth into that session. Use `--reuse-auth` only when you intentionally want shared state across sessions.

## Prompt, instructions, template

Three ways to shape the agent session:

```bash
safe-ag spawn claude --repo ... --prompt "Fix the flaky tests"
safe-ag spawn claude --repo ... --instructions "Only touch docs and tests"
safe-ag spawn claude --repo ... --instructions-file ./role.md
safe-ag spawn claude --repo ... --template security-audit
safe-ag spawn claude --repo ... --template security-audit --var area=payments
```

Use:
- `--prompt` for the task
- `--instructions` for role/constraints
- `--template` for a reusable built-in prompt
- `--var key=value` for template placeholders

Templates can reference `${repo}`. If `--repo` is omitted, `safe-ag` tries to infer it from the current checkout's `origin` remote.

## Managed worktrees

Use `--worktree` when you want a Codex-app-style isolated checkout for the task while keeping the container sandbox.

```bash
safe-ag spawn claude --worktree --name auth-fix --prompt "Fix the auth tests"
safe-ag spawn codex --worktree --worktree-branch safe-ag/api-review --background
```

Rules:
- run from inside the source git checkout
- do not pass `--repo`; the current checkout is the source
- default path is `~/.safe-ag/worktrees/<container-name>`
- default branch is `safe-ag/<container-name>`
- ignored local files listed in `.safe-aginclude` are copied into the worktree

Handoff:

```bash
safe-ag handoff auth-fix --to-worktree
safe-ag handoff auth-fix --to-local ./workspace-copy
```

Snapshots and cleanup:

```bash
safe-ag worktree snapshot auth-fix "before review fixes"
safe-ag worktree restore auth-fix stash@{0}
safe-ag worktree list
safe-ag worktree cleanup --dry-run
```

## Runtime and network options

Docker:

```bash
safe-ag spawn claude --docker --repo ...
safe-ag spawn claude --docker-socket --repo ...
safe-ag spawn claude --no-docker --repo ...
```

Use `--no-docker` or `--no-docker-socket` to override Docker defaults from `~/.safe-ag/config.toml`.

AWS:

```bash
safe-ag spawn claude --aws my-profile --repo ...
```

Custom network:

```bash
safe-ag spawn claude --network my-net --repo ...
```

Resource tuning:

```bash
safe-ag spawn claude --memory 12g --cpus 6 --pids-limit 1024 --repo ...
```

## Naming and behavior flags

```bash
safe-ag spawn claude --name api-fix --repo ...
safe-ag spawn claude --background --repo ...
safe-ag spawn claude --auto-trust --repo ...
safe-ag spawn claude --dry-run --repo ...
```

Meaning:
- `--name`: stable human-readable suffix
- `--background`: do not attach immediately
- `--auto-trust`: skip the first trust prompt for the agent CLI
- `--dry-run`: print the resolved launch command only; sensitive env and labels are redacted

## What happens after spawn

1. safe-agentic resolves defaults and validates flags
2. it creates or joins the Docker network
3. it starts the container with the hardened runtime flags
4. it clones repos into `/workspace`
5. if `--allow-setup-scripts` is set, it runs `safe-agentic.json` setup hooks
6. it launches the agent inside tmux
7. it attaches unless you used `--background`

## Related commands

```bash
safe-ag list
safe-ag attach --latest
safe-ag peek --latest
safe-ag diff --latest
safe-ag output --latest
```
