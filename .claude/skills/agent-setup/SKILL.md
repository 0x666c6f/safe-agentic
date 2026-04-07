---
name: agent-setup
description: Set up or update the safe-agentic environment — VM creation, hardening, and image building. Use when the user asks to set up, install, update, rebuild, or troubleshoot safe-agentic.
---

# Setup & Maintain Safe-Agentic

First-time setup, image rebuilds, and VM management.

## First-time setup

```bash
agent setup
```

This is idempotent and does everything:
1. Creates the OrbStack VM (`safe-agentic`)
2. Hardens the VM (blocks macOS filesystem, removes integration commands)
3. Installs Docker CE inside the VM
4. Builds the agent Docker image

### Prerequisites

Check before running setup:
```bash
# OrbStack must be installed
command -v orb && echo "OK" || echo "Install: brew install orbstack"
```

For SSH repos, enable 1Password SSH agent:
**1Password → Settings → Developer → "Use the SSH Agent"**

### PATH setup

The user needs `safe-agentic/bin` in their PATH:
```bash
export PATH="$PATH:$HOME/perso/safe-agentic/bin"
```

## Rebuild the image

After changes to the Dockerfile, entrypoint, or when CLI tools need updating:

```bash
# Standard rebuild (uses Docker cache)
agent update

# Fast: rebuild only the AI CLI layer (Claude Code / Codex updates)
agent update --quick

# Full: no cache, rebuild everything from scratch
agent update --full
```

### When to use each

| Scenario | Command |
|----------|---------|
| Claude Code or Codex released a new version | `agent update --quick` |
| Changed Dockerfile, entrypoint, or config | `agent update` |
| Need fresh OS packages or full clean slate | `agent update --full` |

## VM management

```bash
# Start VM and re-apply hardening (use instead of `orb start`)
agent vm start

# Stop the VM
agent vm stop

# SSH into VM for debugging
agent vm ssh
```

**Important:** Always use `agent vm start` instead of `orb start` directly — it re-applies filesystem hardening that OrbStack may reset on restart.

## Troubleshooting

### "VM not found" error
```bash
agent setup   # Creates the VM
```

### "'orb' is required but not installed"
```bash
brew install orbstack
```

### Image build fails
```bash
# SSH into VM and check Docker
agent vm ssh
docker info
docker images
exit

# Try a full rebuild
agent update --full
```

### OrbStack restored macOS mounts
```bash
agent vm start   # Re-applies hardening
```

### Need to start over
```bash
agent cleanup            # Remove all containers, auth, networks
agent update --full      # Rebuild image from scratch
```
