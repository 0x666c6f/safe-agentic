package main

import "github.com/gdamore/tcell/v2"

var (
	colorTitle        = tcell.ColorAqua
	colorHeader       = tcell.ColorWhite
	colorSelected     = tcell.ColorDarkCyan
	colorRunning      = tcell.ColorGreen
	colorDeleting     = tcell.ColorYellow
	colorStopped      = tcell.ColorYellow
	colorShortcutKey  = tcell.ColorAqua
	colorShortcutDesc = tcell.ColorWhite
	colorError        = tcell.ColorRed
	colorStale        = tcell.ColorYellow
	colorBorder       = tcell.ColorDimGray
	colorBannerBg     = tcell.ColorDarkRed // full-width alert bar (VM unreachable)

	// State/condition colors — deliberately distinct so a row that needs a human
	// never reads as a failure, and a clean stop never reads as an error:
	//   blocked      → orange (needs attention)
	//   working      → aqua
	//   done         → green (finished successfully)
	//   idle         → grey
	//   exited-clean → grey (stopped, exit 0, nothing to do)
	//   failed       → red (non-zero exit / crash / error)
	// The exited/failed condition uses a single red everywhere (STATE, STATUS,
	// ACTIVITY) so it can't show two different reds for the same row.
	colorStateBlocked = tcell.ColorOrange
	colorStateWorking = tcell.ColorAqua
	colorStateDone    = tcell.ColorGreen
	colorStateIdle    = tcell.ColorGray
	colorFailed       = tcell.ColorRed
	colorExitedClean  = tcell.ColorGray
	colorStateExited  = colorFailed
)
