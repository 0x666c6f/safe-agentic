package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/worktrees"
	"github.com/spf13/cobra"
)

var worktreeCleanupAll bool
var worktreeCleanupDryRun bool

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage safe-agentic host worktrees",
}

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed worktrees",
	Args:  cobra.NoArgs,
	RunE:  runWorktreeList,
}

var worktreeCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Clean missing or all managed worktrees",
	Args:  cobra.NoArgs,
	RunE:  runWorktreeCleanup,
}

var worktreeSnapshotCmd = &cobra.Command{
	Use:   "snapshot <agent|--latest> [label]",
	Short: "Create a git stash snapshot in a managed worktree",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runWorktreeSnapshot,
}

var worktreeRestoreCmd = &cobra.Command{
	Use:   "restore <agent|--latest> <ref>",
	Short: "Restore a git stash snapshot in a managed worktree",
	Args:  cobra.ExactArgs(2),
	RunE:  runWorktreeRestore,
}

func init() {
	worktreeCleanupCmd.Flags().BoolVar(&worktreeCleanupAll, "all", false, "Remove all registered worktrees")
	worktreeCleanupCmd.Flags().BoolVar(&worktreeCleanupDryRun, "dry-run", false, "Show cleanup actions without removing")
	addLatestFlag(worktreeSnapshotCmd)
	addLatestFlag(worktreeRestoreCmd)
	worktreeCmd.AddCommand(worktreeListCmd, worktreeCleanupCmd, worktreeSnapshotCmd, worktreeRestoreCmd)
	rootCmd.AddCommand(worktreeCmd)
}

func runWorktreeList(cmd *cobra.Command, args []string) error {
	entries, err := worktrees.ReadRegistry(worktrees.RegistryPath())
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("No managed worktrees.")
		return nil
	}
	for _, wt := range entries {
		status := "ok"
		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			status = "missing"
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\n", wt.Container, wt.Branch, status, wt.Path, wt.RepoRoot)
	}
	return nil
}

func runWorktreeCleanup(cmd *cobra.Command, args []string) error {
	entries, err := worktrees.ReadRegistry(worktrees.RegistryPath())
	if err != nil {
		return err
	}
	var kept []worktrees.Worktree
	removed := 0
	for _, wt := range entries {
		_, statErr := os.Stat(wt.Path)
		missing := os.IsNotExist(statErr)
		remove := missing || worktreeCleanupAll
		if !remove {
			kept = append(kept, wt)
			continue
		}
		removed++
		action := "drop registry entry"
		if worktreeCleanupAll && !missing {
			action = "git worktree remove --force"
		}
		fmt.Printf("%s: %s\n", action, wt.Path)
		if worktreeCleanupDryRun {
			kept = append(kept, wt)
			continue
		}
		if worktreeCleanupAll && !missing {
			if out, err := gitWorktreeCommand(wt.RepoRoot, "worktree", "remove", "--force", wt.Path); err != nil {
				return fmt.Errorf("remove worktree %s: %w\n%s", wt.Path, err, out)
			}
		}
	}
	if !worktreeCleanupDryRun {
		if err := worktrees.WriteRegistry(worktrees.RegistryPath(), kept); err != nil {
			return err
		}
	}
	fmt.Printf("Removed %d worktree entries\n", removed)
	return nil
}

func runWorktreeSnapshot(cmd *cobra.Command, args []string) error {
	path, err := managedWorktreePath(cmd, args[:1])
	if err != nil {
		return err
	}
	label := "snapshot"
	if len(args) == 2 {
		label = args[1]
	}
	out, err := gitWorktreeCommand(path, "stash", "push", "-m", "safe-ag snapshot: "+label)
	if err != nil {
		return fmt.Errorf("git stash: %w\n%s", err, out)
	}
	fmt.Print(out)
	return nil
}

func runWorktreeRestore(cmd *cobra.Command, args []string) error {
	path, err := managedWorktreePath(cmd, args[:1])
	if err != nil {
		return err
	}
	ref := args[1]
	if !validStashRef.MatchString(ref) {
		return fmt.Errorf("invalid stash ref %q: must be stash@{N} or a numeric index", ref)
	}
	out, err := gitWorktreeCommand(path, "stash", "pop", ref)
	if err != nil {
		return fmt.Errorf("git stash pop: %w\n%s", err, out)
	}
	fmt.Print(out)
	return nil
}

func managedWorktreePath(cmd *cobra.Command, args []string) (string, error) {
	ctx := context.Background()
	exec := newExecutor()
	name, err := docker.ResolveTarget(ctx, exec, targetFromArgs(cmd, args))
	if err != nil {
		return "", err
	}
	path, _ := docker.InspectLabel(ctx, exec, name, labels.Worktree)
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("container %s has no managed worktree", name)
	}
	return strings.TrimSpace(path), nil
}

func gitWorktreeCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
