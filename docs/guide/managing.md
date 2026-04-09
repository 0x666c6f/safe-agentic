# Managing Agents

## List

```bash
agent list
```

Shows all containers (running + stopped) with name, type, repo, auth, network, and status.

## Attach

```bash
agent attach api-refactor
agent attach --latest
```

Reattaches to the agent's tmux session. If the container is stopped, it restarts first. Detach without stopping: `Ctrl-b d`.

## Peek

```bash
agent peek api-refactor           # last 30 lines
agent peek --latest --lines 50    # more context
```

See what an agent is doing without attaching. Works on running tmux containers only.

## Copy files out

```bash
agent cp api-refactor /workspace/tmp/test.log ./test.log
agent cp --latest /workspace/dist ./dist
```

Extract files from a container without bind mounts.

## Stop

```bash
agent stop api-refactor      # one agent
agent stop --latest          # newest
agent stop --all             # everything
```

Removes the container, its network, and any DinD sidecar.

## Cleanup

```bash
agent cleanup          # stop all, keep auth volumes
agent cleanup --auth   # also remove auth volumes
```

## Export sessions

```bash
agent sessions api-refactor
agent sessions --latest ~/my-sessions/
```

Copies conversation history from the container to host. Works on running and stopped containers.

## Container lifecycle

```mermaid
graph LR
    spawn["agent spawn"] --> running["Running"]
    running -->|"agent exits"| stopped["Stopped"]
    running -->|"agent attach"| running
    stopped -->|"agent attach"| running
    running -->|"agent stop"| gone["Removed"]
    stopped -->|"agent stop"| gone

    style spawn fill:#e3f2fd,stroke:#1565c0
    style running fill:#dfd,stroke:#393
    style stopped fill:#fff3e0,stroke:#e65100
    style gone fill:#f5f5f5,stroke:#999
```

Containers persist after exit. Reattach to resume, or stop to remove.

## Interactive TUI

```bash
agent tui
```

k9s-style terminal dashboard with live stats (CPU, MEM, PIDs), activity detection, and keybindings for all operations:

| Key | Action | Key | Action |
|-----|--------|-----|--------|
| `a` | Attach | `f` | Diff |
| `r` | Resume | `R` | Review |
| `s` | Stop | `t` | Todos |
| `l` | Logs | `x` | Checkpoint |
| `d` | Describe | `g` | Create PR |
| `n` | New agent | `$` | Cost |
| `p` | Preview | `A` | Audit |
| `e` | Export | `/` | Filter |
| `c` | Copy | `q` | Quit |
