package main

import "testing"

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

type errString string

func (e errString) Error() string { return string(e) }

func assertErr(msg string) error { return errString(msg) }

func TestLogsOverlayFallsBackToDockerLogs(t *testing.T) {
	oldSession := fetchSessionLogsFunc
	oldPlain := fetchPlainLogsFunc
	defer func() {
		fetchSessionLogsFunc = oldSession
		fetchPlainLogsFunc = oldPlain
	}()

	fetchSessionLogsFunc = func(ac *Actions, name, tailLines string) []byte {
		return nil
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
