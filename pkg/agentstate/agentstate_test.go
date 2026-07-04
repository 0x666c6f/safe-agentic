package agentstate

import (
	"strings"
	"testing"
)

// lines splits a raw multi-line fixture the way a captured tmux pane arrives.
func lines(s string) []string {
	return strings.Split(strings.TrimPrefix(s, "\n"), "\n")
}

func TestDetectClaude(t *testing.T) {
	tests := []struct {
		name string
		pane string
		want State
	}{
		{
			name: "working - streaming with interrupt hint",
			pane: `
● I'll refactor the auth module now.

● Update(src/auth.go)
  ⎿  Updated src/auth.go with 12 additions

✳ Cogitating… (28s · ↑ 3.1k tokens · esc to interrupt)`,
			want: StateWorking,
		},
		{
			name: "blocked - tool permission prompt",
			pane: `
╭─────────────────────────────────────────────╮
│ Bash command                                 │
│ rm -rf build/                                │
│                                              │
│ Do you want to proceed?                      │
│ ❯ 1. Yes                                     │
│   2. Yes, and don't ask again                │
│   3. No, and tell Claude what to do          │
╰─────────────────────────────────────────────╯`,
			want: StateBlocked,
		},
		{
			name: "blocked - trust folder prompt",
			pane: `
Do you trust the files in this folder?

/workspace/repo

❯ 1. Yes, proceed
  2. No, exit`,
			want: StateBlocked,
		},
		{
			name: "blocked - plan approval",
			pane: `
Here is my plan:
  1. Add validation
  2. Wire the handler

Would you like to proceed?
❯ 1. Yes, and auto-accept edits
  2. Yes, and manually approve edits
  3. No, keep planning`,
			want: StateBlocked,
		},
		{
			name: "blocked - login flow",
			pane: `
[entrypoint] First run — authenticating via device code flow...
[entrypoint] A URL will appear. Open it in your macOS browser to log in.
https://claude.ai/device`,
			want: StateBlocked,
		},
		{
			name: "idle - awaiting prompt",
			pane: `
╭──────────────────────────────────────────────╮
│ >                                             │
╰──────────────────────────────────────────────╯
  ? for shortcuts`,
			want: StateIdle,
		},
		{
			name: "working wins over stale prompt in scrollback",
			pane: `
● Do you want to proceed? (answered earlier)
● Yes, proceeding.

● Running the test suite now.
✳ Testing… (esc to interrupt)`,
			want: StateWorking,
		},
		{
			name: "unknown - nothing recognizable",
			pane: `
● Here is a summary of the changes I made to the codebase.
The refactor touched three files and all tests pass.`,
			want: StateUnknown,
		},
		{
			name: "empty pane",
			pane: ``,
			want: StateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect("claude", lines(tt.pane))
			if got.State != tt.want {
				t.Fatalf("Detect() state = %q (reason %q, matched %q), want %q",
					got.State, got.Reason, got.Matched, tt.want)
			}
			if tt.want != StateUnknown && got.Reason == "" {
				t.Errorf("expected a non-empty reason for state %q", got.State)
			}
		})
	}
}

func TestDetectCodex(t *testing.T) {
	tests := []struct {
		name string
		pane string
		want State
	}{
		{
			name: "working - interrupt hint",
			pane: `
▌ Reading src/main.rs and planning the change.

  • ran cargo check
▌ Esc to interrupt`,
			want: StateWorking,
		},
		{
			name: "blocked - login",
			pane: `
[entrypoint] First run — authenticating via device code flow...
[entrypoint] A URL will appear. Open it in your macOS browser to log in.`,
			want: StateBlocked,
		},
		{
			name: "blocked - command approval",
			pane: `
Codex wants to run:
  git push origin main

Would you like to run this command?
  y / n`,
			want: StateBlocked,
		},
		{
			name: "idle - composer",
			pane: `
  workspace: /workspace/repo

  Type a message and press enter`,
			want: StateIdle,
		},
		{
			name: "unknown",
			pane: `
Applied patch to src/main.rs.
The build succeeded.`,
			want: StateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect("codex", lines(tt.pane))
			if got.State != tt.want {
				t.Fatalf("Detect() state = %q (reason %q, matched %q), want %q",
					got.State, got.Reason, got.Matched, tt.want)
			}
		})
	}
}

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name string
		pane string
		want State
	}{
		{
			name: "idle prompt",
			pane: `
agent@container:/workspace$ ls
README.md  src
agent@container:/workspace$`,
			want: StateIdle,
		},
		{
			name: "blocked - sudo password",
			pane: `
agent@container:/workspace$ sudo apt update
[sudo] password for agent:`,
			want: StateBlocked,
		},
		{
			name: "blocked - ssh host key",
			pane: `
The authenticity of host 'github.com' can't be established.
Are you sure you want to continue connecting (yes/no)?`,
			want: StateBlocked,
		},
		{
			name: "unknown - mid-command output",
			pane: `
Cloning into 'repo'...
Receiving objects:  42% (420/1000)`,
			want: StateUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect("shell", lines(tt.pane))
			if got.State != tt.want {
				t.Fatalf("Detect() state = %q (reason %q, matched %q), want %q",
					got.State, got.Reason, got.Matched, tt.want)
			}
		})
	}
}

func TestDetectMatchedLineReported(t *testing.T) {
	got := Detect("claude", lines("\n● foo\nDo you want to proceed?\n❯ 1. Yes"))
	if got.State != StateBlocked {
		t.Fatalf("state = %q, want blocked", got.State)
	}
	if got.Matched == "" {
		t.Fatal("expected Matched to carry the triggering pane line")
	}
}

func TestUnknownAgentUsesClaudeTable(t *testing.T) {
	got := Detect("mystery", lines("\n✳ Working… (esc to interrupt)"))
	if got.State != StateWorking {
		t.Fatalf("state = %q, want working (unknown agent falls back to claude table)", got.State)
	}
}

// A question in prose with no selectable option on screen must NOT be blocked.
func TestQuestionInProseIsNotBlocked(t *testing.T) {
	pane := lines(`
● I've finished the first pass on the refactor.
  Do you want to add regression tests as well? I can do that next.
  For now the build is green.`)
	got := Detect("claude", pane)
	if got.State == StateBlocked {
		t.Fatalf("prose question must not be blocked, got reason %q matched %q", got.Reason, got.Matched)
	}
}

// A stale, already-answered prompt in scrollback with the agent now idle at the
// input box must resolve to idle, not blocked.
func TestStalePromptThenIdleIsNotBlocked(t *testing.T) {
	pane := lines(`
Do you want to proceed?
❯ 1. Yes
  2. No
● Yes, proceeding. Ran the command successfully.
● Done. Anything else?
╭──────────────────────────────────────────────╮
│ >                                             │
╰──────────────────────────────────────────────╯
  ? for shortcuts`)
	got := Detect("claude", pane)
	if got.State == StateBlocked {
		t.Fatalf("stale answered prompt must not be blocked, got reason %q", got.Reason)
	}
}

// The tool-permission box (question + caret) must be blocked.
func TestEditPromptWithOptionsIsBlocked(t *testing.T) {
	pane := lines(`
Do you want to make this edit to src/auth.go?
❯ 1. Yes
  2. No, and tell Claude what to do differently`)
	got := Detect("claude", pane)
	if got.State != StateBlocked {
		t.Fatalf("edit prompt with options should be blocked, got %q", got.State)
	}
}

func TestStateHelpers(t *testing.T) {
	if !StateBlocked.NeedsAttention() {
		t.Error("blocked should need attention")
	}
	for _, s := range []State{StateWorking, StateDone, StateIdle, StateExited, StateUnknown} {
		if s.NeedsAttention() {
			t.Errorf("%q should not need attention", s)
		}
	}
	if StateBlocked.String() != "blocked" {
		t.Errorf("String() = %q, want blocked", StateBlocked.String())
	}
}
