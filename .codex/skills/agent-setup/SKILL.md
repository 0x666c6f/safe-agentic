---
name: agent-setup
description: Set up or update the berth environment — VM creation, hardening, and image building. Use when the user asks to set up, install, update, rebuild, or troubleshoot berth.
---

# Setup & Maintain Berth

First-time setup, image rebuilds, and VM management.

## First-time setup

```bash
berth setup
```

This is idempotent and does everything:
1. Starts Apple container if needed
2. Creates the Apple container machine (`berth`)
3. Hardens the VM (blocks macOS filesystem mounts)
4. Configures Apple vmnet host NAT for VM and nested Docker egress
5. Installs Docker inside the VM
6. Builds the agent Docker image

### Prerequisites

Check before running setup:
```bash
# Apple container must be installed
command -v container && echo "OK" || echo "Install: https://github.com/apple/container/releases"
container system status || container system start
```

For SSH repos, enable 1Password SSH agent:
**1Password → Settings → Developer → "Use the SSH Agent"**

### PATH setup

The user needs `berth/bin` in their PATH:
```bash
export PATH="$PATH:/path/to/berth/bin"
```

## Rebuild the image

After changes to the Dockerfile, entrypoint, or when CLI tools need updating:

```bash
# Standard rebuild (uses Docker cache)
berth update

# Fast: rebuild only the AI CLI layer (Claude Code / Codex updates)
berth update --quick

# Full: no cache, rebuild everything from scratch
berth update --full
```

### When to use each

| Scenario | Command |
|----------|---------|
| Claude Code or Codex released a new version | `berth update --quick` |
| Changed Dockerfile, entrypoint, or config | `berth update` |
| Need fresh OS packages or full clean slate | `berth update --full` |

## VM management

```bash
# Start VM and re-apply hardening
berth vm start

# Stop the VM
berth vm stop

# SSH into VM for debugging
berth vm ssh
```

**Important:** Prefer `berth vm start` over raw `container machine run` because it re-applies filesystem hardening.

## Policy rules

Spawn-time policy lives outside the VM:

```bash
~/.berth/rules.toml
.berth/rules.toml
```

Rules deny risky options before networks, worktrees, or containers are created. Use them to lock down Docker mode, networks, AWS profiles, SSH forwarding, shared auth, auth seeding, and repo setup scripts.

## Troubleshooting

### "VM not found" error
```bash
berth setup   # Creates the VM
```

### "'container' is required but not installed"
```bash
open https://github.com/apple/container/releases
```

### Image build fails
```bash
# SSH into VM and check Docker
berth vm ssh
docker info
docker images
exit

# Try a full rebuild
berth update --full
```

### Apple container egress times out
`berth setup` loads host PF NAT rules under `com.apple/berth` and enables IP forwarding. If the admin prompt times out, rerun `berth setup` in an interactive macOS session and approve the prompt.

### macOS mounts became visible
```bash
berth vm start   # Re-applies hardening
```

### Spawn blocked by policy
```bash
cat ~/.berth/rules.toml
find .. -path '*/.berth/rules.toml'
```

Use a command that matches the allowlist; do not widen policy unless the user explicitly asks.

### Need to start over
```bash
berth cleanup            # Remove containers and networks; keep auth volumes
berth update --full      # Rebuild image from scratch
```
