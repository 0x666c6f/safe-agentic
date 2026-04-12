# Quickstart

Get a sandboxed Claude Code or Codex session running in a few minutes.

## 1. Install

```bash
brew install orbstack
brew tap 0x666c6f/tap && brew install safe-agentic
safe-ag --version
```

From source:

```bash
brew install orbstack
git clone git@github.com:0x666c6f/safe-agentic.git
cd safe-agentic
make build-all
export PATH="$PWD/bin:$PATH"
```

## 2. Build VM + image

```bash
safe-ag setup
```

This creates the OrbStack VM, reapplies hardening, and builds `safe-agentic:latest`.

## 3. Spawn an agent

```bash
# Public repo
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git

# Private repo
safe-ag spawn claude --ssh --repo git@github.com:myorg/myrepo.git

# Codex
safe-ag spawn codex --ssh --reuse-auth --repo git@github.com:myorg/myrepo.git
```

First run usually opens an OAuth flow. Containers persist after exit; reattach later with `safe-ag attach --latest`.

## 4. Inspect + clean up

```bash
safe-ag list
safe-ag peek --latest
safe-ag diff --latest
safe-ag stop --all
safe-ag cleanup --auth
```

## Notes

- `safe-ag tui` launches the terminal dashboard.
- `safe-ag diagnose` checks OrbStack, VM, Docker, image, auth, and defaults.
- `safe-ag spawn ... --repo ... --repo ...` clones multiple repos into one container.
- Defaults live in `~/.config/safe-agentic/defaults.sh`.
