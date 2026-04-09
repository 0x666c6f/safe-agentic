# Architecture Overview

safe-agentic creates a multi-layered sandbox for AI coding agents. The design philosophy is **safe by default** — every dangerous capability (SSH, auth persistence, Docker access, internet) requires explicit opt-in. Inside the sandbox, agents run with full autonomy.

## System diagram

```mermaid
graph TB
    subgraph mac["macOS Host"]
        user["User (terminal)"]
        cli["bin/agent CLI"]
        browser["Browser (OAuth)"]
        op["1Password<br/>SSH Agent"]
        orbctl["OrbStack"]

        user -->|"agent spawn / agent-claude"| cli
        user -->|"open OAuth URL"| browser
    end

    subgraph vm["OrbStack VM — safe-agentic (Ubuntu 24.04)"]
        direction TB
        hardening["VM Hardening<br/>tmpfs over /Users, /mnt/mac<br/>open, osascript removed<br/>fstab blocking"]
        dockerd["Docker daemon<br/>userns-remap enabled"]

        subgraph netA["Bridge network: agent-claude-task-net"]
            subgraph containerA["Container: agent-claude-task"]
                rootfs["Read-only rootfs"]
                entrypoint["entrypoint.sh"]
                agent["Claude Code<br/>--dangerously-skip-permissions"]
                workspace["/workspace/org/repo"]
                tmpfs_home["tmpfs: .ssh, .config, .local"]
                volumes["Ephemeral volumes:<br/>auth, caches, workspace"]
            end
        end

        subgraph netB["Bridge network: agent-codex-fix-net"]
            subgraph containerB["Container: agent-codex-fix"]
                agentB["Codex --yolo"]
            end
        end
    end

    cli -->|"orb run -m safe-agentic<br/>docker run ..."| dockerd
    op -.->|"SSH socket (only with --ssh)<br/>mounted read-only"| containerA
    browser -.->|"OAuth callback"| agent
    dockerd --> containerA
    dockerd --> containerB
    orbctl --> vm
```

## Key design decisions

### Why OrbStack instead of native Docker?

Docker Desktop on macOS shares the host filesystem by default — containers can read `~/.ssh`, `~/.aws`, browser cookies, and anything else in your home directory. OrbStack VMs provide a real Linux kernel with controllable filesystem sharing. safe-agentic blocks all macOS mount points inside the VM with tmpfs overlays, creating an air gap that Docker Desktop cannot provide.

### Why userns-remap?

Without user namespace remapping, root inside a container is root on the host (within the VM). With `userns-remap`, container UID 0 maps to an unprivileged UID on the host. Even if an agent escapes the container, it has no privileges in the VM.

### Why read-only rootfs?

A read-only root filesystem prevents agents from modifying system binaries, installing backdoors, or tampering with the container's own tooling. All writable paths are explicit — tmpfs for ephemeral data, volumes for workspace and caches.

### Why cap-drop ALL?

Linux capabilities grant specific privileges (mounting filesystems, binding to privileged ports, loading kernel modules, etc.). Dropping all capabilities means the agent process has the minimum privileges needed to run user-space code. Combined with `no-new-privileges`, this prevents any privilege escalation.

### Why per-container networks?

Each container gets its own bridge network so agents cannot communicate with each other. A compromised agent cannot lateral-move to attack other running agents. Network egress is filtered to TCP 22/80/443 only — no arbitrary outbound connections.

## Component map

```mermaid
graph TD
    subgraph host["macOS Host"]
        agent_cli["bin/agent<br/>CLI dispatcher"]
        agent_lib["bin/agent-lib.sh<br/>Container/network helpers"]
        docker_rt["bin/docker-runtime.sh<br/>Docker/DinD helpers"]
        alias_claude["bin/agent-claude<br/>Quick alias"]
        alias_codex["bin/agent-codex<br/>Quick alias"]

        alias_claude -->|"exec agent spawn claude"| agent_cli
        alias_codex -->|"exec agent spawn codex"| agent_cli
        agent_cli -->|"source"| agent_lib
        agent_cli -->|"source"| docker_rt
    end

    subgraph image["Docker Image (built once)"]
        dockerfile["Dockerfile<br/>Pinned + checksummed binaries"]
        entrypoint["entrypoint.sh<br/>Runtime init"]
        session["bin/agent-session.sh<br/>Agent launcher"]
        bashrc["config/bashrc<br/>Shell environment"]
        ssh_baked[".ssh.baked/<br/>GitHub host keys"]
    end

    subgraph vm_scripts["VM Scripts"]
        setup["vm/setup.sh<br/>Harden VM + install Docker"]
    end

    agent_cli -->|"agent setup / vm start"| setup
    agent_cli -->|"agent update"| dockerfile
    agent_cli -->|"agent spawn"| entrypoint
```

### Host-side components

| File | Role |
|------|------|
| `bin/agent` | Main CLI dispatcher. All commands are `cmd_*` functions. |
| `bin/agent-lib.sh` | Shared functions: input validation, network lifecycle, container runtime construction, volume helpers. Docker commands built as bash arrays to prevent injection. |
| `bin/docker-runtime.sh` | Docker-in-Docker sidecar and host socket access helpers. |
| `bin/agent-claude`, `bin/agent-codex` | Quick aliases that auto-detect SSH URLs and delegate to `bin/agent spawn`. |
| `bin/agent-session.sh` | Runs inside the container's tmux session. Handles Claude (`--dangerously-skip-permissions` via `script` PTY) and Codex (`--yolo`). |
| `bin/repo-url.sh` | URL parsing and validation. Rejects traversal attacks, dot-prefixed names, special characters. |

### Container-side components

| File | Role |
|------|------|
| `entrypoint.sh` | Container init: copies SSH config to tmpfs, writes git config, injects host config, validates and clones repos, runs lifecycle scripts, launches agent inside tmux. |
| `config/bashrc` | Shell environment with modern tool aliases (rg, fd, bat, eza). |
| `Dockerfile` | Multi-layer image build. All binary downloads pinned with SHA256 checksums or GPG verification. Non-root `agent` user, no sudo. |

### VM-side components

| File | Role |
|------|------|
| `vm/setup.sh` | Idempotent VM bootstrap. Blocks macOS mounts, removes integration commands, installs Docker CE with userns-remap, installs socat for SSH relay. |
