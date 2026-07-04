package poll

import "testing"

func TestParsePS(t *testing.T) {
	line1 := "agent-claude-1\tclaude\torg/repo\ton\tper-container\toff\toff\tdedicated\tmyfleet\tpipeline/stage1\ttmux\tUp 2 hours"
	line2 := "agent-codex-2\tcodex\t\toff\t\toff\toff\t\t\t\ttmux\tExited (0) 3 minutes ago"
	agents := ParsePS([]byte(line1 + "\n" + line2 + "\n"))
	if len(agents) != 2 {
		t.Fatalf("want 2 agents, got %d", len(agents))
	}
	a := agents[0]
	if a.Name != "agent-claude-1" || a.Type != "claude" || a.Repo != "org/repo" ||
		a.Fleet != "myfleet" || a.Hierarchy != "pipeline/stage1" || a.Terminal != "tmux" {
		t.Fatalf("bad parse: %+v", a)
	}
	if !a.Running || a.Finished {
		t.Fatalf("line1 should be running: %+v", a)
	}
	b := agents[1]
	if b.Running || !b.Finished {
		t.Fatalf("line2 should be finished: %+v", b)
	}
}

func TestParsePSSkipsMalformed(t *testing.T) {
	if got := ParsePS([]byte("too\tfew\tfields\n\n")); len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}
}

func TestPSFormatFieldCount(t *testing.T) {
	// 12 tab-separated template fields — must match ParsePS expectations.
	if n := len(splitFormat(PSFormat())); n != 12 {
		t.Fatalf("want 12 fields, got %d", n)
	}
}
