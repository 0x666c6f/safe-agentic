package main

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

type errString string

func (e errString) Error() string { return string(e) }

func assertErr(msg string) error { return errString(msg) }

func TestNewAppLoadingAndExecAfterArgs(t *testing.T) {
	a := NewApp()

	if got := a.header.view.GetText(true); !strings.Contains(got, "Loading...") {
		t.Fatalf("header text = %q", got)
	}
	if got := a.table.table.GetCell(0, 0).Text; !strings.Contains(got, "Connecting to VM") {
		t.Fatalf("table loading text = %q", got)
	}
	if name, _ := a.pages.GetFrontPage(); name != "main" {
		t.Fatalf("front page = %q, want main", name)
	}

	a.ExecAfterExit([]string{"orb", "run"})
	if got := a.ExecAfterArgs(); len(got) != 2 || got[0] != "orb" || got[1] != "run" {
		t.Fatalf("ExecAfterArgs() = %#v", got)
	}
}

func TestAppUpdatePreviewHandleCommandAndOverlayHelpers(t *testing.T) {
	installFakeOrb(t)

	a := NewApp()
	agents := testAgents()
	a.table.Update(append([]Agent(nil), agents...))

	a.preview.Toggle()
	a.rebuildLayout()
	a.updatePreview(&agents[0])
	if got := a.preview.textView.GetText(false); !strings.Contains(got, "line one") {
		t.Fatalf("preview text = %q", got)
	}

	a.updatePreview(&agents[1])
	if got := a.preview.textView.GetText(false); got != "Agent not running" {
		t.Fatalf("stopped preview text = %q", got)
	}

	a.handleCommand("bogus cmd")
	if a.footer.Mode() != FooterModeStatus || !strings.Contains(a.footer.hints.GetText(true), "Unknown command") {
		t.Fatalf("command status = %q", a.footer.hints.GetText(true))
	}

	ShowOverlay(a, "help", "Help", "body")
	if name, _ := a.pages.GetFrontPage(); name != "help" {
		t.Fatalf("front page after overlay = %q", name)
	}
	live := ShowOverlayLive(a, "live", "Live", "tail")
	if got := live.GetText(false); got != "tail" {
		t.Fatalf("live overlay text = %q", got)
	}
	if name, _ := a.pages.GetFrontPage(); name != "live" {
		t.Fatalf("front page after live overlay = %q", name)
	}
}

func TestAppHandleInputModesAndSorting(t *testing.T) {
	a := NewApp()
	a.table.Update(append([]Agent(nil), testAgents()...))

	if got := a.handleInput(tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone)); got != nil {
		t.Fatalf("filter input should return nil, got %#v", got)
	}
	if a.footer.Mode() != FooterModeFilter {
		t.Fatalf("Mode() = %v, want filter", a.footer.Mode())
	}
	if got := a.handleInput(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); got != nil {
		t.Fatalf("escape in filter should return nil, got %#v", got)
	}
	if a.footer.Mode() != FooterModeShortcuts {
		t.Fatalf("Mode() after filter escape = %v", a.footer.Mode())
	}

	if got := a.handleInput(tcell.NewEventKey(tcell.KeyRune, ':', tcell.ModNone)); got != nil {
		t.Fatalf("command input should return nil, got %#v", got)
	}
	if a.footer.Mode() != FooterModeCommand {
		t.Fatalf("Mode() = %v, want command", a.footer.Mode())
	}
	if got := a.handleInput(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); got != nil {
		t.Fatalf("escape in command should return nil, got %#v", got)
	}

	a.footer.ShowStatus("ok", false)
	ev := tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone)
	if got := a.handleInput(ev); got != ev {
		t.Fatalf("status mode should return original event")
	}
	if a.footer.Mode() != FooterModeShortcuts {
		t.Fatalf("Mode() after status reset = %v", a.footer.Mode())
	}

	a.showHelpOverlay()
	if got := a.handleInput(tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)); got != nil {
		t.Fatalf("overlay escape should return nil, got %#v", got)
	}
	if name, _ := a.pages.GetFrontPage(); name != "main" {
		t.Fatalf("front page after overlay escape = %q", name)
	}

	if got := a.handleInput(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone)); got == nil || got.Key() != tcell.KeyDown {
		t.Fatalf("j should map to down, got %#v", got)
	}
	if got := a.handleInput(tcell.NewEventKey(tcell.KeyRune, 'k', tcell.ModNone)); got == nil || got.Key() != tcell.KeyUp {
		t.Fatalf("k should map to up, got %#v", got)
	}

	if got := a.handleInput(tcell.NewEventKey(tcell.KeyRune, '2', tcell.ModNone)); got != nil {
		t.Fatalf("sort key should return nil, got %#v", got)
	}
	if a.table.sortCol != 1 || !a.table.sortAsc {
		t.Fatalf("sort state = col %d asc %v, want col 1 asc true", a.table.sortCol, a.table.sortAsc)
	}
}

func TestPreviewUpdatesOnSelectionChange(t *testing.T) {
	oldCapture := previewCaptureFunc
	oldLogs := previewLogsFunc
	previewCaptureFunc = func(name string, lines int) (string, error) {
		return "preview:" + name, nil
	}
	previewLogsFunc = func(name string, lines int) (string, error) {
		return "logs:" + name, nil
	}
	defer func() {
		previewCaptureFunc = oldCapture
		previewLogsFunc = oldLogs
	}()

	app := NewApp()
	app.table.Update([]Agent{
		{Name: "agent-one", Type: "codex", Running: true},
		{Name: "agent-two", Type: "codex", Running: true},
	})

	app.preview.Toggle()
	app.table.Table().Select(2, 0)

	if got := app.preview.AgentName(); got != "agent-two" {
		t.Fatalf("preview agent = %q, want %q", got, "agent-two")
	}
}

func TestPreviewFallsBackToLogsForStoppedAgent(t *testing.T) {
	oldCapture := previewCaptureFunc
	oldLogs := previewLogsFunc
	previewCaptureFunc = func(name string, lines int) (string, error) {
		return "preview:" + name, nil
	}
	previewLogsFunc = func(name string, lines int) (string, error) {
		return "logs:" + name, nil
	}
	defer func() {
		previewCaptureFunc = oldCapture
		previewLogsFunc = oldLogs
	}()

	app := NewApp()
	agent := &Agent{Name: "agent-claude-done", Type: "claude", Running: false}
	app.updatePreview(agent)

	if got := app.preview.AgentName(); got != "agent-claude-done" {
		t.Fatalf("preview agent = %q, want %q", got, "agent-claude-done")
	}
	if got := app.preview.textView.GetText(false); got != "logs:agent-claude-done" {
		t.Fatalf("preview text = %q, want log fallback", got)
	}
}

func TestPreviewFallsBackToLogsWhenCaptureFails(t *testing.T) {
	oldCapture := previewCaptureFunc
	oldLogs := previewLogsFunc
	previewCaptureFunc = func(name string, lines int) (string, error) {
		return "", assertErr("capture failed")
	}
	previewLogsFunc = func(name string, lines int) (string, error) {
		return "logs:" + name, nil
	}
	defer func() {
		previewCaptureFunc = oldCapture
		previewLogsFunc = oldLogs
	}()

	app := NewApp()
	agent := &Agent{Name: "agent-claude-live", Type: "claude", Running: true}
	app.updatePreview(agent)

	if got := app.preview.textView.GetText(false); got != "logs:agent-claude-live" {
		t.Fatalf("preview text = %q, want log fallback", got)
	}
}

func TestLogsOverlayFallsBackToDockerLogs(t *testing.T) {
	oldSession := fetchSessionLogsFunc
	oldAgent := fetchAgentLogsFunc
	oldPlain := fetchPlainLogsFunc
	defer func() {
		fetchSessionLogsFunc = oldSession
		fetchAgentLogsFunc = oldAgent
		fetchPlainLogsFunc = oldPlain
	}()

	fetchSessionLogsFunc = func(ac *Actions, name, tailLines string) []byte {
		return nil
	}
	fetchAgentLogsFunc = func(name, tailLines string) string {
		return ""
	}
	fetchPlainLogsFunc = func(name, tailLines string) []byte {
		return []byte("docker-log-line")
	}

	app := NewApp()
	app.table.Update([]Agent{
		{Name: "agent-claude-done", Type: "claude", Running: false},
	})
	app.actions = NewActions(app)

	app.actions.Logs()

	name, prim := app.pages.GetFrontPage()
	if name != "logs" {
		t.Fatalf("front page = %q, want logs", name)
	}
	tv2, ok := prim.(interface{ GetText(bool) string })
	if !ok {
		t.Fatal("logs overlay not created")
	}
	if got := tv2.GetText(false); got != "docker-log-line" {
		t.Fatalf("logs overlay text = %q, want docker fallback", got)
	}
}

func TestLogsOverlayPrefersAgentLogsOverDockerLogs(t *testing.T) {
	oldSession := fetchSessionLogsFunc
	oldAgent := fetchAgentLogsFunc
	oldPlain := fetchPlainLogsFunc
	defer func() {
		fetchSessionLogsFunc = oldSession
		fetchAgentLogsFunc = oldAgent
		fetchPlainLogsFunc = oldPlain
	}()

	fetchSessionLogsFunc = func(ac *Actions, name, tailLines string) []byte {
		return nil
	}
	fetchAgentLogsFunc = func(name, tailLines string) string {
		return "agent-log-line"
	}
	fetchPlainLogsFunc = func(name, tailLines string) []byte {
		return []byte("docker-log-line")
	}

	app := NewApp()
	app.table.Update([]Agent{
		{Name: "agent-claude-done", Type: "claude", Running: false},
	})
	app.actions = NewActions(app)

	app.actions.Logs()

	_, prim := app.pages.GetFrontPage()
	tv, ok := prim.(interface{ GetText(bool) string })
	if !ok {
		t.Fatal("logs overlay not created")
	}
	if got := tv.GetText(false); got != "agent-log-line" {
		t.Fatalf("logs overlay text = %q, want agent-log fallback", got)
	}
}
