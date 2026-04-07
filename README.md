# safe-agentic

Isolated environment for running AI coding agents (Claude Code, Codex) safely. Two layers of isolation: an OrbStack VM running Docker, with ephemeral per-agent containers inside.

## Architecture

```
macOS Host
  ├── 1Password (SSH keys for git)
  ├── OrbStack
  ├── bin/agent CLI (in your PATH)
  │
  └── OrbStack VM: "safe-agentic" (persistent Ubuntu 24.04, hardened)
       ├── macOS filesystem access blocked (tmpfs over /Users, /mnt/mac)
       ├── macOS integration commands removed (open, osascript, code)
       ├── Docker daemon
       ├── Auth volumes (OAuth tokens for Claude/Codex — shared)
       ├── SSH agent (forwarded from macOS → 1Password)
       │
       └── Ephemeral per-agent containers (isolated from each other)
            ├── agent-claude-task1  (own cache volumes, cloned repos)
            ├── agent-codex-task2   (own cache volumes, cloned repos)
            └── ...
```

## Threat model

**Goal:** Protect your macOS host and your repos from unintended agent side-effects. This is a **damage containment** setup, not a security sandbox against actively malicious code.

**What this protects against:**
- Agents modifying files outside their cloned repo
- Agents accessing your macOS filesystem (hardened: macOS mounts blocked in VM)
- Agents interfering with each other (per-agent containers + per-agent caches)
- Credential sprawl (OAuth tokens in volumes, SSH via forwarded agent)

**Known limitations:**
- **OrbStack hardening is best-effort.** OrbStack does not yet support per-VM file sharing disable ([#169](https://github.com/orbstack/orbstack/issues/169)). `vm/setup.sh` mounts tmpfs over macOS paths and removes mac commands, but OrbStack may re-enable sharing on VM restart. Re-run `agent setup` after VM restarts, and disable file sharing in OrbStack UI (Settings > Linux) for defense-in-depth.
- **`--dangerously-skip-permissions` is broad.** Claude Code in this mode can execute any command inside the container. Combined with SSH agent forwarding and network access, a malicious repo could push to other repos or exfiltrate data. This setup assumes you trust the repos you clone. For untrusted repos, add Docker network restrictions (see below).
- **SSH agent is forwarded.** Any container can use your SSH keys for git operations. 1Password SSH agent may prompt per-use (if configured), but this is not guaranteed.
- **OAuth volumes are type-scoped.** Claude containers only see `agent-claude-auth`, Codex only sees `agent-codex-auth`, plain shells see neither. But all Claude containers share the same Claude token volume.

**For untrusted repos (optional hardening):**
```bash
# Create an isolated Docker network with no internet access (one-time)
agent vm ssh
docker network create --internal agent-isolated
exit

# Extra args after -- are passed as docker run flags (before the image name)
agent spawn claude --repo <untrusted-repo> -- --network agent-isolated
```

## Prerequisites

1. **OrbStack**: `brew install orbstack`
2. **1Password Desktop App** (for SSH keys):
   - Settings → Developer → Enable "Use the SSH Agent"
   - SSH key for GitHub configured in 1Password
3. **PATH**: Add `safe-agentic/bin` to your shell:
   ```bash
   export PATH="$PATH:$HOME/perso/safe-agentic/bin"
   ```

## Setup

```bash
agent setup
```

This creates the OrbStack VM, hardens it (blocks macOS filesystem access, removes mac integration commands), installs Docker inside, and builds the agent container image.

**After VM restarts:** Re-run `agent setup` to re-apply hardening (OrbStack may restore mounts on restart).

## Usage

### Spawn an agent

```bash
# Claude Code on a repo
agent spawn claude --repo git@github.com:myorg/myrepo.git

# Codex on a repo
agent spawn codex --repo git@github.com:myorg/myrepo.git

# Named session
agent spawn claude --repo git@github.com:myorg/api.git --name api-refactor

# Multiple repos (cloned as org/repo to avoid name collisions)
agent spawn claude --repo git@github.com:myorg/api.git --repo git@github.com:other/api.git

# Quick aliases
agent-claude git@github.com:myorg/myrepo.git
agent-codex git@github.com:myorg/myrepo.git
```

### Manage agents

```bash
agent list                  # List running agents
agent attach <name>         # Open second shell in running agent
agent stop <name>           # Stop specific agent
agent stop --all            # Stop all agents
agent cleanup               # Stop all + remove containers + prune per-agent cache volumes
```

### Interactive shell (no agent)

```bash
agent shell --repo git@github.com:myorg/myrepo.git
```

### Maintenance

```bash
agent update                # Rebuild image
agent update --quick        # Rebuild AI CLI layer only (fast)
agent update --full         # Full rebuild, no cache
```

### VM management

```bash
agent vm ssh                # SSH into the VM for debugging
agent vm stop               # Stop the VM
agent vm start              # Start the VM
```

## Tools included

### AI Agents
- Claude Code (`claude`)
- Codex (`codex`)

### SRE/DevOps
terraform, kubectl, helm, aws-cli v2, vault, k9s

### Modern CLI
ripgrep (`rg`), fd, bat, eza, zoxide (`z`), fzf, jq, yq, delta, gh

### Runtimes
Node.js 22, Python 3.12, Go 1.23

## How auth works

### Claude Code / Codex (OAuth)

On first `agent spawn`, the CLI shows an OAuth URL. Open it in your macOS browser to authenticate with your subscription. The OAuth token is persisted in a shared Docker volume (`agent-claude-auth` / `agent-codex-auth`), so you only log in once.

### Git (SSH via 1Password)

```
git clone/push inside container
  → SSH agent socket forwarded: container → VM → macOS → 1Password
  → Uses SSH keys managed by 1Password
```

### Build context safety

`agent update` uses `git archive` to send only git-tracked files to the VM. Local `.env`, untracked files, and secrets are never copied.

## Per-agent isolation

Each agent container gets its own cache volumes (npm, pip, go, terraform). This prevents cache poisoning across agents. The `agent cleanup` command removes these per-agent volumes along with stopped containers.
