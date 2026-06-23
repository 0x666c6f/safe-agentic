package reviewcomments

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/config"
)

type Comment struct {
	ID         string `json:"id"`
	CreatedAt  string `json:"created_at"`
	ResolvedAt string `json:"resolved_at,omitempty"`
	Agent      string `json:"agent"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Body       string `json:"body"`
	Resolved   bool   `json:"resolved"`
}

type Store struct {
	Path string
}

type Filter struct {
	Agent           string
	IncludeResolved bool
}

func DefaultPath() string {
	return filepath.Join(config.StateDir(), "review-comments.jsonl")
}

func (s Store) Add(comment Comment) (Comment, error) {
	if err := validate(comment); err != nil {
		return Comment{}, err
	}
	now := time.Now().UTC()
	comment.ID = fmt.Sprintf("rc-%d", now.UnixNano())
	comment.CreatedAt = now.Format(time.RFC3339)
	comment.Resolved = false
	comment.ResolvedAt = ""

	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return Comment{}, fmt.Errorf("create review comment dir: %w", err)
	}
	data, err := json.Marshal(comment)
	if err != nil {
		return Comment{}, fmt.Errorf("marshal review comment: %w", err)
	}
	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Comment{}, fmt.Errorf("open review comments: %w", err)
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return Comment{}, fmt.Errorf("chmod review comments: %w", err)
	}
	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		return Comment{}, fmt.Errorf("write review comment: %w", err)
	}
	return comment, nil
}

func (s Store) List(filter Filter) ([]Comment, error) {
	comments, err := s.readAll()
	if err != nil {
		return nil, err
	}
	var result []Comment
	for _, comment := range comments {
		if filter.Agent != "" && comment.Agent != filter.Agent {
			continue
		}
		if !filter.IncludeResolved && comment.Resolved {
			continue
		}
		result = append(result, comment)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CreatedAt < result[j].CreatedAt
	})
	return result, nil
}

func (s Store) Resolve(id string) (Comment, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Comment{}, fmt.Errorf("comment id is required")
	}
	comments, err := s.readAll()
	if err != nil {
		return Comment{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var resolved Comment
	found := false
	for i := range comments {
		if comments[i].ID != id {
			continue
		}
		comments[i].Resolved = true
		comments[i].ResolvedAt = now
		resolved = comments[i]
		found = true
		break
	}
	if !found {
		return Comment{}, fmt.Errorf("review comment %q not found", id)
	}
	if err := s.writeAll(comments); err != nil {
		return Comment{}, err
	}
	return resolved, nil
}

func (s Store) ClearAgent(agent string) (int, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return 0, fmt.Errorf("agent is required")
	}
	comments, err := s.readAll()
	if err != nil {
		return 0, err
	}
	kept := comments[:0]
	removed := 0
	for _, comment := range comments {
		if comment.Agent == agent {
			removed++
			continue
		}
		kept = append(kept, comment)
	}
	if err := s.writeAll(kept); err != nil {
		return 0, err
	}
	return removed, nil
}

func (s Store) readAll() ([]Comment, error) {
	f, err := os.Open(s.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open review comments: %w", err)
	}
	defer f.Close()

	var comments []Comment
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var comment Comment
		if err := json.Unmarshal([]byte(line), &comment); err != nil {
			return nil, fmt.Errorf("parse review comment: %w", err)
		}
		comments = append(comments, comment)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read review comments: %w", err)
	}
	return comments, nil
}

func (s Store) writeAll(comments []Comment) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return fmt.Errorf("create review comment dir: %w", err)
	}
	var b strings.Builder
	for _, comment := range comments {
		data, err := json.Marshal(comment)
		if err != nil {
			return fmt.Errorf("marshal review comment: %w", err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(s.Path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write review comments: %w", err)
	}
	return nil
}

func validate(comment Comment) error {
	if strings.TrimSpace(comment.Agent) == "" {
		return fmt.Errorf("agent is required")
	}
	if strings.TrimSpace(comment.File) == "" {
		return fmt.Errorf("file is required")
	}
	if comment.Line < 1 {
		return fmt.Errorf("line must be >= 1")
	}
	if strings.TrimSpace(comment.Body) == "" {
		return fmt.Errorf("body is required")
	}
	for field, value := range map[string]string{
		"agent": comment.Agent,
		"file":  comment.File,
		"body":  comment.Body,
	} {
		if strings.Contains(value, "\x00") {
			return fmt.Errorf("%s contains NUL byte", field)
		}
	}
	return nil
}
