package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/orb"

	"github.com/spf13/cobra"
)

// workspaceFindCmd returns the shell snippet that cd's into the first git repo
// in /workspace, handling both /workspace/repo and /workspace/org/repo layouts.
func workspaceFindCmd() string {
	return `repo_dir=$(find /workspace -mindepth 1 -maxdepth 4 -name .git -type d -exec dirname {} \; 2>/dev/null | head -1); ` +
		`if [ -n "$repo_dir" ]; then cd "$repo_dir"; else cd /workspace; fi`
}

// workspaceExec builds a docker exec command that cds into the first git repo in /workspace.
// Uses find to locate the .git directory, handling org/repo nested layouts.
func workspaceExec(containerName string, gitCmd string) []string {
	return []string{
		"docker", "exec", containerName,
		"bash", "-c",
		fmt.Sprintf("%s && %s", workspaceFindCmd(), gitCmd),
	}
}

func workspaceExecCommand(containerName string, args ...string) []string {
	cmd := []string{
		"docker", "exec", containerName,
		"bash", "-lc",
		fmt.Sprintf("%s && exec \"$@\"", workspaceFindCmd()),
		"bash",
	}
	return append(cmd, args...)
}

// ─── diff ──────────────────────────────────────────────────────────────────

var diffStat bool

var diffCmd = &cobra.Command{
	Use:   "diff [name|--latest]",
	Short: "Show git diff of agent changes",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDiff,
}

func init() {
	diffCmd.Flags().BoolVar(&diffStat, "stat", false, "Show diffstat instead of full diff")
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := ""
	if len(args) > 0 {
		target = args[0]
	}
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	gitArg := "git diff"
	if diffStat {
		gitArg = "git diff --stat"
	}

	running, _ := docker.IsRunning(ctx, exec, name)
	var out []byte
	if running {
		out, err = exec.Run(ctx, workspaceExec(name, gitArg)...)
	} else {
		out, err = runGitOnStoppedWorkspace(ctx, exec, name, gitArg)
	}
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}
	fmt.Print(string(out))
	return nil
}

// ─── checkpoint ────────────────────────────────────────────────────────────

var checkpointCmd = &cobra.Command{
	Use:   "checkpoint",
	Short: "Manage working tree snapshots",
}

func init() {
	rootCmd.AddCommand(checkpointCmd)
}

// checkpoint create

var checkpointCreateCmd = &cobra.Command{
	Use:   "create <name|--latest> [label]",
	Short: "Create a working tree snapshot",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runCheckpointCreate,
}

func init() {
	checkpointCmd.AddCommand(checkpointCreateCmd)
}

func runCheckpointCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := args[0]
	label := "snapshot"
	if len(args) >= 2 {
		label = args[1]
	}

	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	// Stash changes inside the workspace
	stashMsg := fmt.Sprintf("checkpoint: %s", label)
	stashCmd := fmt.Sprintf("git stash push -m %q", stashMsg)
	out, err := exec.Run(ctx, workspaceExec(name, stashCmd)...)
	if err != nil {
		return fmt.Errorf("git stash: %w", err)
	}
	stashOutput := strings.TrimSpace(string(out))

	// docker commit to capture the full container state
	imageTag := fmt.Sprintf("safe-agentic-checkpoint:%s-%s", name, label)
	_, err = exec.Run(ctx, "docker", "commit", name, imageTag)
	if err != nil {
		return fmt.Errorf("docker commit: %w", err)
	}

	fmt.Printf("Checkpoint created: %s\n", imageTag)
	if stashOutput != "" {
		fmt.Printf("Stash: %s\n", stashOutput)
	}
	return nil
}

// checkpoint list

var checkpointListCmd = &cobra.Command{
	Use:   "list <name|--latest>",
	Short: "List working tree snapshots",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCheckpointList,
}

func init() {
	checkpointCmd.AddCommand(checkpointListCmd)
}

func runCheckpointList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := ""
	if len(args) > 0 {
		target = args[0]
	}
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	out, err := exec.Run(ctx, workspaceExec(name, "git stash list")...)
	if err != nil {
		return fmt.Errorf("git stash list: %w", err)
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		fmt.Println("No checkpoints found.")
		return nil
	}
	fmt.Println(output)
	return nil
}

// checkpoint restore

var checkpointRestoreCmd = &cobra.Command{
	Use:     "restore <name|--latest> <ref>",
	Aliases: []string{"revert"},
	Short:   "Restore a working tree snapshot",
	Args:    cobra.ExactArgs(2),
	RunE:    runCheckpointRevert,
}

func init() {
	checkpointCmd.AddCommand(checkpointRestoreCmd)
}

var validStashRef = regexp.MustCompile(`^(stash@\{\d+\}|\d+)$`)

func runCheckpointRevert(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := args[0]
	ref := args[1]

	// Validate ref to prevent shell injection — only allow stash@{N} or plain integers.
	if !validStashRef.MatchString(ref) {
		return fmt.Errorf("invalid stash ref %q: must be stash@{N} or a numeric index", ref)
	}

	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	popCmd := fmt.Sprintf("git stash pop %s", ref)
	out, err := exec.Run(ctx, workspaceExec(name, popCmd)...)
	if err != nil {
		return fmt.Errorf("git stash pop: %w", err)
	}
	fmt.Print(string(out))
	return nil
}

// ─── todo ──────────────────────────────────────────────────────────────────

type todoItem struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

var todoCmd = &cobra.Command{
	Use:   "todo",
	Short: "Manage merge requirement todos",
}

func init() {
	rootCmd.AddCommand(todoCmd)
}

// readTodos fetches the todos.json from the container.
func readTodos(ctx context.Context, exec orb.Executor, containerName string) ([]todoItem, error) {
	out, err := exec.Run(ctx, "docker", "exec", containerName,
		"bash", "-c", "cat /workspace/.safe-agentic/todos.json 2>/dev/null || echo '[]'")
	if err != nil {
		return nil, fmt.Errorf("read todos: %w", err)
	}
	var items []todoItem
	if err := json.Unmarshal(out, &items); err != nil {
		// Return empty list if file is empty or malformed
		return []todoItem{}, nil
	}
	return items, nil
}

// writeTodos writes todos back to the container.
func writeTodos(ctx context.Context, exec orb.Executor, containerName string, items []todoItem) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal todos: %w", err)
	}
	// Use printf to avoid heredoc quoting issues; escape single quotes in the JSON.
	jsonStr := strings.ReplaceAll(string(data), "'", "'\\''")
	writeCmd := fmt.Sprintf(
		"mkdir -p /workspace/.safe-agentic && printf '%%s' '%s' > /workspace/.safe-agentic/todos.json",
		jsonStr,
	)
	_, err = exec.Run(ctx, "docker", "exec", containerName, "bash", "-c", writeCmd)
	if err != nil {
		return fmt.Errorf("write todos: %w", err)
	}
	return nil
}

// todo add

var todoAddCmd = &cobra.Command{
	Use:   "add <name|--latest> <text>",
	Short: "Add a merge requirement todo",
	Args:  cobra.ExactArgs(2),
	RunE:  runTodoAdd,
}

func init() {
	todoCmd.AddCommand(todoAddCmd)
}

func runTodoAdd(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := args[0]
	text := args[1]

	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	items, err := readTodos(ctx, exec, name)
	if err != nil {
		return err
	}

	items = append(items, todoItem{Text: text, Done: false})

	if err := writeTodos(ctx, exec, name, items); err != nil {
		return err
	}

	fmt.Printf("Added: [ ] %s\n", text)
	return nil
}

// todo list

var todoListCmd = &cobra.Command{
	Use:   "list <name|--latest>",
	Short: "List merge requirement todos",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTodoList,
}

func init() {
	todoCmd.AddCommand(todoListCmd)
}

func runTodoList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := ""
	if len(args) > 0 {
		target = args[0]
	}
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	items, err := readTodos(ctx, exec, name)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		fmt.Println("No todos.")
		return nil
	}

	for i, item := range items {
		check := " "
		if item.Done {
			check = "x"
		}
		fmt.Printf("%d. [%s] %s\n", i+1, check, item.Text)
	}
	return nil
}

// todo check

var todoCheckCmd = &cobra.Command{
	Use:   "check <name|--latest> <index>",
	Short: "Mark a todo as done (1-based index)",
	Args:  cobra.ExactArgs(2),
	RunE:  runTodoCheck,
}

func init() {
	todoCmd.AddCommand(todoCheckCmd)
}

func runTodoCheck(cmd *cobra.Command, args []string) error {
	return setTodoDone(args, true)
}

// todo uncheck

var todoUncheckCmd = &cobra.Command{
	Use:   "uncheck <name|--latest> <index>",
	Short: "Mark a todo as not done (1-based index)",
	Args:  cobra.ExactArgs(2),
	RunE:  runTodoUncheck,
}

func init() {
	todoCmd.AddCommand(todoUncheckCmd)
}

func runTodoUncheck(cmd *cobra.Command, args []string) error {
	return setTodoDone(args, false)
}

func setTodoDone(args []string, done bool) error {
	ctx := context.Background()
	exec := newExecutor()

	target := args[0]
	var idx int
	if _, err := fmt.Sscanf(args[1], "%d", &idx); err != nil || idx < 1 {
		return fmt.Errorf("index must be a positive integer")
	}

	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	items, err := readTodos(ctx, exec, name)
	if err != nil {
		return err
	}

	if idx > len(items) {
		return fmt.Errorf("index %d out of range (have %d todos)", idx, len(items))
	}

	items[idx-1].Done = done

	if err := writeTodos(ctx, exec, name, items); err != nil {
		return err
	}

	mark := " "
	if done {
		mark = "x"
	}
	fmt.Printf("Updated: [%s] %s\n", mark, items[idx-1].Text)
	return nil
}

// ─── pr ────────────────────────────────────────────────────────────────────

var (
	prTitle string
	prBase  string
)

var prCmd = &cobra.Command{
	Use:   "pr [name|--latest]",
	Short: "Create GitHub PR from agent changes",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPR,
}

func init() {
	prCmd.Flags().StringVar(&prTitle, "title", "", "PR title")
	prCmd.Flags().StringVar(&prBase, "base", "main", "Base branch for PR")
	rootCmd.AddCommand(prCmd)
}

var validBranchName = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func runPR(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := ""
	if len(args) > 0 {
		target = args[0]
	}
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	// Validate prBase to prevent shell injection — only allow branch name characters.
	if !validBranchName.MatchString(prBase) {
		return fmt.Errorf("invalid base branch: %s", prBase)
	}

	// Push current branch
	pushOut, err := exec.Run(ctx, workspaceExec(name, "git push -u origin HEAD")...)
	if err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	if s := strings.TrimSpace(string(pushOut)); s != "" {
		fmt.Println(s)
	}

	ghArgs := []string{"gh", "pr", "create", "--base", prBase, "--fill"}
	if prTitle != "" {
		ghArgs = []string{"gh", "pr", "create", "--title", prTitle, "--base", prBase, "--fill"}
	}

	out, err := exec.Run(ctx, workspaceExecCommand(name, ghArgs...)...)
	if err != nil {
		return fmt.Errorf("gh pr create: %w", err)
	}
	fmt.Print(string(out))
	return nil
}

// ─── review ────────────────────────────────────────────────────────────────

var reviewBase string

var reviewCmd = &cobra.Command{
	Use:   "review [name|--latest]",
	Short: "Run AI code review on agent changes",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReview,
}

func init() {
	reviewCmd.Flags().StringVar(&reviewBase, "base", "main", "Base branch for review diff")
	rootCmd.AddCommand(reviewCmd)
}

func runReview(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := ""
	if len(args) > 0 {
		target = args[0]
	}
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	// Validate reviewBase to prevent shell injection — only allow branch name characters.
	if !validBranchName.MatchString(reviewBase) {
		return fmt.Errorf("invalid base branch: %s", reviewBase)
	}

	// Try codex review first
	codexCmd := fmt.Sprintf("codex review --base %s 2>/dev/null", reviewBase)
	out, err := exec.Run(ctx, workspaceExec(name, codexCmd)...)
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		fmt.Print(string(out))
		return nil
	}

	// Fallback to git diff
	diffCmd := fmt.Sprintf("git diff %s...HEAD", reviewBase)
	out, err = exec.Run(ctx, workspaceExec(name, diffCmd)...)
	if err != nil {
		return fmt.Errorf("git diff %s...HEAD: %w", reviewBase, err)
	}
	fmt.Print(string(out))
	return nil
}
