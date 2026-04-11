package main

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type tableRow struct {
	agentIndex int
	groupName  string
	prefix     string
	isGroup    bool
}

type treeNode struct {
	name     string
	children map[string]*treeNode
	order    []string
	agents   []int
}

// AgentTable wraps a tview.Table for displaying agents.
type AgentTable struct {
	table        *tview.Table
	agents       []Agent // currently displayed (after filter)
	allAgents    []Agent // all agents from poller
	rawAgents    []Agent // latest agents from poller before local overlays
	filter       string
	sortCol      int
	sortAsc      bool
	selectedName string      // track selection across refreshes by name
	loading      bool        // true until first real Update
	rowToAgent   map[int]int // table row → agents index (skips separator rows)
	deleting     map[string]Agent
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
		table:    t,
		sortCol:  0,
		sortAsc:  true,
		deleting: make(map[string]Agent),
	}
}

// Update refreshes the table with new agent data.
func (at *AgentTable) Update(agents []Agent) {
	at.loading = false
	at.rawAgents = cloneAgents(agents)
	at.rebuildAllAgents()
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

// MarkDeleting overlays a transient deleting state for an agent until the action finishes.
func (at *AgentTable) MarkDeleting(agent Agent) {
	agent.Deleting = true
	agent.Running = false
	agent.Status = "Deleting"
	agent.Activity = "Deleting"
	agent.Progress = spinnerFrames[0]
	agent.CPU = "-"
	agent.Memory = "-"
	agent.NetIO = "-"
	agent.PIDs = "-"
	at.deleting[agent.Name] = agent
	at.rebuildAllAgents()
	at.refresh()
}

// ClearDeleting removes transient deleting state for the provided agents.
func (at *AgentTable) ClearDeleting(names ...string) {
	for _, name := range names {
		delete(at.deleting, name)
	}
	at.rebuildAllAgents()
	at.refresh()
}

// FinishDeleting removes completed deletions from both poller cache and local overlays.
func (at *AgentTable) FinishDeleting(names ...string) {
	if len(names) == 0 {
		return
	}
	remove := make(map[string]struct{}, len(names))
	for _, name := range names {
		remove[name] = struct{}{}
		delete(at.deleting, name)
	}
	filtered := at.rawAgents[:0]
	for _, agent := range at.rawAgents {
		if _, ok := remove[agent.Name]; ok {
			continue
		}
		filtered = append(filtered, agent)
	}
	at.rawAgents = filtered
	at.rebuildAllAgents()
	at.refresh()
}

// SetDeletingFrame updates the spinner frame for all deleting rows.
func (at *AgentTable) SetDeletingFrame(frame string) {
	if len(at.deleting) == 0 {
		return
	}
	for name, agent := range at.deleting {
		agent.Progress = frame
		at.deleting[name] = agent
	}
	at.rebuildAllAgents()
	at.refresh()
}

// HasDeleting reports whether any rows are in a transient deleting state.
func (at *AgentTable) HasDeleting() bool {
	return len(at.deleting) > 0
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

func buildTableRows(agents []Agent) []tableRow {
	root := &treeNode{children: make(map[string]*treeNode)}
	var standalone []int

	for idx, agent := range agents {
		segments := groupSegments(agent)
		if len(segments) == 0 {
			standalone = append(standalone, idx)
			continue
		}

		node := root
		for _, seg := range segments {
			child, ok := node.children[seg]
			if !ok {
				child = &treeNode{name: seg, children: make(map[string]*treeNode)}
				node.children[seg] = child
				node.order = append(node.order, seg)
			}
			node = child
		}
		node.agents = append(node.agents, idx)
	}

	var rows []tableRow
	for _, name := range root.order {
		buildGroupRows(root.children[name], "", " ", true, &rows)
	}
	for _, idx := range standalone {
		rows = append(rows, tableRow{agentIndex: idx})
	}
	return rows
}

func buildGroupRows(node *treeNode, headerPrefix, childBase string, top bool, rows *[]tableRow) {
	row := tableRow{
		groupName: node.name,
		prefix:    headerPrefix,
		isGroup:   true,
	}
	if top {
		row.prefix = " "
	}
	*rows = append(*rows, row)

	totalChildren := len(node.agents) + len(node.order)
	childIndex := 0
	for _, agentIdx := range node.agents {
		last := childIndex == totalChildren-1
		*rows = append(*rows, tableRow{
			agentIndex: agentIdx,
			prefix:     childBase + treeConnector(last),
		})
		childIndex++
	}
	for _, name := range node.order {
		last := childIndex == totalChildren-1
		buildGroupRows(
			node.children[name],
			childBase+treeConnector(last),
			childBase+treeSpacer(last),
			false,
			rows,
		)
		childIndex++
	}
}

func treeConnector(last bool) string {
	if last {
		return "└── "
	}
	return "├── "
}

func treeSpacer(last bool) string {
	if last {
		return "    "
	}
	return "│   "
}

func (at *AgentTable) rebuildAllAgents() {
	base := cloneAgents(at.rawAgents)
	seen := make(map[string]bool, len(base))
	for i := range base {
		if pending, ok := at.deleting[base[i].Name]; ok {
			base[i] = pending
		}
		seen[base[i].Name] = true
	}
	for name, pending := range at.deleting {
		if !seen[name] {
			base = append(base, pending)
		}
	}
	at.allAgents = base
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

	row := 1
	at.rowToAgent = make(map[int]int)
	for _, displayRow := range buildTableRows(at.agents) {
		if displayRow.isGroup {
			cell := tview.NewTableCell(displayRow.prefix + "🔄 " + displayRow.groupName).
				SetTextColor(tcell.ColorGreen).
				SetAttributes(tcell.AttrBold).
				SetSelectable(false).
				SetExpansion(1)
			at.table.SetCell(row, 0, cell)
			for ci := 1; ci < len(visibleCols); ci++ {
				at.table.SetCell(row, ci, tview.NewTableCell("").SetSelectable(false))
			}
			row++
			continue
		}

		agentIdx := displayRow.agentIndex
		agent := at.agents[agentIdx]
		for ci, colIdx := range visibleCols {
			value := fieldByColumn(agent, colIdx)

			// Add tree connector + type icon to name column.
			if colIdx == 0 {
				typeIcon := ""
				switch agent.Type {
				case "claude":
					typeIcon = "🟠 "
				case "codex":
					typeIcon = "🔵 "
				}
				value = displayRow.prefix + typeIcon + value
			}

			cell := tview.NewTableCell(padRight(value, columns[colIdx].Width)).
				SetExpansion(1)

			// Color status column
			if colIdx == 8 {
				if agent.Deleting {
					cell.SetTextColor(colorDeleting)
				} else if agent.Running {
					cell.SetTextColor(colorRunning)
				} else {
					cell.SetTextColor(colorExited)
				}
			}
			// Color activity column
			if colIdx == 9 {
				switch {
				case agent.Deleting:
					cell.SetTextColor(colorDeleting)
				case agent.Activity == "Working":
					cell.SetTextColor(colorRunning)
				case agent.Activity == "Idle":
					cell.SetTextColor(colorStopped)
				case agent.Activity == "Stopped":
					cell.SetTextColor(colorExited)
				}
			}
			// Color type column.
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

func cloneAgents(agents []Agent) []Agent {
	if len(agents) == 0 {
		return nil
	}
	out := make([]Agent, len(agents))
	copy(out, agents)
	return out
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
