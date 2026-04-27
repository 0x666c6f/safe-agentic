# TUI Reference

This page is the exhaustive reference for the terminal UI.

## Entry points

Terminal UI:

```bash
safe-ag tui
```

Standalone binary help:

```bash
safe-ag-tui --help
```

Current binary usage:

```text
Usage: safe-ag-tui
```

## Interaction model

The TUI is a polling terminal UI backed by the same host-side model as the CLI.

Main pieces:
- header
- agent table
- footer
- overlays/modals
- optional preview pane

## Header

The header shows:
- app name
- target VM name
- poll interval
- running/total agent count
- stale indicator when polling data is old

## Agent table

The table is the core view.

Current columns:

| Column | Meaning |
|---|---|
| `NAME` | container name |
| `TYPE` | `claude`, `codex`, or `shell` |
| `REPO` | repo display label |
| `SSH` | SSH state |
| `AUTH` | Claude/Codex auth mode |
| `GH-AUTH` | GitHub auth mode |
| `DOCKER` | Docker mode |
| `NETWORK` | network mode |
| `STATUS` | container status |
| `ACTIVITY` | working/idle/stopped or deleting spinner |
| `CPU` | CPU usage |
| `MEM` | memory usage |
| `NET I/O` | network I/O |
| `PIDS` | process count |

Behavior:
- fleet/pipeline agents are grouped visually
- selection is restored by agent name across refreshes
- columns are hidden automatically on narrow terminals
- deleting agents get transient overlay state

## Footer modes

The footer has five modes:

| Mode | Meaning |
|---|---|
| `shortcuts` | default key hint grid |
| `filter` | free-text filter input |
| `command` | command bar |
| `confirm` | y/n confirmation prompt |
| `status` | transient status message |

## Keybindings

### Global keys

| Key | Action |
|---|---|
| `Ctrl-c` | quit |
| `Ctrl-d` | stop selected agent |
| `Ctrl-k` | stop all agents |
| `Enter` | attach to selected agent |

On macOS, `attach` and `resume` open iTerm2 by default. If iTerm2 is not installed, they fall back to Terminal.app. On other platforms, or if terminal launch fails, the TUI exits and attaches in the current terminal.

### Rune keys

| Key | Action |
|---|---|
| `q` | quit |
| `a` | attach |
| `r` | resume |
| `s` | stop selected agent |
| `l` | logs overlay |
| `d` | inspect overlay |
| `e` | export sessions |
| `c` | transfer files |
| `n` | open spawn form |
| `p` | toggle preview pane |
| `f` | diff |
| `x` | checkpoint create |
| `t` | todo list |
| `m` | MCP login |
| `g` | create PR |
| `R` | review |
| `$` | cost |
| `A` | audit |
| `?` | help overlay |
| `/` | filter mode |
| `:` | command mode |
| `j` | move down |
| `k` | move up |
| `1`-`9` | sort by column index |

### `Esc`

`Esc` does different things depending on mode:
- close overlays
- exit filter mode
- exit command mode
- cancel confirmation mode

## Command bar

Current `:` commands:

| Command | Action |
|---|---|
| `q` / `quit` | quit the TUI |
| `fleet <file>` | run `safe-ag fleet <file>` |
| `pipeline <file>` | run `safe-ag pipeline <file>` |
| `audit` | open the audit view |

Unknown commands produce a footer error status.

## Preview pane

Toggle with `p`.

Behavior:
- if the selected agent is running and uses tmux, preview tries pane capture first
- otherwise it falls back to recent logs
- if preview data is unavailable, the pane shows the reason instead

Default preview depth:
- `30` lines

## Overlays and modals

Current overlay/modal types:

| Name | Purpose |
|---|---|
| help overlay | keybinding help |
| logs overlay | session/log output |
| describe overlay | formatted container inspect |
| copy form | transfer files between container and VM |
| spawn form | spawn a new agent |

## Spawn form

Current spawn form fields:

| Field | Type | Notes |
|---|---|---|
| `Type` | dropdown | `claude` or `codex` |
| `Repo URL (optional)` | text | HTTPS GitHub URLs auto-convert to SSH when SSH is enabled |
| `Name (optional)` | text | container name suffix |
| `Prompt (optional)` | text | initial task |
| `SSH` | checkbox | default `true` in the form |
| `Reuse auth` | checkbox | default `true` in the form |
| `Reuse GH auth` | checkbox | default `false` |
| `AWS profile (optional)` | text | AWS profile name |
| `Docker` | checkbox | enable DinD |
| `Identity (optional)` | text | git identity |

Submit behavior:
- the TUI launches `safe-ag spawn ... --background`
- success message tries to extract the spawned container name from command output

## Copy form

Current fields:

| Field | Meaning |
|---|---|
| `Agent path:` | source path for pull from the container |
| `VM path (pull dest):` | pull destination path in the VM environment |
| `VM source (push):` | source path in the VM environment for push |
| `Agent path (push dest):` | destination path in the container for push |

## Table sorting and filtering

Sorting:
- `1` maps to column 1, `2` to column 2, and so on
- selecting the same column again toggles ascending/descending

Filtering:
- case-insensitive substring match
- matches across multiple fields, including repo/network/status metrics

## Activity and delete state

Special states used by the table:

| State | Meaning |
|---|---|
| `Working` | active session |
| `Idle` | running but not actively working |
| `Stopped` | container not running |
| `Deleting` | transient overlay while stop/remove is in progress |

Deleting rows also show spinner frames from the internal animation ticker.

## When to use this page

Use this page when you need exact interaction details.

Use the higher-level guides when you need:
- [Managing Agents](../guide/managing.md) for container lifecycle
- [Workflow](../guide/workflow.md) for review/retry/PR flow
- [Command Map](../usage.md) for CLI task routing
