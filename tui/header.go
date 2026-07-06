package main

import (
	"fmt"
	"time"

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

// Update refreshes the header content. A non-zero staleSince means the poller
// can no longer reach the VM: the whole bar turns into an unmissable red alert
// so stale rows are never mistaken for live ones.
func (h *Header) Update(running, total int, staleSince time.Time) {
	if !staleSince.IsZero() {
		h.view.SetBackgroundColor(colorBannerBg)
		h.view.SetText(fmt.Sprintf(
			" [white::b]⚠ VM UNREACHABLE[white::-] — data stale since %s. Press [white::b]S[white::-] to run 'safe-ag vm start'.    agents: %d/%d",
			staleSince.Format("15:04:05"),
			running,
			total,
		))
		return
	}

	h.view.SetBackgroundColor(tcell.ColorDefault)
	h.view.SetText(fmt.Sprintf(
		" [%s::b]safe-agentic[-::-]        ctx: %s VM    [::d]⏱ %ds[-::-]    agents: [%s]%d[-]/%d",
		colorToTag(colorTitle),
		vmName,
		pollInterval,
		colorToTag(colorRunning),
		running,
		total,
	))
}

// Primitive returns the underlying tview primitive.
func (h *Header) Primitive() tview.Primitive {
	return h.view
}

func colorToTag(c tcell.Color) string {
	return fmt.Sprintf("#%06x", c.Hex())
}
