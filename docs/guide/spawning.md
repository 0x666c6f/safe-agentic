# Spawning Agents

## Base command

```bash
safe-ag spawn <claude|codex|shell> [flags]
```

## Common examples

```bash
# Public repo
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git

# Private repo
safe-ag spawn claude --ssh --repo git@github.com:myorg/myrepo.git

# Codex + shared auth
safe-ag spawn codex --ssh --reuse-auth --repo git@github.com:myorg/myrepo.git

# Multiple repos
safe-ag spawn claude \
  --repo git@github.com:myorg/frontend.git \
  --repo git@github.com:myorg/backend.git

# Background
safe-ag spawn claude --background --ssh --repo git@github.com:org/api.git \
  --prompt "Fix the failing tests"
```

## Common flags

| Flag | Meaning |
|---|---|
| `--repo URL` | Repo to clone; repeatable |
| `--ssh` | Forward SSH agent |
| `--reuse-auth` | Persist Claude/Codex auth |
| `--reuse-gh-auth` | Persist `gh` auth |
| `--aws PROFILE` | Inject AWS credentials |
| `--docker` | Start DinD sidecar |
| `--docker-socket` | Mount VM Docker socket |
| `--background` | Detach immediately |
| `--prompt TEXT` | Initial task |
| `--instructions TEXT` | Prepended context |
| `--template NAME` | Built-in template |
| `--name NAME` | Explicit container name |
| `--network NAME` | Custom Docker network |
| `--auto-trust` | Skip first-run trust prompt |
| `--dry-run` | Print resolved plan only |

## Behavior

1. Container starts with read-only rootfs, dropped caps, filtered network.
2. Repos clone into `/workspace`.
3. `safe-agentic.json` `setup` hook runs if present.
4. Claude or Codex starts inside tmux.
5. Container persists after exit until `safe-ag stop` / `safe-ag cleanup`.

## Related commands

```bash
safe-ag attach <name>
safe-ag list
safe-ag peek --latest
safe-ag summary --latest
safe-ag diff --latest
safe-ag tui
```
