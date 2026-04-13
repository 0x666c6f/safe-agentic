# Spawning Agents

Use `safe-ag spawn` when you want explicit control over the agent session.

## Base form

```bash
safe-ag spawn <claude|codex|shell> [flags]
```

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
| `--reuse-auth` | persist Claude/Codex auth |
| `--reuse-gh-auth` | persist GitHub CLI auth |

## Prompt, instructions, template

Three ways to shape the agent session:

```bash
safe-ag spawn claude --repo ... --prompt "Fix the flaky tests"
safe-ag spawn claude --repo ... --instructions "Only touch docs and tests"
safe-ag spawn claude --repo ... --instructions-file ./role.md
safe-ag spawn claude --repo ... --template security-audit
```

Use:
- `--prompt` for the task
- `--instructions` for role/constraints
- `--template` for a reusable built-in prompt

## Runtime and network options

Docker:

```bash
safe-ag spawn claude --docker --repo ...
safe-ag spawn claude --docker-socket --repo ...
```

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
- `--dry-run`: print the resolved launch command only

## What happens after spawn

1. safe-agentic resolves defaults and validates flags
2. it creates or joins the Docker network
3. it starts the container with the hardened runtime flags
4. it clones repos into `/workspace`
5. it runs `safe-agentic.json` setup hooks if present
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
