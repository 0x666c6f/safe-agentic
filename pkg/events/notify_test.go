package events

import (
	"bytes"
	"encoding/base64"
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestParseNotifyTargetsTerminalOnly(t *testing.T) {
	targets := ParseNotifyTargets("terminal")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Kind != "terminal" {
		t.Errorf("Kind: expected %q, got %q", "terminal", targets[0].Kind)
	}
	if targets[0].Value != "" {
		t.Errorf("Value: expected empty, got %q", targets[0].Value)
	}
}

func TestParseNotifyTargetsTerminalAndSlack(t *testing.T) {
	targets := ParseNotifyTargets("terminal,slack:https://hooks.slack.com/foo")
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	if targets[0].Kind != "terminal" {
		t.Errorf("targets[0].Kind: expected %q, got %q", "terminal", targets[0].Kind)
	}
	if targets[0].Value != "" {
		t.Errorf("targets[0].Value: expected empty, got %q", targets[0].Value)
	}

	if targets[1].Kind != "slack" {
		t.Errorf("targets[1].Kind: expected %q, got %q", "slack", targets[1].Kind)
	}
	if targets[1].Value != "https://hooks.slack.com/foo" {
		t.Errorf("targets[1].Value: expected %q, got %q", "https://hooks.slack.com/foo", targets[1].Value)
	}
}

func TestParseNotifyTargetsCommand(t *testing.T) {
	targets := ParseNotifyTargets("command:/usr/local/bin/notify.sh")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Kind != "command" {
		t.Errorf("Kind: expected %q, got %q", "command", targets[0].Kind)
	}
	if targets[0].Value != "/usr/local/bin/notify.sh" {
		t.Errorf("Value: expected %q, got %q", "/usr/local/bin/notify.sh", targets[0].Value)
	}
}

func TestParseNotifyTargetsEmpty(t *testing.T) {
	targets := ParseNotifyTargets("")
	if len(targets) != 0 {
		t.Errorf("expected 0 targets for empty string, got %d", len(targets))
	}
}

func TestParseNotifyTargetsWhitespace(t *testing.T) {
	targets := ParseNotifyTargets("terminal, slack:url")
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[1].Kind != "slack" {
		t.Errorf("targets[1].Kind: expected %q, got %q", "slack", targets[1].Kind)
	}
}

func TestParseNotifyTargetsMultiple(t *testing.T) {
	targets := ParseNotifyTargets("terminal,slack:url,command:script")
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	if targets[2].Kind != "command" {
		t.Errorf("targets[2].Kind: expected %q, got %q", "command", targets[2].Kind)
	}
	if targets[2].Value != "script" {
		t.Errorf("targets[2].Value: expected %q, got %q", "script", targets[2].Value)
	}
}

func TestParseNotifyTargetsSystem(t *testing.T) {
	targets := ParseNotifyTargets("system")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Kind != TargetSystem {
		t.Errorf("Kind: expected %q, got %q", TargetSystem, targets[0].Kind)
	}
	if targets[0].Value != "" {
		t.Errorf("Value: expected empty, got %q", targets[0].Value)
	}
}

// The whole --notify string is persisted base64 in a container label and later
// reconstructed. Verify a system target survives that round-trip alongside the
// existing kinds.
func TestNotifyTargetsBase64RoundTrip(t *testing.T) {
	raw := "terminal,system,slack:https://hooks.slack.com/x,command:/n.sh"
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	targets := ParseNotifyTargets(string(decoded))
	if len(targets) != 4 {
		t.Fatalf("expected 4 targets, got %d: %+v", len(targets), targets)
	}
	want := []NotifyTarget{
		{Kind: TargetTerminal},
		{Kind: TargetSystem},
		{Kind: TargetSlack, Value: "https://hooks.slack.com/x"},
		{Kind: TargetCommand, Value: "/n.sh"},
	}
	for i, w := range want {
		if targets[i] != w {
			t.Errorf("target[%d] = %+v, want %+v", i, targets[i], w)
		}
	}
}

func TestKnownTargetKind(t *testing.T) {
	for _, kind := range []string{TargetTerminal, TargetSlack, TargetCommand, TargetSystem} {
		if !KnownTargetKind(kind) {
			t.Errorf("%q should be a known target kind", kind)
		}
	}
	if KnownTargetKind("email") {
		t.Error("email should not be a known target kind")
	}
}

func TestSoundForStatus(t *testing.T) {
	tests := map[string]string{
		StatusBlocked:        SoundAttention,
		StatusFailed:         SoundAttention,
		StatusFailedTests:    SoundAttention,
		StatusNeedsAuth:      SoundAttention,
		StatusStuck:          SoundAttention,
		StatusReadyForReview: SoundSuccess,
		StatusReadyForPR:     SoundSuccess,
		"done":               SoundSuccess,
		"success":            SoundSuccess,
		StatusInfo:           SoundNeutral,
		"":                   SoundNeutral,
		"whatever":           SoundNeutral,
	}
	for status, want := range tests {
		if got := SoundForStatus(status); got != want {
			t.Errorf("SoundForStatus(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestSystemNotificationTitle(t *testing.T) {
	if got := (SystemNotification{Container: "agent-foo"}).Title(); got != "safe-ag: agent-foo" {
		t.Errorf("Title() = %q", got)
	}
	if got := (SystemNotification{}).Title(); got != "safe-ag" {
		t.Errorf("Title() with no container = %q", got)
	}
}

func TestTerminalNotifierArgs(t *testing.T) {
	n := SystemNotification{Container: "agent-foo", Message: "is blocked", Sound: SoundAttention}
	args := TerminalNotifierArgs(n)
	joined := strings.Join(args, " ")
	if args[0] != "terminal-notifier" {
		t.Errorf("expected terminal-notifier binary, got %q", args[0])
	}
	if !strings.Contains(joined, "-title safe-ag: agent-foo") {
		t.Errorf("missing title: %q", joined)
	}
	if !strings.Contains(joined, "-message is blocked") {
		t.Errorf("missing message: %q", joined)
	}
	if !strings.Contains(joined, "-sound Basso") {
		t.Errorf("missing sound: %q", joined)
	}

	// No sound => no -sound flag.
	quiet := TerminalNotifierArgs(SystemNotification{Container: "c", Message: "m"})
	if strings.Contains(strings.Join(quiet, " "), "-sound") {
		t.Errorf("unexpected -sound flag: %v", quiet)
	}
}

func TestOsascriptArgs(t *testing.T) {
	n := SystemNotification{Container: "agent-foo", Message: `done "phase"`, Sound: SoundSuccess}
	args := OsascriptArgs(n)
	if args[0] != "osascript" || args[1] != "-e" {
		t.Fatalf("expected osascript -e, got %v", args[:2])
	}
	script := args[2]
	if !strings.HasPrefix(script, "display notification ") {
		t.Errorf("script should start with display notification: %q", script)
	}
	if !strings.Contains(script, `with title "safe-ag: agent-foo"`) {
		t.Errorf("missing title in script: %q", script)
	}
	if !strings.Contains(script, `sound name "Glass"`) {
		t.Errorf("missing sound in script: %q", script)
	}
	// The embedded quotes in the message must be escaped.
	if !strings.Contains(script, `\"phase\"`) {
		t.Errorf("message quotes not escaped: %q", script)
	}

	quiet := OsascriptArgs(SystemNotification{Container: "c", Message: "m"})
	if strings.Contains(quiet[2], "sound name") {
		t.Errorf("unexpected sound in quiet script: %q", quiet[2])
	}
}

func TestSystemNotifyCommandSelection(t *testing.T) {
	origLook := lookPath
	defer func() { lookPath = origLook }()

	// terminal-notifier available -> use it.
	lookPath = func(file string) (string, error) {
		if file == "terminal-notifier" {
			return "/usr/local/bin/terminal-notifier", nil
		}
		return "", errors.New("not found")
	}
	if got := SystemNotifyCommand(SystemNotification{Container: "c", Message: "m"}); got[0] != "terminal-notifier" {
		t.Errorf("expected terminal-notifier, got %q", got[0])
	}

	// terminal-notifier missing -> fall back to osascript.
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if got := SystemNotifyCommand(SystemNotification{Container: "c", Message: "m"}); got[0] != "osascript" {
		t.Errorf("expected osascript fallback, got %q", got[0])
	}
}

// Dispatch must deliver to every target kind through the package seams, without
// running a real command or making a real HTTP request.
func TestDispatch(t *testing.T) {
	origCmd, origSlack := commandRunner, slackPoster
	defer func() { commandRunner, slackPoster = origCmd, origSlack }()

	var gotCmd, gotCmdEnv, gotSlackURL, gotSlackText string
	commandRunner = func(command string, env []string) error {
		gotCmd = command
		gotCmdEnv = strings.Join(env, "\n")
		return nil
	}
	slackPoster = func(url, text string) error {
		gotSlackURL, gotSlackText = url, text
		return nil
	}

	targets := ParseNotifyTargets("terminal,slack:https://hooks.slack.com/x,command:/n.sh")
	note := SystemNotification{Container: "agent-foo", Message: "Agent finished", Sound: SoundSuccess}

	var buf bytes.Buffer
	if err := Dispatch(targets, note, &buf); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if !strings.Contains(buf.String(), "agent-foo") || !strings.Contains(buf.String(), "Agent finished") {
		t.Errorf("terminal target missing content: %q", buf.String())
	}
	if gotSlackURL != "https://hooks.slack.com/x" {
		t.Errorf("slack url = %q", gotSlackURL)
	}
	if !strings.Contains(gotSlackText, "Agent finished") {
		t.Errorf("slack text = %q", gotSlackText)
	}
	if gotCmd != "/n.sh" {
		t.Errorf("command = %q", gotCmd)
	}
	if !strings.Contains(gotCmdEnv, "SAFE_AG_CONTAINER=agent-foo") {
		t.Errorf("command env missing container: %q", gotCmdEnv)
	}
}

// A target with a missing required value must surface an error, and a broken
// target must not stop the others from firing.
func TestDispatchErrorsAreCollectedNotFatal(t *testing.T) {
	origSlack := slackPoster
	defer func() { slackPoster = origSlack }()

	slackCalled := false
	slackPoster = func(url, text string) error {
		slackCalled = true
		return nil
	}

	// A bare "command" (no value) errors; the following slack target must still fire.
	targets := ParseNotifyTargets("command,slack:https://hooks.slack.com/x")
	err := Dispatch(targets, SystemNotification{Container: "c", Message: "m"}, nil)
	if err == nil {
		t.Fatal("expected an error from the valueless command target")
	}
	if !slackCalled {
		t.Error("slack target should still fire despite the earlier error")
	}
}

// NotifySystem must build and dispatch the right argv without ever executing a
// real notifier. runCommand is stubbed to capture the argv instead.
func TestNotifySystemDispatch(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("system notifications only fire on macOS")
	}
	origLook, origRun := lookPath, runCommand
	defer func() { lookPath, runCommand = origLook, origRun }()

	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	var captured []string
	runCommand = func(argv []string) error {
		captured = argv
		return nil
	}

	if err := NotifySystem(SystemNotification{Container: "agent-x", Message: "blocked", Sound: SoundAttention}); err != nil {
		t.Fatalf("NotifySystem: %v", err)
	}
	if len(captured) == 0 || captured[0] != "osascript" {
		t.Fatalf("expected osascript dispatch, captured %v", captured)
	}
	if !strings.Contains(captured[2], "safe-ag: agent-x") {
		t.Errorf("dispatched script missing title: %q", captured[2])
	}
}
