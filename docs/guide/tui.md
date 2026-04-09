# Terminal UI (TUI)

A k9s-style interactive dashboard for monitoring and managing all your agents.

```bash
agent tui
```

## Layout

```text
┌───────────────────────────────────────────────────────────────────────┐
│  safe-agentic        ctx: safe-agentic VM    ⏱ 2s    agents: 2/3    │
├───────────────────────────────────────────────────────────────────────┤
│ NAME              TYPE   REPO         SSH  STATUS    ACTIVITY CPU MEM│
│ ► agent-claude-r  claude org/repo     on   Up 2h     Working  12% 1G│
│   agent-codex-o   codex  org/other    off  Up 30m    Idle      8% 0G│
│   agent-shell-x   shell  -            off  Exited    Stopped   -   - │
├───────────────────────────────────────────────────────────────────────┤
│ [a]ttach [r]esume [s]top [l]ogs [f]Diff [R]eview [t]odos [x]Chkpt  │
│ [g]PR [$]Cost [A]udit [n]ew [p]review [e]xport [c]opy [q]uit      │
│ [/] Filter  [:] Command  [ctrl-k] Kill All                          │
└───────────────────────────────────────────────────────────────────────┘
```

Three zones:

- **Header** — app title, VM context, refresh interval, running/total agent count
- **Table** — sortable columns, arrow keys to navigate, selected row highlighted
- **Footer** — shortcut hints (replaced by filter/command/confirm during input modes)

## Keybindings

### Agent lifecycle

| Key | Action | Description |
|-----|--------|-------------|
| `a` / `Enter` | Attach | Open the agent's tmux session (restarts stopped containers) |
| `r` | Resume | Reconnect to agent, or restart with CLI resume command |
| `s` | Stop | Confirm, then stop and remove the container |
| `Ctrl-d` | Delete | Same as stop |
| `Ctrl-k` | Kill all | Confirm, then stop all containers |
| `n` | New | Open spawn form to create a new agent |

### Observability

| Key | Action | Description |
|-----|--------|-------------|
| `l` | Logs | Show session log in scrollable overlay |
| `d` | Describe | Show `docker inspect` in formatted overlay |
| `y` | YAML | Show raw `docker inspect` JSON |
| `p` | Preview | Toggle live tmux pane preview panel |
| `f` | Diff | Show `git diff` from agent's working tree |
| `R` | Review | Run AI code review (codex review or git diff) |

### Workflow

| Key | Action | Description |
|-----|--------|-------------|
| `t` | Todos | Show agent's todo list |
| `x` | Checkpoint | Create a working tree snapshot |
| `g` | PR | Create a GitHub PR from agent's branch |

### Analytics

| Key | Action | Description |
|-----|--------|-------------|
| `$` | Cost | Estimate API spend from session token data |
| `A` | Audit | Show the operation audit log |

### Data

| Key | Action | Description |
|-----|--------|-------------|
| `e` | Export | Export session history to host |
| `c` | Copy | Open form to copy files from container |
| `m` | MCP login | Run MCP OAuth login interactively |

### Navigation

| Key | Action | Description |
|-----|--------|-------------|
| `j` / `↓` | Down | Move selection down |
| `k` / `↑` | Up | Move selection up |
| `1`-`9` | Sort | Sort by column N (toggles ascending/descending) |
| `/` | Filter | Filter agents by substring match on any field |
| `:` | Command | Open command bar |
| `Esc` | Back | Close overlay, filter, command, or modal |
| `q` / `Ctrl-c` | Quit | Exit the TUI |

## Command bar

Press `:` to open the command bar. Available commands:

| Command | Description |
|---------|-------------|
| `:q` / `:quit` | Exit |
| `:fleet <file>` | Spawn agents from a YAML manifest |
| `:pipeline <file>` | Run a multi-step agent pipeline |
| `:audit` | Show the audit log |

## Preview pane

Press `p` to toggle a split view showing the last 30 lines of the selected agent's tmux output. Updates on each poll cycle (every 2 seconds). Only available for running Claude/Codex containers.

## Spawn form

Press `n` to open the spawn form:

- **Type** — Claude or Codex
- **Repo URL** — optional, auto-converts HTTPS GitHub URLs to SSH when SSH is enabled
- **Name** — optional human-readable name
- **Prompt** — optional initial task
- **SSH** — enable SSH forwarding (default: on)
- **Reuse auth** — persist OAuth tokens (default: on)
- **Reuse GH auth** — persist GitHub CLI auth
- **AWS profile** — inject AWS credentials
- **Docker** — enable Docker-in-Docker
- **Identity** — git author attribution

The agent spawns in the background. Press `r` to connect once it's ready.

## Activity detection

The TUI probes each running agent's CPU usage by sampling `/proc/<pid>/stat` twice with a 1-second gap. This shows:

- **Working** — agent process consumed CPU ticks (actively processing)
- **Idle** — agent process is alive but not consuming CPU
- **Stopped** — container is not running

## Live stats

Polled every 2 seconds via `docker stats`:

- **CPU** — percentage of allocated CPUs
- **MEM** — current memory usage / limit
- **NET I/O** — network bytes in/out
- **PIDs** — process count inside container

## Columns

The table shows up to 14 columns. When the terminal is too narrow, lowest-priority columns are hidden automatically (NET I/O, PIDs, Docker, GH-AUTH drop first).

| Column | Description |
|--------|-------------|
| NAME | Container name |
| TYPE | claude / codex / shell |
| REPO | Repository display label |
| SSH | on / off |
| AUTH | shared / ephemeral |
| GH-AUTH | shared / ephemeral |
| DOCKER | off / dind / host-socket |
| NETWORK | managed / custom / none |
| STATUS | Docker status (Up 2h, Exited, etc.) |
| ACTIVITY | Working / Idle / Stopped |
| CPU | CPU usage percentage |
| MEM | Memory usage |
| NET I/O | Network bytes |
| PIDS | Process count |

## Building

```bash
make -C tui build     # build the binary
make -C tui install   # build and copy to bin/
```

Requires Go 1.22+. No CGO. Single binary output.
