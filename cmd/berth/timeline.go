package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/0x666c6f/berth/pkg/audit"
	"github.com/0x666c6f/berth/pkg/events"
	"github.com/spf13/cobra"
)

type timelineEntry struct {
	Timestamp string
	Type      string
	Status    string
	Container string
	Summary   string
}

var timelineLines int
var inboxAll bool

var timelineCmd = &cobra.Command{
	Use:     "timeline",
	Short:   "Show recent berth events",
	GroupID: groupObserve,
	Args:    cobra.NoArgs,
	RunE:    runTimeline,
}

var inboxCmd = &cobra.Command{
	Use:     "inbox",
	Short:   "Show events that may need attention",
	GroupID: groupObserve,
	Args:    cobra.NoArgs,
	RunE:    runInbox,
}

func init() {
	timelineCmd.Flags().IntVar(&timelineLines, "lines", 50, "Number of recent entries")
	inboxCmd.Flags().BoolVar(&inboxAll, "all", false, "Include informational entries")
	rootCmd.AddCommand(timelineCmd, inboxCmd)
}

func runTimeline(cmd *cobra.Command, args []string) error {
	entries, err := loadTimelineEntries(timelineLines)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No timeline events.")
		return nil
	}
	for _, entry := range entries {
		fmt.Printf("%s\t%s\t%s\t%s\n", entry.Timestamp, entry.Type, entry.Container, entry.Summary)
	}
	return nil
}

func runInbox(cmd *cobra.Command, args []string) error {
	entries, err := loadTimelineEntries(0)
	if err != nil {
		return err
	}

	// Fold in live agent states: an agent blocked on a prompt right now is a
	// needs-attention item even if no event was ever emitted for it.
	if blocked := liveBlockedEntries(context.Background(), newExecutor()); len(blocked) > 0 {
		entries = append(entries, blocked...)
		sort.SliceStable(entries, func(i, j int) bool {
			return entries[i].Timestamp < entries[j].Timestamp
		})
	}

	var items []timelineEntry
	for _, entry := range entries {
		if inboxAll || needsAttention(entry) {
			items = append(items, entry)
		}
	}
	if len(items) == 0 {
		fmt.Println("Inbox empty.")
		return nil
	}
	for _, item := range items {
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", item.Timestamp, item.Status, item.Type, item.Container, item.Summary)
	}
	return nil
}

func loadTimelineEntries(limit int) ([]timelineEntry, error) {
	var entries []timelineEntry

	evs, err := events.Read(events.DefaultEventsPath(), 0)
	if err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}
	for _, event := range evs {
		entries = append(entries, timelineEntry{
			Timestamp: event.Timestamp,
			Type:      event.Type,
			Status:    events.Classify(event),
			Container: event.Payload["container"],
			Summary:   payloadSummary(event.Payload),
		})
	}

	logger := &audit.Logger{Path: audit.DefaultPath()}
	auditEntries, err := logger.Read(0)
	if err != nil {
		return nil, fmt.Errorf("read audit: %w", err)
	}
	for _, entry := range auditEntries {
		entries = append(entries, timelineEntry{
			Timestamp: entry.Timestamp,
			Type:      "audit." + entry.Action,
			Status:    events.ClassifyFields("audit."+entry.Action, entry.Details),
			Container: entry.Container,
			Summary:   payloadSummary(entry.Details),
		})
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}

func needsAttention(entry timelineEntry) bool {
	return events.NeedsAttentionStatus(entry.Status)
}

func payloadSummary(payload map[string]string) string {
	if len(payload) == 0 {
		return ""
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range keys {
		if payload[key] == "" {
			continue
		}
		parts = append(parts, key+"="+payload[key])
	}
	return strings.Join(parts, " ")
}
