package reviewcomments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreAddListResolveAndClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review-comments.jsonl")
	store := Store{Path: path}

	comment, err := store.Add(Comment{
		Agent: "agent-claude-test",
		File:  "cmd/main.go",
		Line:  12,
		Body:  "tighten this branch",
	})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if comment.ID == "" || comment.CreatedAt == "" {
		t.Fatalf("comment missing generated fields: %#v", comment)
	}

	comments, err := store.List(Filter{Agent: "agent-claude-test"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(comments) != 1 || comments[0].Body != "tighten this branch" {
		t.Fatalf("comments = %#v", comments)
	}

	resolved, err := store.Resolve(comment.ID)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !resolved.Resolved || resolved.ResolvedAt == "" {
		t.Fatalf("resolved comment = %#v", resolved)
	}

	open, err := store.List(Filter{Agent: "agent-claude-test"})
	if err != nil {
		t.Fatalf("List(open) error = %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("open comments = %#v, want none", open)
	}
	all, err := store.List(Filter{Agent: "agent-claude-test", IncludeResolved: true})
	if err != nil {
		t.Fatalf("List(all) error = %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("all comments len = %d, want 1", len(all))
	}

	removed, err := store.ClearAgent("agent-claude-test")
	if err != nil {
		t.Fatalf("ClearAgent() error = %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	comments, err = store.List(Filter{IncludeResolved: true})
	if err != nil {
		t.Fatalf("List(after clear) error = %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments after clear = %#v", comments)
	}
}

func TestStoreRejectsInvalidComment(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "review-comments.jsonl")}
	_, err := store.Add(Comment{Agent: "agent", File: "file.go", Line: 0, Body: "no"})
	if err == nil || !strings.Contains(err.Error(), "line") {
		t.Fatalf("Add() error = %v, want line error", err)
	}
}

func TestStorePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "review-comments.jsonl")
	store := Store{Path: path}
	if _, err := store.Add(Comment{Agent: "agent", File: "file.go", Line: 1, Body: "note"}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat comments: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode = %v, want 0600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat comments dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir mode = %v, want 0700", got)
	}
}
