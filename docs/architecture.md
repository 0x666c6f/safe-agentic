# Architecture

safe-agentic runs Go-hosted orchestration on macOS, then executes agents inside Docker containers inside an OrbStack VM.

## System overview

```mermaid
graph TB
    subgraph mac["macOS host"]
        user["User"]
        cli["safe-ag / safe-ag-tui"]
        orb["OrbStack"]
        ssh["1Password SSH agent"]
    end

    subgraph vm["OrbStack VM: safe-agentic"]
        setup["vm/setup.sh hardening"]
        dockerd["Docker daemon with userns-remap"]

        subgraph net["Per-agent bridge network"]
            container["Agent container"]
            entry["entrypoint.sh"]
            tmux["agent-session.sh / tmux"]
            workspace["/workspace"]
        end
    end

    user --> cli
    cli -->|"orb run -m safe-agentic"| dockerd
    cli --> setup
    ssh -.->|"only with --ssh"| container
    dockerd --> container
    container --> entry --> tmux --> workspace
    orb --> vm
```

## Boundaries

1. macOS host -> OrbStack VM
2. OrbStack VM -> Docker container
3. Container -> container via separate networks, volumes, namespaces

## Main components

| Path | Role |
|---|---|
| `cmd/safe-ag` | Go CLI commands |
| `tui/` | Go TUI + dashboard |
| `pkg/docker` | Runtime, network, volume, DinD helpers |
| `pkg/config` | Defaults + identity parsing |
| `vm/setup.sh` | VM hardening + Docker install |
| `entrypoint.sh` | Container boot logic |
| `bin/agent-session.sh` | tmux session launcher |
| `bin/repo-url.sh` | repo URL validation |

## Security posture

- Read-only rootfs
- `cap-drop ALL`
- `no-new-privileges`
- non-root `agent` user
- per-agent bridge network
- auth reuse only by explicit flag
- SSH forwarding only by explicit flag
