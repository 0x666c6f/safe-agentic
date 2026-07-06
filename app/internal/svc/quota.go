package svc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/inject"
)

// claudeTTL bounds how often the Claude usage endpoint is called — it
// rate-limits, so a short frontend poll must not translate into a request each
// time. Quotas move slowly; 5 minutes is plenty.
const claudeTTL = 5 * time.Minute

// QuotaService reports remaining subscription quota for the host's Claude and
// Codex logins. Both agents expose a 5-hour window and a weekly window as
// used-percent; the app renders them as bars.
type QuotaService struct {
	Home string // home dir override (tests); empty → os.UserHomeDir

	mu          sync.Mutex // guards the Claude usage cache
	claudeCache Quota
	claudeAt    time.Time
}

// QuotaWindow is one rate-limit window (used percent + reset time).
type QuotaWindow struct {
	Label    string  `json:"label"`    // "5h" | "week"
	Percent  float64 `json:"percent"`  // used, 0..100
	ResetsAt int64   `json:"resetsAt"` // unix seconds; 0 if unknown
}

// Quota is one agent's quota, or an Error explaining why it's unavailable.
type Quota struct {
	Agent   string        `json:"agent"` // "claude" | "codex"
	OK      bool          `json:"ok"`
	Error   string        `json:"error"`
	Windows []QuotaWindow `json:"windows"`
}

func (q *QuotaService) home() string {
	if q.Home != "" {
		return q.Home
	}
	h, _ := os.UserHomeDir()
	return h
}

// Quotas returns Claude then Codex quota. Never errors: a per-agent failure is
// carried in that Quota's Error so the other still renders.
func (q *QuotaService) Quotas() []Quota {
	return []Quota{q.claude(), q.codex()}
}

// claude returns the Claude quota, hitting the network at most once per
// claudeTTL — and the cache is persisted to disk so an app restart within the
// window reuses it instead of re-calling (the usage endpoint rate-limits).
// On a fetch failure it keeps serving the last good numbers and won't retry
// until the TTL elapses, so a 429 never gets hammered.
func (q *QuotaService) claude() Quota {
	q.mu.Lock()
	defer q.mu.Unlock()
	// in-memory cache fresh?
	if q.claudeCache.Agent == "claude" && time.Since(q.claudeAt) < claudeTTL {
		return q.claudeCache
	}
	// disk cache fresh? (survives restarts — the main reason it kept 429ing)
	if cached, at, ok := q.readClaudeDisk(); ok && time.Since(at) < claudeTTL {
		q.claudeCache, q.claudeAt = cached, at
		return cached
	}
	fresh := q.fetchClaude()
	if !fresh.OK {
		// Failure (e.g. 429): serve the last good value from memory or disk if we
		// have one, and stamp now so we don't retry until the TTL elapses.
		q.claudeAt = time.Now()
		if q.claudeCache.OK {
			return q.claudeCache
		}
		if cached, _, ok := q.readClaudeDisk(); ok && cached.OK {
			q.claudeCache = cached
			return cached
		}
		q.claudeCache = fresh
		return fresh
	}
	q.claudeCache, q.claudeAt = fresh, time.Now()
	q.writeClaudeDisk(fresh)
	return fresh
}

func (q *QuotaService) claudeCachePath() string {
	return filepath.Join(q.home(), ".safe-ag", "claude-usage-cache.json")
}

type claudeDiskCache struct {
	Quota     Quota `json:"quota"`
	FetchedAt int64 `json:"fetched_at"`
}

func (q *QuotaService) readClaudeDisk() (Quota, time.Time, bool) {
	data, err := os.ReadFile(q.claudeCachePath())
	if err != nil {
		return Quota{}, time.Time{}, false
	}
	var c claudeDiskCache
	if json.Unmarshal(data, &c) != nil || c.Quota.Agent != "claude" {
		return Quota{}, time.Time{}, false
	}
	return c.Quota, time.Unix(c.FetchedAt, 0), true
}

func (q *QuotaService) writeClaudeDisk(quota Quota) {
	data, err := json.Marshal(claudeDiskCache{Quota: quota, FetchedAt: time.Now().Unix()})
	if err != nil {
		return
	}
	path := q.claudeCachePath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, data, 0o600)
}

// fetchClaude hits the OAuth usage endpoint with the host's live token
// (keychain, kept fresh by Claude Code) — the same data Claude Code's /usage
// shows.
func (q *QuotaService) fetchClaude() Quota {
	r := Quota{Agent: "claude"}
	m, _ := inject.ReadClaudeOAuthToken()
	token := m["CLAUDE_CODE_OAUTH_TOKEN"]
	if token == "" {
		r.Error = "not logged in"
		return r
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.Error = "usage request failed"
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			r.Error = "rate limited — retrying"
		} else {
			r.Error = fmt.Sprintf("usage http %d", resp.StatusCode)
		}
		return r
	}
	var u struct {
		FiveHour struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay struct {
			Utilization float64 `json:"utilization"`
			ResetsAt    string  `json:"resets_at"`
		} `json:"seven_day"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		r.Error = "usage parse failed"
		return r
	}
	r.OK = true
	r.Windows = []QuotaWindow{
		{Label: "5h", Percent: u.FiveHour.Utilization, ResetsAt: unixFromRFC(u.FiveHour.ResetsAt)},
		{Label: "week", Percent: u.SevenDay.Utilization, ResetsAt: unixFromRFC(u.SevenDay.ResetsAt)},
	}
	return r
}

// codex reads the newest session rollout under ~/.codex/sessions and pulls the
// last rate_limits snapshot the Codex server sent — a reliable local source.
func (q *QuotaService) codex() Quota {
	r := Quota{Agent: "codex"}
	rl := latestCodexRateLimits(q.home())
	if rl == nil || (rl.Primary == nil && rl.Secondary == nil) {
		r.Error = "no recent session"
		return r
	}
	r.OK = true
	if rl.Primary != nil {
		r.Windows = append(r.Windows, QuotaWindow{Label: "5h", Percent: rl.Primary.UsedPercent, ResetsAt: rl.Primary.ResetsAt})
	}
	if rl.Secondary != nil {
		r.Windows = append(r.Windows, QuotaWindow{Label: "week", Percent: rl.Secondary.UsedPercent, ResetsAt: rl.Secondary.ResetsAt})
	}
	return r
}

func unixFromRFC(s string) int64 {
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix()
	}
	return 0
}

type codexWindow struct {
	UsedPercent float64 `json:"used_percent"`
	ResetsAt    int64   `json:"resets_at"`
}

type codexRateLimits struct {
	Primary   *codexWindow `json:"primary"`
	Secondary *codexWindow `json:"secondary"`
}

// latestCodexRateLimits walks the session dir newest-first and returns the last
// rate_limits found in the first rollout that has any (caps the scan so a burst
// of empty just-started sessions doesn't hide the real data).
func latestCodexRateLimits(home string) *codexRateLimits {
	root := filepath.Join(home, ".codex", "sessions")
	type ent struct {
		path string
		mod  time.Time
	}
	var files []ent
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jsonl") {
			return nil
		}
		if info, e := d.Info(); e == nil {
			files = append(files, ent{p, info.ModTime()})
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	for i, f := range files {
		if i >= 8 {
			break
		}
		if rl := scanRateLimits(f.path); rl != nil {
			return rl
		}
	}
	return nil
}

// scanRateLimits returns the last rate_limits object anywhere in a rollout file.
// It prefilters lines by substring, then recursively finds the key (rate_limits
// nests inside an event payload whose exact shape we don't want to hard-code).
func scanRateLimits(path string) *codexRateLimits {
	fh, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // rollout lines can be large
	var last *codexRateLimits
	for sc.Scan() {
		line := sc.Bytes()
		if !strings.Contains(string(line), "rate_limits") {
			continue
		}
		var generic any
		if json.Unmarshal(line, &generic) != nil {
			continue
		}
		if rl := findRateLimits(generic); rl != nil {
			last = rl
		}
	}
	return last
}

func findRateLimits(v any) *codexRateLimits {
	switch t := v.(type) {
	case map[string]any:
		if raw, ok := t["rate_limits"]; ok {
			b, _ := json.Marshal(raw)
			var out codexRateLimits
			if json.Unmarshal(b, &out) == nil && (out.Primary != nil || out.Secondary != nil) {
				return &out
			}
		}
		for _, sub := range t {
			if r := findRateLimits(sub); r != nil {
				return r
			}
		}
	case []any:
		for _, sub := range t {
			if r := findRateLimits(sub); r != nil {
				return r
			}
		}
	}
	return nil
}
