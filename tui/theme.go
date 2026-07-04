package main

import "github.com/gdamore/tcell/v2"

var (
	colorTitle        = tcell.ColorAqua
	colorHeader       = tcell.ColorWhite
	colorSelected     = tcell.ColorDarkCyan
	colorRunning      = tcell.ColorGreen
	colorDeleting     = tcell.ColorYellow
	colorExited       = tcell.ColorRed
	colorStopped      = tcell.ColorYellow
	colorShortcutKey  = tcell.ColorAqua
	colorShortcutDesc = tcell.ColorWhite
	colorError        = tcell.ColorRed
	colorStale        = tcell.ColorYellow
	colorBorder       = tcell.ColorDimGray

	// Agent-state column colors (blocked=attention, working=neutral, done=ok,
	// idle=dim, exited=error).
	colorStateBlocked = tcell.ColorRed
	colorStateWorking = tcell.ColorAqua
	colorStateDone    = tcell.ColorGreen
	colorStateIdle    = tcell.ColorGray
	colorStateExited  = tcell.ColorOrangeRed
)
