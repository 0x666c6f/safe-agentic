package main

import (
	"errors"
	"reflect"
	"testing"
)

func withTerminalMocks(t *testing.T, goos string, look func(string) (string, error), run func(string, ...string) error) {
	t.Helper()
	oldGOOS := terminalGOOS
	oldLookPath := lookPath
	oldRun := runTerminalCommand
	terminalGOOS = goos
	lookPath = look
	runTerminalCommand = run
	t.Cleanup(func() {
		terminalGOOS = oldGOOS
		lookPath = oldLookPath
		runTerminalCommand = oldRun
	})
}

func TestOpenTerminalPrefersITerm(t *testing.T) {
	var calls [][]string
	withTerminalMocks(t, "darwin",
		func(name string) (string, error) { return "/usr/bin/" + name, nil },
		func(name string, args ...string) error {
			calls = append(calls, append([]string{name}, args...))
			return nil
		},
	)

	if err := openTerminal([]string{"safe-ag", "attach", "agent demo"}); err != nil {
		t.Fatalf("openTerminal() error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v, want app lookup and osascript", calls)
	}
	if !reflect.DeepEqual(calls[0], []string{"osascript", "-e", `id of application "iTerm"`}) {
		t.Fatalf("app lookup call = %#v", calls[0])
	}
	if got := calls[1]; got[0] != "osascript" || !containsArg(got, `tell application "iTerm"`) {
		t.Fatalf("osascript call = %#v, want iTerm", got)
	}
}

func TestOpenTerminalFallsBackToTerminal(t *testing.T) {
	var calls [][]string
	withTerminalMocks(t, "darwin",
		func(name string) (string, error) { return "/usr/bin/" + name, nil },
		func(name string, args ...string) error {
			calls = append(calls, append([]string{name}, args...))
			if len(args) == 2 && args[0] == "-e" && args[1] == `id of application "iTerm"` {
				return errors.New("missing app")
			}
			if len(args) == 2 && args[0] == "-e" && args[1] == `id of application "iTerm2"` {
				return errors.New("missing app")
			}
			return nil
		},
	)

	if err := openTerminal([]string{"safe-ag", "attach", "agent-demo"}); err != nil {
		t.Fatalf("openTerminal() error = %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("calls = %#v, want iTerm/iTerm2 lookup and Terminal osascript", calls)
	}
	if got := calls[2]; got[0] != "osascript" || !containsArg(got, `tell application "Terminal"`) {
		t.Fatalf("osascript call = %#v, want Terminal", got)
	}
}

func TestOpenTerminalUnsupportedOS(t *testing.T) {
	withTerminalMocks(t, "linux",
		func(name string) (string, error) { return "", nil },
		func(name string, args ...string) error { return nil },
	)

	if err := openTerminal([]string{"safe-ag", "attach", "agent-demo"}); err == nil {
		t.Fatal("openTerminal() error = nil, want unsupported OS error")
	}
}

func TestAppleScriptStringEscapesQuotesAndBackslashes(t *testing.T) {
	got := appleScriptString(`echo "x\y"`)
	want := `echo \"x\\y\"`
	if got != want {
		t.Fatalf("appleScriptString() = %q, want %q", got, want)
	}
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
