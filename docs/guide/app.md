# Desktop App

`berth-app` is a native macOS app (Wails v3) for orchestrating berth agents when you want windows, notifications, and embedded terminals instead of a terminal multiplexer.

## What it gives you

- **live sidebar** — every agent with its state: working / needs-you / review / failed
- **embedded terminals** — xterm.js attached to each container's tmux session; sessions survive app restarts
- **diff & checkpoint review** — inspect and gate changes without leaving the app
- **spawn form and fleet view** — launch single agents or watch a whole fleet
- **timeline and cost views** — what happened, and what it cost
- **command palette** (++cmd+k++), menubar agent counts, and native notifications carrying the agent's last message

"Needs-you" detection comes from the same tmux pane classification and event classifier the CLI's `berth status` and `berth inbox` use — the app is a window onto the same state.

## Prerequisites

- `berth setup` completed and the VM running
- `berth` on PATH (all mutations shell out to the CLI)
- to build: [Wails v3 CLI](https://v3.wails.io) (`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha2.112`) and Node 22+

## Build and run

```bash
make -C app dev       # live-reload development window
make -C app build     # binary at app/bin/berth-app
make -C app bundle    # signed .app bundle with dock icon
make -C app test      # go tests + vitest
```

Troubleshooting: `wails3 doctor`. Linker warnings about macOS versions during `go build` are cosmetic.

## How it works

The Go backend polls Docker labels through the same VM executor as the CLI every 2 seconds and watches `~/.berth/state/events.jsonl`, pushing change-diffed events to the React frontend. Terminals are PTYs wrapping `docker exec -it <container> tmux attach` inside the VM — tmux keeps sessions alive across app restarts. All mutations (spawn, stop, checkpoint, …) shell out to `berth`, so the CLI, TUI, and app can be used interchangeably on the same agents.

## When to use what

| Situation | Surface |
|---|---|
| scripting, one-off commands, CI | [CLI](../reference/cli.md) |
| several live agents, terminal-native | [TUI](tui.md) |
| embedded terminals, notifications, mouse | Desktop app |
