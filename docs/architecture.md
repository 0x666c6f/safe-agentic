# Architecture

berth is a host-side sandbox runner. Nothing agent-controlled runs on macOS itself:

```text
macOS host (berth CLI / TUI / app)
  -> Apple container machine "berth" (Alpine, hardened)
    -> Docker daemon (userns-remap)
      -> one container per agent
```

Design goals:

- keep agent autonomy **inside** the sandbox — the boundary is the product
- make dangerous capabilities explicit flags, never ambient defaults
- keep the operational model simple enough for daily use

## Component map

| Component | Role |
|---|---|
| `berth` | CLI entrypoint |
| `berth-tui` | terminal dashboard |
| `berth-app` | macOS desktop app (shells out to `berth`) |
| `vm/setup.sh` | VM bootstrap and hardening |
| `entrypoint.sh` | container boot and repo clone flow |
| `cmd/berth` | command implementations |
| `pkg/vmexec` | Apple container machine execution wrapper — all VM/Docker commands go through it |
| `pkg/docker` | Docker runtime, network, volume, SSH helpers |
| `pkg/fleet` | fleet and pipeline manifest parsing |
| `pkg/agentstate` | live agent-state classification (blocked/working/done/…) |

## The three isolation boundaries

No single layer is perfect; the design assumes defense in depth.

### 1. macOS host → Apple container machine

Keeps agent containers away from the host filesystem and process space.

- dedicated Apple container machine, created with `--home-mount none` by default — **no host directory is shared with the VM at any level**
- VM hardening from `vm/setup.sh`
- blocked or overlaid macOS mount paths

The only exception is the opt-in [worktree mount](guide/worktrees.md), which trades part of this boundary for local-checkout workflows.

### 2. Apple container machine → container

Limits what an agent can do even when it runs arbitrary commands.

- read-only rootfs
- `cap-drop ALL` + `no-new-privileges`
- non-root runtime user, no sudo
- resource limits (memory / CPU / PIDs)
- Docker `userns-remap` in the VM

### 3. Container → container

Stops agents from becoming one shared trust domain.

- managed per-agent bridge networks by default
- per-agent workspaces and transient runtime state
- no implicit shared auth unless you ask for it

## Networking

Each agent gets a dedicated managed Docker bridge. VM setup pins each managed bridge interface to a `bt*` name so the `BERTH_EGRESS` iptables chain can apply default guardrails: private/link-local/reserved ranges are blocked, normal TCP egress (22/80/443) is allowed.

```bash
berth spawn claude --network my-net --repo ...   # custom topology — you own the consequences
```

For especially untrusted work, use an internal (no-internet) network:

```bash
berth vm ssh
docker network create --internal agent-isolated
exit
berth spawn claude --network agent-isolated --repo https://github.com/org/repo.git
```

### api-only egress mode

```bash
berth spawn claude --network api-only --repo https://github.com/org/repo.git
```

api-only containers attach to a managed bridge like any other agent, but the bridge interface is pinned to a distinct `bti` prefix instead of the normal `bt` + hex used for managed bridges — hex digits never include `i`, so the two prefixes can't collide. `vm/setup.sh` inserts one rule ahead of the normal guardrails: `iptables -A BERTH_EGRESS -i 'bti+' -j REJECT`, which drops **all** forwarded traffic from api-only bridges — direct internet, external DNS, everything.

The only reachable path out is a `tinyproxy` instance the VM runs on `0.0.0.0:8119`, default-deny with an exact-host allowlist (`api.anthropic.com`, `statsig.anthropic.com`, `sentry.io`, `github.com`, `codeload.github.com`, `objects.githubusercontent.com`). The container reaches it via `--add-host berth-proxy:host-gateway` plus an injected `HTTPS_PROXY=http://berth-proxy:8119`: that traffic is the container talking to the VM's own host-gateway address, which lands on the VM's `INPUT` chain, not `FORWARD` — so the `bti+` REJECT rule (which only matches forwarded traffic) never sees it. Name resolution for allowlisted hosts happens proxy-side, in the VM. The container's own DNS lookups are forwarded packets too, so mechanically the same `bti+` REJECT should drop them — the live acceptance gate (see the api-only egress mode plan, Task 7) is what actually verifies the container's default resolver path is blocked; treat that verification, not this description, as the source of truth.

`--ssh` changes **auth exposure, not topology**: it forwards an SSH agent socket into the container through a socat relay in the VM. The container gains signing/auth ability; the private key material stays in your host agent (or 1Password).

!!! note "Host NAT"
    VM egress relies on host PF NAT plus `net.inet.ip.forwarding=1`, applied during `berth setup`. A macOS reboot resets both — `berth vm start` re-applies them and `berth diagnose` flags missing egress.

## Inside an agent container

Three storage classes:

| Area | Purpose |
|---|---|
| read-only rootfs | installed tools and baked config |
| tmpfs mounts | transient writable runtime state |
| Docker volumes | workspace, caches, optional shared auth |

Startup flow:

1. container starts; entrypoint prepares runtime config (SSH, git identity, injected host config)
2. repos clone into `/workspace`
3. repo setup hooks run only when `--allow-setup-scripts` was given
4. a security preamble is injected into the agent's CLAUDE.md / AGENTS.md
5. the agent launches inside tmux

tmux is what makes the workflows practical: attach later, detach without killing the session, `peek` via pane capture, resume stopped containers.

## Related

- [Security](security.md) — the threat model behind these boundaries
- [Worktrees](guide/worktrees.md) — the one deliberate boundary trade-off
