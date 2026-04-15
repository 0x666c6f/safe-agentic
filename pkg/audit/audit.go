package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/config"
)

type Entry struct {
	Timestamp string            `json:"timestamp"`
	Action    string            `json:"action"`
	Container string            `json:"container"`
	Details   map[string]string `json:"details,omitempty"`
}

type Logger struct {
	Path string
}

func DefaultPath() string {
	return config.AuditPath()
}

func (l *Logger) Log(action, container string, details map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0755); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}
	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Action:    action,
		Container: container,
		Details:   details,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func (l *Logger) Read(n int) ([]Entry, error) {
	f, err := os.Open(l.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		all = append(all, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if n > 0 && len(all) > n {
		all = all[len(all)-n:]
	}
	return all, nil
}
