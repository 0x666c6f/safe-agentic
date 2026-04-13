package main

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestFooterShortcutsAndRows(t *testing.T) {
	f := NewFooter()

	if f.Mode() != FooterModeShortcuts {
		t.Fatalf("Mode() = %v, want shortcuts", f.Mode())
	}
	if f.Rows() < 1 {
		t.Fatalf("Rows() = %d", f.Rows())
	}
	if f.InputField() == nil || f.Primitive() == nil {
		t.Fatal("footer primitives should not be nil")
	}
	text := f.hints.GetText(true)
	if !strings.Contains(text, "Attach") || !strings.Contains(text, "Quit") {
		t.Fatalf("shortcut text = %q", text)
	}

	grid, rows := renderShortcutGrid([]shortcut{{"a", "Attach"}, {"q", "Quit"}, {"n", "New"}}, 20)
	if rows != 3 {
		t.Fatalf("rows = %d, want 3", rows)
	}
	if !strings.Contains(grid, "Attach") || !strings.Contains(grid, "Quit") || !strings.Contains(grid, "<a>") || !strings.Contains(grid, "<q>") {
		t.Fatalf("grid = %q", grid)
	}
}

func TestFooterFilterAndCommandMode(t *testing.T) {
	f := NewFooter()

	var filterValue string
	f.ShowFilter(func(v string) { filterValue = v })
	if f.Mode() != FooterModeFilter {
		t.Fatalf("Mode() = %v, want filter", f.Mode())
	}
	if got := f.input.GetLabel(); got != "/ " {
		t.Fatalf("filter label = %q", got)
	}
	f.input.SetText("codex")
	f.input.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), nil)
	if filterValue != "codex" {
		t.Fatalf("filter callback = %q, want %q", filterValue, "codex")
	}
	if f.Mode() != FooterModeShortcuts {
		t.Fatalf("Mode() after filter enter = %v, want shortcuts", f.Mode())
	}

	var commandValue string
	f.ShowCommand(func(v string) { commandValue = v })
	if f.Mode() != FooterModeCommand {
		t.Fatalf("Mode() = %v, want command", f.Mode())
	}
	if got := f.input.GetLabel(); got != ": " {
		t.Fatalf("command label = %q", got)
	}
	f.input.SetText("fleet examples/demo.yaml")
	f.input.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), nil)
	if commandValue != "fleet examples/demo.yaml" {
		t.Fatalf("command callback = %q", commandValue)
	}
	if f.Mode() != FooterModeShortcuts {
		t.Fatalf("Mode() after command enter = %v, want shortcuts", f.Mode())
	}
}

func TestFooterConfirmStatusAndReset(t *testing.T) {
	f := NewFooter()

	var confirmed *bool
	f.ShowConfirm("Stop agent-beta?", func(v bool) { confirmed = &v })
	if f.Mode() != FooterModeConfirm {
		t.Fatalf("Mode() = %v, want confirm", f.Mode())
	}
	if !strings.Contains(f.hints.GetText(true), "Stop agent-beta?") {
		t.Fatalf("confirm text = %q", f.hints.GetText(true))
	}
	if !f.HandleConfirmKey('y') {
		t.Fatal("expected HandleConfirmKey(y) to handle input")
	}
	if confirmed == nil || !*confirmed {
		t.Fatalf("confirm callback = %#v", confirmed)
	}
	if f.Mode() != FooterModeShortcuts {
		t.Fatalf("Mode() after confirm = %v, want shortcuts", f.Mode())
	}
	if f.HandleConfirmKey('x') {
		t.Fatal("expected non-confirm mode to ignore key")
	}

	f.ShowConfirm("Stop agent-gamma?", func(v bool) { confirmed = &v })
	if !f.HandleConfirmKey('N') {
		t.Fatal("expected HandleConfirmKey(N) to handle input")
	}
	if confirmed == nil || *confirmed {
		t.Fatalf("confirm callback for N = %#v", confirmed)
	}

	f.ShowStatus("all good", false)
	if f.Mode() != FooterModeStatus || !strings.Contains(f.hints.GetText(true), "all good") {
		t.Fatalf("status text = %q", f.hints.GetText(true))
	}
	f.ShowStatus("boom", true)
	if !strings.Contains(f.hints.GetText(true), "boom") {
		t.Fatalf("error status text = %q", f.hints.GetText(true))
	}
	f.Reset()
	if f.Mode() != FooterModeShortcuts {
		t.Fatalf("Mode() after reset = %v, want shortcuts", f.Mode())
	}
}
