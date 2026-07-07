# TUI Reference

This page is the exhaustive reference for the terminal UI.

## Entry points

Terminal UI:

```bash
berth tui
```

Standalone binary help:

```bash
berth-tui --help
```

Current binary usage:

```text
Usage: berth-tui
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
| `STATE` | agent state: `blocked`/`working`/`done`/`idle`/`exited` (see below) |
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

### STATE column

The `STATE` column is the same `agentstate` detection used by `berth status`.
The poller captures each running tmux agent's pane once per cycle and classifies
it; stopped containers resolve to `done` (clean exit) or `exited` (non-zero).

Colors: `blocked` is bold red (needs a human), `working` cyan, `done` green,
`idle` dim, `exited` orange-red.

The **default sort is a priority sort on STATE** — "which agent needs me first":
`blocked` > `done` > `exited` > `working` > `idle`, stable within each state group
and within fleet/pipeline groups. Pressing a number key switches to a normal
column sort; press `9` to sort by STATE explicitly (toggles ascending/descending).

### State-change notifications

On macOS, when a poll observes an agent transitioning **into** `blocked`,
`done`, or `exited`, the TUI fires a native desktop notification (attention sound
for blocked/exited, success chime for done). Notifications are debounced per
container: the first observation of each agent seeds its state silently, so
neither startup nor a freshly spawned agent produces a burst, and a state only
notifies once per transition.

Set `BERTH_TUI_NOTIFY=off` (or `0`/`false`/`no`) to disable. Notifications are
a no-op on non-macOS hosts.

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
| `f` | diff (plain; press `s` inside for side-by-side) |
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
| `fleet <file>` | run `berth fleet <file>` |
| `pipeline <file>` | run `berth pipeline <file>` |
| `profile <name> [prompt]` | run a saved agent profile |
| `action <name>` | run a configured action in the selected agent |
| `comments` | list saved review comments for the selected agent |
| `timeline` | open recent events |
| `inbox` | open events that may need attention |
| `audit` | open the audit view |

Unknown commands produce a footer error status.

## Diff view

`f` opens the selected agent's `git diff` in a plain, scrollable overlay. tview
overlays render plain text only, so delta's truecolor side-by-side output cannot
display cleanly in the bordered pane. Pressing `s` inside the diff overlay
suspends the TUI and renders `berth diff --side-by-side` (delta, run inside the
container) through a pager in the restored terminal, where its ANSI colors show
correctly; on quit the TUI resumes.

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
| `Type` | dropdown | `claude`, `codex`, or `shell` |
| `Repo URL (optional)` | text | HTTPS GitHub URLs auto-convert to SSH when SSH is enabled |
| `Name (optional)` | text | container name suffix |
| `Prompt (optional)` | text | initial task |
| `SSH` | checkbox | default follows `~/.berth/config.toml`; SSH repo URLs enable SSH automatically |
| `Reuse auth` | checkbox | default follows `~/.berth/config.toml` |
| `Reuse GH auth` | checkbox | default follows `~/.berth/config.toml` |
| `Seed host auth` | checkbox | copy host Claude/Codex auth into this session; default follows `~/.berth/config.toml` |
| `AWS profile (optional)` | text | AWS profile name |
| `Docker` | checkbox | enable DinD; default follows `~/.berth/config.toml` |
| `Docker socket` | checkbox | mount the VM Docker socket; default follows `~/.berth/config.toml` |
| `Identity (optional)` | text | git identity |

Submit behavior:
- the TUI launches `berth spawn ... --background`
- when a checkbox differs from config defaults, the TUI emits the matching `--flag` or `--no-flag`
- success message tries to extract the spawned container name from command output

## Copy form

Current fields:

| Field | Meaning |
|---|---|
| `Agent path:` | source path for pull from the container |
| `VM path (pull dest):` | pull destination path in the VM environment |
| `VM source (push):` | source path in the VM environment for push |
| `Agent path (push dest):` | destination path in the container for push |

Agent paths are normalized and must stay under `/workspace`. VM paths are normalized absolute paths inside the Apple container VM; relative paths and Docker `container:path` syntax are rejected.

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
