package events

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Notify target kinds. A target is written by the user as "kind" or
// "kind:value" (e.g. "terminal", "slack:https://…", "command:/path",
// "system"). The whole comma-separated list is persisted base64 in the
// safe-agentic.notify-b64 container label and reconstructed with
// ParseNotifyTargets.
const (
	TargetTerminal = "terminal"
	TargetSlack    = "slack"
	TargetCommand  = "command"
	TargetSystem   = "system"
)

// macOS system sounds used by the "system" target. Harsh for things that need
// attention, soft for successful completion, silent otherwise.
const (
	SoundAttention = "Basso" // blocked / failed / needs-auth / stuck
	SoundSuccess   = "Glass" // ready-for-review / ready-for-pr / done
	SoundNeutral   = ""      // no sound
)

type NotifyTarget struct {
	Kind  string
	Value string
}

func ParseNotifyTargets(s string) []NotifyTarget {
	var targets []NotifyTarget
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if colonIdx := strings.Index(part, ":"); colonIdx > 0 {
			targets = append(targets, NotifyTarget{
				Kind:  part[:colonIdx],
				Value: part[colonIdx+1:],
			})
		} else {
			targets = append(targets, NotifyTarget{Kind: part})
		}
	}
	return targets
}

// KnownTargetKind reports whether kind is a notify target this build handles.
func KnownTargetKind(kind string) bool {
	switch kind {
	case TargetTerminal, TargetSlack, TargetCommand, TargetSystem:
		return true
	default:
		return false
	}
}

// SoundForStatus maps a classifier status (see classifier.go) to a macOS
// system sound name for the "system" notify target.
func SoundForStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusBlocked, StatusFailed, StatusFailedTests, StatusNeedsAuth, StatusStuck:
		return SoundAttention
	case StatusReadyForReview, StatusReadyForPR, "done", "success", "completed":
		return SoundSuccess
	default:
		return SoundNeutral
	}
}

// SystemNotification is a resolved macOS notification to display.
type SystemNotification struct {
	Container string
	Message   string
	Sound     string // macOS sound name; empty = silent
}

// Title renders the notification title, e.g. "safe-ag: agent-foo".
func (n SystemNotification) Title() string {
	if n.Container == "" {
		return "safe-ag"
	}
	return "safe-ag: " + n.Container
}

// TerminalNotifierArgs builds the argv for the terminal-notifier binary.
func TerminalNotifierArgs(n SystemNotification) []string {
	args := []string{"terminal-notifier", "-title", n.Title(), "-message", n.Message}
	if n.Sound != "" {
		args = append(args, "-sound", n.Sound)
	}
	return args
}

// OsascriptArgs builds the argv for the osascript fallback.
func OsascriptArgs(n SystemNotification) []string {
	script := fmt.Sprintf("display notification %s with title %s",
		appleScriptString(n.Message), appleScriptString(n.Title()))
	if n.Sound != "" {
		script += " sound name " + appleScriptString(n.Sound)
	}
	return []string{"osascript", "-e", script}
}

// lookPath and runCommand are indirected so tests can exercise selection and
// dispatch without ever running terminal-notifier or osascript.
var (
	lookPath   = exec.LookPath
	runCommand = func(argv []string) error {
		if len(argv) == 0 {
			return nil
		}
		return exec.Command(argv[0], argv[1:]...).Run()
	}
)

// SystemNotifyCommand returns the argv to display a macOS notification,
// preferring terminal-notifier when it is on PATH and otherwise falling back to
// osascript.
func SystemNotifyCommand(n SystemNotification) []string {
	if _, err := lookPath("terminal-notifier"); err == nil {
		return TerminalNotifierArgs(n)
	}
	return OsascriptArgs(n)
}

// NotifySystem displays a native macOS notification. It is a no-op on non-macOS
// hosts so it is safe to call unconditionally from the host CLI.
func NotifySystem(n SystemNotification) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return runCommand(SystemNotifyCommand(n))
}

// appleScriptString double-quotes and escapes a string for embedding in an
// AppleScript literal.
func appleScriptString(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(s) + `"`
}
