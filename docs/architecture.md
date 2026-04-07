# Architecture

## System overview

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

## Isolation boundaries

Three nested boundaries separate the agent from your host:

```mermaid
graph LR
    subgraph B1["Boundary 1: VM"]
        direction TB
        b1_1["macOS mounts blocked<br/>(tmpfs over /Users, /mnt/mac)"]
        b1_2["Integration commands removed<br/>(open, osascript, code)"]
        b1_3["fstab + tmpfs overlay persistence"]
        b1_4["Docker userns-remap"]
    end

    subgraph B2["Boundary 2: Container"]
        direction TB
        b2_1["Read-only root filesystem"]
        b2_2["cap-drop ALL + no-new-privileges"]
        b2_3["Memory / CPU / PID limits"]
        b2_4["Non-root user, no sudo"]
        b2_5["Dedicated bridge network"]
    end

    subgraph B3["Boundary 3: Container-to-container"]
        direction TB
        b3_1["Separate network namespaces"]
        b3_2["Separate volume mounts"]
        b3_3["No shared state"]
    end

    B1 --> B2 --> B3

    style B1 fill:#fee,stroke:#c33
    style B2 fill:#ffd,stroke:#c93
    style B3 fill:#dfd,stroke:#393
```

## Component map

```mermaid
graph TD
    subgraph host["macOS Host"]
        agent_cli["bin/agent<br/>CLI dispatcher"]
        agent_lib["bin/agent-lib.sh<br/>Container/network helpers"]
        alias_claude["bin/agent-claude<br/>Quick alias"]
        alias_codex["bin/agent-codex<br/>Quick alias"]

        alias_claude -->|"exec agent spawn claude"| agent_cli
        alias_codex -->|"exec agent spawn codex"| agent_cli
        agent_cli -->|"source"| agent_lib
    end

    subgraph image["Docker Image (built once)"]
        dockerfile["Dockerfile<br/>6 layers, pinned + checksummed"]
        entrypoint["entrypoint.sh<br/>Runtime init"]
        bashrc["config/bashrc<br/>Shell environment"]
        ssh_baked[".ssh.baked/<br/>GitHub host keys"]
        cli_bundle["/opt/agent-cli<br/>claude, codex (npm ci)"]
    end

    subgraph vm_scripts["VM Scripts"]
        setup["vm/setup.sh<br/>Harden VM + install Docker"]
    end

    agent_cli -->|"agent setup / vm start"| setup
    agent_cli -->|"agent update"| dockerfile
    agent_cli -->|"agent spawn"| entrypoint
```

## Flows

### First-time setup

```mermaid
sequenceDiagram
    actor User
    participant CLI as bin/agent
    participant OrbStack
    participant VM as OrbStack VM
    participant Docker as Docker (in VM)

    User->>CLI: agent setup
    CLI->>OrbStack: orb create ubuntu safe-agentic
    OrbStack-->>CLI: VM created
    CLI->>OrbStack: orb start safe-agentic
    CLI->>VM: copy vm/setup.sh
    CLI->>VM: bash /tmp/setup.sh

    Note over VM: Harden VM
    VM->>VM: Unmount /Users, /mnt/mac, /Volumes
    VM->>VM: Overlay read-only tmpfs
    VM->>VM: Add fstab entries
    VM->>VM: Remove open, osascript, code
    VM->>VM: Install Docker CE
    VM->>VM: Enable userns-remap

    CLI->>VM: tar tracked files → VM
    CLI->>Docker: docker build -t safe-agentic
    Note over Docker: Build 6-layer image<br/>SHA256-verify all binaries
    Docker-->>CLI: Image ready
    CLI-->>User: Setup complete
```

### Spawning an agent

```mermaid
sequenceDiagram
    actor User
    participant CLI as bin/agent
    participant Lib as agent-lib.sh
    participant VM as OrbStack VM
    participant Docker as Docker (in VM)
    participant Container
    participant Entry as entrypoint.sh
    participant Agent as Claude/Codex

    User->>CLI: agent spawn claude --ssh --repo git@...

    CLI->>CLI: Parse flags, read git identity
    CLI->>Lib: prepare_network()
    Lib->>Docker: docker network create agent-claude-*-net
    CLI->>Lib: build_container_runtime()

    Note over Lib: Build docker run command array:<br/>--cap-drop=ALL --read-only<br/>--security-opt=no-new-privileges<br/>--memory 8g --cpus 4 --pids-limit 512<br/>tmpfs mounts, ephemeral volumes<br/>SSH socket (if --ssh)

    CLI->>Docker: orb run -m VM docker run ...
    Docker->>Container: Create container
    Container->>Entry: /usr/local/bin/entrypoint.sh

    Note over Entry: Copy baked SSH config to tmpfs<br/>Write git config from env vars

    Entry->>Entry: Validate & clone repo URL
    Entry->>Entry: cd /workspace/org/repo

    alt AGENT_TYPE=claude
        Entry->>Agent: exec claude --dangerously-skip-permissions
    else AGENT_TYPE=codex
        Entry->>Agent: codex login --device-auth (first run)
        Entry->>Agent: exec codex --yolo
    end

    Agent-->>User: OAuth URL displayed
    User->>User: Open URL in browser, authenticate
    Agent-->>User: Agent ready, interactive session
```

### SSH authentication chain

```mermaid
sequenceDiagram
    participant Agent as Agent (in container)
    participant Socket as SSH Socket<br/>(mounted :ro)
    participant VM as OrbStack VM
    participant Mac as macOS Host
    participant 1P as 1Password Agent
    participant GH as GitHub

    Agent->>Socket: git push (SSH)
    Socket->>VM: Forward via unix socket
    VM->>Mac: OrbStack SSH forwarding
    Mac->>1P: Sign request
    Note over 1P: Private key never leaves<br/>1Password
    1P-->>Mac: Signed challenge
    Mac-->>VM: Response
    VM-->>Socket: Response
    Socket-->>Agent: Authenticated

    Agent->>GH: SSH connection (signed)
    Note over Agent,GH: Host key verified against<br/>baked known_hosts<br/>(StrictHostKeyChecking yes)
    GH-->>Agent: Push accepted
```

### OAuth authentication flow

```mermaid
sequenceDiagram
    actor User
    participant Agent as Claude Code<br/>(in container)
    participant API as Anthropic API

    Agent->>API: Request OAuth device code
    API-->>Agent: Device code + verification URL
    Agent-->>User: Display URL in terminal

    User->>API: Open URL in browser, approve
    API-->>Agent: OAuth token issued

    Note over Agent: Token stored in<br/>ephemeral volume<br/>(lost on container exit)

    alt --reuse-auth
        Note over Agent: Token stored in<br/>named volume<br/>(survives container exit)
    end
```

### Container lifecycle

```mermaid
sequenceDiagram
    actor User
    participant CLI as bin/agent
    participant Docker as Docker (in VM)
    participant Net as Bridge Network
    participant Container
    participant Volumes as Ephemeral Volumes

    User->>CLI: agent spawn claude --name task

    CLI->>Net: Create agent-claude-task-net
    CLI->>Docker: docker run --rm ...
    Docker->>Volumes: Create anonymous volumes<br/>(workspace, auth, caches)
    Docker->>Container: Start container

    Note over Container: Agent runs...<br/>user works interactively

    alt User exits agent normally
        Container-->>Docker: Exit 0
    else User runs agent stop
        User->>CLI: agent stop task
        CLI->>Docker: docker stop agent-claude-task
        Docker-->>Container: SIGTERM
    end

    Docker->>Container: Remove (--rm)
    Docker->>Volumes: Remove anonymous volumes
    CLI->>Net: Remove agent-claude-task-net

    Note over Volumes: Auth token, workspace,<br/>caches all gone

    alt --reuse-auth was used
        Note over Volumes: Named auth volume<br/>survives until<br/>agent cleanup
    end
```

### Image build flow

```mermaid
sequenceDiagram
    actor User
    participant CLI as bin/agent
    participant Git as git (host)
    participant VM as OrbStack VM
    participant Docker as Docker (in VM)

    User->>CLI: agent update [--quick|--full]

    CLI->>Git: git ls-files -c (tracked files only)
    Note over Git: Filter: exists on disk<br/>Excludes: .env, untracked, deleted

    CLI->>CLI: tar -czf (build context)
    CLI->>VM: Copy tarball to VM
    CLI->>VM: Extract to /tmp/safe-agentic

    alt --quick
        CLI->>Docker: docker build --build-arg CLI_CACHE_BUST=...
        Note over Docker: Rebuilds only the<br/>npm ci layer (AI CLIs)
    else --full
        CLI->>Docker: docker build --no-cache
        Note over Docker: Rebuilds everything<br/>from scratch
    else default
        CLI->>Docker: docker build
        Note over Docker: Uses Docker cache<br/>for unchanged layers
    end

    Docker-->>CLI: Image built
```

## Container internals

### Filesystem layout

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
        tmp["/tmp (512m, noexec)"]
        var_tmp["/var/tmp (256m, noexec)"]
        run["/run (16m, noexec)"]
        ssh_live["/home/agent/.ssh (1m)"]
        config["/home/agent/.config (32m)"]
        local["/home/agent/.local (64m)"]
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
    rootfs -.->|"immutable"| rootfs
    tmpfs -.->|"lost on exit"| tmpfs
    volumes -.->|"anonymous = lost on exit<br/>named (--reuse-auth) = persists"| volumes

    style rootfs fill:#f5f5f5,stroke:#999
    style tmpfs fill:#fff3e0,stroke:#e65100
    style volumes fill:#e3f2fd,stroke:#1565c0
```

### Docker run flags

Every container is launched with this hardening applied by `agent-lib.sh`:

```mermaid
graph LR
    subgraph flags["Docker run flags"]
        cap["--cap-drop=ALL"]
        sec["--security-opt=no-new-privileges:true"]
        ro["--read-only"]
        mem["--memory 8g"]
        cpu["--cpus 4"]
        pids["--pids-limit 512"]
        net["--network (dedicated bridge)"]
        rm["--rm (auto-cleanup)"]
    end

    subgraph result["Container properties"]
        no_caps["No Linux capabilities"]
        no_priv["No privilege escalation"]
        ro_fs["Immutable filesystem"]
        limited["Resource-constrained"]
        isolated["Network-isolated"]
        ephemeral["Destroyed on exit"]
    end

    cap --> no_caps
    sec --> no_priv
    ro --> ro_fs
    mem --> limited
    cpu --> limited
    pids --> limited
    net --> isolated
    rm --> ephemeral
```

## Network topology

```mermaid
graph TB
    internet["Internet"]

    subgraph vm["OrbStack VM"]
        subgraph net_a["agent-claude-task-net (bridge)"]
            container_a["agent-claude-task"]
        end

        subgraph net_b["agent-codex-fix-net (bridge)"]
            container_b["agent-codex-fix"]
        end

        subgraph net_isolated["agent-isolated (--internal)"]
            container_c["agent-claude-untrusted"]
        end
    end

    container_a -->|"NAT via VM"| internet
    container_b -->|"NAT via VM"| internet
    container_c -.-x|"blocked"| internet

    container_a -.-x|"separate network"| container_b
    container_a -.-x|"separate network"| container_c
    container_b -.-x|"separate network"| container_c
```

Each container gets its own bridge network by default. Containers cannot communicate with each other unless explicitly placed on the same network. The `--internal` flag on a network blocks internet access entirely.
