# Usage Guide

> See [architecture.md](architecture.md) for system diagrams and sequence flows.

## Commands at a glance

| Command | What it does |
|---------|-------------|
| `agent setup` | First-time: create VM, harden it, build image |
| `agent spawn claude` | Launch Claude Code in a sandboxed container |
| `agent spawn codex` | Launch Codex in a sandboxed container |
| `agent-claude <url>` | Shortcut for `agent spawn claude --repo <url>` |
| `agent-codex <url>` | Shortcut for `agent spawn codex --repo <url>` |
| `agent shell` | Interactive shell (no agent, no auth) |
| `agent list` | Show running agent containers |
| `agent attach <name>` | Open a second shell in a running agent |
| `agent stop <name>` | Stop a specific agent |
| `agent stop --all` | Stop all agents |
| `agent cleanup` | Stop all, remove shared auth, prune networks |
| `agent update` | Rebuild the Docker image |
| `agent vm start` | Start VM and re-apply hardening |
| `agent vm stop` | Stop the VM |
| `agent vm ssh` | SSH into the VM for debugging |

## Spawning agents

### Basic: public repo, no SSH

```bash
agent spawn claude --repo https://github.com/myorg/myrepo.git
```

### Private repo with SSH

```bash
agent spawn claude --ssh --repo git@github.com:myorg/myrepo.git
```

The `--ssh` flag forwards your 1Password SSH agent into the container so `git clone`, `git push`, etc. work with your SSH keys.

By default, safe-agentic-managed networks block private/local egress and only allow outbound TCP `22`, `80`, and `443`. Passing `--network <name>` opts out of those guardrails and joins the named Docker network as-is.

### Quick aliases (recommended)

The aliases auto-detect whether SSH is needed based on the URL:

```bash
# HTTPS repo — no SSH forwarded
agent-claude https://github.com/myorg/myrepo.git

# SSH repo — SSH auto-enabled
agent-claude git@github.com:myorg/myrepo.git

# Codex variant
agent-codex git@github.com:myorg/myrepo.git
```

### Named sessions

Give your session a name so it's easy to find and attach to:

```bash
agent spawn claude --ssh --name api-refactor --repo git@github.com:myorg/api.git
```

The container will be named `agent-claude-api-refactor` (visible in `agent list`).

### Multiple repos

Clone several repos into the same container:

```bash
agent-claude git@github.com:myorg/frontend.git git@github.com:myorg/backend.git
```

Each repo is cloned to `/workspace/org/repo` inside the container.

### Persistent auth

By default, you re-authenticate every time (OAuth token is discarded when the container exits). To keep your login across sessions:

```bash
agent spawn claude --ssh --reuse-auth --repo git@github.com:myorg/myrepo.git
```

The token is stored in a named Docker volume (`agent-claude-auth` / `agent-codex-auth`) that persists until `agent cleanup`.

### Git identity

Containers default to `Agent <agent@localhost>`. If you want commits attributed to you, export identity explicitly before launch:

```bash
GIT_AUTHOR_NAME="Your Name" \
GIT_AUTHOR_EMAIL="you@example.com" \
agent spawn claude --repo https://github.com/myorg/myrepo.git
```

`GIT_COMMITTER_NAME` / `GIT_COMMITTER_EMAIL` are also respected if set.

### Custom resource limits

Defaults: 8 GB memory, 4 CPUs, 512 PIDs. Override when needed:

```bash
agent spawn claude --memory 16g --cpus 8 --pids-limit 1024 --repo https://github.com/myorg/big-repo.git
```

Same flags work for `agent shell`.

### Untrusted repos

For repos you don't trust, create an isolated network with no internet:

```bash
# One-time: create the network inside the VM
agent vm ssh
docker network create --internal agent-isolated
exit

# Spawn without SSH, on the isolated network
agent spawn claude --repo https://github.com/sketchy/repo.git --network agent-isolated
```

The agent can work on the code but can't reach the internet, your SSH keys, or other containers.

## Managing running agents

### List

```bash
agent list
```

Shows all running agent containers with their names, status, and creation time.

### Attach

Open a second shell into a running agent. Useful for checking logs, running tests in parallel, etc.

```bash
agent attach api-refactor
# or with full name:
agent attach agent-claude-api-refactor
```

### Stop

```bash
agent stop api-refactor      # Stop one
agent stop --all              # Stop all
```

Stopping removes the container and its per-session network.

### Cleanup

```bash
agent cleanup
```

Stops all containers, removes shared auth volumes (from `--reuse-auth`), prunes managed networks and dangling images.

### Container lifecycle

```mermaid
graph LR
    spawn["agent spawn"] --> running["Running<br/>(interactive session)"]
    running -->|"agent exit"| gone["Destroyed<br/>(auto --rm)"]
    running -->|"agent stop"| gone
    running -->|"agent attach"| running

    gone -->|"ephemeral volumes"| cleaned["Volumes removed"]
    gone -->|"--reuse-auth volume"| persisted["Auth persists"]
    persisted -->|"agent cleanup"| cleaned

    style spawn fill:#e3f2fd,stroke:#1565c0
    style running fill:#dfd,stroke:#393
    style gone fill:#fff3e0,stroke:#e65100
    style cleaned fill:#f5f5f5,stroke:#999
    style persisted fill:#ffd,stroke:#c93
```

## Interactive shell

Get a shell with all the tools but no agent running:

```bash
agent shell --ssh --repo git@github.com:myorg/myrepo.git
```

Same hardening as agent containers (read-only rootfs, dropped capabilities, etc.) but no Claude/Codex auth volumes.

## Image maintenance

### Rebuild image

```bash
agent update              # Standard rebuild (uses Docker cache)
agent update --quick      # Rebuild only the AI CLI layer (fast)
agent update --full       # Full rebuild from scratch (no cache)
```

Use `--quick` after Claude Code or Codex releases a new version. Use `--full` to pick up OS package updates.

Launches are local-image only: if `safe-agentic:latest` is missing in the VM, `agent spawn` / `agent shell` will fail until you run `agent update` or `agent setup`.

### VM management

```bash
agent vm start            # Start VM + re-apply hardening
agent vm stop             # Stop the VM (containers stop too)
agent vm ssh              # Debug the VM itself
```

Always use `agent vm start` (not `orb start`) — it re-applies the filesystem hardening that OrbStack may reset.

## Tools available inside containers

| Category | Tools |
|----------|-------|
| AI agents | `claude`, `codex` |
| SRE | `terraform`, `kubectl`, `helm`, `aws`, `vault` |
| Modern CLI | `rg`, `fd`, `bat`, `eza`, `z` (zoxide), `fzf`, `jq`, `yq`, `delta`, `gh` |
| Runtimes | Node.js 22, Python 3.12, Go 1.23 |
| Build | `gcc`, `make`, `npm`, `pip`, `go build` |

## Typical workflows

### Daily development

```bash
# Start your session
agent-claude git@github.com:myorg/service.git

# ... Claude Code opens, work as usual ...

# When done, the container is removed automatically on exit
```

### Parallel sessions

Run multiple agents simultaneously on different repos or tasks:

```bash
agent spawn claude --ssh --name feature-a --repo git@github.com:myorg/repo.git
agent spawn claude --ssh --name feature-b --repo git@github.com:myorg/repo.git
agent spawn codex --ssh --name codex-review --repo git@github.com:myorg/repo.git
```

Each gets its own container, network, and workspace — fully isolated from each other:

```mermaid
graph TB
    subgraph vm["OrbStack VM"]
        subgraph nA["net: feature-a"]
            cA["agent-claude-feature-a<br/>Claude Code"]
            wA["/workspace/myorg/repo"]
        end
        subgraph nB["net: feature-b"]
            cB["agent-claude-feature-b<br/>Claude Code"]
            wB["/workspace/myorg/repo"]
        end
        subgraph nC["net: codex-review"]
            cC["agent-codex-codex-review<br/>Codex"]
            wC["/workspace/myorg/repo"]
        end
    end

    cA -.-x cB
    cB -.-x cC
    cA -.-x cC
```

### Review untrusted code

```bash
agent spawn claude --repo https://github.com/unknown/suspicious.git --network agent-isolated
```

No SSH keys, no internet — the agent can only read and modify the cloned code locally.
