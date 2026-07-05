package state

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEventsTailClassifies(t *testing.T) {
	dir := t.TempDir()
	ev := filepath.Join(dir, "events.jsonl")
	writeFile(t, ev,
		`{"timestamp":"2026-07-04T10:00:00Z","type":"agent.exit","payload":{"container":"agent-x","status":"needs-auth"}}`+"\n"+
			`{"timestamp":"2026-07-04T10:01:00Z","type":"agent.exit","payload":{"container":"agent-y","status":"info"}}`+"\n")
	s := &Service{EventsPath: ev}
	items, err := s.EventsTail(10)
	if err != nil || len(items) != 2 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	if items[0].Status != "needs-auth" || items[0].Container != "agent-x" {
		t.Fatalf("bad classify: %+v", items[0])
	}
	inbox, err := s.Inbox(10)
	if err != nil || len(inbox) != 1 || inbox[0].Container != "agent-x" {
		t.Fatalf("inbox=%v err=%v", inbox, err)
	}
}

func TestPipelineFilesMissingDir(t *testing.T) {
	s := &Service{PipelinesDir: filepath.Join(t.TempDir(), "nope")}
	files, err := s.PipelineFiles()
	if err != nil || len(files) != 0 {
		t.Fatalf("files=%v err=%v", files, err)
	}
}

func TestPipelineFilesStripsExtensions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "review.yaml"), "steps: []\n")
	writeFile(t, filepath.Join(dir, "fix.yml"), "steps: []\n")
	writeFile(t, filepath.Join(dir, "notes.txt"), "ignore me\n")
	s := &Service{PipelinesDir: dir}
	files, err := s.PipelineFiles()
	if err != nil || len(files) != 2 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	for _, f := range files {
		if f != "review" && f != "fix" {
			t.Fatalf("extension not stripped: %q", f)
		}
	}
}

func TestProjectsSortAndRemove(t *testing.T) {
	s := &Service{ProjectsPath: filepath.Join(t.TempDir(), "p.json")}
	s.ProjectUse("git@github.com:o/a.git")
	s.ProjectUse("git@github.com:o/b.git")
	s.ProjectUse("git@github.com:o/b.git")
	s.ProjectUse(" ")
	list := s.Projects()
	if len(list) != 2 || list[0].URL != "git@github.com:o/b.git" || list[0].Count != 2 {
		t.Fatalf("list=%+v", list)
	}
	s.ProjectRemove("git@github.com:o/b.git")
	if l := s.Projects(); len(l) != 1 {
		t.Fatalf("remove failed: %+v", l)
	}
	if got := ShortRepoName("https://github.com/o/repo.git"); got != "o/repo" {
		t.Fatalf("short name: %q", got)
	}
}
