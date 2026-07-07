package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0x666c6f/berth/pkg/config"
)

type Event struct {
	Timestamp string            `json:"timestamp"`
	Type      string            `json:"type"`
	Payload   map[string]string `json:"payload"`
}

func Emit(path, eventType string, payload map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
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
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func DefaultEventsPath() string {
	return config.EventsPath()
}

func Read(path string, n int) ([]Event, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var all []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		all = append(all, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
