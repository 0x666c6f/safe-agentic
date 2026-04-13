# Quickstart

This gets you from zero to a working agent session with the fewest moving parts.

Canonical CLI: `safe-ag`.
Shortcuts also ship in `bin/`: `safe-ag-claude`, `safe-ag-codex`.

## 1. Install prerequisites

```bash
brew install orbstack
brew tap 0x666c6f/tap
brew install safe-agentic
```

If you are working from source:

```bash
git clone git@github.com:0x666c6f/safe-agentic.git
cd safe-agentic
make build-all
export PATH="$PWD/bin:$PATH"
```

## 2. Build the VM and image

```bash
safe-ag setup
safe-ag diagnose
```

`safe-ag setup`:
- creates the OrbStack VM if needed
- reapplies VM hardening
- builds the local Docker image

## 3. Spawn your first agent

Public repo:

```bash
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git
```

Private repo:

```bash
safe-ag spawn claude --ssh --repo git@github.com:myorg/myrepo.git
```

With an immediate task:

```bash
safe-ag spawn claude \
  --ssh \
  --repo git@github.com:myorg/myrepo.git \
  --prompt "Fix the failing CI tests"
```

## 4. Check what happened

```bash
safe-ag list
safe-ag peek --latest
safe-ag attach --latest
```

Notes:
- the first auth flow may open or print a device-code login
- agents run in tmux inside the container
- detach with `Ctrl-b d`
- stopped containers can be reattached later

## 5. Review the work

```bash
safe-ag diff --latest
safe-ag output --latest
safe-ag review --latest
```

## 6. Clean up

```bash
safe-ag stop --latest
safe-ag cleanup
safe-ag cleanup --auth
```

Use `--auth` only when you want to remove shared auth volumes too.

## Good next steps

- [Spawning Agents](guide/spawning.md)
- [Managing Agents](guide/managing.md)
- [Workflow](guide/workflow.md)
- [Security](security.md)
