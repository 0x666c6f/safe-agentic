package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/0x666c6f/safe-agentic/app/internal/cli"
	"github.com/0x666c6f/safe-agentic/app/internal/emit"
	"github.com/0x666c6f/safe-agentic/app/internal/poll"
	"github.com/0x666c6f/safe-agentic/app/internal/state"
	"github.com/0x666c6f/safe-agentic/app/internal/svc"
	"github.com/0x666c6f/safe-agentic/app/internal/term"
	"github.com/0x666c6f/safe-agentic/app/internal/watch"
	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/events"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
//
//go:embed all:frontend/dist
var assets embed.FS

// wailsEmitter late-binds the app so services can be constructed first.
type wailsEmitter struct {
	mu  sync.RWMutex
	app *application.App
}

func (w *wailsEmitter) set(app *application.App) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.app = app
}

func (w *wailsEmitter) Emit(name string, data any) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.app != nil {
		w.app.Event.Emit(name, data)
	}
}

var _ emit.Emitter = (*wailsEmitter)(nil)

func trayLabel(agents []poll.Agent, needsYou map[string]bool) string {
	working, needs, idle := 0, 0, 0
	for _, a := range agents {
		if !a.Running {
			continue
		}
		switch {
		case needsYou[a.Name] || a.State == "blocked":
			needs++
		case a.Activity == "Working":
			working++
		default:
			idle++
		}
	}
	return fmt.Sprintf("🟢%d 🟡%d ⚪%d", working, needs, idle)
}

func vmName() string {
	if v := os.Getenv("SAFE_AGENTIC_VM_NAME"); v != "" {
		return v
	}
	return "safe-agentic"
}

func main() {
	em := &wailsEmitter{}
	exec := &vmexec.MachineExecutor{VMName: vmName()}

	poller := poll.NewPoller(exec, em, 2*time.Second)
	runner := cli.NewRunner()
	runner.OnCommand = func(argv []string) { log.Printf("exec: %v", argv) }
	termMgr := term.NewManager(em, term.DefaultFactory(vmName()))
	stateSvc := state.NewService()
	watcher := watch.NewWatcher(config.EventsPath(), em, 5*time.Second)

	agentSvc := &svc.AgentService{Runner: runner, Poller: poller}
	termSvc := &svc.TerminalService{Manager: termMgr}

	app := application.New(application.Options{
		Name: "safe-ag-app",
		Services: []application.Service{
			application.NewService(agentSvc),
			application.NewService(termSvc),
			application.NewService(stateSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	em.set(app)

	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "safe-ag",
		Width:  1400,
		Height: 900,
		URL:    "/",
	})

	// Systray: counts + per-agent focus items.
	var needsMu sync.Mutex
	needsYou := map[string]bool{}
	tray := app.SystemTray.New()
	rebuild := func(agents []poll.Agent) {
		needsMu.Lock()
		defer needsMu.Unlock()
		for name := range needsYou { // drop stopped agents
			found := false
			for _, a := range agents {
				if a.Name == name && a.Running {
					found = true
				}
			}
			if !found {
				delete(needsYou, name)
			}
		}
		tray.SetLabel(trayLabel(agents, needsYou))
		menu := application.NewMenu()
		for _, a := range agents {
			if !a.Running {
				continue
			}
			name := a.Name
			menu.Add(fmt.Sprintf("%s — %s", name, a.Activity)).OnClick(func(*application.Context) {
				win.Show()
				win.Focus()
				em.Emit("focus.agent", name)
			})
		}
		menu.AddSeparator()
		menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
		tray.SetMenu(menu)
	}
	poller.OnSnapshot = rebuild
	app.Event.On("event.new", func(e *application.CustomEvent) {
		// Mark needs-you from watcher events; next poller snapshot redraws the
		// tray. Go-side listeners receive the ORIGINAL value emitted by the
		// watcher: map[string]any{"event": events.Event, "status": string}.
		if m, ok := e.Data.(map[string]any); ok {
			if st, _ := m["status"].(string); st == "needs-auth" || st == "stuck" || st == "blocked" {
				if ev, ok := m["event"].(events.Event); ok {
					if c := ev.Payload["container"]; c != "" {
						needsMu.Lock()
						needsYou[c] = true
						needsMu.Unlock()
					}
				}
			}
		}
	})

	poller.Start()
	if err := watcher.Start(); err != nil {
		log.Printf("watcher: %v", err)
	}
	app.OnShutdown(func() {
		poller.Stop()
		watcher.Stop()
		termMgr.CloseAll()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
