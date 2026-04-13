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
	if sel := at.SelectedAgent(); sel != nil {
		at.selectedName = sel.Name
	}

	at.agents = FilterAgents(at.allAgents, at.filter)
	SortAgents(at.agents, at.sortCol, at.sortAsc)

	at.table.Clear()
	visibleCols := at.visibleColumns()
	at.renderHeaderRow(visibleCols)
	if at.renderEmptyState() {
		return
	}

	row := 1
	at.rowToAgent = make(map[int]int)
	for _, displayRow := range buildTableRows(at.agents) {
		if displayRow.isGroup {
			at.renderGroupRow(row, visibleCols, displayRow)
			row++
			continue
		}
		at.renderAgentRow(row, visibleCols, displayRow)
		row++
	}
	at.restoreSelection()
}

func (at *AgentTable) visibleColumns() []int {
	_, _, width, _ := at.table.GetInnerRect()
	if width == 0 {
		width = 160
	}
	return VisibleColumns(width)
}

func (at *AgentTable) renderHeaderRow(visibleCols []int) {
	for ci, colIdx := range visibleCols {
		at.table.SetCell(0, ci, headerCell(at.headerTitle(colIdx)))
	}
}

func (at *AgentTable) headerTitle(colIdx int) string {
	title := columns[colIdx].Title
	if colIdx != at.sortCol {
		return title
	}
	if at.sortAsc {
		return title + "▲"
	}
	return title + "▼"
}

func headerCell(title string) *tview.TableCell {
	return tview.NewTableCell(title).
		SetTextColor(colorHeader).
		SetAttributes(tcell.AttrBold).
		SetSelectable(false).
		SetExpansion(1)
}

func (at *AgentTable) renderEmptyState() bool {
	if len(at.agents) > 0 {
		return false
	}
	cell := tview.NewTableCell("  No agents found. Press 'n' to spawn one.").
		SetTextColor(colorStopped).
		SetSelectable(false).
		SetExpansion(1)
	at.table.SetCell(1, 0, cell)
	return true
}

func (at *AgentTable) renderGroupRow(row int, visibleCols []int, displayRow tableRow) {
	cell := tview.NewTableCell(displayRow.prefix + "🔄 " + displayRow.groupName).
		SetTextColor(tcell.ColorGreen).
		SetAttributes(tcell.AttrBold).
		SetSelectable(false).
		SetExpansion(1)
	at.table.SetCell(row, 0, cell)
	for ci := 1; ci < len(visibleCols); ci++ {
		at.table.SetCell(row, ci, tview.NewTableCell("").SetSelectable(false))
	}
}

func (at *AgentTable) renderAgentRow(row int, visibleCols []int, displayRow tableRow) {
	agentIdx := displayRow.agentIndex
	agent := at.agents[agentIdx]
	for ci, colIdx := range visibleCols {
		at.table.SetCell(row, ci, at.renderAgentCell(agent, colIdx, displayRow.prefix))
	}
	at.rowToAgent[row] = agentIdx
}

func (at *AgentTable) renderAgentCell(agent Agent, colIdx int, prefix string) *tview.TableCell {
	value := agentCellValue(agent, colIdx, prefix)
	cell := tview.NewTableCell(padRight(value, columns[colIdx].Width)).SetExpansion(1)
	colorAgentCell(cell, agent, colIdx)
	return cell
}

func agentCellValue(agent Agent, colIdx int, prefix string) string {
	value := fieldByColumn(agent, colIdx)
	if colIdx != 0 {
		return value
	}
	return prefix + typeIcon(agent.Type) + value
}

func typeIcon(agentType string) string {
	switch agentType {
	case "claude":
		return "🟠 "
	case "codex":
		return "🔵 "
	default:
		return ""
	}
}

func colorAgentCell(cell *tview.TableCell, agent Agent, colIdx int) {
	switch colIdx {
	case 1:
		colorTypeCell(cell, agent.Type)
	case 8:
		cell.SetTextColor(statusColor(agent))
	case 9:
		cell.SetTextColor(activityColor(agent))
	}
}

func colorTypeCell(cell *tview.TableCell, agentType string) {
	switch agentType {
	case "claude":
		cell.SetTextColor(tcell.ColorOrange)
	case "codex":
		cell.SetTextColor(tcell.ColorDodgerBlue)
	}
}

func statusColor(agent Agent) tcell.Color {
	if agent.Deleting {
		return colorDeleting
	}
	if agent.Running {
		return colorRunning
	}
	return colorExited
}

func activityColor(agent Agent) tcell.Color {
	switch {
	case agent.Deleting:
		return colorDeleting
	case agent.Activity == "Working":
		return colorRunning
	case agent.Activity == "Idle":
		return colorStopped
	case agent.Activity == "Stopped":
		return colorExited
	default:
		return tcell.ColorDefault
	}
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
		for row := range at.rowToAgent {
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
	for row := range at.rowToAgent {
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
