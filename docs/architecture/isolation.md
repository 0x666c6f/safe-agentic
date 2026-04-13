# Isolation Boundaries

safe-agentic relies on three boundaries, not one.

## Boundary 1: macOS host -> OrbStack VM

Purpose:
- keep agent containers away from the host filesystem and host process space

Main controls:
- dedicated OrbStack VM
- VM hardening from `vm/setup.sh`
- blocked or overlaid macOS mount paths

## Boundary 2: OrbStack VM -> container

Purpose:
- limit what an agent can do even if it runs arbitrary commands

Main controls:
- read-only rootfs
- `cap-drop ALL`
- `no-new-privileges`
- non-root runtime user
- resource limits

## Boundary 3: container -> container

Purpose:
- stop agents from becoming one shared trust domain

Main controls:
- managed per-agent networks by default
- per-agent workspaces and transient runtime state
- no implicit shared auth unless you ask for it

## Why this matters

No single layer is perfect. The design assumes defense in depth.
