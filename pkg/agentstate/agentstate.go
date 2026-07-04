// Package agentstate infers what a coding agent (Claude Code, Codex, or a
// plain shell) is currently doing from the last lines of its live tmux pane.
//
// The design mirrors herdr's screen-manifest detection: matching is done
// against small, per-agent tables of substrings scoped to the bottom of the
// pane (where the current state is rendered). It is deliberately conservative
// about the "blocked" state — a false "blocked" is worse than a miss, because
// it trains the operator to ignore the signal. When nothing matches with
// confidence the result is "unknown", and on ambiguity "working" is preferred
// over "blocked" (an actively streaming agent still shows its interrupt hint).
//
// The detection tables are plain Go data so a user-supplied override could be
// layered on later; that override system is intentionally not built yet.
package agentstate

import "strings"

// State is the inferred lifecycle state of an agent session.
type State string

const (
	// StateBlocked means the agent is waiting on a human: a permission,
	// trust, or approval prompt, or an interactive login.
	StateBlocked State = "blocked"
	// StateWorking means the agent is actively producing output.
	StateWorking State = "working"
	// StateDone means the agent finished its run (clean exit / end marker).
	StateDone State = "done"
	// StateIdle means the agent is up but sitting at an empty prompt.
	StateIdle State = "idle"
	// StateExited means the process is gone with a non-zero status.
	StateExited State = "exited"
	// StateUnknown means no state marker matched with confidence.
	StateUnknown State = "unknown"
)

// String returns the raw state token (e.g. "blocked").
func (s State) String() string { return string(s) }

// NeedsAttention reports whether a state should be surfaced to the operator.
// Only "blocked" demands human action; everything else is passive.
func (s State) NeedsAttention() bool { return s == StateBlocked }

// Result is a single detection outcome.
type Result struct {
	State   State  `json:"state"`
	Reason  string `json:"reason"`            // short human-readable explanation
	Matched string `json:"matched,omitempty"` // the pane line that triggered the match
}

// signal is one substring-to-reason mapping. The needle is always lowercase.
type signal struct {
	needle string
	reason string
}

// table holds the ordered detection signals for one agent type.
//
// blockedStrong needles are self-sufficient — their mere presence in the tail
// means the agent is blocked. blockedAsk needles are question phrases that
// could also occur in ordinary agent prose ("Do you want to add tests?"), so
// they only count as blocked when an interactive option marker (a "❯" caret or
// a "1. Yes"/"y/n" choice) is also present in the tail. This keeps "blocked"
// conservative.
type table struct {
	working       []signal
	blockedStrong []signal
	blockedAsk    []signal
	done          []signal
	idle          []signal
}

// optionMarkers are substrings that indicate a live selectable prompt is on
// screen. They are the co-signal required to promote a blockedAsk match.
var optionMarkers = []string{"❯", "1. yes", "1. no", "2. no", "3. no", "[y/n]", "y/n", "yes/no"}

func hasOptionMarker(lines []string) bool {
	for _, line := range lines {
		low := strings.ToLower(line)
		for _, m := range optionMarkers {
			if strings.Contains(low, m) {
				return true
			}
		}
	}
	return false
}

const (
	// liveWindow bounds "working" detection to the very bottom of the pane so a
	// stale interrupt hint left in scrollback above a newer prompt cannot win.
	liveWindow = 6
	// bottomWindow bounds prompt/idle/done detection to the recent tail.
	bottomWindow = 16
)

// Detect infers the agent's state from the last lines of its tmux pane. The
// caller passes the pane top-to-bottom (as `tmux capture-pane -p` emits it).
func Detect(agentType string, paneLines []string) Result {
	lines := compact(paneLines)
	if len(lines) == 0 {
		return Result{State: StateUnknown, Reason: "no output captured"}
	}
	switch agentType {
	case "shell", "":
		return detectShell(lines)
	default:
		return detectAgent(tableFor(agentType), lines)
	}
}

// detectAgent infers the state from the pane tail.
//
// Working is checked first, scoped to the live tail, so an actively streaming
// agent wins on ambiguity. Otherwise the tail is scanned bottom-up: the LOWEST
// line carrying a state marker reflects the current state, because a live
// prompt (blocked) or input box (idle) is always the bottom-most UI. This makes
// a stale, already-answered prompt sitting higher in scrollback lose to the
// fresh idle box below it. Within a single line the order is blocked → done →
// idle.
func detectAgent(t table, lines []string) Result {
	if sig, matched, ok := matchSignals(tail(lines, liveWindow), t.working); ok {
		return Result{State: StateWorking, Reason: sig.reason, Matched: matched}
	}

	bottom := tail(lines, bottomWindow)
	optionPresent := hasOptionMarker(bottom)
	for i := len(bottom) - 1; i >= 0; i-- {
		low := strings.ToLower(bottom[i])
		if sig, ok := firstNeedle(low, t.blockedStrong); ok {
			return Result{State: StateBlocked, Reason: sig.reason, Matched: bottom[i]}
		}
		if optionPresent {
			if sig, ok := firstNeedle(low, t.blockedAsk); ok {
				return Result{State: StateBlocked, Reason: sig.reason, Matched: bottom[i]}
			}
		}
		if sig, ok := firstNeedle(low, t.done); ok {
			return Result{State: StateDone, Reason: sig.reason, Matched: bottom[i]}
		}
		if sig, ok := firstNeedle(low, t.idle); ok {
			return Result{State: StateIdle, Reason: sig.reason, Matched: bottom[i]}
		}
	}
	return Result{State: StateUnknown, Reason: "no known state markers"}
}

// firstNeedle returns the first signal in sigs whose needle is contained in the
// already-lowercased line.
func firstNeedle(low string, sigs []signal) (signal, bool) {
	for _, sig := range sigs {
		if strings.Contains(low, sig.needle) {
			return sig, true
		}
	}
	return signal{}, false
}

// detectShell handles plain shells, where "working" is not observable from a
// static capture. It recognises common interactive prompts (blocked) and a
// returned shell prompt (idle); otherwise unknown.
func detectShell(lines []string) Result {
	bottom := tail(lines, bottomWindow)
	if sig, matched, ok := matchSignals(bottom, shellBlocked); ok {
		return Result{State: StateBlocked, Reason: sig.reason, Matched: matched}
	}
	last := lines[len(lines)-1]
	if looksLikeShellPrompt(last) {
		return Result{State: StateIdle, Reason: "shell prompt", Matched: last}
	}
	return Result{State: StateUnknown, Reason: "no known state markers"}
}

// matchSignals scans lines bottom-to-top (most recent first) and returns the
// first signal that matches, so the freshest pane line defines the outcome.
func matchSignals(lines []string, sigs []signal) (signal, string, bool) {
	for i := len(lines) - 1; i >= 0; i-- {
		low := strings.ToLower(lines[i])
		for _, sig := range sigs {
			if strings.Contains(low, sig.needle) {
				return sig, lines[i], true
			}
		}
	}
	return signal{}, "", false
}

// looksLikeShellPrompt reports whether a line resembles an idle shell prompt.
func looksLikeShellPrompt(line string) bool {
	line = strings.TrimRight(line, " ")
	if line == "" {
		return false
	}
	switch line[len(line)-1] {
	case '$', '#', '%':
		return true
	default:
		return false
	}
}

// compact trims each line and drops blank lines, preserving order.
func compact(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// tail returns the last n elements of lines (or all of them if shorter).
func tail(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func tableFor(agentType string) table {
	switch agentType {
	case "codex":
		return codexTable
	default: // claude and any unknown agent use the Claude table.
		return claudeTable
	}
}
