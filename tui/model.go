package main

import (
	"sort"
	"strings"
)

// Agent represents a single safe-agentic container with its metadata and live stats.
type Agent struct {
	Name        string
	Type        string // "claude", "codex", "shell"
	Repo        string
	SSH         string
	Auth        string
	GHAuth      string
	Docker      string
	NetworkMode string
	Status      string
	Running     bool
	Activity    string // "Working", "Idle", "Stopped"
	CPU         string
	Memory      string
	NetIO       string
	PIDs        string
}

// Column defines a table column.
type Column struct {
	Title    string
	Width    int // minimum width; 0 = flexible
	Priority int // lower = dropped first when terminal is narrow
}

var columns = []Column{
	{Title: "NAME", Width: 20, Priority: 100},
	{Title: "TYPE", Width: 6, Priority: 90},
	{Title: "REPO", Width: 15, Priority: 85},
	{Title: "SSH", Width: 3, Priority: 30},
	{Title: "AUTH", Width: 9, Priority: 35},
	{Title: "GH-AUTH", Width: 9, Priority: 25},
	{Title: "DOCKER", Width: 8, Priority: 20},
	{Title: "NETWORK", Width: 10, Priority: 28},
	{Title: "STATUS", Width: 12, Priority: 80},
	{Title: "ACTIVITY", Width: 8, Priority: 95},
	{Title: "CPU", Width: 6, Priority: 70},
	{Title: "MEM", Width: 10, Priority: 65},
	{Title: "NET I/O", Width: 14, Priority: 15},
	{Title: "PIDS", Width: 5, Priority: 40},
}

// SortAgents sorts agents by column index.
func SortAgents(agents []Agent, col int, ascending bool) {
	sort.SliceStable(agents, func(i, j int) bool {
		a, b := fieldByColumn(agents[i], col), fieldByColumn(agents[j], col)
		if ascending {
			return a < b
		}
		return a > b
	})
}

func fieldByColumn(a Agent, col int) string {
	switch col {
	case 0:
		return a.Name
	case 1:
		return a.Type
	case 2:
		return a.Repo
	case 3:
		return a.SSH
	case 4:
		return a.Auth
	case 5:
		return a.GHAuth
	case 6:
		return a.Docker
	case 7:
		return a.NetworkMode
	case 8:
		return a.Status
	case 9:
		return a.Activity
	case 10:
		return a.CPU
	case 11:
		return a.Memory
	case 12:
		return a.NetIO
	case 13:
		return a.PIDs
	default:
		return ""
	}
}


// FilterAgents returns agents matching the filter string (case-insensitive substring on any field).
func FilterAgents(agents []Agent, filter string) []Agent {
	if filter == "" {
		return agents
	}
	filter = strings.ToLower(filter)
	var result []Agent
	for _, a := range agents {
		if matchesFilter(a, filter) {
			result = append(result, a)
		}
	}
	return result
}

func matchesFilter(a Agent, filter string) bool {
	fields := []string{a.Name, a.Type, a.Repo, a.SSH, a.Auth, a.GHAuth, a.Docker, a.NetworkMode, a.Status, a.CPU, a.Memory, a.NetIO, a.PIDs}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), filter) {
			return true
		}
	}
	return false
}

type colEntry struct {
	index    int
	priority int
	width    int
}

// VisibleColumns returns column indices that fit within the given terminal width.
func VisibleColumns(totalWidth int) []int {
	entries := make([]colEntry, len(columns))
	for i, c := range columns {
		entries[i] = colEntry{index: i, priority: c.Priority, width: c.Width}
	}

	// Start with all columns; remove lowest priority until they fit.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].priority > entries[j].priority
	})

	visible := make([]colEntry, len(entries))
	copy(visible, entries)

	for totalUsed(visible) > totalWidth && len(visible) > 1 {
		visible = visible[:len(visible)-1]
	}

	// Return in original column order.
	sort.SliceStable(visible, func(i, j int) bool {
		return visible[i].index < visible[j].index
	})

	result := make([]int, len(visible))
	for i, e := range visible {
		result[i] = e.index
	}
	return result
}

func totalUsed(entries []colEntry) int {
	total := 0
	for _, e := range entries {
		total += e.width + 1 // +1 for separator
	}
	return total
}
