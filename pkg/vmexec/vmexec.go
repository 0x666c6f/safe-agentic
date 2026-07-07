package vmexec

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"golang.org/x/term"
)

type Executor interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
	// RunStreaming runs args in the VM and tees combined stdout+stderr live to w
	// instead of buffering until exit. Use it for multi-minute operations (image
	// build, VM bootstrap) so the caller sees progress as it happens.
	RunStreaming(ctx context.Context, w io.Writer, args ...string) error
	RunInteractive(args ...string) error
}

type MachineExecutor struct {
	VMName string
}

var stdinIsTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func (e *MachineExecutor) buildBaseArgs(interactive bool) []string {
	base := []string{"machine", "run", "-n", e.VMName, "-u", "root"}
	if interactive && stdinIsTerminal() {
		base = []string{"machine", "run", "--interactive", "--tty", "-n", e.VMName, "-u", "root"}
	}
	return base
}

func (e *MachineExecutor) buildArgs(args ...string) []string {
	return e.buildArgsWithBase(e.buildBaseArgs(false), args...)
}

func (e *MachineExecutor) buildInteractiveArgs(args ...string) []string {
	return e.buildArgsWithBase(e.buildBaseArgs(true), args...)
}

func (e *MachineExecutor) buildArgsWithBase(base []string, args ...string) []string {
	if len(args) == 0 {
		return base
	}
	wrapped := []string{"/usr/local/bin/safe-ag-exec", args[0]}
	for _, arg := range args[1:] {
		wrapped = append(wrapped, base64.StdEncoding.EncodeToString([]byte(arg)))
	}
	return append(append(base, "--"), wrapped...)
}

// BuildInteractiveArgs returns the host argv (for exec.Command("container", args...))
// that runs cmdArgs inside the VM through the safe-ag-exec relay with an
// interactive TTY, regardless of whether the caller's stdin is a terminal
// (GUI callers allocate their own PTY). Mirrors the executor's interactive path.
func BuildInteractiveArgs(vmName string, cmdArgs ...string) []string {
	base := []string{"machine", "run", "--interactive", "--tty", "-n", vmName, "-u", "root"}
	if len(cmdArgs) == 0 {
		return base
	}
	wrapped := []string{"/usr/local/bin/safe-ag-exec", cmdArgs[0]}
	for _, arg := range cmdArgs[1:] {
		wrapped = append(wrapped, base64.StdEncoding.EncodeToString([]byte(arg)))
	}
	return append(append(base, "--"), wrapped...)
}

func (e *MachineExecutor) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "container", e.buildArgs(args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("container machine run %s: %w\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func (e *MachineExecutor) RunStreaming(ctx context.Context, w io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "container", e.buildArgs(args...)...)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("container machine run %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func (e *MachineExecutor) RunInteractive(args ...string) error {
	cmd := exec.Command("container", e.buildInteractiveArgs(args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type FakeExecutor struct {
	mu        sync.Mutex
	Log       [][]string
	responses map[string]string
	errors    map[string]string
}

func NewFake() *FakeExecutor {
	return &FakeExecutor{
		responses: make(map[string]string),
		errors:    make(map[string]string),
	}
}

func (f *FakeExecutor) SetResponse(prefix, output string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[prefix] = output
}

func (f *FakeExecutor) SetError(prefix, msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors[prefix] = msg
}

func (f *FakeExecutor) Run(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = append(f.Log, args)
	joined := strings.Join(args, " ")
	for prefix, msg := range f.errors {
		if strings.HasPrefix(joined, prefix) {
			return nil, fmt.Errorf("%s", msg)
		}
	}
	for prefix, out := range f.responses {
		if strings.HasPrefix(joined, prefix) {
			return []byte(out), nil
		}
	}
	return []byte(""), nil
}

func (f *FakeExecutor) RunStreaming(_ context.Context, w io.Writer, args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = append(f.Log, args)
	joined := strings.Join(args, " ")
	for prefix, msg := range f.errors {
		if strings.HasPrefix(joined, prefix) {
			return fmt.Errorf("%s", msg)
		}
	}
	for prefix, out := range f.responses {
		if strings.HasPrefix(joined, prefix) {
			_, _ = io.WriteString(w, out)
			return nil
		}
	}
	return nil
}

func (f *FakeExecutor) RunInteractive(args ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = append(f.Log, args)
	return nil
}

func (f *FakeExecutor) LastCommand() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.Log) == 0 {
		return nil
	}
	return f.Log[len(f.Log)-1]
}

func (f *FakeExecutor) CommandsMatching(substr string) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result [][]string
	for _, cmd := range f.Log {
		if strings.Contains(strings.Join(cmd, " "), substr) {
			result = append(result, cmd)
		}
	}
	return result
}

func (f *FakeExecutor) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = nil
	f.responses = make(map[string]string)
	f.errors = make(map[string]string)
}
