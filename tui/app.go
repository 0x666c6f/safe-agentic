package main

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// App is the top-level TUI application.
type App struct {
	tapp      *tview.Application
	pages     *tview.Pages
	header    *Header
	table     *AgentTable
	footer    *Footer
	preview   *PreviewPane
	poller    *Poller
	actions   *Actions
	loaded    chan struct{} // closed after first successful poll
	execAfter []string     // if set, syscall.Exec this command after tview exits
}

// NewApp creates and wires up the full TUI.
func NewApp() *App {
	a := &App{
		tapp:   tview.NewApplication(),
		pages:  tview.NewPages(),
		header: NewHeader(),
		table:  NewAgentTable(),
		footer: NewFooter(),
		loaded: make(chan struct{}),
	}

	a.preview = NewPreviewPane()

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
		// Signal first poll done
		select {
		case <-a.loaded:
		default:
			close(a.loaded)
		}
	})

	a.actions = NewActions(a)

	// Show loading state until first poll
	a.header.ShowLoading()
	a.table.ShowLoading()

	// Main layout: header (1 row) + table (flex) + footer (shortcutRows)
	mainLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header.Primitive(), 1, 0, false).
		AddItem(a.table.Primitive(), 0, 1, true).
		AddItem(a.footer.Primitive(), shortcutRows, 0, false)

	a.pages.AddPage("main", mainLayout, true, true)

	a.tapp.SetInputCapture(a.handleInput)

	return a
}

// Run starts the poller and the TUI event loop.
func (a *App) Run() error {
	a.poller.Start()
	go a.spinLoading()
	return a.tapp.SetRoot(a.pages, true).EnableMouse(false).Run()
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

func (a *App) spinLoading() {
	i := 0
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.loaded:
			return
		case <-ticker.C:
			frame := spinnerFrames[i%len(spinnerFrames)]
			a.tapp.QueueUpdateDraw(func() {
				a.table.SetLoadingFrame(frame)
			})
			i++
		}
	}
}

func (a *App) handleInput(event *tcell.EventKey) *tcell.EventKey {
	// In filter/command mode, let the input field handle keys
	if a.footer.Mode() == FooterModeFilter || a.footer.Mode() == FooterModeCommand {
		if event.Key() == tcell.KeyEscape {
			a.footer.Reset()
			a.tapp.SetFocus(a.table.Table())
			return nil
		}
		return event
	}

	// In confirm mode, handle y/n
	if a.footer.Mode() == FooterModeConfirm {
		if event.Key() == tcell.KeyEscape {
			a.footer.Reset()
			return nil
		}
		if event.Key() == tcell.KeyRune {
			if a.footer.HandleConfirmKey(event.Rune()) {
				return nil
			}
		}
		return nil
	}

	// In status mode, any key resets
	if a.footer.Mode() == FooterModeStatus {
		a.footer.Reset()
		return event
	}

	// Overlay pages: Esc closes them
	if name, _ := a.pages.GetFrontPage(); name != "main" {
		if event.Key() == tcell.KeyEscape {
			a.pages.SwitchToPage("main")
			a.pages.RemovePage(name)
			a.tapp.SetFocus(a.table.Table())
			return nil
		}
		return event
	}

	// Global keybindings
	switch event.Key() {
	case tcell.KeyCtrlC:
		a.tapp.Stop()
		return nil
	case tcell.KeyCtrlD:
		a.actions.StopAgent()
		return nil
	case tcell.KeyCtrlK:
		a.actions.KillAll()
		return nil
	case tcell.KeyEnter:
		a.actions.Attach()
		return nil
	}

	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case 'q':
			a.tapp.Stop()
			return nil
		case 'a':
			a.actions.Attach()
			return nil
		case 'r':
			a.actions.Resume()
			return nil
		case 's':
			a.actions.StopAgent()
			return nil
		case 'l':
			a.actions.Logs()
			return nil
		case 'd':
			a.actions.Describe()
			return nil
		case 'y':
			a.actions.YAMLView()
			return nil
		case 'e':
			a.actions.ExportSessions()
			return nil
		case 'c':
			a.actions.CopyFiles()
			return nil
		case 'n':
			a.actions.SpawnNew()
			return nil
		case 'p':
			a.preview.Toggle()
			if a.preview.Visible() {
				if agent := a.table.SelectedAgent(); agent != nil {
					a.updatePreview(agent)
				}
			}
			a.rebuildLayout()
			return nil
		case 'f':
			a.actions.Diff()
			return nil
		case 'm':
			a.actions.McpLogin()
			return nil
		case '/':
			a.footer.ShowFilter(func(text string) {
				a.table.SetFilter(text)
				a.tapp.SetFocus(a.table.Table())
			})
			a.tapp.SetFocus(a.footer.InputField())
			return nil
		case ':':
			a.footer.ShowCommand(func(cmd string) {
				a.handleCommand(cmd)
				a.tapp.SetFocus(a.table.Table())
			})
			a.tapp.SetFocus(a.footer.InputField())
			return nil
		case 'j':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		case '1', '2', '3', '4', '5', '6', '7', '8', '9':
			col := int(event.Rune() - '1')
			if col < len(columns) {
				a.table.SetSort(col)
			}
			return nil
		}
	}

	return event
}

func (a *App) handleCommand(cmd string) {
	switch cmd {
	case "q", "quit":
		a.tapp.Stop()
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

	mainLayout.AddItem(a.footer.Primitive(), shortcutRows, 0, false)

	a.pages.RemovePage("main")
	a.pages.AddPage("main", mainLayout, true, true)
	a.tapp.SetFocus(a.table.Table())
}

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
