package state

import (
	"os"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/audit"
	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/events"
)

type EventItem struct {
	events.Event
	Status    string `json:"status"`
	Container string `json:"container"`
}

type Service struct {
	AuditPath, EventsPath, PipelinesDir string
}

func NewService() *Service {
	return &Service{
		AuditPath:    config.AuditPath(),
		EventsPath:   config.EventsPath(),
		PipelinesDir: config.PipelinesDir(),
	}
}

func (s *Service) AuditTail(n int) ([]audit.Entry, error) {
	l := &audit.Logger{Path: s.AuditPath}
	return l.Read(n)
}

func (s *Service) EventsTail(n int) ([]EventItem, error) {
	evs, err := events.Read(s.EventsPath, n)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	items := make([]EventItem, 0, len(evs))
	for _, e := range evs {
		items = append(items, EventItem{
			Event:     e,
			Status:    events.Classify(e),
			Container: e.Payload["container"],
		})
	}
	return items, nil
}

func (s *Service) Inbox(n int) ([]EventItem, error) {
	items, err := s.EventsTail(n)
	if err != nil {
		return nil, err
	}
	var out []EventItem
	for _, it := range items {
		if events.NeedsAttentionStatus(it.Status) {
			out = append(out, it)
		}
	}
	return out, nil
}

func (s *Service) PipelineFiles() ([]string, error) {
	entries, err := os.ReadDir(s.PipelinesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			// Bare names: safe-ag's catalog resolves "<name>" as
			// <PipelinesDir>/<name>.{yaml,yml}; passing "foo.yaml" would
			// probe for foo.yaml.yaml and fail.
			name = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
			out = append(out, name)
		}
	}
	return out, nil
}
