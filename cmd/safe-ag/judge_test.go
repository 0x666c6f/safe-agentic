package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0x666c6f/safe-agentic/pkg/fleet"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
)

// ─── pure: JSON extraction & verdict parsing ───────────────────────────────────

func TestExtractJSONObjects(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"plain", `{"a":1}`, []string{`{"a":1}`}},
		{"embedded in prose", "Here is my verdict:\n{\"winner\":\"x\"}\nThanks!", []string{`{"winner":"x"}`}},
		{
			name: "markdown fence",
			in:   "```json\n{\"winner\":\"c1\",\"reason\":\"ok\"}\n```",
			want: []string{`{"winner":"c1","reason":"ok"}`},
		},
		{"brace in string ignored", `{"reason":"use {braces} here"}`, []string{`{"reason":"use {braces} here"}`}},
		{"nested", `{"a":{"b":2}}`, []string{`{"a":{"b":2}}`}},
		{"two objects", `noise {"a":1} more {"b":2} end`, []string{`{"a":1}`, `{"b":2}`}},
		{"none", `no json here`, nil},
		{"escaped quote in string", `{"reason":"say \"hi\" now"}`, []string{`{"reason":"say \"hi\" now"}`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSONObjects(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("extractJSONObjects(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("object[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseVerdict(t *testing.T) {
	candidates := []string{"agent-claude-a", "agent-codex-b"}
	tests := []struct {
		name       string
		raw        string
		wantWinner string
		wantErr    string
	}{
		{
			name:       "valid",
			raw:        `{"winner":"agent-claude-a","reason":"cleaner","summary":"Adds X"}`,
			wantWinner: "agent-claude-a",
		},
		{
			name:       "embedded in noise",
			raw:        "I reviewed both.\n```json\n{\"winner\":\"agent-codex-b\",\"reason\":\"tests\",\"summary\":\"Fix Y\"}\n```\nDone.",
			wantWinner: "agent-codex-b",
		},
		{
			name:    "no json",
			raw:     "agent-claude-a is best because it is cleaner",
			wantErr: "no JSON object found",
		},
		{
			name:    "winner not a candidate",
			raw:     `{"winner":"agent-claude-ghost","reason":"x","summary":"y"}`,
			wantErr: "not one of the candidates",
		},
		{
			name:       "skips prose object picks valid verdict",
			raw:        `{"note":"thinking"} then {"winner":"agent-claude-a","reason":"r","summary":"s"}`,
			wantWinner: "agent-claude-a",
		},
		{
			name:       "malformed then valid",
			raw:        `{"winner": } {"winner":"agent-codex-b","reason":"r","summary":"s"}`,
			wantWinner: "agent-codex-b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := parseVerdict(tt.raw, candidates)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseVerdict() err = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVerdict() unexpected err = %v", err)
			}
			if v.Winner != tt.wantWinner {
				t.Fatalf("winner = %q, want %q", v.Winner, tt.wantWinner)
			}
		})
	}
}

// ─── pure: prompt construction & truncation ────────────────────────────────────

func TestBuildJudgePrompt(t *testing.T) {
	candidates := []JudgeCandidate{
		{Name: "agent-claude-a", Agent: "claude", Diff: "diff --git a b", Message: "did A"},
		{Name: "agent-codex-b", Agent: "codex", Diff: "diff --git c d", Message: "did B"},
	}
	prompt := buildJudgePrompt("my-pipeline / pick", candidates, "correctness first", defaultJudgeDiffCap)

	for _, want := range []string{
		"my-pipeline / pick",
		"correctness first",
		"agent-claude-a",
		"agent-codex-b",
		"did A",
		"diff --git a b",
		`{"winner":`,
		"MUST be exactly one of these container names",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildJudgePromptDefaultCriteria(t *testing.T) {
	prompt := buildJudgePrompt("ctx", []JudgeCandidate{
		{Name: "a"}, {Name: "b"},
	}, "  ", defaultJudgeDiffCap)
	if !strings.Contains(prompt, "correctness first") {
		t.Fatalf("expected default criteria in prompt:\n%s", prompt)
	}
	// empty message/diff placeholders
	if !strings.Contains(prompt, "no final message captured") {
		t.Fatalf("expected empty-message placeholder:\n%s", prompt)
	}
	if !strings.Contains(prompt, "empty diff") {
		t.Fatalf("expected empty-diff placeholder:\n%s", prompt)
	}
}

func TestTruncateForPrompt(t *testing.T) {
	if got := truncateForPrompt("short", 100); got != "short" {
		t.Fatalf("under cap should be unchanged, got %q", got)
	}
	long := strings.Repeat("x", 500)
	got := truncateForPrompt(long, 100)
	if !strings.Contains(got, "truncated 400 bytes") {
		t.Fatalf("expected truncation note, got tail: %q", got[len(got)-40:])
	}
	if len(got) >= len(long) {
		t.Fatalf("expected shorter output, got len=%d", len(got))
	}
}

func TestTruncateForPromptRuneBoundary(t *testing.T) {
	// A string of 3-byte runes; cap mid-rune must not split a rune.
	s := strings.Repeat("€", 50) // 150 bytes
	got := truncateForPrompt(s, 100)
	head := strings.TrimSuffix(got, got[strings.Index(got, "\n…"):])
	if len(head)%3 != 0 {
		t.Fatalf("truncation split a multibyte rune: head len %d not multiple of 3", len(head))
	}
	if !strings.HasPrefix(s, head) {
		t.Fatalf("truncated head is not a prefix of the input")
	}
}

// ─── persistence ───────────────────────────────────────────────────────────────

func TestPersistJudgeRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	record := judgeRecord{
		Stage:      "pick",
		Pipeline:   "best-of-two",
		Timestamp:  "20260101-000000",
		Candidates: []string{"agent-claude-a", "agent-codex-b"},
		Verdict:    JudgeVerdict{Winner: "agent-claude-a", Reason: "cleaner", Summary: "Adds X"},
	}
	path, err := persistJudgeRecord(record)
	if err != nil {
		t.Fatalf("persistJudgeRecord() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat verdict file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("verdict file mode = %o, want 600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read verdict file: %v", err)
	}
	var round judgeRecord
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("verdict file is not valid JSON: %v", err)
	}
	if round.Verdict.Winner != "agent-claude-a" {
		t.Fatalf("round-tripped winner = %q", round.Verdict.Winner)
	}
	if !strings.HasPrefix(filepath.Base(path), "best-of-two-pick-20260101-000000") {
		t.Fatalf("unexpected verdict filename: %s", filepath.Base(path))
	}
}

// ─── candidate collection (running container path) ─────────────────────────────

func TestCollectJudgeCandidatesRunning(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker inspect --format {{.State.Running}} agent-claude-a", "true")
	fake.SetResponse("docker inspect --format {{.State.Running}} agent-codex-b", "true")
	fake.SetResponse("docker inspect --format {{index .Config.Labels", "claude") // label lookups
	// Running-container diffs run as `docker exec <name> bash -c "<find> && git diff"`.
	fake.SetResponse("docker exec agent-claude-a bash -c", "diff --git a/f b/f\n+change")
	fake.SetResponse("docker exec agent-codex-b bash -c", "diff --git c/f d/f\n+change")

	origRead := judgeReadOutput
	defer func() { judgeReadOutput = origRead }()
	judgeReadOutput = func(_ context.Context, _ vmexec.Executor, name string) string {
		return "final message for " + name
	}

	cands, err := collectJudgeCandidatesImpl(context.Background(), fake, []string{"agent-claude-a", "agent-codex-b"}, "main")
	if err != nil {
		t.Fatalf("collectJudgeCandidatesImpl() error = %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(cands))
	}
	if !strings.Contains(cands[0].Diff, "change") {
		t.Fatalf("candidate[0] diff = %q", cands[0].Diff)
	}
	if cands[1].Message != "final message for agent-codex-b" {
		t.Fatalf("candidate[1] message = %q", cands[1].Message)
	}
}

// ─── execution flow (seams stubbed) ────────────────────────────────────────────

type judgeSeams struct {
	spawned  []SpawnOpts
	prCalls  []string
	readBack string
}

func stubJudgeSeams(t *testing.T, s *judgeSeams, candidates []JudgeCandidate) {
	t.Helper()
	origSpawn := judgeSpawn
	origRead := judgeReadOutput
	origCollect := judgeCollectCandidates
	origPR := judgeCreatePR
	t.Cleanup(func() {
		judgeSpawn = origSpawn
		judgeReadOutput = origRead
		judgeCollectCandidates = origCollect
		judgeCreatePR = origPR
	})
	judgeSpawn = func(opts SpawnOpts) error {
		s.spawned = append(s.spawned, opts)
		return nil
	}
	judgeReadOutput = func(_ context.Context, _ vmexec.Executor, _ string) string {
		return s.readBack
	}
	judgeCollectCandidates = func(_ context.Context, _ vmexec.Executor, _ []string, _ string) ([]JudgeCandidate, error) {
		return candidates, nil
	}
	judgeCreatePR = func(_ context.Context, _ vmexec.Executor, winner, branch, base, title, body string) (string, error) {
		s.prCalls = append(s.prCalls, strings.Join([]string{winner, branch, base, title, body}, "|"))
		return "https://github.com/org/repo/pull/7", nil
	}
}

// fakeExitedContainer wires a fake so waitForContainers sees a clean exit.
func fakeExitedContainer(fake *vmexec.FakeExecutor, name string) {
	fake.SetResponse("docker inspect --format {{.State.Status}} "+name, "exited")
	fake.SetResponse("docker inspect --format {{.State.ExitCode}} "+name, "0")
}

func TestRunJudgeStage_Success(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := vmexec.NewFake()
	judgeContainer := "agent-claude-pick-judge-20260101-000000"
	fakeExitedContainer(fake, judgeContainer)

	seams := &judgeSeams{readBack: `{"winner":"agent-claude-a","reason":"cleaner","summary":"Adds validation"}`}
	stubJudgeSeams(t, seams, []JudgeCandidate{
		{Name: "agent-claude-a", Agent: "claude"},
		{Name: "agent-codex-b", Agent: "codex"},
	})

	stage := fleet.PipelineStage{
		Name:      "pick",
		DependsOn: []string{"implement-claude", "implement-codex"},
		Judge:     &fleet.JudgeSpec{Criteria: "correctness"},
	}
	stageContainers := map[string][]string{
		"implement-claude": {"agent-claude-a"},
		"implement-codex":  {"agent-codex-b"},
	}

	out := captureOutput(func() {
		if err := runJudgeStage(context.Background(), fake, stage, stageContainers, "best-of-two", "20260101-000000"); err != nil {
			t.Fatalf("runJudgeStage() error = %v", err)
		}
	})

	if !strings.Contains(out, "selected winner: agent-claude-a") {
		t.Fatalf("expected winner announcement, got:\n%s", out)
	}
	if len(seams.spawned) != 1 || seams.spawned[0].AgentType != "claude" || !seams.spawned[0].Background {
		t.Fatalf("judge spawn = %#v", seams.spawned)
	}
	if !seams.spawned[0].ReuseAuth {
		t.Fatalf("judge spawn should reuse auth to run claude")
	}
	if len(seams.prCalls) != 0 {
		t.Fatalf("auto_pr disabled but PR was created: %v", seams.prCalls)
	}
	// verdict persisted
	verdictPath := filepath.Join(os.Getenv("HOME"), ".safe-ag", "state", "judge", "best-of-two-pick-20260101-000000.json")
	if _, err := os.Stat(verdictPath); err != nil {
		t.Fatalf("verdict not persisted at %s: %v", verdictPath, err)
	}
}

func TestRunJudgeStage_AutoPR(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := vmexec.NewFake()
	fakeExitedContainer(fake, "agent-claude-pick-judge-20260101-000000")

	seams := &judgeSeams{readBack: `{"winner":"agent-codex-b","reason":"passing tests","summary":"Implements feature Z"}`}
	stubJudgeSeams(t, seams, []JudgeCandidate{
		{Name: "agent-claude-a", Agent: "claude"},
		{Name: "agent-codex-b", Agent: "codex"},
	})

	stage := fleet.PipelineStage{
		Name:      "pick",
		DependsOn: []string{"implement"},
		Judge:     &fleet.JudgeSpec{AutoPR: true, Base: "develop"},
	}
	stageContainers := map[string][]string{"implement": {"agent-claude-a", "agent-codex-b"}}

	_ = captureOutput(func() {
		if err := runJudgeStage(context.Background(), fake, stage, stageContainers, "p", "20260101-000000"); err != nil {
			t.Fatalf("runJudgeStage() error = %v", err)
		}
	})

	if len(seams.prCalls) != 1 {
		t.Fatalf("expected 1 PR call, got %v", seams.prCalls)
	}
	parts := strings.Split(seams.prCalls[0], "|")
	if parts[0] != "agent-codex-b" {
		t.Fatalf("PR winner = %q, want agent-codex-b", parts[0])
	}
	// P1: a dedicated head branch, never the candidate's cloned default branch.
	if !strings.HasPrefix(parts[1], "safe-ag/judge-") {
		t.Fatalf("PR head branch = %q, want safe-ag/judge- prefix", parts[1])
	}
	if parts[2] != "develop" {
		t.Fatalf("PR base = %q, want develop", parts[2])
	}
	if !strings.Contains(parts[4], "Implements feature Z") {
		t.Fatalf("PR body missing summary: %q", parts[4])
	}
	if !strings.Contains(parts[4], "passing tests") {
		t.Fatalf("PR body missing reason: %q", parts[4])
	}
	// P2 regression: the winner container must NOT be restarted (that re-runs the
	// agent entrypoint and can mutate the workspace / race the push).
	if got := len(fake.CommandsMatching("docker start agent-codex-b")); got != 0 {
		t.Fatalf("winner must not be restarted, docker start count = %d", got)
	}
}

// ─── P1-1 regression: diff against base captures committed work ─────────────────

func TestCandidateDiffGitCmd(t *testing.T) {
	withBase := candidateDiffGitCmd("main")
	for _, want := range []string{
		"rev-parse --verify --quiet origin/main",
		"git diff origin/main",
		"rev-parse --verify --quiet main",
		"git diff main",
		"git diff HEAD",
	} {
		if !strings.Contains(withBase, want) {
			t.Fatalf("candidateDiffGitCmd(main) missing %q:\n%s", want, withBase)
		}
	}

	// Empty/unsafe base falls back to HEAD/working-tree only (no base ref, no
	// shell injection).
	for _, bad := range []string{"", "bad;rm -rf /", "$(touch x)"} {
		got := candidateDiffGitCmd(bad)
		if strings.Contains(got, "origin/") {
			t.Fatalf("candidateDiffGitCmd(%q) leaked a base ref: %s", bad, got)
		}
		if !strings.Contains(got, "git diff HEAD") {
			t.Fatalf("candidateDiffGitCmd(%q) missing HEAD fallback: %s", bad, got)
		}
	}
}

func TestCollectCandidateDiffUsesBaseRange(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker inspect --format {{.State.Running}} agent-claude-a", "true")
	fake.SetResponse("docker exec agent-claude-a bash -c", "diff --git a/f b/f\n+committed change")

	diff := collectCandidateDiff(context.Background(), fake, "agent-claude-a", "main")
	if !strings.Contains(diff, "committed change") {
		t.Fatalf("diff = %q", diff)
	}
	// The exec command must diff against the base ref, not a plain `git diff`.
	execs := fake.CommandsMatching("docker exec agent-claude-a bash -c")
	if len(execs) != 1 {
		t.Fatalf("expected 1 diff exec, got %d", len(execs))
	}
	joined := strings.Join(execs[0], " ")
	if !strings.Contains(joined, "git diff origin/main") {
		t.Fatalf("candidate diff did not use base range:\n%s", joined)
	}
}

func TestRunJudgeStage_ParseFailureSavesRaw(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := vmexec.NewFake()
	fakeExitedContainer(fake, "agent-claude-pick-judge-20260101-000000")

	seams := &judgeSeams{readBack: "I could not decide, sorry."}
	stubJudgeSeams(t, seams, []JudgeCandidate{
		{Name: "agent-claude-a"}, {Name: "agent-codex-b"},
	})

	stage := fleet.PipelineStage{
		Name:      "pick",
		DependsOn: []string{"implement"},
		Judge:     &fleet.JudgeSpec{},
	}
	stageContainers := map[string][]string{"implement": {"agent-claude-a", "agent-codex-b"}}

	var err error
	_ = captureOutput(func() {
		err = runJudgeStage(context.Background(), fake, stage, stageContainers, "p", "20260101-000000")
	})
	if err == nil || !strings.Contains(err.Error(), "raw output saved") {
		t.Fatalf("expected parse failure error mentioning saved output, got %v", err)
	}
	verdictPath := filepath.Join(os.Getenv("HOME"), ".safe-ag", "state", "judge", "p-pick-20260101-000000.json")
	data, readErr := os.ReadFile(verdictPath)
	if readErr != nil {
		t.Fatalf("failed verdict record not saved: %v", readErr)
	}
	var rec judgeRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("record not JSON: %v", err)
	}
	if !rec.Failed || rec.RawOutput != "I could not decide, sorry." {
		t.Fatalf("record did not capture raw output/failure: %#v", rec)
	}
}

func TestRunJudgeStage_TooFewCandidates(t *testing.T) {
	fake := vmexec.NewFake()
	stage := fleet.PipelineStage{
		Name:      "pick",
		DependsOn: []string{"implement"},
		Judge:     &fleet.JudgeSpec{},
	}
	stageContainers := map[string][]string{"implement": {"agent-claude-a"}}
	err := runJudgeStage(context.Background(), fake, stage, stageContainers, "p", "20260101-000000")
	if err == nil || !strings.Contains(err.Error(), "at least 2 candidate containers") {
		t.Fatalf("expected too-few-candidates error, got %v", err)
	}
}

func TestWinnerPRBranch(t *testing.T) {
	got := winnerPRBranch("my pipe", "pick/winner", "20260101-000000")
	if !strings.HasPrefix(got, "safe-ag/judge-") {
		t.Fatalf("branch = %q, want safe-ag/judge- prefix", got)
	}
	if !validBranchName.MatchString(got) {
		t.Fatalf("branch %q is not a valid git branch name", got)
	}
}

func TestCreateWinnerPRImplUsesVolumesFromHelper(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker inspect --format {{.Config.Image}} agent-codex-b", "safe-agentic:latest")
	fake.SetResponse("docker inspect --format {{.HostConfig.NetworkMode}} agent-codex-b", "agent-codex-b-net")

	if _, err := createWinnerPRImpl(context.Background(), fake, "agent-codex-b", "safe-ag/judge-p-pick-ts", "main", "My title", "My body"); err != nil {
		t.Fatalf("createWinnerPRImpl() error = %v", err)
	}

	runs := fake.CommandsMatching("docker run")
	if len(runs) != 1 {
		t.Fatalf("expected exactly 1 helper docker run, got %d", len(runs))
	}
	last := runs[0]
	joined := strings.Join(last, " ")
	for _, want := range []string{
		"--rm", "--cap-drop=ALL", "--security-opt=no-new-privileges:true",
		"--volumes-from agent-codex-b", "--network agent-codex-b-net",
		"--entrypoint bash", "safe-agentic:latest",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("helper run missing %q:\n%s", want, joined)
		}
	}
	// P2: the winner container must never be restarted.
	if got := len(fake.CommandsMatching("docker start")); got != 0 {
		t.Fatalf("helper must not restart the winner, docker start count = %d", got)
	}

	// Trailing argv: ... -c <script> safe-ag-judge-pr <branch> <base> <title> <body>
	if last[len(last)-1] != "My body" || last[len(last)-2] != "My title" {
		t.Fatalf("title/body must be argv, tail = %v", last[len(last)-4:])
	}
	if last[len(last)-3] != "main" || last[len(last)-4] != "safe-ag/judge-p-pick-ts" {
		t.Fatalf("branch/base argv wrong, tail = %v", last[len(last)-4:])
	}
	script := last[len(last)-6]
	// P1: untrusted values must not be embedded in the script, and the cloned
	// default branch must not be pushed directly.
	if strings.Contains(script, "My title") || strings.Contains(script, "git push -u origin HEAD") {
		t.Fatalf("script must not embed untrusted values or push cloned HEAD:\n%s", script)
	}
	if !strings.Contains(script, `checkout -B "$branch"`) || !strings.Contains(script, `--head "$branch"`) {
		t.Fatalf("script must create + push a dedicated head branch:\n%s", script)
	}
}
