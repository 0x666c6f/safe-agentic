# Usage Guide

> See [architecture.md](architecture.md) for system diagrams and sequence flows.

> **Homebrew users:** When installed via `brew install safe-agentic`, the commands are `safe-ag`, `safe-ag-claude`, and `safe-ag-codex` instead of `agent`, `agent-claude`, and `agent-codex`. All flags and subcommands are identical. Run `safe-ag --version` to check the installed version.

## Commands at a glance

| Command | What it does |
|---------|-------------|
| `agent setup` | First-time: create VM, harden it, build image |
| `agent spawn claude` | Launch Claude Code in a sandboxed container |
| `agent spawn codex` | Launch Codex in a sandboxed container |
| `agent-claude <url>` | Shortcut for `agent spawn claude --repo <url>` |
| `agent-codex <url>` | Shortcut for `agent spawn codex --repo <url>` |
| `agent shell` | Interactive shell (no agent, no auth) |
| `agent list` | Show running + stopped agent containers |
| `agent attach <name>` | Attach to running container, or restart a stopped one |
| `agent stop <name>` | Stop and remove a specific agent |
| `agent stop --all` | Stop and remove all agents |
| `agent cleanup` | Stop all, keep shared auth, prune networks |
| `agent cleanup --auth` | Also remove shared auth volumes |
| `agent mcp-login <server>` | MCP OAuth login (token persists in auth volume) |
| `agent sessions <name>` | Export session history from a container |
| `agent peek <name>` | Show last 30 lines of agent's tmux pane output |
| `agent aws-refresh <name>` | Refresh AWS credentials in a running container |
| `agent diagnose` | Check common setup/runtime issues |
| `agent update` | Rebuild the Docker image |
| `agent vm start` | Start VM and re-apply hardening |
| `agent vm stop` | Stop the VM |
| `agent vm ssh` | SSH into the VM for debugging |
| **Workflow** | |
| `agent diff <name>` | Show git diff from agent's working tree |
| `agent checkpoint create <name>` | Snapshot the working tree |
| `agent checkpoint list <name>` | List snapshots |
| `agent checkpoint revert <name> <ref>` | Revert to a snapshot |
| `agent todo add <name> "text"` | Add a merge requirement |
| `agent todo list <name>` | Show todos with completion status |
| `agent todo check <name> <index>` | Mark a todo as done |
| `agent pr <name>` | Create a GitHub PR from agent's branch |
| `agent review <name>` | AI code review (codex review or git diff) |
| **Fleet & Orchestration** | |
| `agent fleet manifest.yaml` | Spawn agents from a YAML manifest |
| `agent pipeline pipeline.yaml` | Run a multi-step agent pipeline |
| **Analytics** | |
| `agent cost <name>` | Estimate API spend from session data |
| `agent audit` | Show append-only operation log |

## Spawning agents

### Basic: public repo, no SSH

```bash
agent spawn claude --repo https://github.com/myorg/myrepo.git
```

### Private repo with SSH

```bash
agent spawn claude --ssh --repo git@github.com:myorg/myrepo.git
```

### With an initial prompt

Pass a task to the agent at launch:

```bash
agent spawn codex --ssh --prompt 'Fix the failing CI tests' --repo git@github.com:myorg/myrepo.git
```

For Claude this passes `-p 'TASK'`; for Codex it becomes a positional argument.

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

# Advanced flags still work
agent-claude --name api-fix --reuse-auth --identity 'You <you@example.com>' git@github.com:myorg/myrepo.git

# GitHub CLI auth + Docker access also pass through
agent-codex --reuse-gh-auth --docker git@github.com:myorg/myrepo.git
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

The token is stored in a named Docker volume (`agent-claude-auth` / `agent-codex-auth`) that persists until `agent cleanup --auth` or manual volume removal.

### Persistent GitHub CLI auth

`gh` is installed in the image. By default, `gh auth login` state is per-session and disappears on exit.

To persist it:

```bash
agent spawn claude --reuse-gh-auth --repo https://github.com/myorg/myrepo.git
```

This stores GitHub CLI auth in `agent-gh-auth` until `agent cleanup --auth`.

### Docker inside the agent

`docker` and Compose are installed in the image, but daemon access is opt-in:

```bash
# Safer default: per-session Docker-in-Docker sidecar
agent shell --docker --repo https://github.com/myorg/myrepo.git

# Broader access: mount the VM Docker socket directly
agent shell --docker-socket --repo https://github.com/myorg/myrepo.git
```

Use `--docker` unless you explicitly need the VM daemon. `--docker-socket` gives the agent direct control over Docker in the VM.

### AWS credentials

Inject AWS credentials from your host `~/.aws/credentials` into the container:

```bash
agent spawn claude --ssh --aws my-aws-profile --repo git@github.com:myorg/infra.git
```

This injects the specified profile and sets `AWS_PROFILE` inside the container. The credentials are written to a tmpfs at `~/.aws/credentials`.

Since assumed-role sessions expire (~1 hour), refresh credentials in a running container without restarting:

```bash
# Re-reads ~/.aws/credentials from host, writes into the container
agent aws-refresh my-task

# Explicit profile override
agent aws-refresh my-task my-profile

# Target the latest container
agent aws-refresh --latest
```

AWS SDKs re-read the credentials file on each call, so no container restart is needed.

### Host config injection

Your host `~/.codex/config.toml` and `~/.claude/settings.json` are automatically injected into containers on first launch. MCP server definitions, model settings, and feature flags carry over. If config already exists in the auth volume (from a prior run or `mcp-login`), the host config is not re-injected — existing config is preserved.

If no host config exists, safe-agentic writes minimal defaults:

- `~/.codex/config.toml` gets `approval_policy = "never"` and `sandbox_mode = "danger-full-access"`
- `~/.claude/settings.json` gets bypass-permissions mode

This matters mainly for `agent shell` and `agent attach`, where you may launch `codex` or `claude` manually after the container is already running.

### Git identity

Containers default to `Agent <agent@localhost>`. If you want commits attributed to you, export identity explicitly before launch:

```bash
GIT_AUTHOR_NAME="Your Name" \
GIT_AUTHOR_EMAIL="you@example.com" \
agent spawn claude --repo https://github.com/myorg/myrepo.git
```

`GIT_COMMITTER_NAME` / `GIT_COMMITTER_EMAIL` are also respected if set.

Or use a one-off flag:

```bash
agent spawn claude --identity "Your Name <you@example.com>" --repo https://github.com/myorg/myrepo.git
```

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

Shows all agent containers (running and stopped) with their names, agent type, repo, flags, and status.

### Attach

Attach to the running agent session, or restart a stopped container and reopen it. Claude and Codex containers use `tmux`; shell containers use the direct TTY path.

```bash
agent attach api-refactor
# or with full name:
agent attach agent-claude-api-refactor
agent attach --latest
```

If the container is stopped (exited), `agent attach` automatically restarts it and reattaches.

Claude/Codex detach without stopping the container: `Ctrl-b d`

### Copy files out

Use `agent cp` to pull logs, test output, or build artifacts back to the host without adding bind mounts:

```bash
agent cp api-refactor /workspace/tmp/test.log ./test.log
agent cp --latest /workspace/dist ./dist
```

This copies through a temp directory in the hardened VM, then writes to the host path you asked for.

### Stop

```bash
agent stop api-refactor      # Stop and remove one
agent stop --latest          # Stop and remove newest session
agent stop --all             # Stop and remove all
```

Stopping removes the container, its per-session network, and any DinD sidecar.

### Cleanup

```bash
agent cleanup
agent cleanup --auth
```

`agent cleanup` stops containers, removes managed networks, and prunes safe-agentic image layers. Shared auth volumes are kept by default.

Add `--auth` to remove shared auth volumes too.

Older releases removed shared auth volumes on plain `agent cleanup`. Use `agent cleanup --auth` if you want the old full-reset behavior.

### Container lifecycle

Containers persist after the agent exits (no auto-remove). This lets you reattach to resume work or export session history.

```mermaid
graph LR
    spawn["agent spawn"] --> running["Running<br/>(interactive session)"]
    running -->|"agent exits"| stopped["Stopped<br/>(container persists)"]
    running -->|"agent attach"| running
    stopped -->|"agent attach"| running
    stopped -->|"agent sessions"| stopped
    running -->|"agent stop"| gone["Removed"]
    stopped -->|"agent stop"| gone

    gone -->|"ephemeral volumes"| cleaned["Volumes removed"]
    gone -->|"--reuse-auth volume"| persisted["Auth persists"]
    persisted -->|"agent cleanup --auth"| cleaned

    style spawn fill:#e3f2fd,stroke:#1565c0
    style running fill:#dfd,stroke:#393
    style stopped fill:#fff3e0,stroke:#e65100
    style gone fill:#f5f5f5,stroke:#999
    style persisted fill:#ffd,stroke:#c93
```

## MCP OAuth login

Authenticate MCP servers (Linear, Notion, etc.) so tokens persist in the auth volume for all agents:

```bash
agent mcp-login linear
agent mcp-login notion
```

Runs OAuth in a temporary container with `--network=host`. The token is stored in the shared codex auth volume (`agent-codex-auth`). To target a specific container's auth volume instead:

```bash
agent mcp-login my-agent linear
agent mcp-login --latest notion
```

One-time setup per MCP server.

## Session export

Export agent session history (conversations, session index) from a container to the host:

```bash
agent sessions api-refactor
agent sessions --latest ~/my-sessions/
```

Default destination: `./agent-sessions/<container-name>/`. Works on both running and stopped containers.

## Peek at agent output

See what an agent is doing without attaching:

```bash
agent peek api-refactor           # last 30 lines
agent peek --latest --lines 50    # more lines from the latest container
```

Only works on running containers with tmux sessions (Claude/Codex agents). For shell containers or stopped agents, use `agent attach` instead.

## Developer workflow

### Diff — review agent's changes

```bash
agent diff api-refactor          # full git diff
agent diff --latest --stat       # diffstat summary
```

### Checkpoints — snapshot and revert

```bash
agent checkpoint create api-refactor "before refactor"
agent checkpoint list api-refactor
agent checkpoint revert api-refactor checkpoint-1712678400
```

Checkpoints use git stash refs (`refs/safe-agentic/checkpoints/`) so they don't pollute the branch history.

### Todos — merge gates

```bash
agent todo add api-refactor "Run tests"
agent todo add api-refactor "Update docs"
agent todo list api-refactor
agent todo check api-refactor 1
```

`agent pr` blocks if incomplete todos exist.

### PR creation

```bash
agent pr api-refactor --title "feat: add caching" --base dev
agent pr --latest
```

Requires `--ssh` (for push) and `gh` auth inside the container. Commits uncommitted changes, pushes, creates PR via `gh pr create`.

### Code review

```bash
agent review api-refactor               # codex review --uncommitted (or git diff fallback)
agent review --latest --base main       # codex review --base main
```

### Lifecycle scripts

Add a `safe-agentic.json` to your repo root:

```json
{
  "scripts": {
    "setup": "npm install && cp .env.example .env"
  }
}
```

The `setup` script runs automatically after the repo is cloned inside the container.

## Fleet & orchestration

### Fleet — spawn from manifest

```bash
agent fleet fleet.yaml
agent fleet fleet.yaml --dry-run
```

Manifest format:
```yaml
agents:
  - name: api-worker
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    reuse_auth: true
    prompt: "Fix the failing tests"
  - name: frontend
    type: codex
    repo: https://github.com/org/frontend.git
```

### Pipeline — multi-step workflows

```bash
agent pipeline pipeline.yaml
agent pipeline pipeline.yaml --dry-run
```

Pipeline format:
```yaml
name: test-and-fix
steps:
  - name: run-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Run all tests and report results"
    on_failure: fix-tests
  - name: fix-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Fix the failing tests"
    retry: 2
  - name: create-pr
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Create a PR with the fixes"
    depends_on: fix-tests
```

Steps execute sequentially. `depends_on` skips if dependency hasn't completed. `retry` re-attempts with backoff. `on_failure` triggers a handler step.

## Analytics

### Cost estimation

```bash
agent cost api-refactor
agent cost --latest
```

Parses session JSONL for token usage, estimates cost per model.

### Audit log

```bash
agent audit               # last 50 entries
agent audit --lines 100
```

Append-only JSONL log at `~/.config/safe-agentic/audit.jsonl`. Records every spawn, stop, and attach with timestamp and details.

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

Build logs now stream during `agent update`, so long rebuilds stay visible.

Launches are local-image only: if `safe-agentic:latest` is missing in the VM, `agent spawn` / `agent shell` will fail until you run `agent update` or `agent setup`.

### VM management

```bash
agent vm start            # Start VM + re-apply hardening
agent vm stop             # Stop the VM (containers stop too)
agent vm ssh              # Debug the VM itself
agent diagnose            # Check orb/VM/docker/image/SSH/defaults
```

Always use `agent vm start` (not `orb start`) — it re-applies the filesystem hardening that OrbStack may reset.

## Defaults / Profiles

safe-agentic loads `${XDG_CONFIG_HOME:-~/.config}/safe-agentic/defaults.sh` when present.
Use simple `KEY=value` assignments only. The file is treated as config, not sourced as shell.

Example:

```bash
SAFE_AGENTIC_DEFAULT_MEMORY=16g
SAFE_AGENTIC_DEFAULT_CPUS=8
SAFE_AGENTIC_DEFAULT_NETWORK=agent-isolated
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=true
SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH=true
SAFE_AGENTIC_DEFAULT_DOCKER=true
SAFE_AGENTIC_DEFAULT_SSH=false
SAFE_AGENTIC_DEFAULT_IDENTITY="Your Name <you@example.com>"
```

You can also set `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`, `GIT_COMMITTER_NAME`, and `GIT_COMMITTER_EMAIL` directly there if you prefer explicit env vars.

## Troubleshooting

### `No SSH_AUTH_SOCK in VM`

Run:

```bash
agent diagnose
agent vm start
```

If it still fails, confirm 1Password SSH agent is enabled on macOS, then re-run with `--ssh`.

### `Docker may need a re-login for group changes`

Re-run:

```bash
agent vm start
agent diagnose
```

If Docker is still unavailable inside the VM, run `agent setup` again.

### `Image 'safe-agentic:latest' not found in VM`

Run:

```bash
agent update
```

This is expected after a fresh VM or image cleanup.

### OAuth appears to hang

For Claude or Codex first run, wait for the login URL/device code prompt inside the container. If you want to preserve the session afterward, launch with `--reuse-auth`.

## Tools available inside containers

| Category | Tools |
|----------|-------|
| AI agents | `claude`, `codex` |
| SRE | `terraform`, `kubectl`, `helm`, `aws`, `vault`, `docker`, `docker compose` |
| Modern CLI | `rg`, `fd`, `bat`, `eza`, `z` (zoxide), `fzf`, `jq`, `yq`, `delta`, `gh` |
| Runtimes | Node.js 22, `npm`, `pnpm`, `bun`, Python 3.12, Go 1.23 |
| Build | `pip`, `go build`, `docker build` (with `--docker` or `--docker-socket`) |

## Typical workflows

### Daily development

```bash
# Start your session
agent-claude git@github.com:myorg/service.git

# ... Claude Code opens, work as usual ...

# When done, the container stops but persists (reattach with agent attach)
# To remove it:
agent stop --latest
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
