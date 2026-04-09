# Container Internals

This page describes what's inside each agent container — the filesystem layout, how the image is built, and the lifecycle from spawn to exit.

## Filesystem layout

```mermaid
graph TD
    subgraph rootfs["Read-only rootfs (image layers)"]
        usr["/usr — system binaries"]
        opt["/opt/agent-cli — claude, codex"]
        etc["/etc — system config"]
        ssh_baked["/home/agent/.ssh.baked — GitHub host keys"]
        bashrc_baked["/home/agent/.bashrc"]
    end

    subgraph tmpfs["tmpfs (RAM, discarded on exit)"]
        tmp["/tmp (noexec, 512m)"]
        var_tmp["/var/tmp (noexec, 256m)"]
        run["/run (noexec, 16m)"]
        shm["/dev/shm (noexec, 64m)"]
        ssh_live["/home/agent/.ssh (1m)"]
        config["/home/agent/.config (32m)"]
    end

    subgraph volumes["Docker volumes (ephemeral by default)"]
        workspace["/workspace — cloned repos"]
        npm["/home/agent/.npm"]
        pip["/home/agent/.cache/pip"]
        gopath["/home/agent/go"]
        tf["/home/agent/.terraform.d"]
        auth_claude["/home/agent/.claude"]
        auth_codex["/home/agent/.codex"]
    end

    ssh_baked -.->|"entrypoint copies"| ssh_live

    style rootfs fill:#f5f5f5,stroke:#999
    style tmpfs fill:#fff3e0,stroke:#e65100
    style volumes fill:#e3f2fd,stroke:#1565c0
```

### Three types of storage

**Read-only rootfs** — The Docker image layers. Contains all installed tools, system config, and baked SSH host keys. Cannot be modified at runtime. This prevents agents from tampering with their own environment.

**tmpfs (RAM-backed)** — Ephemeral writable paths mounted as tmpfs. Used for SSH config (copied from baked originals), git config, and temporary files. Size-limited and marked `noexec`. Lost when the container exits.

**Docker volumes** — Writable persistent storage for the workspace, caches, and auth tokens. By default these are anonymous volumes (destroyed with the container). With `--reuse-auth`, the auth volume is a named volume that survives container removal.

## Image build

The Dockerfile is a multi-layer build with strict supply chain controls:

1. **Base**: Ubuntu 24.04 pinned by digest (not just tag)
2. **System packages**: Standard apt packages, GPG-verified
3. **Binary tools**: Each binary download is pinned to a specific version and verified with SHA256 checksums
4. **AWS CLI**: Installed via official installer with GPG signature verification
5. **AI CLIs**: Claude Code via official installer (version-pinned, verified by `claude --version`), Codex via `npm ci` with lockfile
6. **User setup**: Non-root `agent` user (UID 1000), no sudo, no supplemental groups

```bash
# Build the image
agent update              # uses Docker cache
agent update --quick      # rebuilds only AI CLI layer
agent update --full       # no cache, rebuilds everything
```

The build context is constructed from `git ls-files -c` filtered by `test -e` — only tracked files that exist on disk are sent. This prevents `.env` files, credentials, or untracked data from leaking into the image.

## Entrypoint flow

When a container starts, `entrypoint.sh` runs through this sequence:

```mermaid
graph TD
    start["Container starts"] --> ssh["Copy .ssh.baked/* to tmpfs .ssh/"]
    ssh --> git["Write git config from env vars"]
    git --> config["Ensure Claude/Codex config exists"]
    config --> aws{"AWS creds injected?"}
    aws -->|yes| write_aws["Write to tmpfs ~/.aws/credentials"]
    aws -->|no| repos
    write_aws --> repos
    repos{"REPOS env var set?"}
    repos -->|yes| clone["Validate & clone each repo"]
    repos -->|no| launch
    clone --> lifecycle["Run safe-agentic.json setup script"]
    lifecycle --> launch["Launch agent or shell"]
    launch --> claude{"AGENT_TYPE?"}
    claude -->|claude| tmux_c["tmux → claude --dangerously-skip-permissions"]
    claude -->|codex| tmux_x["tmux → codex --yolo"]
    claude -->|shell| bash["bash -l"]
```

Key security checks during entrypoint:

- **Repo URL validation**: `repo_clone_path()` rejects traversal attacks (`../`), dot-prefixed names, special characters, and non-standard URL schemes. Only `https://`, `ssh://`, and `git@host:org/repo` patterns are accepted.
- **Config writing**: All config writes check for writable directories first. On read-only filesystems, writes are silently skipped.
- **Lifecycle scripts**: `safe-agentic.json` setup scripts run in a subshell with `bash -c`. Failures are warned but don't block agent startup.

## Agent session management

Claude and Codex run inside a tmux session named `safe-agentic`. This enables:

- **Detach without stopping**: `Ctrl-b d` detaches the tmux session while the agent keeps running
- **Reattach**: `agent attach` reconnects to the live tmux session
- **Resume**: If the container was stopped, `agent attach` restarts it and the agent's `--continue` / `resume --last` picks up the previous conversation
- **Preview**: `agent peek` captures the last N lines of the tmux pane without attaching
- **Session state**: A state file at `/workspace/.safe-agentic/started` tracks whether this is a fresh start or resume

## Container lifecycle

```mermaid
sequenceDiagram
    actor User
    participant CLI as bin/agent
    participant Docker as Docker (in VM)
    participant Container
    participant Entry as entrypoint.sh

    User->>CLI: agent spawn claude --ssh --repo git@...
    CLI->>CLI: Parse flags, validate inputs
    CLI->>Docker: Create dedicated bridge network
    CLI->>Docker: docker run (detached, hardened flags)
    Docker->>Container: Start container
    Container->>Entry: Run entrypoint.sh
    Entry->>Entry: Setup (SSH, git, config, clone, lifecycle scripts)
    Entry->>Entry: Start tmux session with agent
    CLI->>Docker: docker exec tmux attach
    Note over User,Container: User interacts with agent

    alt User detaches (Ctrl-b d)
        Note over Container: Container keeps running
        User->>CLI: agent attach <name>
        CLI->>Docker: docker exec tmux attach
    else Agent exits
        Note over Container: Container stops, persists
        User->>CLI: agent attach <name>
        CLI->>Docker: docker start <name>
        CLI->>Docker: docker exec tmux attach
    else User stops explicitly
        User->>CLI: agent stop <name>
        CLI->>Docker: docker stop + docker rm
        CLI->>Docker: Remove bridge network
    end
```
