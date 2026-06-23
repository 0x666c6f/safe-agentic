package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var workspaceYes bool

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Stage, unstage, or revert files in an agent workspace",
}

var workspaceStageCmd = &cobra.Command{
	Use:   "stage <agent|--latest> <path...>",
	Short: "git add files inside an agent workspace",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runWorkspaceStage,
}

var workspaceUnstageCmd = &cobra.Command{
	Use:   "unstage <agent|--latest> <path...>",
	Short: "unstage files inside an agent workspace",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runWorkspaceUnstage,
}

var workspaceRevertCmd = &cobra.Command{
	Use:   "revert <agent|--latest> <path...>",
	Short: "discard file changes inside an agent workspace",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runWorkspaceRevert,
}

var workspaceStagePatchCmd = &cobra.Command{
	Use:   "stage-patch <agent|--latest> <patch-file>",
	Short: "stage selected hunks from a patch file",
	Args:  cobra.ExactArgs(2),
	RunE:  runWorkspaceStagePatch,
}

var workspaceRevertPatchCmd = &cobra.Command{
	Use:   "revert-patch <agent|--latest> <patch-file>",
	Short: "revert selected hunks from a patch file",
	Args:  cobra.ExactArgs(2),
	RunE:  runWorkspaceRevertPatch,
}

func init() {
	workspaceRevertCmd.Flags().BoolVar(&workspaceYes, "yes", false, "Confirm destructive file revert")
	workspaceRevertPatchCmd.Flags().BoolVar(&workspaceYes, "yes", false, "Confirm destructive patch revert")
	workspaceCmd.AddCommand(workspaceStageCmd, workspaceUnstageCmd, workspaceRevertCmd, workspaceStagePatchCmd, workspaceRevertPatchCmd)
	rootCmd.AddCommand(workspaceCmd)
}

func runWorkspaceStage(cmd *cobra.Command, args []string) error {
	return runWorkspaceGit(args[0], args[1:], []string{"git", "add", "--"})
}

func runWorkspaceUnstage(cmd *cobra.Command, args []string) error {
	return runWorkspaceGit(args[0], args[1:], []string{"git", "restore", "--staged", "--"})
}

func runWorkspaceRevert(cmd *cobra.Command, args []string) error {
	paths, err := cleanWorkspacePaths(args[1:])
	if err != nil {
		return err
	}
	if !workspaceYes {
		if ok, err := confirmWorkspaceRevert(paths); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("revert cancelled")
		}
	}
	return runWorkspaceGit(args[0], paths, []string{"git", "checkout", "--"})
}

func runWorkspaceStagePatch(cmd *cobra.Command, args []string) error {
	return runWorkspacePatch(args[0], args[1], false, []string{"git", "apply", "--cached", "--whitespace=nowarn"})
}

func runWorkspaceRevertPatch(cmd *cobra.Command, args []string) error {
	if !workspaceYes {
		if ok, err := confirmWorkspaceRevert([]string{args[1]}); err != nil {
			return err
		} else if !ok {
			return fmt.Errorf("revert cancelled")
		}
	}
	return runWorkspacePatch(args[0], args[1], true, []string{"git", "apply", "--reverse", "--whitespace=nowarn"})
}

func runWorkspaceGit(target string, rawPaths []string, gitArgs []string) error {
	paths, err := cleanWorkspacePaths(rawPaths)
	if err != nil {
		return err
	}
	ctx := context.Background()
	exec := newExecutor()
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}
	args := append([]string{}, gitArgs...)
	args = append(args, paths...)
	out, err := exec.Run(ctx, workspaceExecCommand(name, args...)...)
	if err != nil {
		return fmt.Errorf("%s in %s: %w", strings.Join(gitArgs[:2], " "), name, err)
	}
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	return nil
}

func runWorkspacePatch(target, patchFile string, requireClean bool, gitArgs []string) error {
	patchFile = strings.TrimSpace(patchFile)
	if patchFile == "" {
		return fmt.Errorf("patch file is required")
	}
	if filepath.IsAbs(patchFile) {
		return fmt.Errorf("patch file must be a local relative path")
	}
	patchFile = filepath.Clean(patchFile)
	if patchFile == ".." || strings.HasPrefix(patchFile, "../") {
		return fmt.Errorf("patch file %q escapes current directory", patchFile)
	}
	data, err := os.ReadFile(patchFile)
	if err != nil {
		return fmt.Errorf("read patch file: %w", err)
	}
	if err := validateWorkspacePatch(data); err != nil {
		return err
	}
	localPatch, err := writeTempWorkspacePatch(data)
	if err != nil {
		return err
	}
	defer os.Remove(localPatch)

	ctx := context.Background()
	exec := newExecutor()
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}
	base := filepath.Base(localPatch)
	vmPatch := filepath.Join("/tmp", base)
	containerPatch := filepath.Join("/tmp", base)
	if err := copyFileToVM(configuredVMName(), localPatch, vmPatch); err != nil {
		return fmt.Errorf("copy patch to VM: %w", err)
	}
	defer func() {
		_, _ = exec.Run(ctx, "rm", "-f", vmPatch)
	}()
	if _, err := exec.Run(ctx, "docker", "cp", vmPatch, name+":"+containerPatch); err != nil {
		return fmt.Errorf("copy patch into %s: %w", name, err)
	}
	defer func() {
		_, _ = exec.Run(ctx, "docker", "exec", name, "rm", "-f", containerPatch)
	}()
	if requireClean {
		checkArgs := append(append([]string{}, gitArgs...), "--check", containerPatch)
		if _, err := exec.Run(ctx, workspaceExecCommand(name, checkArgs...)...); err != nil {
			return fmt.Errorf("patch does not apply cleanly in %s: %w", name, err)
		}
	}
	args := append(append([]string{}, gitArgs...), containerPatch)
	out, err := exec.Run(ctx, workspaceExecCommand(name, args...)...)
	if err != nil {
		return fmt.Errorf("%s in %s: %w", strings.Join(gitArgs[:2], " "), name, err)
	}
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	return nil
}

func writeTempWorkspacePatch(data []byte) (string, error) {
	f, err := os.CreateTemp("", "safe-agentic-patch-*.patch")
	if err != nil {
		return "", fmt.Errorf("create temp patch: %w", err)
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write temp patch: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close temp patch: %w", err)
	}
	return path, nil
}

func validateWorkspacePatch(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("patch is empty")
	}
	text := string(data)
	if strings.Contains(text, "\x00") {
		return fmt.Errorf("patch contains NUL byte")
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "diff --git ") {
			fields := strings.Fields(line)
			for _, field := range fields[1:] {
				path := strings.TrimPrefix(strings.TrimPrefix(field, "a/"), "b/")
				if path == "/dev/null" {
					continue
				}
				if filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, "../") {
					return fmt.Errorf("patch path %q escapes workspace", path)
				}
			}
		}
	}
	return nil
}

func cleanWorkspacePaths(paths []string) ([]string, error) {
	cleaned := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, fmt.Errorf("empty path")
		}
		if strings.Contains(p, "\x00") {
			return nil, fmt.Errorf("path contains NUL byte")
		}
		clean := filepath.Clean(p)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("path %q escapes workspace", p)
		}
		cleaned = append(cleaned, clean)
	}
	return cleaned, nil
}

func confirmWorkspaceRevert(paths []string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, fmt.Errorf("workspace revert requires --yes when stdin is not a terminal")
	}
	fmt.Printf("Discard changes to %s? Type yes to continue: ", strings.Join(paths, ", "))
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(answer) == "yes", nil
}
