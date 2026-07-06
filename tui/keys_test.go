package main

import (
	"strings"
	"testing"
)

// The keyBindings table is the single source of truth. Every rune binding must
// resolve to a real handler, and no rune may be bound twice.
func TestKeyBindingsIntegrity(t *testing.T) {
	a := NewApp()
	handlers := a.keyHandlers()

	seen := map[rune]string{}
	for _, kb := range keyBindings {
		if kb.Handler != "" {
			if _, ok := handlers[kb.Handler]; !ok {
				t.Fatalf("binding %q references unknown handler %q", kb.Key, kb.Handler)
			}
		}
		if r := kb.runeKey(); r != 0 {
			if prev, dup := seen[r]; dup {
				t.Fatalf("rune %q bound twice: %q and %q", string(r), prev, kb.Key)
			}
			seen[r] = kb.Key
		}
	}

	// Every handler id defined must be used by at least one binding (no dead ids).
	used := map[string]bool{}
	for _, kb := range keyBindings {
		if kb.Handler != "" {
			used[kb.Handler] = true
		}
	}
	for id := range handlers {
		if !used[id] {
			t.Fatalf("handler %q is defined but never bound", id)
		}
	}
}

func TestFooterAndHelpDeriveFromTable(t *testing.T) {
	footer := footerShortcutList()
	if len(footer) == 0 {
		t.Fatal("footer shortcut list is empty")
	}
	labels := map[string]bool{}
	for _, s := range footer {
		labels[s.desc] = true // shortcut.desc holds the footer label here
	}
	for _, want := range []string{"Attach", "Steer", "PR", "Quit"} {
		if !labels[want] {
			t.Fatalf("footer missing %q label; got %#v", want, footer)
		}
	}

	help := helpText()
	for _, want := range []string{"Navigation", "Actions", "Inspect", "Go to top", "Go to bottom", "vm start"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help text missing %q:\n%s", want, help)
		}
	}
}

func TestSimpleRuneActionsCoverExpectedKeys(t *testing.T) {
	a := newInputTestApp()
	actions := a.simpleRuneActions()
	for _, r := range []rune{'a', 'g', 'G', 'P', 'i', 'S', '/', ':', 'p', '?', 'q'} {
		if _, ok := actions[r]; !ok {
			t.Fatalf("rune %q missing from simpleRuneActions", string(r))
		}
	}
	// 'g' was rebound from Create-PR to go-to-top; PR now lives on 'P'.
	if _, ok := actions['P']; !ok {
		t.Fatal("Create-PR should be bound to 'P'")
	}
}

func TestSelectFirstAndLast(t *testing.T) {
	at := newLoadedAgentTable()
	at.SelectLast()
	lastRow, _ := at.table.GetSelection()
	at.SelectFirst()
	firstRow, _ := at.table.GetSelection()
	if firstRow >= lastRow {
		t.Fatalf("SelectFirst row %d should be above SelectLast row %d", firstRow, lastRow)
	}
	if firstRow < 1 {
		t.Fatalf("first data row = %d, want >= 1 (row 0 is the header)", firstRow)
	}
}

func TestSteerOverlayOpensAndCloses(t *testing.T) {
	a := newInputTestApp()
	a.actions = NewActions(a)
	a.actions.Steer()
	if name, _ := a.pages.GetFrontPage(); name != "steer" {
		t.Fatalf("front page after Steer = %q, want steer", name)
	}
	closeSteerForm(a)
	if name, _ := a.pages.GetFrontPage(); name != "main" {
		t.Fatalf("front page after close = %q, want main", name)
	}
}
