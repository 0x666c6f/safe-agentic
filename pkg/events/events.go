package events

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Event struct {
	Timestamp string            `json:"timestamp"`
	Type      string            `json:"type"`
	Payload   map[string]string `json:"payload"`
}

func Emit(path, eventType string, payload map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	event := Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Type:      eventType,
		Payload:   payload,
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func DefaultEventsPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = home + "/.config"
	}
	return filepath.Join(dir, "safe-agentic", "events.jsonl")
}
