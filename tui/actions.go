package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const defaultResumeCWD = "/workspace"
const tmuxSessionName = "safe-agentic"

// Actions handles all keybinding-triggered operations.
type Actions struct {
	app *App
}

// NewActions creates a new Actions handler.
func NewActions(app *App) *Actions {
	return &Actions{app: app}
}

func (ac *Actions) selectedOrWarn() *Agent {
	agent := ac.app.table.SelectedAgent()
	if agent == nil {
		ac.app.footer.ShowStatus("No agent selected", true)
	}
	return agent
}

// Attach shells into the selected container.
func (ac *Actions) Attach() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name
	running := agent.Running
	if containerUsesTmux(name) {
		if !running {
			startCmd := exec.Command("orb", "run", "-m", vmName, "docker", "start", name)
			if err := startCmd.Run(); err != nil {
				ac.app.footer.ShowStatus(fmt.Sprintf("Failed to start container: %v", err), true)
				return
			}
			if !waitForTmuxSession(name, 300) {
				ac.app.footer.ShowStatus("Timed out waiting for tmux session", true)
				return
			}
		}
		ac.app.ExecAfterExit(buildTmuxAttachArgs(name))
		return
	}

	ac.app.SuspendAndRun(func() {
		var cmd *exec.Cmd
		if running {
			cmd = exec.Command("orb", "run", "-m", vmName, "docker", "exec", "-it", name, "bash", "-l")
		} else {
			cmd = exec.Command("orb", "run", "-m", vmName, "docker", "start", "-ai", name)
		}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	})
}

// Resume reconnects to the agent TTY when the container is already running.
// For stopped containers, restart detached and exec the CLI resume command
// from the last recorded session cwd.
func (ac *Actions) Resume() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	if containerUsesTmux(agent.Name) {
		if !agent.Running {
			startCmd := exec.Command("orb", "run", "-m", vmName, "docker", "start", agent.Name)
			if err := startCmd.Run(); err != nil {
				ac.app.footer.ShowStatus(fmt.Sprintf("Failed to start container: %v", err), true)
				return
			}
			if !waitForTmuxSession(agent.Name, 300) {
				ac.app.footer.ShowStatus("Timed out waiting for tmux session", true)
				return
			}
		}
		ac.app.ExecAfterExit(buildTmuxAttachArgs(agent.Name))
		return
	}

	if !resumeSupported(agent.Type) {
		ac.app.footer.ShowStatus("Resume not supported for shell containers", true)
		return
	}

	if agent.Running {
		ac.app.ExecAfterExit(buildAttachArgs(agent.Name))
		return
	}

	cwd := defaultResumeCWD
	meta, err := loadLatestSessionMeta(agent.Name, agent.Type, false)
	if err == nil && meta.CWD != "" {
		cwd = meta.CWD
	}

	startCmd := exec.Command("orb", "run", "-m", vmName, "docker", "start", agent.Name)
	if err := startCmd.Run(); err != nil {
		ac.app.footer.ShowStatus(fmt.Sprintf("Failed to start container: %v", err), true)
		return
	}

	fullArgs, err := buildResumeExecArgs(agent.Type, agent.Name, cwd)
	if err != nil {
		ac.app.footer.ShowStatus(err.Error(), true)
		return
	}

	ac.app.ExecAfterExit(fullArgs)
}

func resumeSupported(agentType string) bool {
	switch agentType {
	case "codex", "claude":
		return true
	default:
		return false
	}
}

func buildAttachArgs(name string) []string {
	return []string{"orb", "run", "-m", vmName, "docker", "attach", "--sig-proxy=false", name}
}

func buildTmuxAttachArgs(name string) []string {
	return []string{"orb", "run", "-m", vmName, "docker", "exec", "-it", name, "tmux", "attach", "-t", tmuxSessionName}
}

func buildResumeExecArgs(agentType, name, cwd string) ([]string, error) {
	resumeArgs, err := resumeCLIArgs(agentType)
	if err != nil {
		return nil, err
	}
	if cwd == "" {
		cwd = defaultResumeCWD
	}
	script := "cd " + shellQuoteArgs([]string{cwd}) + " && exec " + shellQuoteArgs(resumeArgs)
	return []string{"orb", "run", "-m", vmName, "docker", "exec", "-it", name, "bash", "-lc", script}, nil
}

func resumeCLIArgs(agentType string) ([]string, error) {
	switch agentType {
	case "codex":
		return []string{"codex", "--yolo", "resume", "--last"}, nil
	case "claude":
		return []string{"claude", "--dangerously-skip-permissions", "--continue"}, nil
	default:
		return nil, fmt.Errorf("resume not supported for %s containers", agentType)
	}
}

func loadLatestSessionMeta(name, agentType string, running bool) (*sessionMeta, error) {
	configDir := "/home/agent/.codex"
	if agentType == "claude" {
		configDir = "/home/agent/.claude"
	}
	sessionsDir := configDir + "/sessions/"

	var data []byte
	var err error
	if running {
		data, err = readSessionViaExec(name, sessionsDir)
	} else {
		data, err = readSessionViaCp(name, sessionsDir)
	}
	if err != nil {
		return nil, err
	}
	meta, err := parseSessionMeta(data)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func parseSessionMeta(data []byte) (sessionMeta, error) {
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry sessionLogEntry
		if err := json.Unmarshal(line, &entry); err != nil || entry.Type != "session_meta" {
			continue
		}
		var meta sessionMeta
		if err := json.Unmarshal(entry.Payload, &meta); err != nil {
			return sessionMeta{}, err
		}
		return meta, nil
	}
	return sessionMeta{}, fmt.Errorf("no session metadata found")
}

func containerUsesTmux(name string) bool {
	out, err := execOrb("docker", "inspect", "--format", "{{index .Config.Labels \"safe-agentic.terminal\"}}", name)
	return err == nil && strings.TrimSpace(string(out)) == "tmux"
}

func waitForTmuxSession(name string, attempts int) bool {
	for i := 0; i < attempts; i++ {
		if _, err := execOrb("docker", "exec", name, "tmux", "has-session", "-t", tmuxSessionName); err == nil {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// StopAgent stops and removes the selected container with confirmation.
func (ac *Actions) StopAgent() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name
	ac.app.footer.ShowConfirm(fmt.Sprintf("Stop %s?", name), func(yes bool) {
		if !yes {
			return
		}
		ac.app.footer.ShowStatus(fmt.Sprintf("Stopping %s...", name), false)
		go func() {
			cmd := exec.Command("agent", "stop", name)
			out, err := cmd.CombinedOutput()
			ac.app.poller.ForceRefresh()
			ac.app.tapp.QueueUpdateDraw(func() {
				if err != nil {
					ac.app.footer.ShowStatus(fmt.Sprintf("Stop failed: %s", strings.TrimSpace(string(out))), true)
				} else {
					ac.app.footer.ShowStatus(fmt.Sprintf("Stopped %s", name), false)
				}
			})
		}()
	})
}

// Logs shows the agent's latest session log in an overlay pane.
// Containers run with -it (TTY), so `docker logs` is raw escape sequences.
// Instead, we find the latest session JSONL and render it readably.
func (ac *Actions) Logs() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name
	agentType := agent.Type

	configDir := "/home/agent/.codex"
	if agentType == "claude" {
		configDir = "/home/agent/.claude"
	}
	sessionsDir := configDir + "/sessions/"

	var data []byte
	var err error

	if agent.Running {
		// Running container: use docker exec
		data, err = readSessionViaExec(name, sessionsDir)
	} else {
		// Stopped container: use docker cp
		data, err = readSessionViaCp(name, sessionsDir)
	}

	if err != nil || len(data) == 0 {
		ac.app.footer.ShowStatus("No session logs found", true)
		return
	}

	rendered := renderSessionLog(data)
	ShowOverlay(ac.app, "logs", fmt.Sprintf("Session: %s", name), rendered)
}

func readSessionViaExec(name, sessionsDir string) ([]byte, error) {
	latestFile, err := execOrb("docker", "exec", name, "bash", "-c",
		fmt.Sprintf("find %s -name '*.jsonl' ! -name '._*' -type f 2>/dev/null | sort | tail -1", sessionsDir))
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(string(latestFile))
	if path == "" {
		return nil, fmt.Errorf("no session files")
	}
	return execOrb("docker", "exec", name, "cat", path)
}

func readSessionViaCp(name, sessionsDir string) ([]byte, error) {
	// Copy the entire sessions dir to a temp location in the VM,
	// then find and read the latest file.
	tmpDir := "/tmp/satui-sessions-" + name
	execOrb("bash", "-c", "rm -rf "+tmpDir)
	_, err := execOrbLong("docker", "cp", name+":"+sessionsDir, tmpDir+"/")
	if err != nil {
		return nil, err
	}
	defer execOrb("bash", "-c", "rm -rf "+tmpDir)

	latestFile, err := execOrb("bash", "-c",
		fmt.Sprintf("find %s -name '*.jsonl' ! -name '._*' -type f 2>/dev/null | sort | tail -1", tmpDir))
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(string(latestFile))
	if path == "" {
		return nil, fmt.Errorf("no session files")
	}
	return execOrb("cat", path)
}

// Checkpoint creates a named git stash checkpoint of the agent's current working tree.
func (ac *Actions) Checkpoint() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	if !agent.Running {
		ac.app.footer.ShowStatus("Agent is not running", true)
		return
	}
	name := agent.Name
	label := fmt.Sprintf("checkpoint-%d", time.Now().Unix())
	ac.app.footer.ShowStatus(fmt.Sprintf("Creating checkpoint %s...", label), false)
	go func() {
		// Stage all changes and create a stash object without modifying the working tree.
		out, err := execOrbLong("docker", "exec", name, "bash", "-c",
			"cd /workspace/* 2>/dev/null || cd /workspace; git add -A && git stash create")
		if err != nil {
			ac.app.tapp.QueueUpdateDraw(func() {
				ac.app.footer.ShowStatus("Checkpoint failed: could not create stash", true)
			})
			return
		}
		stashSHA := strings.TrimSpace(string(out))
		if stashSHA == "" {
			ac.app.tapp.QueueUpdateDraw(func() {
				ac.app.footer.ShowStatus("No changes to checkpoint", false)
			})
			return
		}
		_, err = execOrbLong("docker", "exec", name, "bash", "-c",
			fmt.Sprintf("cd /workspace/* 2>/dev/null || cd /workspace; git update-ref refs/safe-agentic/checkpoints/%s %s", label, stashSHA))
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus(fmt.Sprintf("Checkpoint failed: could not save ref %s", label), true)
			} else {
				ac.app.footer.ShowStatus(fmt.Sprintf("Checkpoint '%s' created (%s)", label, stashSHA[:7]), false)
			}
		})
	}()
}

// Diff shows the git diff from the agent's working tree in an overlay.
func (ac *Actions) Diff() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	if !agent.Running {
		ac.app.footer.ShowStatus("Agent is not running", true)
		return
	}
	name := agent.Name
	ac.app.footer.ShowStatus(fmt.Sprintf("Loading diff for %s...", name), false)
	go func() {
		out, err := execOrbLong("docker", "exec", name, "bash", "-c",
			"cd /workspace/* 2>/dev/null || cd /workspace; git diff")
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("Failed to get diff", true)
				return
			}
			content := strings.TrimSpace(string(out))
			if content == "" {
				ac.app.footer.ShowStatus("No changes", false)
				return
			}
			ShowOverlay(ac.app, "diff", fmt.Sprintf("Diff: %s", name), content)
		})
	}()
}

// Todo shows the todo list for the selected agent in an overlay.
func (ac *Actions) Todo() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	if !agent.Running {
		ac.app.footer.ShowStatus("Agent is not running", true)
		return
	}
	name := agent.Name
	ac.app.footer.ShowStatus(fmt.Sprintf("Loading todos for %s...", name), false)
	go func() {
		const pyScript = `
import json, os, sys
path = '/workspace/.safe-agentic/todos.json'
if not os.path.exists(path):
    print('No todos yet.')
    sys.exit(0)
try:
    with open(path) as f:
        todos = json.load(f)
except Exception:
    print('No todos yet.')
    sys.exit(0)
if not todos:
    print('No todos yet.')
    sys.exit(0)
done_count = sum(1 for t in todos if t.get('done'))
for i, t in enumerate(todos, 1):
    mark = 'x' if t.get('done') else ' '
    print('[{}] {}. {}'.format(mark, i, t.get('text', '')))
print('')
print('{}/{} complete'.format(done_count, len(todos)))
`
		out, err := execOrbLong("docker", "exec", name, "python3", "-c", pyScript)
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("Failed to load todos", true)
				return
			}
			content := strings.TrimSpace(string(out))
			if content == "" {
				ac.app.footer.ShowStatus("No todos yet", false)
				return
			}
			ShowOverlay(ac.app, "todo", fmt.Sprintf("Todos: %s", name), content)
		})
	}()
}

// Describe shows docker inspect in a formatted overlay.
func (ac *Actions) Describe() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	data, err := execOrb("docker", "inspect", agent.Name)
	if err != nil {
		ac.app.footer.ShowStatus("Failed to inspect container", true)
		return
	}

	// Pretty-print the JSON
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		pretty.Write(data) // fallback to raw
	}

	ShowOverlay(ac.app, "describe", fmt.Sprintf("Describe: %s", agent.Name), pretty.String())
}

// YAMLView shows raw docker inspect JSON in an overlay.
func (ac *Actions) YAMLView() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	data, err := execOrb("docker", "inspect", agent.Name)
	if err != nil {
		ac.app.footer.ShowStatus("Failed to inspect container", true)
		return
	}
	ShowOverlay(ac.app, "yaml", fmt.Sprintf("JSON: %s", agent.Name), string(data))
}

// ExportSessions runs agent sessions export in the background.
func (ac *Actions) ExportSessions() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name
	ac.app.footer.ShowStatus(fmt.Sprintf("Exporting sessions from %s...", name), false)
	go func() {
		cmd := exec.Command("agent", "sessions", name)
		out, err := cmd.CombinedOutput()
		ac.app.poller.ForceRefresh()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus(fmt.Sprintf("Export failed: %s", strings.TrimSpace(string(out))), true)
			} else {
				ac.app.footer.ShowStatus(fmt.Sprintf("Exported sessions from %s", name), false)
			}
		})
	}()
}

// CopyFiles shows a modal form for container path and host path.
func (ac *Actions) CopyFiles() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	ShowCopyForm(ac.app, agent.Name)
}

// SpawnNew shows a modal form for spawning a new agent.
func (ac *Actions) SpawnNew() {
	ShowSpawnForm(ac.app)
}

// McpLogin runs MCP login interactively (suspends TUI).
func (ac *Actions) McpLogin() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name

	ac.app.SuspendAndRun(func() {
		fmt.Printf("Enter MCP server name: ")
		var server string
		fmt.Scanln(&server)
		if server == "" {
			return
		}
		cmd := exec.Command("agent", "mcp-login", name, server)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
	})
}

// KillAll stops all agent containers with confirmation.
func (ac *Actions) KillAll() {
	total := ac.app.table.TotalCount()
	if total == 0 {
		ac.app.footer.ShowStatus("No agents to stop", true)
		return
	}
	ac.app.footer.ShowConfirm(fmt.Sprintf("Stop ALL %d agents?", total), func(yes bool) {
		if !yes {
			return
		}
		ac.app.footer.ShowStatus("Stopping all agents...", false)
		go func() {
			cmd := exec.Command("agent", "stop", "--all")
			out, err := cmd.CombinedOutput()
			ac.app.poller.ForceRefresh()
			ac.app.tapp.QueueUpdateDraw(func() {
				if err != nil {
					ac.app.footer.ShowStatus(fmt.Sprintf("Stop all failed: %s", strings.TrimSpace(string(out))), true)
				} else {
					ac.app.footer.ShowStatus("All agents stopped", false)
				}
			})
		}()
	})
}

// sessionLogEntry represents one line of a session JSONL file.
type sessionLogEntry struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMeta struct {
	CWD        string `json:"cwd"`
	CLIVersion string `json:"cli_version"`
	Model      string `json:"model"`
	Originator string `json:"originator"`
}

type responseItem struct {
	Item struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"item"`
}

type eventMsg struct {
	Msg string `json:"msg"`
}

func renderSessionLog(data []byte) string {
	var b strings.Builder
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var entry sessionLogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		ts := entry.Timestamp
		if len(ts) > 19 {
			ts = ts[:19]
		}

		switch entry.Type {
		case "session_meta":
			var meta sessionMeta
			json.Unmarshal(entry.Payload, &meta)
			fmt.Fprintf(&b, "━━━ Session ━━━\n")
			fmt.Fprintf(&b, "  Time:    %s\n", ts)
			if meta.CWD != "" {
				fmt.Fprintf(&b, "  Dir:     %s\n", meta.CWD)
			}
			if meta.CLIVersion != "" {
				fmt.Fprintf(&b, "  CLI:     %s\n", meta.CLIVersion)
			}
			if meta.Originator != "" {
				fmt.Fprintf(&b, "  Agent:   %s\n", meta.Originator)
			}
			b.WriteString("\n")

		case "response_item":
			var ri responseItem
			json.Unmarshal(entry.Payload, &ri)
			role := ri.Item.Role
			if role == "" || ri.Item.Type != "message" {
				continue
			}
			text := extractText(ri.Item.Content)
			if text == "" {
				continue
			}
			label := "???"
			switch role {
			case "user":
				label = "USER"
			case "assistant":
				label = "AGENT"
			case "developer", "system":
				label = "SYS"
			}
			fmt.Fprintf(&b, "── %s [%s] ──\n%s\n\n", label, ts, text)
		}
	}

	if b.Len() == 0 {
		return "(empty session log)"
	}
	return b.String()
}

// extractText pulls readable text from a message content field.
// Content can be a string or an array of content blocks.
func extractText(raw json.RawMessage) string {
	// Try as string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}

	// Try as array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, bl := range blocks {
			if bl.Text != "" && (bl.Type == "output_text" || bl.Type == "input_text" || bl.Type == "text") {
				parts = append(parts, strings.TrimSpace(bl.Text))
			}
		}
		return strings.Join(parts, "\n")
	}

	return ""
}

// CreatePR creates a GitHub Pull Request from the agent's current branch.
// Requires the container to have been spawned with --ssh.
func (ac *Actions) CreatePR() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name

	// Check SSH label first (needed for push)
	sshLabel, err := execOrb("docker", "inspect", "--format",
		`{{index .Config.Labels "safe-agentic.ssh"}}`, name)
	if err != nil || strings.TrimSpace(string(sshLabel)) != "yes" {
		ac.app.footer.ShowStatus(fmt.Sprintf("%s was not spawned with --ssh; push requires SSH", name), true)
		return
	}

	ac.app.footer.ShowConfirm(fmt.Sprintf("Create PR from %s?", name), func(yes bool) {
		if !yes {
			return
		}
		ac.app.footer.ShowStatus(fmt.Sprintf("Creating PR for %s...", name), false)
		go func() {
			cmd := exec.Command("agent", "pr", name)
			out, err := cmd.CombinedOutput()
			ac.app.poller.ForceRefresh()
			ac.app.tapp.QueueUpdateDraw(func() {
				if err != nil {
					ac.app.footer.ShowStatus(fmt.Sprintf("PR failed: %s", strings.TrimSpace(string(out))), true)
				} else {
					result := strings.TrimSpace(string(out))
					ac.app.footer.ShowStatus(result, false)
				}
			})
		}()
	})
}
