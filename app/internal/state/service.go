package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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
	ProjectsPath                        string
	pmu                                 sync.Mutex
}

func NewService() *Service {
	return &Service{
		AuditPath:    config.AuditPath(),
		EventsPath:   config.EventsPath(),
		PipelinesDir: config.PipelinesDir(),
		ProjectsPath: filepath.Join(config.UserDir(), "app-projects.json"),
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

// --- Projects: saved repos for quick chat launches, most-used first. ---

type Project struct {
	URL   string `json:"url"`
	Count int    `json:"count"`
	Last  int64  `json:"last"`
}

func (s *Service) loadProjects() []Project {
	data, err := os.ReadFile(s.ProjectsPath)
	if err != nil {
		return nil
	}
	var out []Project
	if json.Unmarshal(data, &out) != nil {
		return nil
	}
	return out
}

func (s *Service) saveProjects(list []Project) error {
	if err := os.MkdirAll(filepath.Dir(s.ProjectsPath), 0o755); err != nil {
		return err
	}
	data, _ := json.Marshal(list)
	// ponytail: direct write, no tmp+rename — single-process, tiny file
	return os.WriteFile(s.ProjectsPath, data, 0o600)
}

// Projects returns saved repos sorted by use count desc, then recency desc.
func (s *Service) Projects() []Project {
	s.pmu.Lock()
	defer s.pmu.Unlock()
	list := s.loadProjects()
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return list[i].Last > list[j].Last
	})
	return list
}

// ProjectUse bumps the use count, adding the repo if new.
func (s *Service) ProjectUse(url string) error {
	u := strings.TrimSpace(url)
	if u == "" {
		return nil
	}
	s.pmu.Lock()
	defer s.pmu.Unlock()
	list := s.loadProjects()
	for i := range list {
		if list[i].URL == u {
			list[i].Count++
			list[i].Last = time.Now().Unix()
			return s.saveProjects(list)
		}
	}
	return s.saveProjects(append(list, Project{URL: u, Count: 1, Last: time.Now().Unix()}))
}

func (s *Service) ProjectRemove(url string) error {
	s.pmu.Lock()
	defer s.pmu.Unlock()
	list := s.loadProjects()
	out := list[:0]
	for _, p := range list {
		if p.URL != url {
			out = append(out, p)
		}
	}
	return s.saveProjects(out)
}

// ShortRepoName renders "org/repo" from common git URL shapes.
func ShortRepoName(url string) string {
	u := strings.TrimSuffix(url, ".git")
	for _, p := range []string{"git@github.com:", "https://github.com/", "http://github.com/"} {
		u = strings.TrimPrefix(u, p)
	}
	return u
}
