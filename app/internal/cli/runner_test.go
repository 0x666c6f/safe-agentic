package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func fakeExec(stdout, stderr string, err error) func(context.Context, string, ...string) ([]byte, []byte, error) {
	return func(context.Context, string, ...string) ([]byte, []byte, error) {
		return []byte(stdout), []byte(stderr), err
	}
}

func TestRunSuccessLogsArgv(t *testing.T) {
	var logged []string
	r := &Runner{Bin: "berth", Exec: fakeExec("ok", "", nil),
		OnCommand: func(argv []string) { logged = argv }}
	out, err := r.Run(context.Background(), "stop", "agent-x")
	if err != nil || string(out) != "ok" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if strings.Join(logged, " ") != "berth stop agent-x" {
		t.Fatalf("argv not logged: %v", logged)
	}
}

func TestRunFailureCarriesStderr(t *testing.T) {
	r := &Runner{Bin: "berth", Exec: fakeExec("", "boom: no such container", errors.New("exit status 1"))}
	_, err := r.Run(context.Background(), "stop", "agent-x")
	var ce *CLIError
	if !errors.As(err, &ce) || !strings.Contains(ce.Stderr, "no such container") {
		t.Fatalf("want CLIError with stderr, got %v", err)
	}
}

func TestRedactArgv(t *testing.T) {
	got := redactArgv([]string{"berth", "mcp-login", "--token", "abc123", "--secret-key=xyz", "--repo", "org/repo"})
	want := "berth mcp-login --token *** --secret-key=*** --repo org/repo"
	if strings.Join(got, " ") != want {
		t.Fatalf("redact: %q", strings.Join(got, " "))
	}
}

func TestCommandLogRingAndOnExec(t *testing.T) {
	var fired []CommandEntry
	r := &Runner{Bin: "berth", Exec: fakeExec("line1\nlast line\n", "", nil),
		OnExec: func(e CommandEntry) { fired = append(fired, e) }}
	if _, err := r.Run(context.Background(), "cost", "agent-x"); err != nil {
		t.Fatal(err)
	}
	log := r.CommandLog()
	if len(log) != 1 || len(fired) != 1 {
		t.Fatalf("log=%d fired=%d", len(log), len(fired))
	}
	e := log[0]
	if !e.OK || e.Tail != "last line" || strings.Join(e.Argv, " ") != "berth cost agent-x" || e.TS == 0 {
		t.Fatalf("entry=%+v", e)
	}
}

func TestCommandLogCap(t *testing.T) {
	r := &Runner{Bin: "berth", Exec: fakeExec("ok", "", nil)}
	for i := 0; i < cmdLogCap+50; i++ {
		_, _ = r.Run(context.Background(), "list")
	}
	if n := len(r.CommandLog()); n != cmdLogCap {
		t.Fatalf("ring cap: want %d, got %d", cmdLogCap, n)
	}
}

func TestTailLinePrefersStderrOnError(t *testing.T) {
	r := &Runner{Bin: "berth", Exec: fakeExec("", "boom: bad thing", errors.New("exit 1"))}
	_, _ = r.Run(context.Background(), "pr", "agent-x")
	log := r.CommandLog()
	if len(log) != 1 || log[0].OK || log[0].Tail != "boom: bad thing" {
		t.Fatalf("entry=%+v", log[0])
	}
}

func TestOutputParsesJSON(t *testing.T) {
	r := &Runner{Bin: "berth", Exec: fakeExec(
		`{"name":"agent-x","status":"exited","last_output":"done: all tests pass"}`, "", nil)}
	info, err := r.Output(context.Background(), "agent-x")
	if err != nil || info.LastOutput != "done: all tests pass" || info.Status != "exited" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}
