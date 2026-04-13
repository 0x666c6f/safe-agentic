# Container Internals

This page explains what exists inside an agent container and what happens at startup.

## Storage model

Three storage classes matter:

| Area | Purpose |
|---|---|
| read-only rootfs | installed tools and baked config |
| tmpfs mounts | transient writable runtime state |
| Docker volumes | workspace, caches, optional shared auth |

## Startup flow

At a high level:

1. container starts
2. entrypoint prepares runtime config
3. repos clone into `/workspace`
4. optional `safe-agentic.json` setup hook runs
5. agent starts inside tmux

## Runtime traits

- the agent runs as a non-root user
- the root filesystem is read-only
- writable paths are explicit
- tmux keeps the session attachable after startup

## Why tmux is used

tmux is what makes these workflows practical:
- attach later
- detach without killing the session
- capture pane output for `peek`
- resume stopped containers in a familiar way
