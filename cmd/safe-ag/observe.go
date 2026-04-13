package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/audit"
	"github.com/0x666c6f/safe-agentic/pkg/cost"
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/orb"
	"github.com/0x666c6f/safe-agentic/pkg/tmux"

	"github.com/spf13/cobra"
)

// ─── peek ──────────────────────────────────────────────────────────────────

var peekLines int

var peekCmd = &cobra.Command{
	Use:   "peek [name|--latest]",
	Short: "View last N lines of agent output",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPeek,
}

func init() {
	peekCmd.Flags().IntVar(&peekLines, "lines", 30, "Number of lines to show")
	addLatestFlag(peekCmd)
	rootCmd.AddCommand(peekCmd)
}

// ─── logs ──────────────────────────────────────────────────────────────────

var logsLines int
var logsFollow bool

var logsCmd = &cobra.Command{
	Use:   "logs [name|--latest]",
	Short: "Show agent session conversation log",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runLogs,
}

func init() {
	logsCmd.Flags().IntVar(&logsLines, "lines", 50, "Number of entries to show")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
	addLatestFlag(logsCmd)
	rootCmd.AddCommand(logsCmd)
}

func runLogs(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := targetFromArgs(cmd, args)
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	// Detect agent type for config dir
	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	configDir := "/home/agent/.claude"
	if agentType == "codex" {
		configDir = "/home/agent/.codex"
	}

	// Find the session JSONL that matches this container.
	createdAt, _ := inspectField(ctx, exec, name, "{{.Created}}")
	repoLabel, _ := docker.InspectLabel(ctx, exec, name, labels.RepoDisplay)
	searchDirs := sessionSearchDirs(configDir, repoLabel)
	running, _ := docker.IsRunning(ctx, exec, name)

	// Find the most recent JSONL (simpler than Python matching for reliability)
	findCmd := fmt.Sprintf(
		"find %s/projects -name '*.jsonl' -not -path '*/subagents/*' -not -name 'history.jsonl' -type f -printf '%%T@ %%p\\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-",
		configDir)
	// If we have search dirs, prefer the project-specific one
	if len(searchDirs) > 0 && searchDirs[0] != configDir+"/sessions" {
		findCmd = fmt.Sprintf(
			"find %s -name '*.jsonl' -not -path '*/subagents/*' -type f -printf '%%T@ %%p\\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-",
			searchDirs[0])
	}
	_ = createdAt // used for future per-container matching refinement

	out, err := readLatestSessionLog(ctx, exec, name, configDir, searchDirs, findCmd, logsLines*3, running)
	if err != nil {
		return err
	}

	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		rendered := renderLogEntry(line)
		if rendered != "" {
			fmt.Println(rendered)
			count++
			if count >= logsLines {
				break
			}
		}
	}
	return nil
}

func readLatestSessionLog(ctx context.Context, exec orb.Executor, name, configDir string, searchDirs []string, findCmd string, tailCount int, running bool) ([]byte, error) {
	createdAt, _ := inspectField(ctx, exec, name, "{{.Created}}")
	promptHint, _ := docker.InspectLabel(ctx, exec, name, labels.Prompt)
	promptHint = strings.TrimSuffix(promptHint, "...")
	matchScript := `
import os, json, sys, glob, datetime
container_created = sys.argv[1][:19]
prompt_hint = sys.argv[2]
search_dirs = [p for p in sys.argv[3:] if p]

def parse_ts(raw):
    if not raw:
        return None
    raw = raw[:19]
    try:
        return datetime.datetime.fromisoformat(raw)
    except Exception:
        return None

container_dt = parse_ts(container_created)
files = []
seen = set()
for find_dir in search_dirs:
    for pattern in (os.path.join(find_dir, '*.jsonl'), os.path.join(find_dir, '**', '*.jsonl')):
        for f in glob.glob(pattern, recursive=True):
            if f.endswith('history.jsonl') or '/subagents/' in f or f in seen:
                continue
            seen.add(f)
            files.append(f)

best_file = None
best_score = None
for f in files:
    try:
        session_dt = None
        prompt_match = False
        with open(f) as fh:
            for line in fh:
                line = line.strip()
                if not line:
                    continue
                if prompt_hint and prompt_hint in line:
                    prompt_match = True
                d = json.loads(line)
                ts = d.get('timestamp', '')
                if not ts and 'message' in d and isinstance(d['message'], dict):
                    ts = d['message'].get('timestamp', '')
                if ts:
                    session_dt = parse_ts(ts)
                if prompt_match and session_dt is not None:
                    break
        if session_dt is None:
            session_dt = datetime.datetime.fromtimestamp(os.path.getmtime(f))
        score = abs((session_dt - container_dt).total_seconds()) if container_dt else float('inf')
        key = (0 if prompt_match else 1, score)
        if best_score is None or key < best_score:
            best_score = key
            best_file = f
    except Exception:
        pass

if not best_file and files:
    best_file = max(files, key=os.path.getmtime)
if best_file:
    print(best_file)
`

	if running {
		args := append([]string{"docker", "exec", name, "python3", "-c", matchScript, createdAt, promptHint}, searchDirs...)
		out, err := exec.Run(ctx, args...)
		if err != nil {
			return readLatestSessionLog(ctx, exec, name, configDir, searchDirs, findCmd, tailCount, false)
		}
		jsonlPath := strings.TrimSpace(string(out))
		if jsonlPath == "" {
			return nil, fmt.Errorf("no session log found in %s", configDir)
		}
		tailCmd := fmt.Sprintf("tail -n %d %s", tailCount, jsonlPath)
		out, err = exec.Run(ctx, "docker", "exec", name, "bash", "-c", tailCmd)
		if err != nil {
			return readLatestSessionLog(ctx, exec, name, configDir, searchDirs, findCmd, tailCount, false)
		}
		return out, nil
	}

	tmpDir := "/tmp/safe-agentic-logs-" + strings.ReplaceAll(name, "/", "-")
	if _, err := exec.Run(ctx, "bash", "-c", "rm -rf "+shellQuote(tmpDir)+" && mkdir -p "+shellQuote(tmpDir)); err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer exec.Run(context.Background(), "bash", "-c", "rm -rf "+shellQuote(tmpDir))

	if _, err := exec.Run(ctx, "docker", "cp", name+":"+configDir, tmpDir+"/"); err != nil {
		return nil, fmt.Errorf("copy session logs: %w", err)
	}

	localRoot := filepath.Join(tmpDir, filepath.Base(configDir))
	localDirs := make([]string, 0, len(searchDirs))
	for _, dir := range searchDirs {
		localDirs = append(localDirs, filepath.Join(localRoot, strings.TrimPrefix(dir, configDir+"/")))
	}
	args := append([]string{"python3", "-c", matchScript, createdAt, promptHint}, localDirs...)
	out, err := exec.Run(ctx, args...)
	if err == nil {
		jsonlPath := strings.TrimSpace(string(out))
		if jsonlPath != "" {
			tailCmd := fmt.Sprintf("tail -n %d %s", tailCount, shellQuote(jsonlPath))
			out, err = exec.Run(ctx, "bash", "-c", tailCmd)
			if err != nil {
				return nil, fmt.Errorf("read session log: %w", err)
			}
			return out, nil
		}
	}

	localFindCmd := strings.ReplaceAll(findCmd, configDir, localRoot)
	out, err = exec.Run(ctx, "bash", "-c", localFindCmd)
	if err != nil {
		return nil, fmt.Errorf("find session log: %w", err)
	}
	jsonlPath := strings.TrimSpace(string(out))
	if jsonlPath == "" {
		return nil, fmt.Errorf("no session log found in %s", configDir)
	}
	tailCmd := fmt.Sprintf("tail -n %d %s", tailCount, shellQuote(jsonlPath))
	out, err = exec.Run(ctx, "bash", "-c", tailCmd)
	if err != nil {
		return nil, fmt.Errorf("read session log: %w", err)
	}
	return out, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// ensureRunning starts a stopped container and returns a cleanup function
// that stops it when done. If already running, cleanup is a no-op.
func ensureRunning(ctx context.Context, exec orb.Executor, name string) (func(), error) {
	running, _ := docker.IsRunning(ctx, exec, name)
	if running {
		return func() {}, nil
	}
	if _, err := exec.Run(ctx, "docker", "start", name); err != nil {
		return func() {}, fmt.Errorf("start container %s: %w", name, err)
	}
	time.Sleep(2 * time.Second)
	return func() {
		exec.Run(ctx, "docker", "stop", "-t", "5", name)
	}, nil
}

func sessionSearchDirs(configDir, repo string) []string {
	dirs := make([]string, 0, 3)
	if repo != "" && repo != "-" {
		projSlug := strings.ReplaceAll(repo, "/", "-")
		dirs = append(dirs, fmt.Sprintf("%s/projects/-workspace-%s", configDir, projSlug))
	}
	dirs = append(dirs, configDir+"/sessions", configDir)
	return dirs
}

func renderLogEntry(line string) string {
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return ""
	}

	switch entryType(entry) {
	case "user":
		return renderUserLogEntry(entry)
	case "assistant":
		return renderAssistantLogEntry(entry)
	case "system":
		return renderSystemLogEntry(entry)
	default:
		return ""
	}
}

func entryType(entry map[string]interface{}) string {
	entryType, _ := entry["type"].(string)
	return entryType
}

func renderUserLogEntry(entry map[string]interface{}) string {
	msg := entryMessage(entry)
	if msg == nil {
		return ""
	}
	role, _ := msg["role"].(string)
	if role != "user" {
		return ""
	}
	content := extractUserText(msg)
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\033[0;36m> %s\033[0m", truncateObserveText(content, 200))
}

func entryMessage(entry map[string]interface{}) map[string]interface{} {
	msg, _ := entry["message"].(map[string]interface{})
	return msg
}

func extractUserText(msg map[string]interface{}) string {
	if content, _ := msg["content"].(string); content != "" {
		return content
	}
	blocks, ok := msg["content"].([]interface{})
	if !ok {
		return ""
	}
	for _, item := range blocks {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if textBlockContent(block) != "" {
			return textBlockContent(block)
		}
	}
	return ""
}

func textBlockContent(block map[string]interface{}) string {
	bType, _ := block["type"].(string)
	if bType != "text" {
		return ""
	}
	text, _ := block["text"].(string)
	return text
}

func truncateObserveText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func renderAssistantLogEntry(entry map[string]interface{}) string {
	msg := entryMessage(entry)
	if msg == nil {
		return ""
	}
	content := extractAssistantText(msg)
	if content == "" {
		return ""
	}
	return fmt.Sprintf("\033[0;32m  %s\033[0m", truncateObserveText(content, 300))
}

func renderSystemLogEntry(entry map[string]interface{}) string {
	subtype, _ := entry["subtype"].(string)
	switch subtype {
	case "tool_use":
		return ""
	case "result":
		return renderSystemResult(entry)
	default:
		return ""
	}
}

func renderSystemResult(entry map[string]interface{}) string {
	dur, _ := entry["durationMs"].(float64)
	msgCount, _ := entry["messageCount"].(float64)
	if dur <= 0 {
		return ""
	}
	return fmt.Sprintf("\033[0;33m  [%d messages, %.1fs]\033[0m", int(msgCount), dur/1000)
}

func extractAssistantText(msg map[string]interface{}) string {
	content, ok := msg["content"]
	if !ok {
		return ""
	}
	// String content
	if s, ok := content.(string); ok {
		return s
	}
	// Array of content blocks
	blocks, ok := content.([]interface{})
	if !ok {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		block, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		bType, _ := block["type"].(string)
		switch bType {
		case "text":
			if t, ok := block["text"].(string); ok && t != "" {
				parts = append(parts, t)
			}
		case "tool_use":
			toolName, _ := block["name"].(string)
			parts = append(parts, fmt.Sprintf("[tool: %s]", toolName))
		}
	}
	return strings.Join(parts, " ")
}

func runPeek(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := targetFromArgs(cmd, args)
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	// Verify container is running
	running, err := docker.IsRunning(ctx, exec, name)
	if err != nil {
		return fmt.Errorf("inspect container %s: %w", name, err)
	}
	if !running {
		return fmt.Errorf("container %s is not running", name)
	}

	// Check tmux mode
	termMode, _ := docker.InspectLabel(ctx, exec, name, labels.Terminal)
	usesTmux := termMode == "tmux" || termMode == ""

	if !usesTmux {
		return fmt.Errorf("container %s is not in tmux mode (terminal=%q)", name, termMode)
	}

	// Capture pane output
	captureArgs := tmux.BuildCapturePaneArgs(name, peekLines)
	out, err := exec.Run(ctx, captureArgs...)
	if err != nil {
		return fmt.Errorf("capture tmux pane: %w", err)
	}
	fmt.Print(string(out))
	return nil
}

// ─── output ────────────────────────────────────────────────────────────────

var (
	outputDiff    bool
	outputFiles   bool
	outputCommits bool
	outputJSON    bool
)

var outputCmd = &cobra.Command{
	Use:   "output [name|--latest]",
	Short: "Show agent output or changes",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runOutput,
}

func init() {
	outputCmd.Flags().BoolVar(&outputDiff, "diff", false, "Show git diff")
	outputCmd.Flags().BoolVar(&outputFiles, "files", false, "Show changed files")
	outputCmd.Flags().BoolVar(&outputCommits, "commits", false, "Show git commit log")
	outputCmd.Flags().BoolVar(&outputJSON, "json", false, "Output as JSON")
	addLatestFlag(outputCmd)
	rootCmd.AddCommand(outputCmd)
}

func runOutput(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := targetFromArgs(cmd, args)
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	wsCmd := workspaceFindCmd()
	running, _ := docker.IsRunning(ctx, exec, name)
	if label, gitCmd, ok := outputGitMode(); ok {
		return runOutputGitMode(ctx, exec, name, wsCmd, running, label, gitCmd)
	}
	if outputJSON {
		return runOutputJSONMode(ctx, exec, name)
	}
	return runOutputDefaultMode(ctx, exec, name, running)
}

func outputGitMode() (label, gitCmd string, ok bool) {
	switch {
	case outputDiff:
		return "git diff", "git diff", true
	case outputFiles:
		return "list changed files", "git diff --name-only && git ls-files --others --exclude-standard", true
	case outputCommits:
		return "git log", "git log --oneline", true
	default:
		return "", "", false
	}
}

func runOutputGitMode(ctx context.Context, exec orb.Executor, name, wsCmd string, running bool, label, gitCmd string) error {
	out, err := runOutputGitCommand(ctx, exec, name, wsCmd, running, gitCmd)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	fmt.Print(string(out))
	return nil
}

func runOutputGitCommand(ctx context.Context, exec orb.Executor, name, wsCmd string, running bool, gitCmd string) ([]byte, error) {
	if running {
		return exec.Run(ctx, "docker", "exec", name, "bash", "-c", wsCmd+" && "+gitCmd)
	}
	return runGitOnStoppedWorkspace(ctx, exec, name, gitCmd)
}

func runOutputJSONMode(ctx context.Context, exec orb.Executor, name string) error {
	statusOut, _ := exec.Run(ctx, "docker", "inspect", "--format", "{{.State.Status}}", name)
	result := map[string]string{
		"name":        name,
		"status":      strings.TrimSpace(string(statusOut)),
		"last_output": outputLastMessage(ctx, exec, name),
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func outputLastMessage(ctx context.Context, exec orb.Executor, name string) string {
	if lastMessage, _ := readLastSessionMessage(ctx, exec, name); lastMessage != "" {
		return lastMessage
	}
	if rendered, _ := readRenderedAgentLogs(ctx, exec, name, 200); rendered != "" {
		if summary := summarizeAgentLogs(rendered); summary != "" {
			return summary
		}
	}
	logsOut, _ := exec.Run(ctx, "docker", "logs", "--tail", "20", name)
	lastMessage := strings.TrimSpace(string(logsOut))
	if len(lastMessage) > 500 {
		return lastMessage[len(lastMessage)-500:]
	}
	return lastMessage
}

func runOutputDefaultMode(ctx context.Context, exec orb.Executor, name string, running bool) error {
	if lastMessage, _ := readLastSessionMessage(ctx, exec, name); strings.TrimSpace(lastMessage) != "" {
		fmt.Println(lastMessage)
		return nil
	}
	if rendered, _ := readRenderedAgentLogs(ctx, exec, name, 50); strings.TrimSpace(rendered) != "" {
		if summary := summarizeAgentLogs(rendered); summary != "" {
			fmt.Println(summary)
			return nil
		}
		fmt.Println(rendered)
		return nil
	}
	if paneOut, ok := readTmuxPaneOutput(ctx, exec, name, running); ok {
		fmt.Print(string(paneOut))
		return nil
	}
	out, err := exec.Run(ctx, "docker", "logs", "--tail", "80", name)
	if err != nil {
		return fmt.Errorf("docker logs: %w", err)
	}
	fmt.Print(string(out))
	return nil
}

func readTmuxPaneOutput(ctx context.Context, exec orb.Executor, name string, running bool) ([]byte, bool) {
	if !running {
		return nil, false
	}
	termLabel, _ := docker.InspectLabel(ctx, exec, name, "safe-agentic.terminal")
	if termLabel != "tmux" {
		return nil, false
	}
	paneOut, err := exec.Run(ctx, tmux.BuildCapturePaneArgs(name, 80)...)
	if err != nil || len(strings.TrimSpace(string(paneOut))) == 0 {
		return nil, false
	}
	return paneOut, true
}

func readRenderedAgentLogs(ctx context.Context, exec orb.Executor, name string, lines int) (string, error) {
	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	configDir := agentConfigDir(agentType)
	repoLabel, _ := docker.InspectLabel(ctx, exec, name, labels.RepoDisplay)
	searchDirs := sessionSearchDirs(configDir, repoLabel)
	running, _ := docker.IsRunning(ctx, exec, name)
	findCmd := fmt.Sprintf(
		"find %s -name '*.jsonl' -not -path '*/subagents/*' -not -name 'history.jsonl' -type f -printf '%%T@ %%p\\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-",
		searchDirs[0])

	data, err := readLatestSessionLog(ctx, exec, name, configDir, searchDirs, findCmd, lines*3, running)
	if err != nil {
		return "", err
	}

	rendered := make([]string, 0, lines)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		entry := renderLogEntry(scanner.Text())
		if entry == "" {
			continue
		}
		rendered = append(rendered, entry)
		if len(rendered) >= lines {
			break
		}
	}
	return strings.TrimSpace(strings.Join(rendered, "\n")), nil
}

func summarizeAgentLogs(logs string) string {
	logs = stripANSI(logs)
	var last string
	for _, line := range strings.Split(logs, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ">") || strings.HasPrefix(line, "[tool:") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			continue
		}
		last = line
	}
	return last
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inEsc {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEsc = false
			}
			continue
		}
		if c == 0x1b {
			inEsc = true
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func readLastSessionMessage(ctx context.Context, exec orb.Executor, name string) (string, error) {
	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	configDir := agentConfigDir(agentType)
	repoLabel, _ := docker.InspectLabel(ctx, exec, name, labels.RepoDisplay)
	searchDirs := sessionSearchDirs(configDir, repoLabel)
	running, _ := docker.IsRunning(ctx, exec, name)
	findCmd := fmt.Sprintf(
		"find %s -name '*.jsonl' -not -path '*/subagents/*' -not -name 'history.jsonl' -type f -printf '%%T@ %%p\\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-",
		searchDirs[0])
	data, err := readLatestSessionLog(ctx, exec, name, configDir, searchDirs, findCmd, 400, running)
	if err != nil {
		return "", err
	}
	return extractLastAssistantMessage(data), nil
}

func extractLastAssistantMessage(data []byte) string {
	var last string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}
		if msgRaw, ok := obj["message"]; ok {
			var msg map[string]interface{}
			if err := json.Unmarshal(msgRaw, &msg); err == nil {
				if role, _ := msg["role"].(string); role == "assistant" {
					if content := extractAssistantTextForOutput(msg); strings.TrimSpace(content) != "" {
						last = strings.TrimSpace(content)
					}
				}
			}
		}
		if typ := jsonString(obj, "type"); typ == "assistant" {
			var msg map[string]interface{}
			if msgRaw, ok := obj["message"]; ok && json.Unmarshal(msgRaw, &msg) == nil {
				if content := extractAssistantTextForOutput(msg); strings.TrimSpace(content) != "" {
					last = strings.TrimSpace(content)
				}
			}
		}
	}
	return last
}

func extractAssistantTextForOutput(msg map[string]interface{}) string {
	content, ok := msg["content"]
	if !ok {
		return ""
	}
	if s, ok := content.(string); ok {
		return strings.TrimSpace(s)
	}
	blocks, ok := content.([]interface{})
	if !ok {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		block, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		bType, _ := block["type"].(string)
		switch bType {
		case "text", "output_text", "input_text":
			if t, ok := block["text"].(string); ok && strings.TrimSpace(t) != "" {
				parts = append(parts, strings.TrimSpace(t))
			}
		}
	}
	return strings.Join(parts, "\n")
}

func runGitOnStoppedWorkspace(ctx context.Context, exec orb.Executor, name, gitCmd string) ([]byte, error) {
	tmpDir := "/tmp/safe-agentic-workspace-" + strings.ReplaceAll(name, "/", "-")
	if _, err := exec.Run(ctx, "bash", "-c", "rm -rf "+shellQuote(tmpDir)+" && mkdir -p "+shellQuote(tmpDir)); err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer exec.Run(context.Background(), "bash", "-c", "rm -rf "+shellQuote(tmpDir))
	if _, err := exec.Run(ctx, "docker", "cp", name+":/workspace", tmpDir+"/"); err != nil {
		return nil, fmt.Errorf("copy workspace: %w", err)
	}
	localFind := strings.ReplaceAll(workspaceFindCmd(), "/workspace", tmpDir+"/workspace")
	out, err := exec.Run(ctx, "bash", "-c", localFind+" && "+gitCmd)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ─── summary ───────────────────────────────────────────────────────────────

var summaryCmd = &cobra.Command{
	Use:   "summary [name|--latest]",
	Short: "Show detailed agent summary",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSummary,
}

func init() {
	addLatestFlag(summaryCmd)
	rootCmd.AddCommand(summaryCmd)
}

func runSummary(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := targetFromArgs(cmd, args)
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	// Inspect state fields
	stateStatus, _ := inspectField(ctx, exec, name, "{{.State.Status}}")
	startedAt, _ := inspectField(ctx, exec, name, "{{.State.StartedAt}}")
	finishedAt, _ := inspectField(ctx, exec, name, "{{.State.FinishedAt}}")

	// Labels
	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	repo, _ := docker.InspectLabel(ctx, exec, name, labels.RepoDisplay)
	ssh, _ := docker.InspectLabel(ctx, exec, name, labels.SSH)
	auth, _ := docker.InspectLabel(ctx, exec, name, labels.AuthType)
	dockerMode, _ := docker.InspectLabel(ctx, exec, name, labels.DockerMode)
	networkMode, _ := docker.InspectLabel(ctx, exec, name, labels.NetworkMode)
	resources, _ := docker.InspectLabel(ctx, exec, name, labels.Resources)
	terminal, _ := docker.InspectLabel(ctx, exec, name, labels.Terminal)

	fmt.Printf("Container:  %s\n", name)
	fmt.Println("─────────────────────────────────────────")
	fmt.Printf("Status:     %s\n", stateStatus)
	fmt.Printf("Started:    %s\n", startedAt)
	if stateStatus != "running" {
		fmt.Printf("Finished:   %s\n", finishedAt)
	}
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Printf("  Agent type:   %s\n", agentType)
	fmt.Printf("  Repository:   %s\n", repo)
	fmt.Printf("  SSH:          %s\n", ssh)
	fmt.Printf("  Auth:         %s\n", auth)
	fmt.Printf("  Docker:       %s\n", dockerMode)
	fmt.Printf("  Network:      %s\n", networkMode)
	fmt.Printf("  Resources:    %s\n", resources)
	fmt.Printf("  Terminal:     %s\n", terminal)

	return nil
}

// inspectField runs docker inspect with a Go template and returns trimmed output.
func inspectField(ctx context.Context, exec orb.Executor, name, tmpl string) (string, error) {
	out, err := exec.Run(ctx, "docker", "inspect", "--format", tmpl, name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ─── cost ──────────────────────────────────────────────────────────────────

var costHistory string

var costCmd = &cobra.Command{
	Use:   "cost [name|--latest]",
	Short: "Estimate API cost from session data",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runCost,
}

func init() {
	costCmd.Flags().StringVar(&costHistory, "history", "", "Show historical costs (e.g. 7d, 30d)")
	addLatestFlag(costCmd)
	rootCmd.AddCommand(costCmd)
}

func runCost(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	if costHistory != "" {
		return runCostHistory(ctx, exec, costHistory)
	}

	target := targetFromArgs(cmd, args)
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	return runCostForContainer(ctx, exec, name)
}

func runCostForContainer(ctx context.Context, exec orb.Executor, name string) error {
	// Ensure container is running for docker exec
	stop, err := ensureRunning(ctx, exec, name)
	if err != nil {
		return err
	}
	defer stop()

	// Detect agent type to find config dir
	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	configDir := agentConfigDir(agentType)

	// Find JSONL session files
	findOut, err := exec.Run(ctx, "docker", "exec", name,
		"find", configDir, "-name", "*.jsonl", "-not", "-path", "*/subagents/*")
	if err != nil {
		return fmt.Errorf("find session files: %w", err)
	}

	files := splitLines(string(findOut))
	if len(files) == 0 {
		fmt.Printf("No session files found in %s for %s\n", configDir, name)
		return nil
	}

	// Read all files and parse token usage
	var usages []cost.TokenUsage
	for _, f := range files {
		catOut, err := exec.Run(ctx, "docker", "exec", name, "cat", f)
		if err != nil {
			continue
		}
		usages = append(usages, extractTokenUsage(catOut)...)
	}

	total := cost.SumCost(usages)
	var totalInput, totalOutput int64
	for _, u := range usages {
		totalInput += u.InputTokens
		totalOutput += u.OutputTokens
	}

	fmt.Printf("Container:     %s\n", name)
	fmt.Printf("Session files: %d\n", len(files))
	fmt.Printf("Input tokens:  %d\n", totalInput)
	fmt.Printf("Output tokens: %d\n", totalOutput)
	fmt.Printf("Estimated cost: $%.4f\n", total)
	return nil
}

func runCostHistory(ctx context.Context, exec orb.Executor, period string) error {
	duration, err := parsePeriod(period)
	if err != nil {
		return fmt.Errorf("parse period %q: %w", period, err)
	}

	logger := &audit.Logger{Path: audit.DefaultPath()}
	entries, err := logger.Read(0) // read all
	if err != nil {
		return fmt.Errorf("read audit log: %w", err)
	}

	cutoff := time.Now().Add(-duration)
	var spawns int
	containersSeen := map[string]bool{}
	for _, e := range entries {
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		if ts.Before(cutoff) {
			continue
		}
		if e.Action == "spawn" {
			spawns++
			containersSeen[e.Container] = true
		}
	}

	fmt.Printf("Period:       %s\n", period)
	fmt.Printf("Since:        %s\n", cutoff.Format(time.RFC3339))
	fmt.Printf("Spawns:       %d\n", spawns)
	fmt.Printf("Containers:   %d unique\n", len(containersSeen))
	fmt.Println("(Per-session cost requires live container access)")
	return nil
}

// extractTokenUsage parses JSONL content looking for token usage fields.
func extractTokenUsage(data []byte) []cost.TokenUsage {
	var usages []cost.TokenUsage
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Bytes()
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}

		// Look for usage/token fields at top level or nested in message
		model := jsonString(obj, "model")

		// Try top-level usage field (OpenAI-style)
		if usageRaw, ok := obj["usage"]; ok {
			var usage struct {
				InputTokens  int64 `json:"input_tokens"`
				OutputTokens int64 `json:"output_tokens"`
				PromptTokens int64 `json:"prompt_tokens"`
				CompTokens   int64 `json:"completion_tokens"`
			}
			if err := json.Unmarshal(usageRaw, &usage); err == nil {
				in := usage.InputTokens + usage.PromptTokens
				out := usage.OutputTokens + usage.CompTokens
				if in > 0 || out > 0 {
					usages = append(usages, cost.TokenUsage{
						Model:        model,
						InputTokens:  in,
						OutputTokens: out,
					})
				}
			}
		}

		// Try message.usage (Claude-style JSONL)
		if msgRaw, ok := obj["message"]; ok {
			var msg struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens  int64 `json:"input_tokens"`
					OutputTokens int64 `json:"output_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(msgRaw, &msg); err == nil {
				if msg.Usage.InputTokens > 0 || msg.Usage.OutputTokens > 0 {
					m := msg.Model
					if m == "" {
						m = model
					}
					usages = append(usages, cost.TokenUsage{
						Model:        m,
						InputTokens:  msg.Usage.InputTokens,
						OutputTokens: msg.Usage.OutputTokens,
					})
				}
			}
		}
	}
	return usages
}

// jsonString extracts a string value from a map of raw JSON.
func jsonString(obj map[string]json.RawMessage, key string) string {
	raw, ok := obj[key]
	if !ok {
		return ""
	}
	var s string
	json.Unmarshal(raw, &s)
	return s
}

// parsePeriod parses a period string like "7d", "30d", "24h" into a Duration.
func parsePeriod(period string) (time.Duration, error) {
	if len(period) < 2 {
		return 0, fmt.Errorf("invalid period %q", period)
	}
	suffix := period[len(period)-1]
	value := period[:len(period)-1]
	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
		return 0, fmt.Errorf("invalid period %q", period)
	}
	switch suffix {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown period suffix %q (use d, h, or w)", string(suffix))
	}
}

// agentConfigDir returns the config directory path inside the container for a given agent type.
func agentConfigDir(agentType string) string {
	switch agentType {
	case "codex":
		return "/home/agent/.codex"
	default:
		return "/home/agent/.claude"
	}
}

// ─── audit ─────────────────────────────────────────────────────────────────

var auditLines int

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Show audit log entries",
	RunE:  runAudit,
}

func init() {
	auditCmd.Flags().IntVar(&auditLines, "lines", 50, "Number of entries to show")
	rootCmd.AddCommand(auditCmd)
}

func runAudit(cmd *cobra.Command, args []string) error {
	logger := &audit.Logger{Path: audit.DefaultPath()}
	entries, err := logger.Read(auditLines)
	if err != nil {
		return fmt.Errorf("read audit log: %w", err)
	}
	if len(entries) == 0 {
		fmt.Println("No audit log entries found.")
		return nil
	}
	for _, e := range entries {
		details := ""
		if len(e.Details) > 0 {
			var parts []string
			for k, v := range e.Details {
				parts = append(parts, k+"="+v)
			}
			details = strings.Join(parts, " ")
		}
		fmt.Printf("%s  %-10s  %-30s  %s\n", e.Timestamp, e.Action, e.Container, details)
	}
	return nil
}

// ─── sessions ──────────────────────────────────────────────────────────────

var sessionsCmd = &cobra.Command{
	Use:   "sessions [name|--latest] [dest]",
	Short: "Export session data from container",
	Args:  cobra.RangeArgs(0, 2),
	RunE:  runSessions,
}

func init() {
	addLatestFlag(sessionsCmd)
	rootCmd.AddCommand(sessionsCmd)
}

func runSessions(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := ""
	dest := ""

	latest, _ := cmd.Flags().GetBool("latest")
	if latest {
		target = "--latest"
		if len(args) > 0 {
			dest = args[0]
		}
	} else {
		switch len(args) {
		case 0:
			// resolve latest, default dest
		case 1:
			// Could be a container name or a dest path — treat as container name
			target = args[0]
		case 2:
			target = args[0]
			dest = args[1]
		}
	}

	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	stopFn, err := ensureRunning(ctx, exec, name)
	if err != nil {
		return err
	}
	defer stopFn()

	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	configDir := agentConfigDir(agentType)

	if dest == "" {
		dest = filepath.Join("agent-sessions", name)
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return fmt.Errorf("create dest dir %s: %w", dest, err)
	}

	// Tar from container: sessions/ and projects/ dirs
	tarScript := fmt.Sprintf("tar -cf - -C %s sessions/ projects/ 2>/dev/null || true", configDir)
	tarOut, err := exec.Run(ctx, "docker", "exec", name, "bash", "-c", tarScript)
	if err != nil {
		return fmt.Errorf("tar session data: %w", err)
	}

	if len(tarOut) == 0 {
		fmt.Printf("No session data found in %s for container %s\n", configDir, name)
		return nil
	}

	// Extract tar
	count, err := extractTar(bytes.NewReader(tarOut), dest)
	if err != nil {
		return fmt.Errorf("extract session data: %w", err)
	}

	fmt.Printf("Exported %d file(s) from %s to %s\n", count, name, dest)
	return nil
}

// extractTar extracts a tar archive from r into destDir.
func extractTar(r io.Reader, destDir string) (int, error) {
	tr := tar.NewReader(r)
	count := 0
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
		// Skip directories
		if hdr.Typeflag == tar.TypeDir {
			target := filepath.Join(destDir, filepath.Clean(hdr.Name))
			if !strings.HasPrefix(target+string(os.PathSeparator), cleanDest) {
				return count, fmt.Errorf("tar entry %q escapes destination", hdr.Name)
			}
			os.MkdirAll(target, 0755)
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		target := filepath.Join(destDir, filepath.Clean(hdr.Name))
		if !strings.HasPrefix(target, cleanDest) {
			return count, fmt.Errorf("tar entry %q escapes destination", hdr.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return count, err
		}
		f, err := os.Create(target)
		if err != nil {
			return count, err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return count, err
		}
		f.Close()
		count++
	}
	return count, nil
}

// ─── replay ────────────────────────────────────────────────────────────────

var replayToolsOnly bool

var replayCmd = &cobra.Command{
	Use:   "replay [name|--latest]",
	Short: "Replay session from event log",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReplay,
}

func init() {
	replayCmd.Flags().BoolVar(&replayToolsOnly, "tools-only", false, "Show only tool calls")
	addLatestFlag(replayCmd)
	rootCmd.AddCommand(replayCmd)
}

func runReplay(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	exec := newExecutor()

	target := targetFromArgs(cmd, args)
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return err
	}

	stop, err := ensureRunning(ctx, exec, name)
	if err != nil {
		return err
	}
	defer stop()

	out, err := exec.Run(ctx, "docker", "exec", name,
		"bash", "-c", "cat /workspace/.safe-agentic/session-events.jsonl 2>/dev/null || true")
	if err != nil {
		return fmt.Errorf("read session events: %w", err)
	}

	if len(strings.TrimSpace(string(out))) == 0 {
		fmt.Println("No session events found.")
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		if rendered := renderReplayLine(scanner.Bytes()); rendered != "" {
			fmt.Println(rendered)
		}
	}

	return scanner.Err()
}

func renderReplayLine(line []byte) string {
	var event map[string]json.RawMessage
	if err := json.Unmarshal(line, &event); err != nil {
		return ""
	}
	eventType := jsonStringFromEvent(event, "type")
	if replayToolsOnly && eventType != "tool.call" {
		return ""
	}

	ts := replayTimestamp(event)
	switch eventType {
	case "session.start":
		return fmt.Sprintf("[%s] Session started", ts)
	case "tool.call":
		return replayToolCall(ts, event)
	case "git.commit":
		return replayGitCommit(ts, event)
	case "agent.message":
		return replayAgentMessage(ts, event)
	case "session.end":
		return fmt.Sprintf("[%s] Session ended", ts)
	default:
		return fmt.Sprintf("[%s] %s", ts, eventType)
	}
}

func replayTimestamp(event map[string]json.RawMessage) string {
	tsStr := jsonStringFromEvent(event, "timestamp")
	if tsStr == "" {
		return "??:??:??"
	}
	if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
		return t.Format("15:04:05")
	}
	return tsStr
}

func replayToolCall(ts string, event map[string]json.RawMessage) string {
	toolName := jsonStringFromEvent(event, "tool")
	tokens := jsonInt64FromEvent(event, "tokens")
	if tokens > 0 {
		return fmt.Sprintf("[%s] tool: %s (%d tokens)", ts, toolName, tokens)
	}
	return fmt.Sprintf("[%s] tool: %s", ts, toolName)
}

func replayGitCommit(ts string, event map[string]json.RawMessage) string {
	sha := truncateSHA(jsonStringFromEvent(event, "sha"))
	message := jsonStringFromEvent(event, "message")
	return fmt.Sprintf("[%s] Git commit: %s %q", ts, sha, message)
}

func truncateSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func replayAgentMessage(ts string, event map[string]json.RawMessage) string {
	return fmt.Sprintf("[%s] Agent: %s", ts, truncateObserveText(jsonStringFromEvent(event, "content"), 80))
}

func jsonInt64FromEvent(event map[string]json.RawMessage, key string) int64 {
	raw, ok := event[key]
	if !ok {
		return 0
	}
	var n int64
	json.Unmarshal(raw, &n)
	return n
}

// jsonStringFromEvent extracts a string value from an event map.
func jsonStringFromEvent(event map[string]json.RawMessage, key string) string {
	raw, ok := event[key]
	if !ok {
		return ""
	}
	var s string
	json.Unmarshal(raw, &s)
	return s
}
