package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type CLIError struct {
	Argv   []string
	Stderr string
	Err    error
}

func (e *CLIError) Error() string {
	if len(e.Argv) < 2 {
		return fmt.Sprintf("safe-ag %v failed: %v\n%s", e.Argv, e.Err, e.Stderr)
	}
	return fmt.Sprintf("safe-ag %v failed: %v\n%s", e.Argv[1:], e.Err, e.Stderr)
}
func (e *CLIError) Unwrap() error { return e.Err }

type Runner struct {
	Bin       string
	Exec      func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
	OnCommand func(argv []string)
}

func NewRunner() *Runner {
	return &Runner{
		Bin: "safe-ag",
		Exec: func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			err := cmd.Run()
			return out.Bytes(), errb.Bytes(), err
		},
	}
}

func (r *Runner) Run(ctx context.Context, args ...string) ([]byte, error) {
	argv := append([]string{r.Bin}, args...)
	if r.OnCommand != nil {
		r.OnCommand(argv)
	}
	stdout, stderr, err := r.Exec(ctx, r.Bin, args...)
	if err != nil {
		return stdout, &CLIError{Argv: argv, Stderr: string(stderr), Err: err}
	}
	return stdout, nil
}

type OutputInfo struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"`
	LastOutput string   `json:"last_output"`
	Files      []string `json:"files"`
	Commits    []string `json:"commits"`
}

func (r *Runner) Output(ctx context.Context, name string) (OutputInfo, error) {
	var info OutputInfo
	out, err := r.Run(ctx, "output", "--json", name)
	if err != nil {
		return info, err
	}
	if jerr := json.Unmarshal(out, &info); jerr != nil {
		return info, fmt.Errorf("parse output --json: %w", jerr)
	}
	return info, nil
}
