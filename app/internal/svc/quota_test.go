package svc

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// rollout lines nest rate_limits inside an event payload — the parser must find
// it regardless of depth and take the LAST one in the file.
func TestLatestCodexRateLimits(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions", "2026", "07", "06")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// two rate_limits lines; the second (newer) must win. Nested under msg.payload.
	lines := `{"type":"event","payload":{"msg":{"rate_limits":{"primary":{"used_percent":10,"resets_at":111},"secondary":{"used_percent":20,"resets_at":222}}}}}
{"type":"other","payload":{"nothing":true}}
{"type":"event","payload":{"msg":{"rate_limits":{"primary":{"used_percent":78,"resets_at":333},"secondary":{"used_percent":84,"resets_at":444}}}}}
`
	if err := os.WriteFile(filepath.Join(dir, "rollout-a.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	rl := latestCodexRateLimits(home)
	if rl == nil || rl.Primary == nil || rl.Secondary == nil {
		t.Fatalf("expected rate limits, got %+v", rl)
	}
	if rl.Primary.UsedPercent != 78 || rl.Primary.ResetsAt != 333 {
		t.Errorf("primary: want 78%%/333, got %v%%/%d", rl.Primary.UsedPercent, rl.Primary.ResetsAt)
	}
	if rl.Secondary.UsedPercent != 84 || rl.Secondary.ResetsAt != 444 {
		t.Errorf("secondary: want 84%%/444, got %v%%/%d", rl.Secondary.UsedPercent, rl.Secondary.ResetsAt)
	}
}

func TestLatestCodexRateLimitsNewestFileWins(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "rollout-old.jsonl")
	newer := filepath.Join(dir, "rollout-new.jsonl")
	write := func(p string, pct float64) {
		l := `{"payload":{"rate_limits":{"primary":{"used_percent":` +
			itoa(pct) + `,"resets_at":1}}}}` + "\n"
		if err := os.WriteFile(p, []byte(l), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(old, 5)
	write(newer, 50)
	// make `newer` the most recently modified
	past := time.Now().Add(-time.Hour)
	os.Chtimes(old, past, past)

	rl := latestCodexRateLimits(home)
	if rl == nil || rl.Primary == nil || rl.Primary.UsedPercent != 50 {
		t.Fatalf("expected newest file (50%%), got %+v", rl)
	}
}

// A fresh cache must be served without touching the network (the endpoint
// rate-limits, so this is the guarantee that matters).
func TestClaudeQuotaServesFreshCache(t *testing.T) {
	q := &QuotaService{}
	q.claudeCache = Quota{Agent: "claude", OK: true, Windows: []QuotaWindow{{Label: "5h", Percent: 42}}}
	q.claudeAt = time.Now()
	got := q.claude()
	if !got.OK || len(got.Windows) != 1 || got.Windows[0].Percent != 42 {
		t.Fatalf("expected the cached value, got %+v", got)
	}
}

// A disk cache written within the TTL must be served without the network, so an
// app restart doesn't re-hit the rate-limited usage endpoint.
func TestClaudeQuotaServesDiskCache(t *testing.T) {
	q := &QuotaService{Home: t.TempDir()}
	want := Quota{Agent: "claude", OK: true, Windows: []QuotaWindow{{Label: "5h", Percent: 33}}}
	q.writeClaudeDisk(want)
	got := q.claude() // no in-memory cache → must load from disk, no network
	if !got.OK || len(got.Windows) != 1 || got.Windows[0].Percent != 33 {
		t.Fatalf("expected disk-cached value, got %+v", got)
	}
}

func TestUnixFromRFC(t *testing.T) {
	got := unixFromRFC("2026-07-06T17:19:59+00:00")
	if got == 0 {
		t.Fatal("expected a parsed unix time")
	}
	if unixFromRFC("") != 0 || unixFromRFC("garbage") != 0 {
		t.Fatal("empty/invalid should be 0")
	}
}

func itoa(f float64) string {
	// small helper: whole-number percents only in these fixtures
	return string(rune('0'+int(f)/10)) + string(rune('0'+int(f)%10))
}
