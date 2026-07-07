# berth-app

macOS desktop app for orchestrating berth agents: live sidebar with
agent states (working / needs-you / review / failed), embedded xterm.js
terminals attached to each container's tmux session, diff/checkpoint review
workflow, spawn form, fleet view, timeline, cost view, command palette (⌘K),
menubar counts, and native notifications carrying the agent's last message.

Design: `docs/superpowers/specs/2026-07-03-berth-app-design.md`
Plan: `docs/superpowers/plans/2026-07-04-berth-app.md`

## Prerequisites

- berth set up (`berth setup`) with the VM running.
- `berth` on PATH (mutations shell out to it).
- Wails v3 CLI, pinned: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha2.112`
- Node 22+ (frontend build).

## Develop

```bash
make -C app dev      # wails3 dev — live-reload window
make -C app test     # go tests + vitest
make -C app build    # binary at app/bin/berth-app
```

Troubleshooting: run `wails3 doctor`. Linker warnings about macOS versions
during `go build` are cosmetic.

## Architecture (short)

Go backend polls Docker labels through `pkg/vmexec` every 2s and watches
`~/.berth/state/events.jsonl`; pushes change-diffed Wails events to the
React frontend. All mutations shell out to the `berth` CLI. Terminals are
PTYs wrapping `container machine run … docker exec -it <c> tmux attach`;
tmux keeps sessions alive across app restarts. Needs-you detection comes
from `pkg/agentstate` (tmux pane classification) plus the events classifier.
