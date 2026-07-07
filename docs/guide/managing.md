# Managing Agents

This page is about container lifecycle after a session has been spawned.

## List and inspect

```bash
berth list
berth status --all
berth peek --latest
berth logs --latest
berth search "error text"
berth summary --latest
berth output --latest
berth diff --latest
berth review-comments list --latest
berth timeline
berth inbox
```

Use:
- `list` for all containers
- `status` for the live state of an agent (blocked / working / done / idle / exited)
- `peek` for quick live output
- `logs` for the session log
- `search` for finding prior output across agent session logs
- `summary` for a compact state snapshot
- `output` for the last useful result
- `diff` for the workspace diff
- `review-comments` for saved local file/line notes
- `timeline` for recent event/audit history
- `inbox` for failures or status markers that need attention (including agents blocked on a prompt)

## Attach and resume

```bash
berth attach api-refactor
berth attach --latest
berth steer --latest "continue with the smallest fix"
```

Behavior:
- if the container is running, this attaches to tmux
- if it is stopped, berth restarts it first
- detach without stopping: `Ctrl-b d`
- use `steer` to send a follow-up message without opening a terminal

## Stop and remove

```bash
berth stop api-refactor
berth stop --latest
berth stop --all
```

`stop` removes:
- the agent container
- its managed Docker network
- its DinD sidecar, if one exists

## Cleanup

```bash
berth cleanup
berth cleanup --auth
```

Difference:
- `cleanup`: remove containers, managed networks, and transient runtime state
- `cleanup --auth`: also remove shared and isolated auth volumes

## Export sessions

Push files into a container:

```bash
container machine run -n berth -u root -- docker cp ./report.txt api-refactor:/workspace/tmp/report.txt
container machine run -n berth -u root -- docker cp ./dist/. api-refactor:/workspace/dist
```

Export session history:

```bash
berth sessions api-refactor
berth sessions --latest ~/tmp/agent-sessions
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
berth tui
```

Use these when you want:
- all agents in one view
- live stats
- keyboard-driven attach/stop/log/review flows

## Good operator habits

- use `peek` before `attach` if you only need status
- use `diff` and `output` before creating a PR
- use `cleanup --auth` after high-risk work or shared-machine sessions
