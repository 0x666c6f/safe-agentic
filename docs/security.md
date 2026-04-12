# Security Model

> See [architecture.md](architecture.md) for full system diagrams, sequence flows, and component maps.

## Three isolation boundaries

```mermaid
graph TB
    subgraph B1["Boundary 1 — OrbStack VM"]
        direction LR
        b1a["macOS mounts blocked"]
        b1b["Integration cmds removed"]
        b1c["fstab persistence"]
        b1d["Docker userns-remap"]
    end
    subgraph B2["Boundary 2 — Docker Container"]
        direction LR
        b2a["Read-only rootfs"]
        b2b["cap-drop ALL"]
        b2c["no-new-privileges"]
        b2d["Resource limits"]
        b2e["Non-root, no sudo"]
    end
    subgraph B3["Boundary 3 — Container Isolation"]
        direction LR
        b3a["Separate networks"]
        b3b["Separate volumes"]
        b3c["No shared state"]
    end

    B1 --> B2 --> B3

    style B1 fill:#fee,stroke:#c33
    style B2 fill:#ffd,stroke:#c93
    style B3 fill:#dfd,stroke:#393
```

## Defaults vs opt-in

| Feature | Default (safe) | Opt-in (wider surface) |
|---------|---------------|----------------------|
| SSH agent | OFF — no access to your keys | `--ssh` forwards 1Password agent |
| Auth tokens | Ephemeral — discarded on exit | `--reuse-auth` persists across sessions |
| GitHub CLI auth | Ephemeral — discarded on exit | `--reuse-gh-auth` persists across sessions |
| AWS credentials | OFF — no access to AWS | `--aws <profile>` injects from `~/.aws/credentials` (tmpfs-backed) |
| Docker access | OFF | `--docker` starts DinD; `--docker-socket` mounts the VM daemon |
| Rootfs | Read-only | — |
| Capabilities | ALL dropped + no-new-privileges | — |
| Network | Dedicated bridge per container; private/local egress blocked; TCP 22/80/443 only | `--network <name>` joins existing and bypasses managed guardrails |
| Resources | 8g memory, 4 CPUs, 512 PIDs | `--memory`, `--cpus`, `--pids-limit` |
| Host keys | GitHub baked + StrictHostKeyChecking | — |
| Sudo | Removed | — |

## What each flag exposes

### `--ssh`

Forwards your 1Password SSH agent socket into the container via a socat relay in the VM:

```mermaid
sequenceDiagram
    participant Agent as Agent (container)
    participant Relay as socat relay (VM)
    participant VM as OrbStack VM
    participant Mac as macOS
    participant 1P as 1Password
    participant GH as GitHub

    Agent->>Relay: git push via relayed socket (:ro)
    Note over Relay: Bridges userns-remap<br/>UID permissions
    Relay->>VM: OrbStack SSH socket
    VM->>Mac: OrbStack forwarding
    Mac->>1P: Sign challenge
    Note over 1P: Private key never<br/>leaves 1Password
    1P-->>Mac: Signature
    Mac-->>VM: Response
    VM-->>Relay: Response
    Relay-->>Agent: Authenticated
    Agent->>GH: Push (host key verified)
```

The socat relay is needed because Docker userns-remap maps container UIDs to unprivileged VM UIDs that cannot read the OrbStack SSH socket directly. The relay listens on a world-accessible socket and forwards to the real OrbStack socket.

The agent can:
- Clone and push to any repo your SSH key has access to
- Authenticate to any SSH host your key works with

It **cannot** read your private key (1Password agent never exposes it).

**When to use:** Private repos (`git@` URLs).
**Risk:** A compromised agent could push malicious code to repos you have write access to.

### `--reuse-auth`

Stores the OAuth token in a named Docker volume that survives container restarts.

**When to use:** Avoid re-authenticating every session.
**Risk:** A compromised container could steal the token from the shared volume. Run `safe-ag cleanup --auth` to revoke.

### `--reuse-gh-auth`

Stores GitHub CLI auth in a named Docker volume (`agent-gh-auth`) that survives container restarts.

**When to use:** You want `gh auth login` once and reuse it across sessions.
**Risk:** A compromised container could steal the GitHub token from the shared volume. Run `safe-ag cleanup --auth` to revoke.

### `--aws <profile>`

Injects AWS credentials from the host's `~/.aws/credentials` file into the container. Credentials are written to a tmpfs mount at `~/.aws/credentials` and `AWS_PROFILE` is set. Use `safe-ag aws-refresh` to update expired credentials without restarting.

**When to use:** The agent needs AWS access (terraform, aws-cli, boto3).
**Risk:** A compromised container could use the injected credentials for the session duration. Assumed-role sessions expire (~1 hour), limiting the window. Credentials live on tmpfs and are not persisted to disk.

### `--docker`

Starts a dedicated privileged Docker-in-Docker sidecar for the session and points the agent container at its socket.

**When to use:** The agent needs `docker build`, `docker run`, or Compose, but you do not want to hand it the VM daemon directly.
**Risk:** Wider container attack surface than the default session. The sidecar is still isolated to the session and removed on exit.

### `--docker-socket`

Mounts `/var/run/docker.sock` from the hardened VM directly into the agent container.

**When to use:** You explicitly want the agent to control the VM Docker daemon.
**Risk:** Broadest Docker access. The agent can inspect, stop, or replace other containers in the VM.

### `--network <name>`

Joins an existing Docker network instead of creating a dedicated one.

**When to use:** Multiple containers that need to communicate, or `--network agent-isolated` for air-gapped operation.
**Risk:** Containers on the same network can reach each other, and custom networks bypass the managed egress policy.

## Managed-network egress guardrails

Safe-agentic-managed bridges now get a VM firewall policy:

- outbound TCP only on `22`, `80`, `443`
- no access to local/private address ranges (`127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, etc.)
- no access to OrbStack/macOS mount paths when hardening is healthy

This keeps default safe-ag sessions usable for Git, package downloads, and Claude/Codex traffic while blocking the most dangerous east-west and local-host pivots. If you need broader network access, you must opt into a custom network explicitly.

## Supply chain hardening

All binaries installed in the Docker image are verified:

| Source | Verification |
|--------|-------------|
| Direct downloads (Go, Helm, eza, zoxide, yq, delta) | SHA256 checksum pinned per-architecture |
| AWS CLI | GPG signature verified against embedded public key |
| Apt packages (Node.js, Terraform, kubectl, gh, etc.) | Signed apt repositories with pinned GPG keys |
| AI CLIs (Claude Code, Codex) | npm lockfile pinned (`npm ci`) |

No `curl | bash` install patterns are used.

## Container filesystem layout

```mermaid
graph TD
    subgraph ro["Read-only rootfs (immutable)"]
        usr["/usr — system binaries"]
        opt["/opt/agent-cli — claude, codex"]
        ssh_baked[".ssh.baked — GitHub host keys"]
    end
    subgraph tmpfs["tmpfs (RAM — lost on exit)"]
        tmp["/tmp — 512m, noexec"]
        config[".config — git config, 32m"]
        ssh[".ssh — keys/config, 1m"]
        local[".local — 64m"]
    end
    subgraph vols["Docker volumes (ephemeral by default)"]
        workspace["/workspace — cloned repos"]
        auth[".claude / .codex — OAuth token"]
        caches[".npm, .cache/pip, go, .terraform.d, .docker"]
    end

    style ro fill:#f5f5f5,stroke:#999
    style tmpfs fill:#fff3e0,stroke:#e65100
    style vols fill:#e3f2fd,stroke:#1565c0
```

All writable areas are either tmpfs (discarded on exit) or anonymous Docker volumes (discarded on `safe-ag cleanup`). Named volumes from `--reuse-auth` and `--reuse-gh-auth` persist until `safe-ag cleanup --auth`.

## Git identity

Containers default to `Agent <agent@localhost>`. The host git identity is not copied in automatically.

If you need explicit attribution, export `GIT_AUTHOR_NAME` / `GIT_AUTHOR_EMAIL` (and optionally `GIT_COMMITTER_*`) before launching the container.

## VM hardening details

`vm/setup.sh` applies these protections every time the VM starts:

1. **Unmounts macOS paths** — `/Users`, `/mnt/mac`, `/Volumes`, `/private`, `/opt/orbstack`
2. **Overlays read-only tmpfs** — even if OrbStack re-mounts, the tmpfs hides the content
3. **Adds fstab entries** — persist blocking across VM reboots
4. **Removes OrbStack integration commands** — `open`, `osascript`, `code`, `mac`
5. **Masks OrbStack integration directories** — tmpfs over `/opt/orbstack-guest/`
6. **Verifies hardening** — checks that mounts are blocked and commands are gone
7. **Enables Docker userns-remap** — container UIDs are remapped to unprivileged host UIDs
8. **Installs socat** — required for the SSH agent relay (bridges userns-remap UID permissions) and MCP port bridging

### Known limitation

OrbStack may restore macOS mounts when the VM restarts. Always use `safe-ag vm start` (which re-applies hardening) instead of `orb start` directly.
