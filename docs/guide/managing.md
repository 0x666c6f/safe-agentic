# Managing Agents

This page is about container lifecycle after a session has been spawned.

## List and inspect

```bash
safe-ag list
safe-ag peek --latest
safe-ag logs --latest
safe-ag summary --latest
safe-ag output --latest
safe-ag diff --latest
```

Use:
- `list` for all containers
- `peek` for quick live output
- `logs` for the session log
- `summary` for a compact state snapshot
- `output` for the last useful result
- `diff` for the workspace diff

## Attach and resume

```bash
safe-ag attach api-refactor
safe-ag attach --latest
```

Behavior:
- if the container is running, this attaches to tmux
- if it is stopped, safe-agentic restarts it first
- detach without stopping: `Ctrl-b d`

## Stop and remove

```bash
safe-ag stop api-refactor
safe-ag stop --latest
safe-ag stop --all
```

`stop` removes:
- the agent container
- its managed Docker network
- its DinD sidecar, if one exists

## Cleanup

```bash
safe-ag cleanup
safe-ag cleanup --auth
```

Difference:
- `cleanup`: remove containers, managed networks, and transient runtime state
- `cleanup --auth`: also remove shared auth volumes

## Export sessions

Push files into a container:

```bash
orb run -m safe-agentic docker cp ./report.txt api-refactor:/workspace/tmp/report.txt
orb run -m safe-agentic docker cp ./dist/. api-refactor:/workspace/dist
```

Export session history:

```bash
safe-ag sessions api-refactor
safe-ag sessions --latest ~/tmp/agent-sessions
```

## Container lifecycle

```text
spawn -> running -> stopped -> attach -> running
spawn -> running -> stop -> removed
stopped -> stop -> removed
```

Containers are intentionally persistent until you stop or clean them up.

## TUI

```bash
safe-ag tui
```

Use these when you want:
- all agents in one view
- live stats
- keyboard-driven attach/stop/log/review flows

## Good operator habits

- use `peek` before `attach` if you only need status
- use `diff` and `output` before creating a PR
- use `cleanup --auth` after high-risk work or shared-machine sessions
