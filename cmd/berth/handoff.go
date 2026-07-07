package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0x666c6f/berth/pkg/docker"
	"github.com/0x666c6f/berth/pkg/labels"
	"github.com/spf13/cobra"
)

var handoffToLocal string
var handoffToWorktree bool

var handoffCmd = &cobra.Command{
	Use:     "handoff [name|--latest]",
	Short:   "Handoff an agent workspace to a local path or managed worktree",
	GroupID: groupWorkflow,
	Args:    cobra.MaximumNArgs(1),
	RunE:    runHandoff,
}

func init() {
	handoffCmd.Flags().StringVar(&handoffToLocal, "to-local", "", "Copy /workspace to a local destination path")
	handoffCmd.Flags().BoolVar(&handoffToWorktree, "to-worktree", false, "Print the managed worktree path for this agent")
	addLatestFlag(handoffCmd)
	rootCmd.AddCommand(handoffCmd)
}

func runHandoff(cmd *cobra.Command, args []string) error {
	if (handoffToLocal == "") == !handoffToWorktree {
		return fmt.Errorf("set exactly one of --to-local or --to-worktree")
	}
	ctx := context.Background()
	exec := newExecutor()
	name, err := docker.ResolveTarget(ctx, exec, targetFromArgs(cmd, args))
	if err != nil {
		return err
	}
	if handoffToWorktree {
		path, _ := docker.InspectLabel(ctx, exec, name, labels.Worktree)
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("container %s has no managed worktree", name)
		}
		fmt.Println(path)
		return nil
	}
	dest, err := filepath.Abs(handoffToLocal)
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create destination parent: %w", err)
	}
	if _, err := exec.Run(ctx, "docker", "cp", name+":/workspace", dest); err != nil {
		return fmt.Errorf("copy workspace from %s: %w", name, err)
	}
	fmt.Printf("Workspace copied to %s\n", dest)
	return nil
}
