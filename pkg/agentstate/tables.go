package agentstate

// The tables below are the built-in detection knowledge. Needles are lowercase
// substrings; matching is case-insensitive `strings.Contains`. Ordering within
// a list only affects which reason/line is reported when several match — the
// resulting State is the same. Keep the most specific needles first.
//
// These strings are grounded in how the CLIs render today plus this repo's
// own launcher (bin/agent-session.sh prints "Open it in your macOS browser to
// log in" for the Codex device-auth flow). They are best-effort and chosen to
// be conservative: prefer missing a state over inventing a "blocked".
//
// See table's doc comment for the blockedStrong vs blockedAsk distinction.

var claudeTable = table{
	// Claude renders "(esc to interrupt)" on its status line while streaming.
	working: []signal{
		{"esc to interrupt", "actively working (esc to interrupt)"},
		{"ctrl+c to interrupt", "actively working"},
		{"esc to cancel", "actively working"},
		// Extended-thinking spinner variant renders "(14s · still thinking
		// with xhigh effort)" instead of the esc-to-interrupt hint.
		{"still thinking", "actively working (thinking)"},
	},
	// Self-sufficient UI strings. "no, and tell claude" is the third option of
	// every tool-permission box and appears nowhere else.
	blockedStrong: []signal{
		{"do you trust the files in this folder", "trust prompt"},
		{"do you trust this folder", "trust prompt"},
		{"no, and tell claude", "permission prompt"},
		{"waiting for your input", "waiting for input"},
		{"browser to log in", "waiting for login"},
		{"paste this code", "waiting for login"},
		// Rendered when a prompt is submitted without credentials (e.g. host
		// seeding was off and the auth volume is empty).
		{"please run /login", "waiting for login"},
	},
	// Ambiguous questions: only "blocked" when a selectable option is also on
	// screen (Claude's tool-permission and plan-approval boxes render "❯ 1.").
	blockedAsk: []signal{
		{"do you want to proceed", "permission prompt"},
		{"would you like to proceed", "plan-approval prompt"},
		{"do you want to", "permission prompt"},
		{"would you like to", "approval prompt"},
	},
	done: []signal{
		{"session ended", "session ended"},
	},
	// The idle input box footer shows the shortcuts hint.
	idle: []signal{
		{"? for shortcuts", "idle (awaiting prompt)"},
		{"/ for commands", "idle (awaiting prompt)"},
	},
}

var codexTable = table{
	working: []signal{
		{"esc to interrupt", "actively working (esc to interrupt)"},
		{"ctrl+c to interrupt", "actively working"},
	},
	// In --yolo mode Codex bypasses approvals, so the common block is an
	// interactive login; the approval markers cover non-yolo / future use.
	// All of these are UI strings that do not appear in ordinary prose, so
	// they are strong (no option co-signal required).
	blockedStrong: []signal{
		{"browser to log in", "waiting for login"},
		{"sign in with", "waiting for login"},
		{"enter the code", "waiting for login"},
		{"paste this code", "waiting for login"},
		{"would you like to run", "approval prompt"},
		{"do you want to allow", "approval prompt"},
		{"approve this command", "approval prompt"},
		{"approval required", "approval prompt"},
		{"requires approval", "approval prompt"},
		{"allow command", "approval prompt"},
		{"run command?", "approval prompt"},
	},
	done: []signal{
		{"session ended", "session ended"},
	},
	idle: []signal{
		{"type a message", "idle (awaiting prompt)"},
		{"send a message", "idle (awaiting prompt)"},
	},
}

// shellBlocked holds prompts that block a plain shell on human input.
var shellBlocked = []signal{
	{"[sudo] password", "sudo password prompt"},
	{"password:", "password prompt"},
	{"passphrase", "passphrase prompt"},
	{"are you sure you want to continue connecting", "ssh host-key prompt"},
	{"(yes/no)", "confirmation prompt"},
}
