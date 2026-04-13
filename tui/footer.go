package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// FooterMode determines what the footer displays.
type FooterMode int

const (
	FooterModeShortcuts FooterMode = iota
	FooterModeFilter
	FooterModeCommand
	FooterModeConfirm
	FooterModeStatus
)

// Footer displays shortcuts, filter input, command input, or confirmation prompts.
type Footer struct {
	layout    *tview.Flex
	hints     *tview.TextView
	input     *tview.InputField
	mode      FooterMode
	rows      int
	onFilter  func(string)
	onCommand func(string)
	onConfirm func(bool)
}

// NewFooter creates the footer bar.
func NewFooter() *Footer {
	hints := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	hints.SetBackgroundColor(tcell.ColorDefault)

	input := tview.NewInputField().
		SetFieldBackgroundColor(tcell.ColorDefault).
		SetFieldTextColor(tcell.ColorWhite)
	input.SetBackgroundColor(tcell.ColorDefault)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(hints, 3, 0, false)

	f := &Footer{
		layout: layout,
		hints:  hints,
		input:  input,
		mode:   FooterModeShortcuts,
	}
	f.showShortcuts()
	return f
}

var allShortcuts = []shortcut{
	{"a", "Attach"},
	{"s", "Stop"},
	{"l", "Logs"},
	{"d", "Describe"},
	{"f", "Diff"},
	{"R", "Review"},
	{"t", "Todos"},
	{"x", "Chkpt"},
	{"g", "PR"},
	{"$", "Cost"},
	{"A", "Audit"},
	{"n", "New"},
	{"p", "Preview"},
	{"e", "Export"},
	{"c", "Copy"},
	{"m", "MCP"},
	{"/", "Filter"},
	{":", "Cmd"},
	{"^k", "KillAll"},
	{"q", "Quit"},
}

const shortcutCellWidth = 14 // min column width per shortcut

type shortcut struct {
	key  string
	desc string
}

func (f *Footer) showShortcuts() {
	_, _, width, _ := f.hints.GetInnerRect()
	if width < 20 {
		width = 120 // fallback before first draw
	}
	text, rows := renderShortcutGrid(allShortcuts, width)
	f.hints.SetText(text)
	f.rows = rows
}

// Rows returns the current footer height.
func (f *Footer) Rows() int {
	if f.rows < 1 {
		return 3
	}
	return f.rows
}

func renderShortcutGrid(shortcuts []shortcut, termWidth int) (string, int) {
	// Compute how many columns fit
	cols := termWidth / shortcutCellWidth
	if cols < 1 {
		cols = 1
	}
	rows := (len(shortcuts) + cols - 1) / cols

	// Distribute cell width evenly
	cellWidth := termWidth / cols

	kt := colorToTag(colorShortcutKey)
	var b strings.Builder

	for r := 0; r < rows; r++ {
		if r > 0 {
			b.WriteByte('\n')
		}
		b.WriteByte(' ')
		for c := 0; c < cols; c++ {
			idx := c*rows + r
			if idx >= len(shortcuts) {
				break
			}
			s := shortcuts[idx]
			entry := fmt.Sprintf("<%s> %s", s.key, s.desc)
			pad := cellWidth - len(entry) - 1
			if pad < 1 {
				pad = 1
			}
			fmt.Fprintf(&b, "[%s]<%s>[white] %s%*s", kt, s.key, s.desc, pad, "")
		}
	}
	return b.String(), rows
}

// ShowFilter switches to filter input mode.
func (f *Footer) ShowFilter(onDone func(string)) {
	f.mode = FooterModeFilter
	f.onFilter = onDone
	f.input.SetLabel("/ ")
	f.input.SetText("")
	f.input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			if f.onFilter != nil {
				f.onFilter(f.input.GetText())
			}
		}
		f.Reset()
	})
	f.layout.Clear()
	f.layout.AddItem(f.input, 1, 0, true)
}

// ShowCommand switches to command input mode.
func (f *Footer) ShowCommand(onDone func(string)) {
	f.mode = FooterModeCommand
	f.onCommand = onDone
	f.input.SetLabel(": ")
	f.input.SetText("")
	f.input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			if f.onCommand != nil {
				f.onCommand(f.input.GetText())
			}
		}
		f.Reset()
	})
	f.layout.Clear()
	f.layout.AddItem(f.input, 1, 0, true)
}

// ShowConfirm shows a confirmation prompt.
func (f *Footer) ShowConfirm(message string, onResult func(bool)) {
	f.mode = FooterModeConfirm
	f.onConfirm = onResult
	f.hints.SetText(fmt.Sprintf(" [%s]%s [y/n][-]", colorToTag(colorStale), message))
	f.layout.Clear()
	f.layout.AddItem(f.hints, 1, 0, false)
}

// ShowStatus shows a temporary status message.
func (f *Footer) ShowStatus(message string, isError bool) {
	f.mode = FooterModeStatus
	tag := colorToTag(colorRunning)
	if isError {
		tag = colorToTag(colorError)
	}
	f.hints.SetText(fmt.Sprintf(" [%s]%s[-]", tag, message))
	f.layout.Clear()
	f.layout.AddItem(f.hints, 1, 0, false)
}

// Reset returns to the default shortcut hints.
func (f *Footer) Reset() {
	f.mode = FooterModeShortcuts
	f.onFilter = nil
	f.onCommand = nil
	f.onConfirm = nil
	f.showShortcuts()
	f.layout.Clear()
	f.layout.AddItem(f.hints, f.Rows(), 0, false)
}

// HandleConfirmKey processes y/n input during confirm mode. Returns true if handled.
func (f *Footer) HandleConfirmKey(key rune) bool {
	if f.mode != FooterModeConfirm {
		return false
	}
	switch key {
	case 'y', 'Y':
		if f.onConfirm != nil {
			f.onConfirm(true)
		}
		f.Reset()
		return true
	case 'n', 'N':
		if f.onConfirm != nil {
			f.onConfirm(false)
		}
		f.Reset()
		return true
	}
	return false
}

// Mode returns the current footer mode.
func (f *Footer) Mode() FooterMode {
	return f.mode
}

// InputField returns the input primitive (for focus).
func (f *Footer) InputField() *tview.InputField {
	return f.input
}

// Primitive returns the underlying tview primitive.
func (f *Footer) Primitive() tview.Primitive {
	return f.layout
}
