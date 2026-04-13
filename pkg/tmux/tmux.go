package tmux

import (
	"context"
	"fmt"
	"github.com/0x666c6f/safe-agentic/pkg/orb"
	"os"
	"time"
)

const defaultSessionName = "safe-agentic"

func SessionName() string {
	if n := os.Getenv("SAFE_AGENTIC_TMUX_SESSION_NAME"); n != "" {
		return n
	}
	return defaultSessionName
}

func HasSession(ctx context.Context, exec orb.Executor, containerName string) (bool, error) {
	_, err := exec.Run(ctx, "docker", "exec", containerName,
		"tmux", "has-session", "-t", SessionName())
	if err != nil {
		return false, nil
	}
	return true, nil
}

func WaitForSession(ctx context.Context, exec orb.Executor, containerName string) error {
	for i := 0; i < 300; i++ {
		has, _ := HasSession(ctx, exec, containerName)
		if has {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("tmux session not ready after 60s in container %s", containerName)
}

func BuildAttachArgs(containerName string) []string {
	return []string{
		"docker", "exec", "-it", containerName,
		"tmux", "attach", "-t", SessionName(),
	}
}

func BuildCapturePaneArgs(containerName string, lines int) []string {
	return []string{
		"docker", "exec", containerName,
		"tmux", "capture-pane", "-t", SessionName(), "-p", "-S", fmt.Sprintf("-%d", lines),
	}
}

func Attach(exec orb.Executor, containerName string) error {
	return exec.RunInteractive(BuildAttachArgs(containerName)...)
}
