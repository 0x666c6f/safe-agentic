package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// AgentTable wraps a tview.Table for displaying agents.
type AgentTable struct {
	table        *tview.Table
	agents       []Agent // currently displayed (after filter)
	allAgents    []Agent // all agents from poller
	filter       string
	sortCol      int
	sortAsc      bool
	selectedName string       // track selection across refreshes by name
	loading      bool         // true until first real Update
	rowToAgent   map[int]int  // table row → agents index (skips separator rows)
}

// NewAgentTable creates the table view.
func NewAgentTable() *AgentTable {
	t := tview.NewTable().
		SetSelectable(true, false).
		SetFixed(1, 0).
		SetSeparator(' ')
	t.SetBackgroundColor(tcell.ColorDefault)
	t.SetSelectedStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(colorSelected))

	return &AgentTable{
		table:   t,
		sortCol: 0,
		sortAsc: true,
	}
}

// Update refreshes the table with new agent data.
func (at *AgentTable) Update(agents []Agent) {
	at.loading = false
	at.allAgents = agents
	at.refresh()
}

// SetFilter sets the filter string and refreshes.
func (at *AgentTable) SetFilter(filter string) {
	at.filter = filter
	at.refresh()
}

// SetSort sets the sort column (0-indexed) and refreshes.
func (at *AgentTable) SetSort(col int) {
	if col == at.sortCol {
		at.sortAsc = !at.sortAsc
	} else {
		at.sortCol = col
		at.sortAsc = true
	}
	at.refresh()
}

// ShowLoading displays a loading indicator in the table.
func (at *AgentTable) ShowLoading() {
	at.loading = true
	at.SetLoadingFrame("⠋")
}

// SetLoadingFrame updates the spinner frame in the loading indicator.
// No-op once real data has arrived via Update().
func (at *AgentTable) SetLoadingFrame(frame string) {
	if !at.loading {
		return
	}
	at.table.Clear()
	cell := tview.NewTableCell("  " + frame + " Connecting to VM and fetching containers...").
		SetTextColor(colorStopped).
		SetSelectable(false).
		SetExpansion(1)
	at.table.SetCell(0, 0, cell)
}

// SelectedAgent returns the currently selected agent, or nil.
func (at *AgentTable) SelectedAgent() *Agent {
	row, _ := at.table.GetSelection()
	if row < 1 {
		return nil
	}
	idx, ok := at.rowToAgent[row]
	if !ok {
		return nil
	}
	if idx >= 0 && idx < len(at.agents) {
		a := at.agents[idx]
		return &a
	}
	return nil
}

// RunningCount returns the number of running agents.
func (at *AgentTable) RunningCount() int {
	count := 0
	for _, a := range at.allAgents {
		if a.Running {
			count++
		}
	}
	return count
}

// TotalCount returns the total number of agents.
func (at *AgentTable) TotalCount() int {
	return len(at.allAgents)
}

// Primitive returns the underlying tview primitive.
func (at *AgentTable) Primitive() tview.Primitive {
	return at.table
}

// Table returns the raw tview.Table (for focus).
func (at *AgentTable) Table() *tview.Table {
	return at.table
}

func (at *AgentTable) refresh() {
	// Save current selection by name
	if sel := at.SelectedAgent(); sel != nil {
		at.selectedName = sel.Name
	}

	at.agents = FilterAgents(at.allAgents, at.filter)
	SortAgents(at.agents, at.sortCol, at.sortAsc)

	at.table.Clear()

	_, _, width, _ := at.table.GetInnerRect()
	if width == 0 {
		width = 160 // default before first draw
	}
	visibleCols := VisibleColumns(width)

	// Header row
	for ci, colIdx := range visibleCols {
		title := columns[colIdx].Title
		if colIdx == at.sortCol {
			arrow := "▲"
			if !at.sortAsc {
				arrow = "▼"
			}
			title = title + arrow
		}
		cell := tview.NewTableCell(title).
			SetTextColor(colorHeader).
			SetAttributes(tcell.AttrBold).
			SetSelectable(false).
			SetExpansion(1)
		at.table.SetCell(0, ci, cell)
	}

	// Empty state
	if len(at.agents) == 0 {
		cell := tview.NewTableCell("  No agents found. Press 'n' to spawn one.").
			SetTextColor(colorStopped).
			SetSelectable(false).
			SetExpansion(1)
		at.table.SetCell(1, 0, cell)
		return
	}

	// Data rows — group by fleet/pipeline if present
	row := 1
	at.rowToAgent = make(map[int]int)
	lastFleet := ""
	for agentIdx, agent := range at.agents {
		// Insert fleet group header when fleet changes
		if agent.Fleet != "" && agent.Fleet != lastFleet {
			cell := tview.NewTableCell(fmt.Sprintf("┄ %s ", agent.Fleet)).
				SetTextColor(colorHeader).
				SetAttributes(tcell.AttrDim).
				SetSelectable(false).
				SetExpansion(1)
			at.table.SetCell(row, 0, cell)
			for ci := 1; ci < len(visibleCols); ci++ {
				at.table.SetCell(row, ci, tview.NewTableCell("").SetSelectable(false))
			}
			row++
		}
		lastFleet = agent.Fleet

		for ci, colIdx := range visibleCols {
			value := fieldByColumn(agent, colIdx)
			cell := tview.NewTableCell(padRight(value, columns[colIdx].Width)).
				SetExpansion(1)

			// Color status column
			if colIdx == 8 {
				if agent.Running {
					cell.SetTextColor(colorRunning)
				} else {
					cell.SetTextColor(colorExited)
				}
			}
			// Color activity column
			if colIdx == 9 {
				switch agent.Activity {
				case "Working":
					cell.SetTextColor(colorRunning)
				case "Idle":
					cell.SetTextColor(colorStopped)
				case "Stopped":
					cell.SetTextColor(colorExited)
				}
			}
			// Color type column
			if colIdx == 1 {
				switch agent.Type {
				case "claude":
					cell.SetTextColor(tcell.ColorOrange)
				case "codex":
					cell.SetTextColor(tcell.ColorDodgerBlue)
				}
			}

			at.table.SetCell(row, ci, cell)
		}
		at.rowToAgent[row] = agentIdx
		row++
	}

	// Restore selection by name
	at.restoreSelection()
}

func (at *AgentTable) restoreSelection() {
	if at.selectedName == "" {
		// Select first data row
		for row, _ := range at.rowToAgent {
			at.table.Select(row, 0)
			return
		}
		return
	}
	for row, idx := range at.rowToAgent {
		if idx < len(at.agents) && at.agents[idx].Name == at.selectedName {
			at.table.Select(row, 0)
			return
		}
	}
	// Name not found — select first data row
	for row, _ := range at.rowToAgent {
		at.table.Select(row, 0)
		return
	}
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + fmt.Sprintf("%*s", width-len(s), "")
}
