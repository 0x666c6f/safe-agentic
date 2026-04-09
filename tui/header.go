package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Header displays the app title, context, refresh interval, and agent counts.
type Header struct {
	view *tview.TextView
}

// NewHeader creates the header bar.
func NewHeader() *Header {
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	tv.SetBackgroundColor(tcell.ColorDefault)
	return &Header{view: tv}
}

// ShowLoading displays the initial loading state.
func (h *Header) ShowLoading() {
	h.view.SetText(fmt.Sprintf(
		" [%s::b]safe-agentic[-::-]        ctx: %s VM    [%s]Loading...[-]",
		colorToTag(colorTitle),
		vmName,
		colorToTag(colorStopped),
	))
}

// Update refreshes the header content.
func (h *Header) Update(running, total int, stale bool) {
	staleIndicator := ""
	if stale {
		staleIndicator = fmt.Sprintf(" [%s]STALE[-]", colorToTag(colorStale))
	}

	text := fmt.Sprintf(
		" [%s::b]safe-agentic[-::-]        ctx: %s VM    [::d]⏱ %ds[-::-]    agents: [%s]%d[-]/%d%s",
		colorToTag(colorTitle),
		vmName,
		pollInterval,
		colorToTag(colorRunning),
		running,
		total,
		staleIndicator,
	)
	h.view.SetText(text)
}

// Primitive returns the underlying tview primitive.
func (h *Header) Primitive() tview.Primitive {
	return h.view
}

func colorToTag(c tcell.Color) string {
	return fmt.Sprintf("#%06x", c.Hex())
}
