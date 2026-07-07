package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/dock"

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

// Menubar glyph: safe-agentic shield rendered as a macOS template icon
// (black + alpha; generated from docs/assets/dashboard-favicon.png).
//
//go:embed assets/trayicon-template.png
var trayIcon []byte

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

// countStates aggregates running agents into working / needs-you / idle.
func countStates(agents []poll.Agent, needsYou map[string]bool) (working, needs, idle int) {
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
	return
}

// trayHeader is the disabled first row: the aggregate "active status".
func trayHeader(agents []poll.Agent, needsYou map[string]bool) string {
	working, needs, idle := countStates(agents, needsYou)
	if working+needs+idle == 0 {
		return "No active chats"
	}
	return fmt.Sprintf("%d working · %d need you · %d idle", working, needs, idle)
}

// statusDot renders a small anti-aliased filled circle PNG for tray menu
// items — native menus can't use the frontend's CSS dots, and emoji circles
// look out of place. 14px point size = menu item image size (72 DPI PNG).
func statusDot(c color.RGBA) []byte {
	const size, radius = 14, 4.5
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var cov float64 // 4×4 supersampled coverage for smooth edges
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					dx := float64(x) + (float64(sx)+0.5)/4 - size/2
					dy := float64(y) + (float64(sy)+0.5)/4 - size/2
					if dx*dx+dy*dy <= radius*radius {
						cov++
					}
				}
			}
			a := uint8(cov / 16 * float64(c.A))
			img.SetRGBA(x, y, color.RGBA{c.R, c.G, c.B, a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil // SetBitmap(nil) is a no-op; the row just has no dot
	}
	return buf.Bytes()
}

// Tray status dots, matching the frontend StatusDot palette.
var (
	dotIdle    = statusDot(color.RGBA{115, 115, 115, 255}) // neutral-500
	dotNeeds   = statusDot(color.RGBA{250, 204, 21, 255})  // yellow-400
	dotWorking = statusDot(color.RGBA{34, 197, 94, 255})   // green-500
)

// chatMenuLine renders one chat row: short name + state, plus a status dot.
func chatMenuLine(a poll.Agent, needsYou map[string]bool) (string, []byte) {
	dot, status := dotIdle, "idle"
	switch {
	case needsYou[a.Name] || a.State == "blocked":
		dot, status = dotNeeds, "needs you"
		if a.StateReason != "" {
			status = a.StateReason
		}
	case a.Activity == "Working":
		dot, status = dotWorking, "working"
	}
	return fmt.Sprintf("%s — %s", strings.TrimPrefix(a.Name, "agent-"), status), dot
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

	agentSvc := &svc.AgentService{Runner: runner, Poller: poller, Exec: exec,
		State: stateSvc, VMName: vmName()}
	termSvc := &svc.TerminalService{Manager: termMgr}
	quotaSvc := &svc.QuotaService{}
	dockSvc := dock.New()

	app := application.New(application.Options{
		Name: "Safe Agentic",
		Services: []application.Service{
			application.NewService(agentSvc),
			application.NewService(termSvc),
			application.NewService(stateSvc),
			application.NewService(quotaSvc),
			application.NewService(dockSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	em.set(app)

	// Native folder picker for "spawn from local folder".
	agentSvc.PickDir = func() (string, error) {
		return app.Dialog.OpenFile().
			CanChooseFiles(false).
			CanChooseDirectories(true).
			PromptForSingleSelection()
	}

	newWindow := func() *application.WebviewWindow {
		return app.Window.NewWithOptions(application.WebviewWindowOptions{
			Title:  "Safe Agentic",
			Width:  1400,
			Height: 900,
			URL:    "/",
		})
	}
	newWindow()

	// showApp raises a live window. Never hold a window pointer across tray
	// clicks: the original window may have been closed while another stays open.
	showApp := func() {
		wins := app.Window.GetAll()
		if len(wins) == 0 {
			newWindow() // shows + focuses on creation
			return
		}
		w := app.Window.Current()
		if w == nil {
			w = wins[0]
		}
		w.Show()
		w.Focus()
	}

	// Stock menu plus File → New Window, which makes the app multi-window.
	// The Edit menu is hand-built: Wails alpha's EditMenu role items are empty
	// stubs on darwin, so they swallow ⌘C/⌘V app-wide while doing nothing.
	// Wiring each item to the native selector restores clipboard everywhere
	// (inputs and the terminal).
	appMenu := application.NewMenu()
	appMenu.AddRole(application.AppMenu)
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.Add("New Window").SetAccelerator("CmdOrCtrl+N").
		OnClick(func(*application.Context) { newWindow() })
	fileMenu.AddRole(application.CloseWindow)
	editMenu := appMenu.AddSubmenu("Edit")
	for _, it := range []struct{ label, accel, sel string }{
		{"Undo", "CmdOrCtrl+z", "undo:"},
		{"Redo", "CmdOrCtrl+Shift+z", "redo:"},
		{"Cut", "CmdOrCtrl+x", "cut:"},
		{"Copy", "CmdOrCtrl+c", "copy:"},
		{"Paste", "CmdOrCtrl+v", "paste:"},
		{"Select All", "CmdOrCtrl+a", "selectAll:"},
	} {
		sel := it.sel
		editMenu.Add(it.label).SetAccelerator(it.accel).
			OnClick(func(*application.Context) { sendEditAction(sel) })
	}
	appMenu.AddRole(application.ViewMenu)
	appMenu.AddRole(application.WindowMenu)
	appMenu.AddRole(application.HelpMenu)
	app.Menu.SetApplicationMenu(appMenu)

	// Systray: counts + per-agent focus items.
	var needsMu sync.Mutex
	needsYou := map[string]bool{}
	tray := app.SystemTray.New()
	// Icon only in the menubar (template icon adapts to light/dark);
	// counts live in the dropdown header.
	tray.SetTemplateIcon(trayIcon)
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
		// Dock badge mirrors the needs-you count (Omnigent-style).
		if _, needs, _ := countStates(agents, needsYou); needs > 0 {
			dockSvc.SetBadge(strconv.Itoa(needs))
		} else {
			dockSvc.RemoveBadge()
		}
		menu := application.NewMenu()
		menu.Add(trayHeader(agents, needsYou)).SetEnabled(false)
		menu.AddSeparator()
		for _, a := range agents {
			if !a.Running {
				continue
			}
			name := a.Name
			line, dot := chatMenuLine(a, needsYou)
			menu.Add(line).SetBitmap(dot).OnClick(func(*application.Context) {
				showApp()
				em.Emit("focus.agent", name)
			})
		}
		menu.AddSeparator()
		menu.Add("Projects").SetEnabled(false)
		for _, p := range stateSvc.Projects() {
			url := p.URL
			menu.Add(state.ShortRepoName(url)).OnClick(func(*application.Context) {
				go func() {
					stateSvc.ProjectUse(url)
					if _, err := agentSvc.Spawn(svc.SpawnRequest{Agent: "claude", Repo: url, SSH: true}); err != nil {
						em.Emit("event.new", map[string]any{"status": "failed",
							"event": events.Event{Type: "tray.spawn-failed", Payload: map[string]string{"container": url}}})
						log.Printf("tray spawn %s: %v", url, err)
					}
				}()
				showApp()
			})
		}
		menu.Add("New chat…").OnClick(func(*application.Context) {
			showApp()
			em.Emit("focus.spawn", nil)
		})
		menu.AddSeparator()
		menu.Add("Open Safe Agentic").OnClick(func(*application.Context) { showApp() })
		menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
		tray.SetMenu(menu)
	}
	// Rebuild runs InvokeSync-backed tray calls; detach from the poller
	// goroutine so OnShutdown's blocking poller.Stop can never deadlock
	// against a main-thread dispatch.
	poller.OnSnapshot = func(a []poll.Agent) { go rebuild(a) }
	app.Event.On("event.new", func(e *application.CustomEvent) {
		// Track needs-you from watcher events (mirrors frontend store: set on
		// needs statuses, clear on anything else); next poller snapshot
		// redraws the tray. Go-side listeners receive the ORIGINAL value:
		// map[string]any{"event": events.Event, "status": string}.
		m, ok := e.Data.(map[string]any)
		if !ok {
			return
		}
		ev, ok := m["event"].(events.Event)
		if !ok {
			return
		}
		c := ev.Payload["container"]
		if c == "" {
			return
		}
		st, _ := m["status"].(string)
		needsMu.Lock()
		if st == "needs-auth" || st == "stuck" || st == "blocked" {
			needsYou[c] = true
		} else {
			delete(needsYou, c)
		}
		needsMu.Unlock()
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
