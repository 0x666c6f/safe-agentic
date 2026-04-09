package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const defaultPreviewLines = 30

// PreviewPane shows a live capture of an agent's tmux pane.
type PreviewPane struct {
	textView  *tview.TextView
	visible   bool
	agentName string
	lines     int
}

// NewPreviewPane creates a hidden preview pane.
func NewPreviewPane() *PreviewPane {
	tv := tview.NewTextView().
		SetScrollable(true).
		SetDynamicColors(false).
		SetWrap(true)
	tv.SetBorder(true).
		SetBorderColor(colorBorder).
		SetBackgroundColor(tcell.ColorDefault)

	return &PreviewPane{
		textView: tv,
		lines:    defaultPreviewLines,
	}
}

// Toggle switches the preview pane on or off.
func (p *PreviewPane) Toggle() {
	p.visible = !p.visible
	if !p.visible {
		p.agentName = ""
		p.textView.SetText("")
		p.textView.SetTitle("")
	}
}

// Visible returns whether the preview pane is showing.
func (p *PreviewPane) Visible() bool {
	return p.visible
}

// AgentName returns the name of the agent being previewed.
func (p *PreviewPane) AgentName() string {
	return p.agentName
}

// Lines returns the number of lines to capture.
func (p *PreviewPane) Lines() int {
	return p.lines
}

// Update sets the preview content for a given agent.
func (p *PreviewPane) Update(name string, content string) {
	p.agentName = name
	p.textView.SetTitle(fmt.Sprintf(" Preview: %s (p to close) ", name))
	p.textView.SetTitleColor(colorTitle)
	// Strip trailing blank lines for cleaner display
	content = strings.TrimRight(content, "\n ")
	p.textView.SetText(content)
	p.textView.ScrollToEnd()
}

// SetUnavailable shows a reason why preview isn't available.
func (p *PreviewPane) SetUnavailable(name string, reason string) {
	p.agentName = name
	p.textView.SetTitle(fmt.Sprintf(" Preview: %s (p to close) ", name))
	p.textView.SetTitleColor(colorTitle)
	p.textView.SetText(reason)
}

// Primitive returns the underlying tview primitive.
func (p *PreviewPane) Primitive() tview.Primitive {
	return p.textView
}
