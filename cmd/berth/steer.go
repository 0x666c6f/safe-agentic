package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/0x666c6f/berth/pkg/docker"
	"github.com/0x666c6f/berth/pkg/labels"
	"github.com/0x666c6f/berth/pkg/tmux"
	"github.com/spf13/cobra"
)

var steerCmd = &cobra.Command{
	Use:     "steer <name|--latest> <message>",
	Short:   "Send a follow-up message into an agent tmux session",
	GroupID: groupManage,
	Args:    cobra.RangeArgs(1, 2),
	RunE:    runSteer,
}

func init() {
	addLatestFlag(steerCmd)
	rootCmd.AddCommand(steerCmd)
}

func runSteer(cmd *cobra.Command, args []string) error {
	target := ""
	messageParts := args
	if len(args) == 2 {
		target = args[0]
		messageParts = []string{args[1]}
	}
	if latestFlag(cmd) {
		target = "--latest"
	}
	message := strings.TrimSpace(strings.Join(messageParts, " "))
	if message == "" {
		return fmt.Errorf("message is required")
	}

	ctx := context.Background()
	exec := newExecutor()
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	terminal, _ := docker.InspectLabel(ctx, exec, name, labels.Terminal)
	if terminal != "" && terminal != "tmux" {
		return fmt.Errorf("container %s does not use tmux terminal", name)
	}

	running, _ := docker.IsRunning(ctx, exec, name)
	if !running {
		if _, err := exec.Run(ctx, "docker", "start", name); err != nil {
			return fmt.Errorf("start container %s: %w", name, err)
		}
	}
	if err := tmux.WaitForSession(ctx, exec, name); err != nil {
		return err
	}
	if _, err := exec.Run(ctx, "docker", "exec", name, "tmux", "send-keys", "-t", tmux.SessionName(), "--", message, "Enter"); err != nil {
		return fmt.Errorf("send message to %s: %w", name, err)
	}
	fmt.Printf("Sent to %s\n", name)
	return nil
}
