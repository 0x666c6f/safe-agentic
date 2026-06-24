package vmexec

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type Executor interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
	RunInteractive(args ...string) error
}

type MachineExecutor struct {
	VMName string
}

func (e *MachineExecutor) buildArgs(args ...string) []string {
	base := []string{"machine", "run", "-n", e.VMName, "-u", "root"}
	if len(args) == 0 {
		return base
	}
	wrapped := []string{"/usr/local/bin/safe-ag-exec", args[0]}
	for _, arg := range args[1:] {
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

func (e *MachineExecutor) RunInteractive(args ...string) error {
	cmd := exec.Command("container", e.buildArgs(args...)...)
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
