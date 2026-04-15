# Architecture

safe-agentic is a host-side sandbox runner.

At a high level:

```text
macOS host
  -> OrbStack VM
    -> Docker daemon
      -> one container per agent
```

## Component map

| Component | Role |
|---|---|
| `safe-ag` | CLI entrypoint |
| `safe-ag-tui` | TUI entrypoint |
| `vm/setup.sh` | VM bootstrap and hardening |
| `entrypoint.sh` | container boot and repo clone flow |
| `cmd/safe-ag` | command implementations |
| `pkg/docker` | Docker runtime, network, volume, SSH helpers |
| `pkg/fleet` | fleet and pipeline manifest parsing |
| `pkg/orb` | OrbStack execution wrapper |

## Design goals

- keep agent autonomy inside the sandbox
- make dangerous capabilities explicit flags, not ambient defaults
- keep the operational model simple enough for daily use

## Read next

- [Isolation Boundaries](architecture/isolation.md)
- [Networking](architecture/networking.md)
- [Container Internals](architecture/container.md)
