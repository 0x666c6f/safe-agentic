package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
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

// slackPoster posts text to a Slack incoming-webhook URL. Indirected so tests
// exercise Dispatch without making a real HTTP request.
var slackPoster = func(webhookURL, text string) error {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned %s", resp.Status)
	}
	return nil
}

// commandRunner runs a "command:" notify target. Indirected for tests.
var commandRunner = func(command string, env []string) error {
	c := exec.Command("bash", "-lc", command)
	c.Env = env
	return c.Run()
}

// Dispatch delivers note to every target. It attempts every target and returns
// the first error encountered (so one broken target does not suppress the rest).
// note.Sound drives the macOS system sound; callers set it via SoundForStatus.
// out receives "terminal" target lines; pass nil to drop them.
func Dispatch(targets []NotifyTarget, note SystemNotification, out io.Writer) error {
	var firstErr error
	record := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, t := range targets {
		switch t.Kind {
		case TargetTerminal:
			if out != nil {
				_, err := fmt.Fprintf(out, "🔔 %s — %s\n", note.Container, note.Message)
				record(err)
			}
		case TargetSystem:
			record(NotifySystem(note))
		case TargetSlack:
			if t.Value == "" {
				record(fmt.Errorf("notify slack target missing webhook url"))
				continue
			}
			record(slackPoster(t.Value, note.Title()+" — "+note.Message))
		case TargetCommand:
			if t.Value == "" {
				record(fmt.Errorf("notify command target missing command"))
				continue
			}
			record(commandRunner(t.Value, notifyCommandEnv(note)))
		default:
			// Unknown kind: KnownTargetKind gates configuration; skip here.
		}
	}
	return firstErr
}

// notifyCommandEnv exposes the notification to a "command:" target as env vars
// so the script can react without argument parsing.
func notifyCommandEnv(note SystemNotification) []string {
	return append(os.Environ(),
		"SAFE_AG_CONTAINER="+note.Container,
		"SAFE_AG_MESSAGE="+note.Message,
	)
}
