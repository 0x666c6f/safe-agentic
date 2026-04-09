package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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
// logsState holds mutable state for the live logs overlay.
type logsState struct {
	autoRefresh bool
	tailLines   string // "500", "2000", or "0" (all)
}

func (ac *Actions) Logs() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name
	state := &logsState{autoRefresh: agent.Running, tailLines: "500"}

	data := ac.fetchDockerLogs(name, state.tailLines)
	if len(data) == 0 {
		ac.app.footer.ShowStatus("No session logs found", true)
		return
	}

	rendered := renderSessionLog(data)
	title := ac.logsTitle(name, state)
	tv := ShowOverlayLive(ac.app, "logs", title, rendered)

	// Keybindings within the logs overlay:
	//   r — toggle auto-refresh
	//   5 — last 500 lines
	//   a — all lines
	//   2 — last 2000 lines
	tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			switch event.Rune() {
			case 'r':
				state.autoRefresh = !state.autoRefresh
				ac.app.tapp.QueueUpdateDraw(func() {
					tv.SetTitle(" " + ac.logsTitle(name, state) + " ")
				})
				return nil
			case '5':
				state.tailLines = "500"
				ac.refreshLogsNow(tv, name, state)
				return nil
			case 'a':
				state.tailLines = "0"
				ac.refreshLogsNow(tv, name, state)
				return nil
			case '2':
				state.tailLines = "2000"
				ac.refreshLogsNow(tv, name, state)
				return nil
			}
		}
		return event
	})

	// Auto-refresh goroutine
	if agent.Running {
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if fp, _ := ac.app.pages.GetFrontPage(); fp != "logs" {
					return
				}
				if !state.autoRefresh {
					continue
				}
				newData := ac.fetchDockerLogs(name, state.tailLines)
				if len(newData) > 0 {
					newRendered := renderSessionLog(newData)
					ac.app.tapp.QueueUpdateDraw(func() {
						if fp, _ := ac.app.pages.GetFrontPage(); fp == "logs" {
							tv.SetText(newRendered)
							tv.ScrollToEnd()
						}
					})
				}
			}
		}()
	}
}

func (ac *Actions) logsTitle(name string, state *logsState) string {
	refresh := "off"
	if state.autoRefresh {
		refresh = "3s"
	}
	lines := state.tailLines
	if lines == "0" {
		lines = "all"
	}
	return fmt.Sprintf("Logs: %s | [r]efresh:%s [5]00/[2]000/[a]ll:%s | Esc close", name, refresh, lines)
}

func (ac *Actions) fetchDockerLogs(name, tailLines string) []byte {
	// Get the container's repo label to find the right project dir
	repoLabel, _ := execOrb("docker", "inspect", "--format",
		`{{index .Config.Labels "safe-agentic.repo-display"}}`, name)
	repo := strings.TrimSpace(string(repoLabel))

	agentType, _ := execOrb("docker", "inspect", "--format",
		`{{index .Config.Labels "safe-agentic.agent-type"}}`, name)
	atype := strings.TrimSpace(string(agentType))

	configDir := "/home/agent/.codex"
	if atype == "claude" {
		configDir = "/home/agent/.claude"
	}

	// Build the project directory path that Claude/Codex uses
	// e.g. repo "0x666c6f/safe-agentic" → projects/-workspace-0x666c6f-safe-agentic/
	var findDir string
	if repo != "" && repo != "-" {
		projSlug := strings.ReplaceAll(repo, "/", "-")
		findDir = fmt.Sprintf("%s/projects/-workspace-%s", configDir, projSlug)
	} else {
		findDir = configDir
	}

	// Get container creation time to match the correct session file
	createdOut, _ := execOrb("docker", "inspect", "--format", "{{.Created}}", name)
	containerCreated := strings.TrimSpace(string(createdOut))

	// Check if container is running
	stateOut, _ := execOrb("docker", "inspect", "--format", "{{.State.Status}}", name)
	running := strings.TrimSpace(string(stateOut)) == "running"

	// Python script: find the session file whose first timestamp is closest
	// to (and after) the container creation time
	matchScript := fmt.Sprintf(`
import os, json, sys, glob
container_created = sys.argv[1][:19]  # trim to seconds
find_dir = sys.argv[2]
tail_lines = sys.argv[3]

# Find all session jsonl files
files = glob.glob(os.path.join(find_dir, '*.jsonl'))
if not files:
    # Fallback: search deeper
    files = glob.glob(os.path.join(find_dir, '**', '*.jsonl'), recursive=True)
files = [f for f in files if not f.endswith('history.jsonl') and '/subagents/' not in f]

best_file = None
best_delta = float('inf')

for f in files:
    try:
        with open(f) as fh:
            for line in fh:
                d = json.loads(line.strip())
                ts = d.get('timestamp', '')
                if not ts and 'message' in d and isinstance(d['message'], dict):
                    ts = d['message'].get('timestamp', '')
                if ts:
                    session_start = ts[:19]
                    # Simple string comparison works for ISO timestamps
                    if session_start >= container_created:
                        delta = ord(session_start[18]) - ord(container_created[18])
                        if delta < best_delta:
                            best_delta = delta
                            best_file = f
                    break
    except:
        pass

if not best_file and files:
    # Fallback: most recently modified
    best_file = max(files, key=os.path.getmtime)

if best_file:
    print(best_file)
`)

	if running {
		// Write match script, run via docker exec
		result, err := execOrbTimeout(15*time.Second, "docker", "exec", name, "python3", "-c", matchScript, containerCreated, findDir, tailLines)
		if err != nil || strings.TrimSpace(string(result)) == "" {
			return nil
		}
		path := strings.TrimSpace(string(result))
		if tailLines == "0" {
			data, _ := execOrbTimeout(60*time.Second, "docker", "exec", name, "cat", path)
			return data
		}
		data, _ := execOrbLong("docker", "exec", name, "tail", "-"+tailLines, path)
		return data
	}

	// Stopped: docker cp the project dir, then match locally
	tmpDir := "/tmp/satui-logs-" + name
	execOrb("rm", "-rf", tmpDir)
	defer execOrb("rm", "-rf", tmpDir)

	execOrbLong("docker", "cp", name+":"+findDir, tmpDir+"/")
	// Also try the full config dir as fallback
	if _, err := execOrb("bash", "-c", fmt.Sprintf("ls %s/*.jsonl 2>/dev/null | head -1", tmpDir)); err != nil || true {
		execOrbLong("docker", "cp", name+":"+configDir+"/projects", tmpDir+"/projects/")
	}

	result, _ := execOrbTimeout(15*time.Second, "python3", "-c", matchScript, containerCreated, tmpDir, tailLines)
	path := strings.TrimSpace(string(result))
	if path == "" {
		return nil
	}
	if tailLines == "0" {
		data, _ := execOrbTimeout(60*time.Second, "cat", path)
		return data
	}
	data, _ := execOrbLong("tail", "-"+tailLines, path)
	return data
}

func (ac *Actions) refreshLogsNow(tv *tview.TextView, name string, state *logsState) {
	ac.app.footer.ShowStatus("Loading...", false)
	go func() {
		data := ac.fetchDockerLogs(name, state.tailLines)
		ac.app.tapp.QueueUpdateDraw(func() {
			if len(data) > 0 {
				tv.SetText(renderSessionLog(data))
				tv.SetTitle(" " + ac.logsTitle(name, state) + " ")
			}
			ac.app.footer.Reset()
		})
	}()
}

func readSessionViaExec(name, sessionsDir string) ([]byte, error) {
	configDir := strings.TrimSuffix(sessionsDir, "/sessions/")
	latestFile, err := execOrbLong("docker", "exec", name, "bash", "-c",
		fmt.Sprintf("find %s -name '*.jsonl' ! -name '._*' ! -name 'history.jsonl' ! -path '*/subagents/*' -type f -printf '%%T@ %%p\\n' 2>/dev/null | sort -n | tail -1 | cut -d' ' -f2-", configDir))
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(string(latestFile))
	if path == "" {
		return nil, fmt.Errorf("no session files")
	}
	return execOrbLong("docker", "exec", name, "cat", path)
}

func readSessionViaCp(name, sessionsDir string) ([]byte, error) {
	// For stopped containers: start briefly, read, then let it re-stop.
	// docker cp of the entire config dir is too slow for large sessions.
	configDir := strings.TrimSuffix(sessionsDir, "/sessions/")

	// Find latest file via docker cp of just the directory listing
	tmpDir := "/tmp/satui-sessions-" + name
	execOrb("rm", "-rf", tmpDir)
	defer execOrb("rm", "-rf", tmpDir)

	_, err := execOrbLong("docker", "cp", name+":"+configDir, tmpDir+"/")
	if err != nil {
		return nil, err
	}

	latestFile, err := execOrbLong("bash", "-c",
		fmt.Sprintf("find %s -name '*.jsonl' ! -name '._*' ! -name 'history.jsonl' ! -path '*/subagents/*' -type f -printf '%%T@ %%p\\n' 2>/dev/null | sort -n | tail -1 | cut -d' ' -f2-", tmpDir))
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(string(latestFile))
	if path == "" {
		return nil, fmt.Errorf("no session files")
	}
	return execOrbLong("cat", path)
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

// claudeEntry represents a Claude Code session JSONL line
type claudeEntry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

// claudeMessage is the message field in a Claude Code entry
type claudeMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Timestamp string          `json:"timestamp"`
}

func renderSessionLog(data []byte) string {
	var b strings.Builder
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		// Try Claude Code format first (type: user/assistant/system with message field)
		var ce claudeEntry
		if err := json.Unmarshal(line, &ce); err == nil {
			switch ce.Type {
			case "user", "assistant", "system":
				var msg claudeMessage
				if err := json.Unmarshal(ce.Message, &msg); err == nil {
					text := extractText(msg.Content)
					if text == "" {
						continue
					}
					label := "???"
					switch ce.Type {
					case "user":
						label = "USER"
					case "assistant":
						label = "AGENT"
					case "system":
						label = "SYS"
					}
					// Pick timestamp from entry or nested message
					ts := ce.Timestamp
					if ts == "" {
						ts = msg.Timestamp
					}
					if len(ts) > 19 {
						ts = ts[:19]
					}
					if ts != "" {
						// Show just HH:MM:SS for compactness
						if len(ts) >= 19 {
							ts = ts[11:19]
						}
						fmt.Fprintf(&b, "── %s [%s] ──\n%s\n\n", label, ts, text)
					} else {
						fmt.Fprintf(&b, "── %s ──\n%s\n\n", label, text)
					}
					continue
				}
			}
		}

		// Fall back to Codex format (type: session_meta/response_item with payload)
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

// Review runs an AI code review of the agent's changes in an overlay.
func (ac *Actions) Review() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	if !agent.Running {
		ac.app.footer.ShowStatus("Agent not running", true)
		return
	}
	name := agent.Name
	ac.app.footer.ShowStatus("Running review...", false)
	go func() {
		out, err := execOrbLong("docker", "exec", name, "bash", "-lc",
			"cd /workspace/* 2>/dev/null || cd /workspace; if command -v codex >/dev/null 2>&1; then codex review --uncommitted 2>&1; else git diff 2>&1; fi")
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("Review failed", true)
				return
			}
			content := string(out)
			if strings.TrimSpace(content) == "" {
				ac.app.footer.ShowStatus("No changes to review", false)
				return
			}
			ShowOverlay(ac.app, "review", fmt.Sprintf("Review: %s", name), content)
		})
	}()
}

// Cost shows estimated API spend for the agent's sessions in an overlay.
func (ac *Actions) Cost() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name
	ac.app.footer.ShowStatus("Analyzing cost...", false)
	go func() {
		cmd := exec.Command("agent", "cost", name)
		out, err := cmd.CombinedOutput()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("Cost analysis failed: "+strings.TrimSpace(string(out)), true)
				return
			}
			content := strings.TrimSpace(string(out))
			if content == "" {
				ac.app.footer.ShowStatus("No session data found", false)
				return
			}
			ShowOverlay(ac.app, "cost", fmt.Sprintf("Cost: %s", name), content)
		})
	}()
}

// Audit shows the operation audit log in an overlay.
func (ac *Actions) Audit() {
	ac.app.footer.ShowStatus("Loading audit log...", false)
	go func() {
		cmd := exec.Command("agent", "audit")
		out, err := cmd.CombinedOutput()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("No audit log yet", false)
				return
			}
			content := strings.TrimSpace(string(out))
			if content == "" {
				ac.app.footer.ShowStatus("No audit log yet", false)
				return
			}
			ShowOverlay(ac.app, "audit", "Audit Log", content)
		})
	}()
}

// Fleet spawns agents from a manifest file (entered via command bar).
func (ac *Actions) Fleet(manifestPath string) {
	if manifestPath == "" {
		ac.app.footer.ShowStatus("Usage: :fleet <manifest.yaml>", true)
		return
	}
	ac.app.footer.ShowStatus("Spawning fleet from "+manifestPath+"...", false)
	go func() {
		cmd := exec.Command("agent", "fleet", manifestPath)
		out, err := cmd.CombinedOutput()
		ac.app.poller.ForceRefresh()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("Fleet failed: "+strings.TrimSpace(string(out)), true)
			} else {
				ac.app.footer.ShowStatus("Fleet spawned. Press 'r' to connect.", false)
			}
		})
	}()
}

// Pipeline runs a multi-step agent pipeline (entered via command bar).
func (ac *Actions) Pipeline(pipelinePath string) {
	if pipelinePath == "" {
		ac.app.footer.ShowStatus("Usage: :pipeline <pipeline.yaml>", true)
		return
	}
	ac.app.footer.ShowStatus("Running pipeline from "+pipelinePath+"...", false)
	go func() {
		cmd := exec.Command("agent", "pipeline", pipelinePath)
		out, err := cmd.CombinedOutput()
		ac.app.poller.ForceRefresh()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("Pipeline failed: "+strings.TrimSpace(string(out)), true)
			} else {
				ac.app.footer.ShowStatus("Pipeline complete", false)
			}
		})
	}()
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
