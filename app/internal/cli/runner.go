package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type CLIError struct {
	Argv   []string
	Stderr string
	Err    error
}

func (e *CLIError) Error() string {
	if len(e.Argv) < 2 {
		return fmt.Sprintf("berth %v failed: %v\n%s", e.Argv, e.Err, e.Stderr)
	}
	return fmt.Sprintf("berth %v failed: %v\n%s", e.Argv[1:], e.Err, e.Stderr)
}
func (e *CLIError) Unwrap() error { return e.Err }

// cmdLogCap bounds the executed-command ring buffer surfaced to the console.
const cmdLogCap = 200

// CommandEntry is one executed berth invocation, for the command console.
type CommandEntry struct {
	TS         int64    `json:"ts"`         // unix milliseconds
	Argv       []string `json:"argv"`       // secret/token values redacted
	OK         bool     `json:"ok"`         // exit status
	DurationMs int64    `json:"durationMs"` // wall time
	Tail       string   `json:"tail"`       // last non-empty output line
}

type Runner struct {
	Bin       string
	Exec      func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)
	ExecIn    func(ctx context.Context, dir, name string, args ...string) (stdout, stderr []byte, err error)
	OnCommand func(argv []string)
	OnExec    func(CommandEntry) // fired per completed command (emits cli.exec)

	mu  sync.Mutex
	log []CommandEntry
}

func NewRunner() *Runner {
	return &Runner{
		Bin: "berth",
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
	start := time.Now()
	stdout, stderr, err := r.Exec(ctx, r.Bin, args...)
	r.record(argv, start, stdout, stderr, err)
	if err != nil {
		return stdout, &CLIError{Argv: argv, Stderr: string(stderr), Err: err}
	}
	return stdout, nil
}

// RunIn is Run with the child's working directory set — worktree spawns need
// the CLI to run inside the source checkout it should git-worktree from.
func (r *Runner) RunIn(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if dir == "" {
		return r.Run(ctx, args...)
	}
	execIn := r.ExecIn
	if execIn == nil {
		execIn = func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Dir = dir
			var out, errb bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errb
			err := cmd.Run()
			return out.Bytes(), errb.Bytes(), err
		}
	}
	argv := append([]string{r.Bin}, args...)
	if r.OnCommand != nil {
		r.OnCommand(argv)
	}
	start := time.Now()
	stdout, stderr, err := execIn(ctx, dir, r.Bin, args...)
	r.record(argv, start, stdout, stderr, err)
	if err != nil {
		return stdout, &CLIError{Argv: argv, Stderr: string(stderr), Err: err}
	}
	return stdout, nil
}

// record appends a redacted command entry to the ring buffer and fires OnExec.
func (r *Runner) record(argv []string, start time.Time, stdout, stderr []byte, err error) {
	entry := CommandEntry{
		TS:         start.UnixMilli(),
		Argv:       redactArgv(argv),
		OK:         err == nil,
		DurationMs: time.Since(start).Milliseconds(),
		Tail:       tailLine(stdout, stderr, err),
	}
	r.mu.Lock()
	r.log = append(r.log, entry)
	if len(r.log) > cmdLogCap {
		r.log = r.log[len(r.log)-cmdLogCap:]
	}
	r.mu.Unlock()
	if r.OnExec != nil {
		r.OnExec(entry)
	}
}

// CommandLog returns a copy of the executed-command ring buffer (oldest first).
func (r *Runner) CommandLog() []CommandEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]CommandEntry(nil), r.log...)
}

// redactArgv masks the value of any flag whose name mentions token/secret,
// covering both "--token x" and "--token=x" forms.
func redactArgv(argv []string) []string {
	out := make([]string, len(argv))
	copy(out, argv)
	for i, a := range out {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		la := strings.ToLower(a)
		if !strings.Contains(la, "token") && !strings.Contains(la, "secret") {
			continue
		}
		if eq := strings.IndexByte(a, '='); eq >= 0 {
			out[i] = a[:eq+1] + "***"
		} else if i+1 < len(out) {
			out[i+1] = "***"
		}
	}
	return out
}

// tailLine is the last non-empty output line, capped, preferring stderr on
// failure. It never contains argv, so no secret redaction is needed here.
func tailLine(stdout, stderr []byte, err error) string {
	src := stdout
	if err != nil && len(bytes.TrimSpace(stderr)) > 0 {
		src = stderr
	}
	s := strings.TrimRight(string(src), "\r\n \t")
	if i := strings.LastIndexAny(s, "\r\n"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
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
