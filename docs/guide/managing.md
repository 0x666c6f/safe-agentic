# Monitor & Manage

Everything after spawn: watching agents, jumping in, and cleaning up.

## See what's happening

```bash
berth list                    # all containers, running and stopped
berth status --all            # live state: blocked / working / done / idle / exited
berth peek --latest           # snapshot of the live terminal (last 30 lines)
berth logs --latest           # session transcript as conversation turns
berth logs --latest -f        # stream it live
berth inbox                   # events that need attention (failures, blocked prompts)
berth timeline                # recent event/audit history across agents
berth search "error text"     # grep across agent session logs
```

Rule of thumb: `inbox` tells you *which* agent needs you, `status` tells you *why*, `peek` shows you the screen.

## Attach, steer, resume

```bash
berth attach api-refactor       # attach to tmux (restarts stopped containers)
berth attach --latest --resume  # continue the previous conversation
berth steer --latest "continue with the smallest fix"
```

- detach without stopping: `Ctrl-b d`
- `steer` sends a follow-up message without opening a terminal; if the container is stopped it restarts it first
- `config-sync <name> --restart` pushes your current host Claude settings into a running agent

## Inspect results

```bash
berth summary --latest        # one-screen overview: repo, status, elapsed, cost, last message
berth output --latest         # the agent's last message
berth output --latest --diff  # or its git diff / --files / --commits / --json
berth diff --latest           # workspace diff (also works on stopped containers)
```

The deeper review loop (review passes, checkpoints, PRs) lives in [Review & Ship](workflow.md).

## Cost and audit

```bash
berth cost --latest           # estimated API spend from token usage
berth cost --history 7d       # spawn activity + spend over a window
berth audit                   # host operation log (spawns, stops, cleanups)
berth audit --lines 100
```

The audit log is append-only JSONL under `~/.berth/state/`.

## Export sessions

```bash
berth sessions api-refactor                   # raw session files → ./agent-sessions/<name>
berth sessions --latest ~/tmp/agent-sessions
berth replay --latest                         # replay the event-log timeline
berth replay --latest --tools-only
```

Push files *into* a container when needed:

```bash
container machine run -n berth -u root -- docker cp ./report.txt api-refactor:/workspace/tmp/report.txt
```

## Stop and clean up

```bash
berth stop api-refactor       # stop + remove one agent (network and DinD sidecar too)
berth stop --all
berth cleanup                 # remove all agents + managed networks (keeps auth)
berth cleanup --auth          # also remove shared and isolated auth volumes
```

Container lifecycle:

```text
spawn -> running -> stopped -> attach -> running
spawn -> running -> stop -> removed
```

Containers are intentionally persistent until you stop or clean them up.

## Operator habits

- `peek` before `attach` if you only need status
- `diff` and `output` before creating a PR
- `cleanup --auth` after high-risk work or shared-machine sessions
- prefer the [TUI](tui.md) or [desktop app](app.md) when juggling more than a couple of agents
