package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"

	"github.com/spf13/cobra"
	"golang.org/x/term"
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

var (
	diffStat       bool
	diffSideBySide bool
)

var diffCmd = &cobra.Command{
	Use:   "diff [name|--latest]",
	Short: "Show git diff of agent changes",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDiff,
}

func init() {
	diffCmd.Flags().BoolVar(&diffStat, "stat", false, "Show diffstat instead of full diff")
	diffCmd.Flags().BoolVarP(&diffSideBySide, "side-by-side", "s", false,
		"Render the diff side-by-side with delta (falls back to plain diff if delta is unavailable)")
	rootCmd.AddCommand(diffCmd)
}

func runDiff(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	if diffStat && diffSideBySide {
		return fmt.Errorf("--stat and --side-by-side are mutually exclusive")
	}

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

	if diffSideBySide {
		if running && deltaAvailable(ctx, exec, name) {
			gitArg = fmt.Sprintf("git diff | COLUMNS=%d delta --side-by-side", terminalWidth())
		} else {
			fmt.Fprintln(os.Stderr, "warning: delta not available in agent image, falling back to plain diff")
		}
	}

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

// deltaAvailable reports whether the delta pager binary is present in the
// running agent container's image. Older images built before delta was
// baked in won't have it, so callers must fall back to a plain diff.
func deltaAvailable(ctx context.Context, exec vmexec.Executor, name string) bool {
	_, err := exec.Run(ctx, "docker", "exec", name, "sh", "-c", "command -v delta")
	return err == nil
}

// terminalWidth returns the host terminal's column width so delta's
// side-by-side rendering can be sized to fit. Falls back to $COLUMNS and
// finally a sane default when no terminal is attached (e.g. piped output).
func terminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	if v := os.Getenv("COLUMNS"); v != "" {
		if w, err := strconv.Atoi(v); err == nil && w > 0 {
			return w
		}
	}
	return 160
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
	out, err := exec.Run(ctx, workspaceExecCommand(name, "git", "stash", "push", "-m", stashMsg)...)
	if err != nil {
		return fmt.Errorf("git stash: %w", err)
	}
	stashOutput := strings.TrimSpace(string(out))

	// docker commit to capture the full container state
	imageTag := fmt.Sprintf("safe-agentic-checkpoint:%s-%s", name, checkpointTagSuffix(label))
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

func checkpointTagSuffix(label string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range label {
		allowed := (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if allowed {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
		if b.Len() >= 80 {
			break
		}
	}
	suffix := strings.Trim(b.String(), ".-_")
	if suffix == "" {
		return "snapshot"
	}
	return suffix
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

	out, err := exec.Run(ctx, workspaceExecCommand(name, "git", "stash", "pop", ref)...)
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
func readTodos(ctx context.Context, exec vmexec.Executor, containerName string) ([]todoItem, error) {
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
func writeTodos(ctx context.Context, exec vmexec.Executor, containerName string, items []todoItem) error {
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal todos: %w", err)
	}
	payload := base64.StdEncoding.EncodeToString(data)
	writeCmd := "mkdir -p /workspace/.safe-agentic && printf %s \"$1\" | base64 -d > /workspace/.safe-agentic/todos.json"
	_, err = exec.Run(ctx, "docker", "exec", containerName, "bash", "-lc", writeCmd, "bash", payload)
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

// reviewRiskInstructions is appended to the codex review invocation so every
// finding carries a risk tag and file:line, and the review ends with a
// one-line verdict — both of which we parse for the terminal risk summary.
const reviewRiskInstructions = `For every finding, prefix the line with a risk tag [HIGH], [MEDIUM], or [LOW] ` +
	`followed by the file:line location, e.g. "[HIGH] pkg/foo.go:42: description". ` +
	`End the review with a single line starting with "VERDICT:" that summarizes the overall risk in one sentence.`

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
	out, err := exec.Run(ctx, workspaceExecCommand(name, "codex", "review", "--base", reviewBase, reviewRiskInstructions)...)
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		text := string(out)
		fmt.Print(text)
		fmt.Print(renderRiskSummary(parseRiskFindings(text)))
		return nil
	}

	// Fallback to git diff
	out, err = exec.Run(ctx, workspaceExecCommand(name, "git", "diff", reviewBase+"...HEAD")...)
	if err != nil {
		return fmt.Errorf("git diff %s...HEAD: %w", reviewBase, err)
	}
	fmt.Print(string(out))
	return nil
}

// ─── review risk parsing ─────────────────────────────────────────────────────

// riskFindings groups review findings by risk level, preserving anything that
// didn't carry a recognizable tag under Untagged rather than dropping it.
type riskFindings struct {
	High     []string
	Medium   []string
	Low      []string
	Untagged []string
	Verdict  string
}

var (
	riskTagPattern = regexp.MustCompile(`(?i)\[\s*(high|medium|low)\s*\]`)
	verdictPattern = regexp.MustCompile(`(?i)^verdict\s*:\s*(.+)$`)
)

// parseRiskFindings scans review output line by line, bucketing each
// non-empty line by its [HIGH]/[MEDIUM]/[LOW] tag. Lines without a tag are
// kept under Untagged. A trailing "VERDICT: ..." line is extracted
// separately and excluded from the buckets.
func parseRiskFindings(text string) riskFindings {
	var rf riskFindings
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := verdictPattern.FindStringSubmatch(line); m != nil {
			rf.Verdict = strings.TrimSpace(m[1])
			continue
		}
		if m := riskTagPattern.FindStringSubmatch(line); m != nil {
			switch strings.ToLower(m[1]) {
			case "high":
				rf.High = append(rf.High, line)
			case "medium":
				rf.Medium = append(rf.Medium, line)
			case "low":
				rf.Low = append(rf.Low, line)
			}
			continue
		}
		rf.Untagged = append(rf.Untagged, line)
	}
	return rf
}

// renderRiskSummary renders a HIGH → LOW grouped summary plus the verdict, to
// be printed after the raw review text so the original output stays intact.
func renderRiskSummary(rf riskFindings) string {
	groups := []struct {
		label string
		items []string
	}{
		{"HIGH", rf.High},
		{"MEDIUM", rf.Medium},
		{"LOW", rf.Low},
		{"UNTAGGED", rf.Untagged},
	}

	var b strings.Builder
	b.WriteString("\n─────────────────────────────────────────\n")
	b.WriteString("Risk Summary\n")
	b.WriteString("─────────────────────────────────────────\n")
	any := false
	for _, g := range groups {
		if len(g.items) == 0 {
			continue
		}
		any = true
		fmt.Fprintf(&b, "\n%s (%d)\n", g.label, len(g.items))
		for _, item := range g.items {
			fmt.Fprintf(&b, "  %s\n", item)
		}
	}
	if !any {
		b.WriteString("\n(no findings parsed)\n")
	}
	if rf.Verdict != "" {
		fmt.Fprintf(&b, "\nVerdict: %s\n", rf.Verdict)
	}
	return b.String()
}
