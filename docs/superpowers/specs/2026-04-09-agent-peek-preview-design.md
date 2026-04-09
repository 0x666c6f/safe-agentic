# Agent Peek & TUI Preview Pane

## Problem

Once an agent is detached (`Ctrl-b d`), there's no way to see what it's doing without fully attaching. With 3-5 parallel agents, constantly attaching/detaching to check progress is tedious. The TUI shows "Working"/"Idle" based on CPU ticks, but not *what* the agent is working on.

## Solution

Two complementary features that share the same underlying mechanism (`tmux capture-pane`):

1. **`agent peek`** — CLI command that dumps the last N lines of a running agent's tmux pane and exits
2. **TUI preview pane** — toggleable bottom split that shows the selected agent's live tmux output

## Underlying Mechanism

Both features use the same command:

```bash
orb run -m safe-agentic docker exec <name> tmux capture-pane -t safe-agentic -p -S -<lines>
```

This captures the visible and scrollback content of the agent's tmux pane as plain text. No new infrastructure needed — tmux already stores the pane content.

## CLI: `agent peek`

### Usage

```bash
agent peek <name>|--latest         # last 30 lines (default)
agent peek <name> --lines 50       # configurable line count
```

### Implementation

New `cmd_peek` function in `bin/agent` (~30 lines):

1. Parse args: container name (or `--latest`), optional `--lines N` (default 30)
2. Resolve container via `resolve_target_container` (reuses existing helper)
3. Validate container is running (`docker inspect` state check)
4. Validate container has tmux terminal mode (`safe-agentic.terminal=tmux` label)
5. Run `vm_exec docker exec <name> tmux capture-pane -t safe-agentic -p -S -<lines>`
6. Print captured output to stdout
7. Exit

### Error cases

- Container not found: "No running agent container matches '<name>'."
- Container not running: "Container <name> is not running."
- No tmux session: "No tmux session in <name>." (container is a shell or tmux exited)

### CLI dispatch

Add to the case statement in `bin/agent`:
- `peek` -> `cmd_peek`

Add to help: `agent peek <name>|--latest [--lines N]`

Add `peek` to `cmd_help` dispatch.

## TUI: Preview Pane

### Interaction

- Press `p` to toggle a bottom split showing the selected agent's tmux pane output
- Press `p` again to hide it
- Content refreshes every poll cycle (2-3 seconds)
- When table selection changes, preview updates on the next poll cycle
- For non-running or non-tmux containers: shows "Agent not running" or "No tmux session"

### Layout

When preview is visible:

```
+------------------------------------------+
| Header: 2 running / 3 total              |  1 row
+------------------------------------------+
| NAME    TYPE  REPO     STATUS   ACTIVITY  |
| agent-1 claude org/r1  Up 5m    Working   |  ~60% height
| agent-2 claude org/r2  Up 3m    Idle      |
+------------------------------------------+
| Preview: agent-1 (p to close)            |
| > Reading src/main.go...                  |  ~40% height
| > I'll fix the authentication bug in...  |
| > [tool use: Edit src/auth.go]           |
+------------------------------------------+
| <a> Attach  <r> Resume  <p> Preview ...  |  3 rows
+------------------------------------------+
```

The preview pane takes approximately 40% of the terminal height. The split uses `tview.Flex` with proportional sizing.

### Implementation

**New file: `tui/preview.go`**

`PreviewPane` struct:
- `textView *tview.TextView` — scrollable text view for captured output
- `visible bool` — toggle state
- `agentName string` — currently previewed agent
- `lines int` — number of lines to capture (default 30)

Methods:
- `Toggle()` — show/hide the preview pane
- `Update(name string, content string)` — update the displayed content
- `SetUnavailable(name string, reason string)` — show "not running" etc.
- `Visible() bool` — check if preview is shown
- `Primitive() tview.Primitive` — return the text view

**Changes to `app.go`:**
- Add `preview *PreviewPane` field to `App`
- Modify main layout: when preview is visible, insert preview pane between table and footer
- Add `p` keybinding to `handleInput` that calls `togglePreview()`
- `togglePreview()`: rebuilds the main layout flex with or without the preview pane

**Changes to `poller.go`:**
- Add `previewName string` and `previewLines int` fields to track what to capture
- When preview is active, capture the tmux pane alongside the regular poll
- Pass preview content through the update callback (new field or separate callback)
- Use `execOrb("docker", "exec", name, "tmux", "capture-pane", "-t", tmuxSessionName, "-p", "-S", fmt.Sprintf("-%d", lines))` — reuses existing `execOrb` helper

**Changes to `footer.go`:**
- Add `{"p", "Preview"}` to `allShortcuts` slice

### Preview capture logic

```go
func capturePreview(name string, lines int) (string, error) {
    if name == "" {
        return "", fmt.Errorf("no agent selected")
    }
    // Check if container uses tmux
    if !containerUsesTmux(name) {
        return "", fmt.Errorf("no tmux session")
    }
    out, err := execOrb("docker", "exec", name, "tmux", "capture-pane",
        "-t", tmuxSessionName, "-p", "-S", fmt.Sprintf("-%d", lines))
    if err != nil {
        return "", err
    }
    return string(out), nil
}
```

This function is called during the poll cycle when preview is visible. It reuses `containerUsesTmux()` (already exists in `actions.go`) and `execOrb()` (already exists in `poller.go`).

## Testing

### CLI tests

Add to `tests/test-cli-dispatch.sh`:
- `agent peek --help` returns usage
- `agent peek` without args shows error

### Manual testing

- Spawn an agent, detach, run `agent peek --latest` — should show agent output
- Open TUI, press `p` — preview pane appears with selected agent's output
- Switch selection — preview updates on next poll
- Press `p` again — preview hides
- Select a stopped agent — preview shows "Agent not running"

## Files Changed

| File | Change |
|------|--------|
| `bin/agent` | Add `cmd_peek`, `print_help_peek`, dispatch entry |
| `tui/preview.go` | New file: `PreviewPane` struct and methods |
| `tui/app.go` | Add preview field, `p` keybinding, layout rebuild |
| `tui/poller.go` | Capture tmux pane when preview is active |
| `tui/footer.go` | Add `p` → `Preview` shortcut |
| `tests/test-cli-dispatch.sh` | Add peek dispatch test |
