# Agent TUI Design Spec

A k9s-style terminal UI for monitoring and managing safe-agentic containers.

## Overview

`agent tui` launches an interactive terminal dashboard showing all agent containers with live stats. Users navigate with keyboard shortcuts matching k9s conventions. Built in Go with tview/tcell, living in `tui/` as a self-contained module.

## Architecture

```
bin/agent (bash) ──"agent tui"──> tui/main.go (Go binary)
                                      │
                                      ├── Poller goroutine (every 2s)
                                      │     └── orb run -m safe-agentic docker ps --format json
                                      │     └── orb run -m safe-agentic docker stats --no-stream --format json
                                      │
                                      ├── tview Application
                                      │     ├── Header (Flex)
                                      │     ├── Table (main view)
                                      │     ├── Overlay pages (describe, logs, JSON, spawn form)
                                      │     └── Footer (shortcuts + command/filter bar)
                                      │
                                      └── Action handlers
                                            └── os/exec → orb run -m safe-agentic docker ...
```

Single-goroutine poller. All Docker interaction through `orb run`. Interactive actions suspend the TUI.

## Data Model

```go
type Agent struct {
    Name        string // container name
    Type        string // "claude" | "codex" | "shell"
    Repo        string // repo display label from label
    SSH         string // "on" | "off"
    Auth        string // auth label value
    GHAuth      string // gh-auth label value
    Docker      string // docker access label
    NetworkMode string // "managed: ..." | "custom: ..."
    Status      string // "Up 2h" | "Exited (0) 5m ago"
    Running     bool   // derived from Status
    CPU         string // "12.5%"
    Memory      string // "1.2GiB / 8GiB"
    NetIO       string // "1.5MB / 300KB"
    PIDs        string // "45"
}
```

## Layout

```
┌─────────────────────────────────────────────────────────────────────┐
│  safe-agentic        ctx: safe-agentic VM    ⏱ 2s    agents: 2/3  │
├─────────────────────────────────────────────────────────────────────┤
│ NAME              TYPE   REPO         SSH  STATUS    CPU  MEM  PIDs│
│ ► agent-claude-r  claude org/repo     on   Up 2h     12%  1.2G  45│
│   agent-codex-o   codex  org/other    off  Up 30m     8%  800M  32│
│   agent-shell-x   shell  -            off  Exited     -    -     - │
├─────────────────────────────────────────────────────────────────────┤
│ <a>ttach <s>top <l>ogs <d>escribe <n>ew <e>xport </> filter       │
│ <y>aml <c>opy <m>cp-login <ctrl-d>elete <ctrl-k>ill-all <q>uit    │
└─────────────────────────────────────────────────────────────────────┘
```

Three zones:
- **Header:** App title, VM context, refresh interval, running/total count.
- **Table:** Sortable by any column. Arrow keys navigate. Selected row highlighted. Columns: Name, Type, Repo, SSH, Auth, GH-Auth, Docker, Network, Status, CPU%, Mem, Net I/O, PIDs. Columns that don't fit the terminal width get truncated right-to-left (PIDs, Net I/O, Docker, GH-Auth drop first).
- **Footer:** Two-line shortcut hints. Replaced by filter input (`/`), command input (`:`), or confirmation prompt during those modes.

## Keybindings

| Key | Action | Behavior |
|-----|--------|----------|
| `Enter` / `a` | Attach | Suspend TUI, exec into container (bash -l for running, start -ai for exited) |
| `s` | Stop | Confirm dialog, then stop + rm + network cleanup |
| `Ctrl-d` | Delete | Same as stop |
| `l` | Logs | Suspend TUI, `docker logs -f` |
| `d` | Describe | Overlay pane with formatted `docker inspect` |
| `y` | YAML/JSON | Overlay pane with raw `docker inspect` JSON |
| `e` | Export sessions | Run `agent sessions`, show result in status bar |
| `c` | Copy | Modal: prompt container path + host path, run `agent cp` |
| `n` | New agent | Modal form: type, repo URL, flags. Runs `agent spawn` |
| `m` | MCP login | Suspend TUI, run `agent mcp-login` interactively |
| `f` | Diff | Overlay pane with git diff from agent's working tree |
| `t` | Todos | Overlay pane with agent's todo list |
| `x` | Checkpoint | Create a working tree snapshot (git stash ref) |
| `g` | PR | Create a GitHub PR from agent's branch (with confirmation) |
| `r` | Resume | Reconnect to agent TTY or restart with CLI resume |
| `p` | Preview | Toggle live tmux pane preview panel |
| `Ctrl-k` | Kill all | Confirm dialog, stop all containers |
| `/` | Filter | Inline text input, filters rows by substring match on any column |
| `:` | Command | Command bar. `:q` quit, `:volumes` switch view, `:networks` switch view |
| `Esc` | Back | Close overlay/filter/command/modal, return to table |
| `q` / `Ctrl-c` | Quit | Exit |
| `↑`/`↓`/`j`/`k` | Navigate | Move selection in table |
| `1`-`9` | Sort | Sort by column N |

## Polling

Single goroutine, 2-second interval:

1. `orb run -m safe-agentic docker ps -a --filter "name=^agent-" --format '{{json .}}'`
2. `orb run -m safe-agentic docker stats --no-stream --filter "name=^agent-" --format '{{json .}}'`
3. Parse JSON lines, merge by container name
4. `tview.QueueUpdateDraw()` to push to UI

Each `orb run` has a 5-second exec timeout. On timeout, previous data is kept (stale indicator in header). On success, stale flag clears.

## Actions Implementation

**Suspend pattern** (for attach, logs, mcp-login):
1. Stop poller
2. `app.Suspend(func() { exec.Command(...).Run() })`
3. Resume poller on return

**Background actions** (stop, export, copy):
1. Show "working..." in status bar
2. Run in goroutine
3. `QueueUpdateDraw` on completion with result message

**Confirm dialog** (stop, delete, kill-all):
Footer replaced with `"Stop agent-claude-repo? [y/n]"`. `y` executes, `n`/`Esc` cancels.

**Modal forms** (new agent, copy):
Centered tview.Form overlay. Fields validated before submission.

## Secondary Views

`:volumes` — Lists Docker volumes with `safe-agentic` labels. Columns: Name, Type, Parent, Size.
`:networks` — Lists managed networks. Columns: Name, Bridge, Container.
`:agents` — Default view (the main table).

These are stretch goals. The main agent table is the MVP.

## File Structure

```
tui/
├── go.mod
├── go.sum
├── main.go          # CLI flags, build binary entry point
├── app.go           # tview.Application setup, layout composition, input routing
├── model.go         # Agent struct, sorting, filtering
├── poller.go        # Polling goroutine, orb exec, JSON parsing, merge
├── table.go         # Table view: render, update, column definitions
├── header.go        # Header bar rendering
├── footer.go        # Footer shortcuts, filter bar, command bar, confirm bar
├── actions.go       # All action handlers (attach, stop, logs, describe, etc.)
├── overlay.go       # Describe/YAML overlay pane, modal forms
└── theme.go         # Colors, styles matching k9s aesthetic
```

## Integration with `bin/agent`

The bash CLI gets a new dispatch case:

```bash
tui) exec "$REPO_DIR/tui/agent-tui" "$@" ;;
```

The Go binary is built separately (`go build -o agent-tui ./tui/`). It's not checked in — users build it or a Makefile target produces it.

A `Makefile` in `tui/` handles:
```makefile
build:
    go build -o agent-tui .
install:
    go build -o ../bin/agent-tui .
```

## Theme

k9s-inspired dark theme:
- Header: bold cyan on dark background
- Table header: bold white
- Selected row: reverse video (white on blue)
- Running status: green
- Exited status: red/yellow
- Shortcuts: cyan keys, white descriptions
- Confirm prompts: yellow/red

## Error Handling

- VM not running: Show error screen with "Run 'agent setup' first", exit cleanly.
- `orb` not found: Same pattern — error screen + exit.
- Poll failures: Show stale indicator in header, keep last good data, retry next cycle.
- Action failures: Show error in status bar (red), don't crash.

## Build Requirements

- Go 1.22+
- Dependencies: `github.com/rivo/tview`, `github.com/gdamore/tcell/v2`
- No CGO required
- Single binary output
