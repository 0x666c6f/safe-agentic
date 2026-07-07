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
	r := &Runner{Bin: "safe-ag", Exec: fakeExec("ok", "", nil),
		OnCommand: func(argv []string) { logged = argv }}
	out, err := r.Run(context.Background(), "stop", "agent-x")
	if err != nil || string(out) != "ok" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if strings.Join(logged, " ") != "safe-ag stop agent-x" {
		t.Fatalf("argv not logged: %v", logged)
	}
}

func TestRunFailureCarriesStderr(t *testing.T) {
	r := &Runner{Bin: "safe-ag", Exec: fakeExec("", "boom: no such container", errors.New("exit status 1"))}
	_, err := r.Run(context.Background(), "stop", "agent-x")
	var ce *CLIError
	if !errors.As(err, &ce) || !strings.Contains(ce.Stderr, "no such container") {
		t.Fatalf("want CLIError with stderr, got %v", err)
	}
}

func TestOutputParsesJSON(t *testing.T) {
	r := &Runner{Bin: "safe-ag", Exec: fakeExec(
		`{"name":"agent-x","status":"exited","last_output":"done: all tests pass"}`, "", nil)}
	info, err := r.Output(context.Background(), "agent-x")
	if err != nil || info.LastOutput != "done: all tests pass" || info.Status != "exited" {
		t.Fatalf("info=%+v err=%v", info, err)
	}
}
