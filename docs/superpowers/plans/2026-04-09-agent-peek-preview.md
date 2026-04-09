# Agent Peek & TUI Preview Pane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users see what agents are doing without attaching — via `agent peek` (CLI) and a toggleable preview pane in the TUI.

**Architecture:** Both features call `tmux capture-pane` via `docker exec` through the OrbStack VM. The CLI prints and exits; the TUI refreshes alongside its existing 2s poll cycle. A new `PreviewPane` widget in the TUI toggles a bottom split with `p`.

**Tech Stack:** Bash (CLI), Go + tview (TUI)

---

### Task 1: CLI `agent peek` command

**Files:**
- Modify: `bin/agent` (add `cmd_peek`, `print_help_peek`, dispatch entry, general help)

- [ ] **Step 1: Add `print_help_peek` function**

In `bin/agent`, after the `print_help_attach` function (around line 168), add:

```bash
print_help_peek() {
  cat <<EOF
Usage: agent peek <name>|--latest [--lines N]

Show the last N lines (default 30) of a running agent's tmux pane.

Examples:
  agent peek api-refactor
  agent peek --latest
  agent peek --latest --lines 50
EOF
}
```

- [ ] **Step 2: Add `cmd_peek` function**

Before the `cmd="${1:-help}"` dispatch line (near end of file, before `cmd_aws_refresh`), add:

```bash
cmd_peek() {
  local topic="peek"
  local name=""
  local latest=false
  local lines=30

  while [ $# -gt 0 ]; do
    case "$1" in
      --latest)
        latest=true
        shift
        ;;
      --lines)
        require_option_value "$topic" "--lines" "${2:-}"
        lines="$2"
        shift 2
        ;;
      -h|--help)
        print_help_peek
        return 0
        ;;
      *)
        [ -z "$name" ] || die_with_help "$topic" "Unexpected argument '$1'."
        name="$1"
        shift
        ;;
    esac
  done

  $latest && [ -n "$name" ] && die "Use a name or --latest, not both."

  require_vm
  name=$(resolve_target_container "$topic" "$name" "$latest")

  local state
  state=$(vm_exec docker inspect --format '{{.State.Status}}' "$name" 2>/dev/null || echo "unknown")
  [ "$state" = "running" ] || die "Container $name is not running (state: $state)."

  local terminal_mode
  terminal_mode=$(trim_whitespace "$(vm_exec docker inspect --format '{{index .Config.Labels "safe-agentic.terminal"}}' "$name" 2>/dev/null || echo "")")
  [ "$terminal_mode" = "tmux" ] || die "No tmux session in $name."

  vm_exec docker exec "$name" tmux capture-pane -t "${SAFE_AGENTIC_TMUX_SESSION_NAME:-safe-agentic}" -p -S "-${lines}"
}
```

- [ ] **Step 3: Add dispatch entry and help topic**

In the main dispatch `case` block (near end of file), add alongside the other commands:

```bash
  peek)       cmd_peek "$@" ;;
```

In `cmd_help`, add a case:

```bash
    peek)
      print_help_peek
      ;;
```

- [ ] **Step 4: Add `peek` to general help text**

In `print_help_general`, in the "Manage" section, add after the `agent cp` line:

```
  agent peek <name>|--latest [--lines N]
```

Also add `peek` to the help topics line:

```
  agent help [spawn|shell|attach|peek|cp|stop|cleanup|update|vm|diagnose]
```

- [ ] **Step 5: Run tests**

Run: `bash tests/run-all.sh`
Expected: All 17 suites pass (peek doesn't break existing dispatch).

- [ ] **Step 6: Add CLI dispatch tests for peek**

In `tests/test-cli-dispatch.sh`, after the `help cp topic` test block (around line 150), add:

```bash
run_ok "help peek topic" bash "$REPO_DIR/bin/agent" help peek
assert_output_contains "Usage: agent peek" "peek help topic"
assert_output_contains "--lines" "peek help shows lines flag"
assert_output_contains "--latest" "peek help shows latest"
```

- [ ] **Step 7: Run tests again**

Run: `bash tests/run-all.sh`
Expected: All 17 suites pass with new peek tests.

- [ ] **Step 8: Commit**

```bash
git add bin/agent tests/test-cli-dispatch.sh
git commit -m "feat: add agent peek command for tmux pane capture"
```

---

### Task 2: TUI preview pane widget

**Files:**
- Create: `tui/preview.go`

- [ ] **Step 1: Create `tui/preview.go`**

```go
package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const defaultPreviewLines = 30

// PreviewPane shows a live capture of an agent's tmux pane.
type PreviewPane struct {
	textView  *tview.TextView
	visible   bool
	agentName string
	lines     int
}

// NewPreviewPane creates a hidden preview pane.
func NewPreviewPane() *PreviewPane {
	tv := tview.NewTextView().
		SetScrollable(true).
		SetDynamicColors(false).
		SetWrap(true)
	tv.SetBorder(true).
		SetBorderColor(colorBorder).
		SetBackgroundColor(tcell.ColorDefault)

	return &PreviewPane{
		textView: tv,
		lines:    defaultPreviewLines,
	}
}

// Toggle switches the preview pane on or off.
func (p *PreviewPane) Toggle() {
	p.visible = !p.visible
	if !p.visible {
		p.agentName = ""
		p.textView.SetText("")
		p.textView.SetTitle("")
	}
}

// Visible returns whether the preview pane is showing.
func (p *PreviewPane) Visible() bool {
	return p.visible
}

// AgentName returns the name of the agent being previewed.
func (p *PreviewPane) AgentName() string {
	return p.agentName
}

// Lines returns the number of lines to capture.
func (p *PreviewPane) Lines() int {
	return p.lines
}

// Update sets the preview content for a given agent.
func (p *PreviewPane) Update(name string, content string) {
	p.agentName = name
	p.textView.SetTitle(fmt.Sprintf(" Preview: %s (p to close) ", name))
	p.textView.SetTitleColor(colorTitle)
	// Strip trailing blank lines for cleaner display
	content = strings.TrimRight(content, "\n ")
	p.textView.SetText(content)
	p.textView.ScrollToEnd()
}

// SetUnavailable shows a reason why preview isn't available.
func (p *PreviewPane) SetUnavailable(name string, reason string) {
	p.agentName = name
	p.textView.SetTitle(fmt.Sprintf(" Preview: %s (p to close) ", name))
	p.textView.SetTitleColor(colorTitle)
	p.textView.SetText(reason)
}

// Primitive returns the underlying tview primitive.
func (p *PreviewPane) Primitive() tview.Primitive {
	return p.textView
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd tui && go build ./...`
Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add tui/preview.go
git commit -m "feat(tui): add PreviewPane widget"
```

---

### Task 3: Wire preview pane into TUI app

**Files:**
- Modify: `tui/app.go`
- Modify: `tui/footer.go`

- [ ] **Step 1: Add preview field and capture callback to App**

In `tui/app.go`, add the `preview` field to the `App` struct:

```go
type App struct {
	tapp      *tview.Application
	pages     *tview.Pages
	header    *Header
	table     *AgentTable
	footer    *Footer
	preview   *PreviewPane
	poller    *Poller
	actions   *Actions
	loaded    chan struct{}
	execAfter []string
}
```

In `NewApp()`, initialize the preview after the footer:

```go
a.preview = NewPreviewPane()
```

- [ ] **Step 2: Add layout rebuild method**

In `tui/app.go`, add the `rebuildLayout` method after `spinLoading`:

```go
func (a *App) rebuildLayout() {
	mainLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header.Primitive(), 1, 0, false)

	if a.preview.Visible() {
		mainLayout.AddItem(a.table.Primitive(), 0, 3, true)
		mainLayout.AddItem(a.preview.Primitive(), 0, 2, false)
	} else {
		mainLayout.AddItem(a.table.Primitive(), 0, 1, true)
	}

	mainLayout.AddItem(a.footer.Primitive(), shortcutRows, 0, false)

	a.pages.RemovePage("main")
	a.pages.AddPage("main", mainLayout, true, true)
	a.tapp.SetFocus(a.table.Table())
}
```

- [ ] **Step 3: Add `p` keybinding**

In the `handleInput` method, in the `case tcell.KeyRune` switch, add after the `'n'` case:

```go
		case 'p':
			a.preview.Toggle()
			if a.preview.Visible() {
				// Trigger immediate preview capture
				if agent := a.table.SelectedAgent(); agent != nil {
					a.updatePreview(agent)
				}
			}
			a.rebuildLayout()
			return nil
```

Add the `updatePreview` method to `tui/app.go`:

```go
func (a *App) updatePreview(agent *Agent) {
	if !agent.Running {
		a.preview.SetUnavailable(agent.Name, "Agent not running")
		return
	}
	content, err := capturePreview(agent.Name, a.preview.Lines())
	if err != nil {
		a.preview.SetUnavailable(agent.Name, err.Error())
		return
	}
	a.preview.Update(agent.Name, content)
}
```

- [ ] **Step 4: Add `capturePreview` function**

Add to `tui/app.go` (or could be in `actions.go` but app.go is cleaner since it's used by the poller callback):

```go
func capturePreview(name string, lines int) (string, error) {
	if !containerUsesTmux(name) {
		return "", fmt.Errorf("No tmux session")
	}
	out, err := execOrb("docker", "exec", name, "tmux", "capture-pane",
		"-t", tmuxSessionName, "-p", "-S", fmt.Sprintf("-%d", lines))
	if err != nil {
		return "", fmt.Errorf("Capture failed")
	}
	return string(out), nil
}
```

- [ ] **Step 5: Add `"fmt"` import to `app.go` if not present**

Check the imports in `app.go`. Add `"fmt"` to the import block if missing. Currently `app.go` does not import `fmt`, so add it:

```go
import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)
```

- [ ] **Step 6: Update poller callback to refresh preview**

In `NewApp()`, update the `onUpdate` callback to also refresh the preview:

```go
a.poller = NewPoller(func(agents []Agent, stale bool) {
	a.tapp.QueueUpdateDraw(func() {
		a.table.Update(agents)
		a.header.Update(a.table.RunningCount(), a.table.TotalCount(), stale)
		if a.preview.Visible() {
			if agent := a.table.SelectedAgent(); agent != nil {
				a.updatePreview(agent)
			}
		}
	})
	select {
	case <-a.loaded:
	default:
		close(a.loaded)
	}
})
```

- [ ] **Step 7: Add `p` shortcut to footer**

In `tui/footer.go`, add to the `allShortcuts` slice after the `{"n", "New"}` entry:

```go
{"p", "Preview"},
```

- [ ] **Step 8: Verify it compiles and runs**

Run: `cd tui && go build -o agent-tui .`
Expected: Compiles without errors.

Run: `./agent-tui` (manual check — press `p`, verify pane appears/hides)

- [ ] **Step 9: Commit**

```bash
git add tui/app.go tui/footer.go
git commit -m "feat(tui): wire preview pane with p toggle and live refresh"
```

---

### Task 4: Update docs and skills

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `docs/usage.md`
- Modify: `.claude/skills/agent-manage/SKILL.md`
- Modify: `.codex/skills/agent-manage/SKILL.md`

- [ ] **Step 1: Update CLAUDE.md commands section**

In `CLAUDE.md`, in the "Commands" section, add after the `agent sessions` lines:

```bash
# Peek at agent output without attaching
agent peek <container>                 # last 30 lines of tmux pane
agent peek --latest --lines 50         # more lines
```

- [ ] **Step 2: Update README.md manage section**

In `README.md`, in the "Manage agents" code block, add after the `agent sessions` line:

```bash
agent peek <name>            # Show last 30 lines of agent's tmux pane
agent peek --latest --lines 50
```

- [ ] **Step 3: Update docs/usage.md**

In `docs/usage.md`, in the "Commands at a glance" table, add after `agent sessions`:

```
| `agent peek <name>` | Show last 30 lines of agent's tmux pane output |
```

After the "Session export" section, add a new section:

```markdown
## Peek at agent output

See what an agent is doing without attaching:

\```bash
agent peek api-refactor           # last 30 lines
agent peek --latest --lines 50    # more lines from the latest container
\```

Only works on running containers with tmux sessions (Claude/Codex agents). For shell containers or stopped agents, use `agent attach` instead.
```

- [ ] **Step 4: Update agent-manage skills**

In `.claude/skills/agent-manage/SKILL.md` and `.codex/skills/agent-manage/SKILL.md`, add after the "Export session history" section:

```markdown
### Peek at agent output

\```bash
agent peek <name>              # last 30 lines of tmux pane
agent peek --latest            # latest container
agent peek <name> --lines 50   # more lines
\```

Shows what the agent is currently doing without attaching. Only works on running tmux containers.
```

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md README.md docs/usage.md .claude/skills/agent-manage/SKILL.md .codex/skills/agent-manage/SKILL.md
git commit -m "docs: add agent peek and TUI preview pane documentation"
```

---

### Task 5: Final verification

- [ ] **Step 1: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All 17 suites pass.

- [ ] **Step 2: Build TUI**

Run: `cd tui && go build -o agent-tui .`
Expected: Compiles without errors.

- [ ] **Step 3: Rebuild image** (not strictly required — peek is host-side only)

Run: `bin/agent update --quick`
Expected: Image builds successfully (entrypoint unchanged, but ensures image is current).

- [ ] **Step 4: Manual smoke test**

1. Spawn an agent: `agent spawn claude --ssh --repo git@github.com:org/repo.git`
2. Detach: `Ctrl-b d`
3. CLI peek: `agent peek --latest` — should show agent output
4. TUI: `agent tui`, select the agent, press `p` — preview pane appears
5. Press `p` again — preview hides
6. Press `q` to exit TUI
