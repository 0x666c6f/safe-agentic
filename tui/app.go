package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App is the top-level TUI application.
type App struct {
	tapp          *tview.Application
	pages         *tview.Pages
	header        *Header
	table         *AgentTable
	footer        *Footer
	preview       *PreviewPane
	poller        *Poller
	actions       *Actions
	loaded        chan struct{} // closed after first successful poll
	stopAnim      chan struct{}
	execAfter     []string // if set, syscall.Exec this command after tview exits
	pendingMu     sync.Mutex
	pendingAgents []Agent
	pendingStale  bool
	pendingUpdate bool
}

// NewApp creates and wires up the full TUI.
func NewApp() *App {
	a := &App{
		tapp:     tview.NewApplication(),
		pages:    tview.NewPages(),
		header:   NewHeader(),
		table:    NewAgentTable(),
		footer:   NewFooter(),
		loaded:   make(chan struct{}),
		stopAnim: make(chan struct{}),
	}

	a.preview = NewPreviewPane()

	a.poller = NewPoller(func(agents []Agent, stale bool) {
		a.pendingMu.Lock()
		a.pendingAgents = make([]Agent, len(agents))
		copy(a.pendingAgents, agents)
		a.pendingStale = stale
		a.pendingUpdate = true
		a.pendingMu.Unlock()
		// Signal first poll done
		select {
		case <-a.loaded:
		default:
			close(a.loaded)
		}
	})

	a.actions = NewActions(a)
	a.table.Table().SetSelectionChangedFunc(func(row, col int) {
		if !a.preview.Visible() {
			return
		}
		if agent := a.table.SelectedAgent(); agent != nil {
			a.updatePreview(agent)
		}
	})

	// Show loading state until first poll
	a.header.ShowLoading()
	a.table.ShowLoading()

	// Main layout: header (1 row) + table (flex) + footer (dynamic rows)
	mainLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header.Primitive(), 1, 0, false).
		AddItem(a.table.Primitive(), 0, 1, true).
		AddItem(a.footer.Primitive(), a.footer.Rows(), 0, false)

	a.pages.AddPage("main", mainLayout, true, true)

	a.tapp.SetInputCapture(a.handleInput)

	return a
}

// Run starts the poller and the TUI event loop.
func (a *App) Run() error {
	go func() {
		time.Sleep(500 * time.Millisecond)
		a.poller.Start()
	}()
	defer a.poller.Stop()
	go a.spinAnimations()
	err := a.tapp.SetRoot(a.pages, true).EnableMouse(false).Run()
	close(a.stopAnim)
	return err
}

// SuspendAndRun suspends the TUI, runs fn with the terminal, then resumes.
// The poller keeps running in the background — its queued draws are processed on resume.
func (a *App) SuspendAndRun(fn func()) {
	a.tapp.Suspend(fn)
}

// ExecAfterExit stops the TUI and schedules a command to be exec'd
// after tview fully restores the terminal. Used for TUI apps (claude/codex)
// that need a direct TTY connection.
func (a *App) ExecAfterExit(args []string) {
	a.execAfter = args
	a.tapp.Stop()
}

// ExecAfterArgs returns the command scheduled for exec-after-exit, if any.
func (a *App) ExecAfterArgs() []string {
	return a.execAfter
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (a *App) spinAnimations() {
	i := 0
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopAnim:
			return
		case <-ticker.C:
			frame := spinnerFrames[i%len(spinnerFrames)]
			a.tapp.QueueUpdateDraw(func() {
				a.flushPendingUpdates()
				select {
				case <-a.loaded:
				default:
					a.table.SetLoadingFrame(frame)
				}
				a.table.SetDeletingFrame(frame)
			})
			i++
		}
	}
}

func (a *App) flushPendingUpdates() {
	a.pendingMu.Lock()
	if !a.pendingUpdate {
		a.pendingMu.Unlock()
		return
	}
	agents := make([]Agent, len(a.pendingAgents))
	copy(agents, a.pendingAgents)
	stale := a.pendingStale
	a.pendingUpdate = false
	a.pendingMu.Unlock()

	a.table.Update(agents)
	a.header.Update(a.table.RunningCount(), a.table.TotalCount(), stale)
	if a.preview.Visible() {
		if agent := a.table.SelectedAgent(); agent != nil {
			a.updatePreview(agent)
		}
	}
}

func (a *App) handleInput(event *tcell.EventKey) *tcell.EventKey {
	if handled, next := a.handleFooterModeInput(event); handled {
		return next
	}
	if handled, next := a.handleOverlayInput(event); handled {
		return next
	}
	if handled, next := a.handleGlobalInput(event); handled {
		return next
	}
	if event.Key() == tcell.KeyRune {
		if handled, next := a.handleRuneInput(event.Rune()); handled {
			return next
		}
	}
	return event
}

func (a *App) handleFooterModeInput(event *tcell.EventKey) (bool, *tcell.EventKey) {
	switch a.footer.Mode() {
	case FooterModeFilter, FooterModeCommand:
		if event.Key() == tcell.KeyEscape {
			a.footer.Reset()
			a.tapp.SetFocus(a.table.Table())
			return true, nil
		}
		return true, event
	case FooterModeConfirm:
		if event.Key() == tcell.KeyEscape {
			a.footer.Reset()
			return true, nil
		}
		if event.Key() == tcell.KeyRune && a.footer.HandleConfirmKey(event.Rune()) {
			return true, nil
		}
		return true, nil
	case FooterModeStatus:
		a.footer.Reset()
		return true, event
	default:
		return false, nil
	}
}

func (a *App) handleOverlayInput(event *tcell.EventKey) (bool, *tcell.EventKey) {
	name, _ := a.pages.GetFrontPage()
	if name == "main" {
		return false, nil
	}
	if event.Key() == tcell.KeyEscape {
		a.pages.SwitchToPage("main")
		a.pages.RemovePage(name)
		a.tapp.SetFocus(a.table.Table())
		return true, nil
	}
	return true, event
}

func (a *App) handleGlobalInput(event *tcell.EventKey) (bool, *tcell.EventKey) {
	switch event.Key() {
	case tcell.KeyCtrlC:
		a.tapp.Stop()
	case tcell.KeyCtrlD:
		a.actions.StopAgent()
	case tcell.KeyCtrlK:
		a.actions.KillAll()
	case tcell.KeyEnter:
		a.actions.Attach()
	default:
		return false, nil
	}
	return true, nil
}

func (a *App) handleRuneInput(r rune) (bool, *tcell.EventKey) {
	if action, ok := a.simpleRuneActions()[r]; ok {
		action()
		return true, nil
	}

	switch r {
	case 'p':
		a.togglePreview()
	case '/':
		a.startFilterInput()
	case ':':
		a.startCommandInput()
	case 'j':
		return true, tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	case 'k':
		return true, tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		a.sortByRune(r)
	default:
		return false, nil
	}
	return true, nil
}

func (a *App) simpleRuneActions() map[rune]func() {
	return map[rune]func(){
		'q': a.tapp.Stop,
		'a': a.actions.Attach,
		'r': a.actions.Resume,
		's': a.actions.StopAgent,
		'l': a.actions.Logs,
		'd': a.actions.Describe,
		'e': a.actions.ExportSessions,
		'c': a.actions.CopyFiles,
		'n': a.actions.SpawnNew,
		'f': a.actions.Diff,
		'x': a.actions.Checkpoint,
		't': a.actions.Todo,
		'm': a.actions.McpLogin,
		'g': a.actions.CreatePR,
		'R': a.actions.Review,
		'$': a.actions.Cost,
		'A': a.actions.Audit,
		'?': a.showHelpOverlay,
	}
}

func (a *App) togglePreview() {
	a.preview.Toggle()
	if a.preview.Visible() {
		if agent := a.table.SelectedAgent(); agent != nil {
			a.updatePreview(agent)
		}
	}
	a.rebuildLayout()
}

func (a *App) startFilterInput() {
	a.footer.ShowFilter(func(text string) {
		a.table.SetFilter(text)
		a.tapp.SetFocus(a.table.Table())
	})
	a.tapp.SetFocus(a.footer.InputField())
}

func (a *App) startCommandInput() {
	a.footer.ShowCommand(func(cmd string) {
		a.handleCommand(cmd)
		a.tapp.SetFocus(a.table.Table())
	})
	a.tapp.SetFocus(a.footer.InputField())
}

func (a *App) sortByRune(r rune) {
	col := int(r - '1')
	if col < len(columns) {
		a.table.SetSort(col)
	}
}

func (a *App) handleCommand(cmd string) {
	parts := strings.SplitN(cmd, " ", 2)
	verb := parts[0]
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch verb {
	case "q", "quit":
		a.tapp.Stop()
	case "fleet":
		a.actions.Fleet(arg)
	case "pipeline":
		a.actions.Pipeline(arg)
	case "pr-review":
		a.actions.PRReview(arg)
	case "pr-fix":
		a.actions.PRFix(arg)
	case "audit":
		a.actions.Audit()
	default:
		a.footer.ShowStatus("Unknown command: "+cmd, true)
	}
}

func (a *App) rebuildLayout() {
	mainLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header.Primitive(), 1, 0, false)

	if a.preview.Visible() {
		mainLayout.AddItem(a.table.Primitive(), 0, 3, true)
		mainLayout.AddItem(a.preview.Primitive(), 0, 2, false)
	} else {
		mainLayout.AddItem(a.table.Primitive(), 0, 1, true)
	}

	a.footer.showShortcuts() // recalc for current width
	mainLayout.AddItem(a.footer.Primitive(), a.footer.Rows(), 0, false)

	a.pages.RemovePage("main")
	a.pages.AddPage("main", mainLayout, true, true)
	a.tapp.SetFocus(a.table.Table())
}

func (a *App) updatePreview(agent *Agent) {
	if agent.Running && containerUsesTmux(agent.Name) {
		content, err := previewCaptureFunc(agent.Name, a.preview.Lines())
		if err == nil {
			a.preview.Update(agent.Name, content)
			return
		}
	}
	content, err := previewLogsFunc(agent.Name, a.preview.Lines())
	if err != nil {
		a.preview.SetUnavailable(agent.Name, err.Error())
		return
	}
	a.preview.Update(agent.Name, content)
}

func (a *App) showHelpOverlay() {
	content := `Keybindings

Navigation
  j / k / Up / Down   Move selection up/down
  1-9                 Sort by column (1=Name, 2=Type, etc.)
  /                   Filter agents by keyword
  :                   Command mode (quit, fleet, pipeline, pr-review, pr-fix, audit)

Actions
  Enter / a           Attach to selected agent (tmux)
  r                   Resume agent session
  n                   Spawn new agent (form)
  s / Ctrl+D          Stop selected agent
  Ctrl+K              Stop all agents

Inspect
  p                   Toggle preview pane (last output)
  l                   Logs (safe-ag logs)
  d                   Describe container (docker inspect)
  f                   Diff (safe-ag diff)
  x                   Checkpoint create
  t                   Todo list
  e                   Export sessions
  c                   Transfer files VM <-> agent
  $                   Cost estimate
  A                   Audit log
  R                   Code review (safe-ag review)
  g                   Create PR
  m                   MCP OAuth login

Other
  ?                   This help overlay
  q / Ctrl+C          Quit
  Esc                 Close overlay / reset filter`
	ShowOverlay(a, "help", "Help", content)
}

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

func captureLogsPreview(name string, lines int) (string, error) {
	out, err := execOrb("docker", "logs", "--tail", fmt.Sprintf("%d", lines), name)
	if err != nil {
		return "", fmt.Errorf("Preview unavailable")
	}
	return string(out), nil
}

var previewCaptureFunc = capturePreview
var previewLogsFunc = captureLogsPreview
