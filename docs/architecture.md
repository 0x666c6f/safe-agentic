# Architecture

berth is a host-side sandbox runner.

At a high level:

```text
macOS host
  -> Apple container machine
    -> Docker daemon
      -> one container per agent
```

## Component map

| Component | Role |
|---|---|
| `berth` | CLI entrypoint |
| `berth-tui` | TUI entrypoint |
| `vm/setup.sh` | VM bootstrap and hardening |
| `entrypoint.sh` | container boot and repo clone flow |
| `cmd/berth` | command implementations |
| `pkg/docker` | Docker runtime, network, volume, SSH helpers |
| `pkg/fleet` | fleet and pipeline manifest parsing |
| `pkg/vmexec` | Apple container machine execution wrapper |

## Design goals

- keep agent autonomy inside the sandbox
- make dangerous capabilities explicit flags, not ambient defaults
- keep the operational model simple enough for daily use

## Read next

- [Isolation Boundaries](architecture/isolation.md)
- [Networking](architecture/networking.md)
- [Container Internals](architecture/container.md)
