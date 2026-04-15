package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const defaultResumeCWD = "/workspace"
const tmuxSessionName = "safe-agentic"
const cliBinaryName = "safe-ag"

var cliBinary = resolveCLIBinary()

var newCLICmd = func(args ...string) *exec.Cmd {
	return exec.Command(cliBinary, args...)
}

func resolveCLIBinary() string {
	self, err := os.Executable()
	if err != nil {
		return cliBinaryName
	}
	return resolveCLIBinaryFrom(self)
}

func resolveCLIBinaryFrom(self string) string {
	candidate := filepath.Join(filepath.Dir(self), cliBinaryName)
	if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
		return candidate
	}
	return cliBinaryName
}

func mcpLoginArgs(server, container string) []string {
	return []string{"mcp-login", server, container}
}

func sshLabelAllowsPush(v string) bool {
	switch strings.TrimSpace(v) {
	case "yes", "true":
		return true
	default:
		return false
	}
}

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
		return nil
	}
	if agent.Deleting {
		ac.app.footer.ShowStatus(fmt.Sprintf("%s is deleting", agent.Name), true)
		return nil
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
		ac.app.table.MarkDeleting(*agent)
		ac.app.footer.ShowStatus(fmt.Sprintf("Stopping %s...", name), false)
		go func() {
			cmd := newCLICmd("stop", name)
			out, err := cmd.CombinedOutput()
			ac.app.poller.ForceRefresh()
			ac.app.tapp.QueueUpdateDraw(func() {
				if err != nil {
					ac.app.table.ClearDeleting(name)
					ac.app.footer.ShowStatus(fmt.Sprintf("Stop failed: %s", strings.TrimSpace(string(out))), true)
				} else {
					ac.app.table.FinishDeleting(name)
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
	rawMode     bool
}

func (ac *Actions) Logs() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name
	state := &logsState{autoRefresh: agent.Running, tailLines: "500"}

	rendered := ac.loadLogsContent(name, state)
	if rendered == "" {
		ac.app.footer.ShowStatus("No session logs found", true)
		return
	}

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
				newRendered := ac.loadLogsContent(name, state)
				if newRendered != "" {
					ac.app.tapp.QueueUpdateDraw(func() {
						if fp, _ := ac.app.pages.GetFrontPage(); fp == "logs" {
							// Preserve scroll position: only auto-scroll if already at bottom
							row, _ := tv.GetScrollOffset()
							_, _, _, height := tv.GetInnerRect()
							totalLines := len(strings.Split(tv.GetText(true), "\n"))
							atBottom := row+height >= totalLines-2
							tv.SetText(newRendered)
							if atBottom {
								tv.ScrollToEnd()
							}
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
	mode := "session"
	if state.rawMode {
		mode = "docker"
	}
	return fmt.Sprintf("Logs: %s | mode:%s [r]efresh:%s [5]00/[2]000/[a]ll:%s | Esc close", name, mode, refresh, lines)
}

func (ac *Actions) loadLogsContent(name string, state *logsState) string {
	data := fetchSessionLogsFunc(ac, name, state.tailLines)
	rendered := renderSessionLog(data)
	if len(data) > 0 && rendered != "(empty session log)" {
		state.rawMode = false
		return rendered
	}
	agentRendered := fetchAgentLogsFunc(name, state.tailLines)
	if strings.TrimSpace(agentRendered) != "" {
		state.rawMode = false
		return agentRendered
	}
	raw := fetchPlainLogsFunc(name, state.tailLines)
	if len(raw) == 0 {
		return ""
	}
	state.rawMode = true
	return strings.TrimSpace(string(raw))
}

func fetchAgentLogsCLI(name, tailLines string) string {
	lines := tailLines
	if lines == "0" {
		lines = "100000"
	}
	out, err := newCLICmd("logs", "--lines", lines, name).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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

func (ac *Actions) fetchDockerLogs(name, tailLines string) []byte {
	repo, configDir, containerCreated, running := inspectLogContext(name)
	searchDirs := sessionSearchDirs(configDir, repo)
	matchScript := `
import os, json, sys, glob, datetime
container_created = sys.argv[1][:19]
search_dirs = [p for p in sys.argv[2:] if p]

files = []
seen = set()
for find_dir in search_dirs:
    for pattern in (os.path.join(find_dir, '*.jsonl'), os.path.join(find_dir, '**', '*.jsonl')):
        for f in glob.glob(pattern, recursive=True):
            if f.endswith('history.jsonl') or '/subagents/' in f or f in seen:
                continue
            seen.add(f)
            files.append(f)

def parse_ts(raw):
    if not raw:
        return None
    raw = raw[:19]
    try:
        return datetime.datetime.fromisoformat(raw)
    except Exception:
        return None

container_dt = parse_ts(container_created)
best_file = None
best_score = None

for f in files:
    try:
        session_dt = None
        with open(f) as fh:
            for line in fh:
                line = line.strip()
                if not line:
                    continue
                d = json.loads(line)
                ts = d.get('timestamp', '')
                if not ts and 'message' in d and isinstance(d['message'], dict):
                    ts = d['message'].get('timestamp', '')
                if ts:
                    session_dt = parse_ts(ts)
                    break
        if session_dt is None:
            session_dt = datetime.datetime.fromtimestamp(os.path.getmtime(f))
        score = abs((session_dt - container_dt).total_seconds()) if container_dt else float('inf')
        if best_score is None or score < best_score:
            best_score = score
            best_file = f
    except:
        pass

if not best_file and files:
    best_file = max(files, key=os.path.getmtime)

if best_file:
    print(best_file)
`

	if running {
		return fetchRunningSessionLogs(name, tailLines, containerCreated, searchDirs, matchScript)
	}
	return fetchStoppedSessionLogs(name, tailLines, configDir, containerCreated, searchDirs, matchScript)
}

func inspectLogContext(name string) (string, string, string, bool) {
	repo := inspectLogLabel(name, `{{index .Config.Labels "safe-agentic.repo-display"}}`)
	agentType := inspectLogLabel(name, `{{index .Config.Labels "safe-agentic.agent-type"}}`)
	configDir := logConfigDir(strings.TrimSpace(agentType))
	createdOut, _ := execOrb("docker", "inspect", "--format", "{{.Created}}", name)
	stateOut, _ := execOrb("docker", "inspect", "--format", "{{.State.Status}}", name)
	return strings.TrimSpace(repo), configDir, strings.TrimSpace(string(createdOut)), strings.TrimSpace(string(stateOut)) == "running"
}

func inspectLogLabel(name, format string) string {
	out, _ := execOrb("docker", "inspect", "--format", format, name)
	return strings.TrimSpace(string(out))
}

func logConfigDir(agentType string) string {
	if agentType == "claude" {
		return "/home/agent/.claude"
	}
	return "/home/agent/.codex"
}

func fetchRunningSessionLogs(name, tailLines, containerCreated string, searchDirs []string, matchScript string) []byte {
	args := append([]string{"docker", "exec", name, "python3", "-c", matchScript, containerCreated}, searchDirs...)
	path := findSessionLogPath(args...)
	if path == "" {
		return nil
	}
	return readSessionLogPath(name, tailLines, path)
}

func findSessionLogPath(args ...string) string {
	result, err := execOrbTimeout(15*time.Second, args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(result))
}

func readSessionLogPath(name, tailLines, path string) []byte {
	if tailLines == "0" {
		data, _ := execOrbTimeout(60*time.Second, "docker", "exec", name, "cat", path)
		return data
	}
	data, _ := execOrbLong("docker", "exec", name, "tail", "-"+tailLines, path)
	return data
}

func fetchStoppedSessionLogs(name, tailLines, configDir, containerCreated string, searchDirs []string, matchScript string) []byte {
	tmpDir := "/tmp/satui-logs-" + name
	execOrb("rm", "-rf", tmpDir)
	defer execOrb("rm", "-rf", tmpDir)
	copyStoppedSearchDirs(name, configDir, searchDirs, tmpDir)
	localSearchDirs := localStoppedSearchDirs(configDir, searchDirs, tmpDir)
	args := append([]string{"python3", "-c", matchScript, containerCreated}, localSearchDirs...)
	path := findSessionLogPath(args...)
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

func copyStoppedSearchDirs(name, configDir string, searchDirs []string, tmpDir string) {
	for _, dir := range searchDirs {
		dst := stoppedCopyTarget(configDir, dir, tmpDir)
		src := dir
		if dir == configDir {
			src = configDir + "/history.jsonl"
		}
		execOrbLong("docker", "cp", name+":"+src, dst)
	}
}

func stoppedCopyTarget(configDir, dir, tmpDir string) string {
	switch {
	case strings.HasSuffix(dir, "/sessions"):
		return tmpDir + "/sessions/"
	case strings.Contains(dir, "/projects/"):
		return tmpDir + "/projects/"
	case dir == configDir:
		return tmpDir + "/history.jsonl"
	default:
		return tmpDir + "/"
	}
}

func localStoppedSearchDirs(configDir string, searchDirs []string, tmpDir string) []string {
	localSearchDirs := make([]string, 0, len(searchDirs))
	for _, dir := range searchDirs {
		localSearchDirs = append(localSearchDirs, localStoppedSearchDir(configDir, dir, tmpDir))
	}
	return localSearchDirs
}

func localStoppedSearchDir(configDir, dir, tmpDir string) string {
	switch {
	case strings.HasSuffix(dir, "/sessions"):
		return tmpDir + "/sessions"
	case strings.Contains(dir, "/projects/"):
		return tmpDir + "/projects"
	case dir == configDir:
		return tmpDir
	default:
		return tmpDir
	}
}

func fetchPlainDockerLogs(name, tailLines string) []byte {
	args := []string{"docker", "logs"}
	if tailLines != "0" {
		args = append(args, "--tail", tailLines)
	}
	args = append(args, name)
	data, _ := execOrbLong(args...)
	return data
}

var fetchSessionLogsFunc = func(ac *Actions, name, tailLines string) []byte {
	return ac.fetchDockerLogs(name, tailLines)
}

var fetchAgentLogsFunc = fetchAgentLogsCLI
var fetchPlainLogsFunc = fetchPlainDockerLogs

func (ac *Actions) refreshLogsNow(tv *tview.TextView, name string, state *logsState) {
	ac.app.footer.ShowStatus("Loading...", false)
	go func() {
		rendered := ac.loadLogsContent(name, state)
		ac.app.tapp.QueueUpdateDraw(func() {
			if strings.TrimSpace(rendered) != "" {
				tv.SetText(rendered)
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
	name := agent.Name
	label := fmt.Sprintf("checkpoint-%d", time.Now().Unix())
	ac.app.footer.ShowStatus(fmt.Sprintf("Creating checkpoint %s...", label), false)
	go func() {
		out, err := newCLICmd("checkpoint", "create", name, label).CombinedOutput()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				msg := strings.TrimSpace(string(out))
				if msg == "" {
					msg = "Checkpoint failed"
				}
				ac.app.footer.ShowStatus(msg, true)
				return
			}
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = fmt.Sprintf("Checkpoint '%s' created", label)
			}
			ac.app.footer.ShowStatus(msg, false)
		})
	}()
}

// Diff shows the git diff from the agent's working tree in an overlay.
func (ac *Actions) Diff() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name
	ac.app.footer.ShowStatus(fmt.Sprintf("Loading diff for %s...", name), false)
	go func() {
		out, err := newCLICmd("diff", name).CombinedOutput()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				msg := strings.TrimSpace(string(out))
				if msg == "" {
					msg = "Failed to get diff"
				}
				ac.app.footer.ShowStatus(msg, true)
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
	name := agent.Name
	ac.app.footer.ShowStatus(fmt.Sprintf("Loading todos for %s...", name), false)
	go func() {
		out, err := newCLICmd("todo", "list", name).CombinedOutput()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				msg := strings.TrimSpace(string(out))
				if msg == "" {
					msg = "Failed to load todos"
				}
				ac.app.footer.ShowStatus(msg, true)
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

// ExportSessions runs agent sessions export in the background.
func (ac *Actions) ExportSessions() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	name := agent.Name
	ac.app.footer.ShowStatus(fmt.Sprintf("Exporting sessions from %s...", name), false)
	go func() {
		cmd := newCLICmd("sessions", name)
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

// CopyFiles shows a modal form for pull/push file transfer paths.
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
		cmd := newCLICmd(mcpLoginArgs(server, name)...)
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
		for _, agent := range ac.app.table.allAgents {
			ac.app.table.MarkDeleting(agent)
		}
		ac.app.footer.ShowStatus("Stopping all agents...", false)
		go func() {
			cmd := newCLICmd("stop", "--all")
			out, err := cmd.CombinedOutput()
			ac.app.poller.ForceRefresh()
			ac.app.tapp.QueueUpdateDraw(func() {
				var names []string
				for _, agent := range ac.app.table.allAgents {
					if agent.Deleting {
						names = append(names, agent.Name)
					}
				}
				if err != nil {
					ac.app.table.ClearDeleting(names...)
					ac.app.footer.ShowStatus(fmt.Sprintf("Stop all failed: %s", strings.TrimSpace(string(out))), true)
				} else {
					ac.app.table.FinishDeleting(names...)
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
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Item    struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"item"`
}

type eventMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Phase   string `json:"phase"`
	Msg     string `json:"msg"`
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
		if renderClaudeSessionLine(&b, line) || renderCodexSessionLine(&b, line) {
			continue
		}
	}

	if b.Len() == 0 {
		return "(empty session log)"
	}
	return b.String()
}

func renderClaudeSessionLine(b *strings.Builder, line []byte) bool {
	var entry claudeEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return false
	}
	label := actorLabel(entry.Type)
	if label == "" {
		return false
	}

	var msg claudeMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return false
	}
	text := extractText(msg.Content)
	if text == "" {
		return true
	}

	ts := entry.Timestamp
	if ts == "" {
		ts = msg.Timestamp
	}
	appendLogBlock(b, label, compactLogTimestamp(ts), text)
	return true
}

func renderCodexSessionLine(b *strings.Builder, line []byte) bool {
	var entry sessionLogEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return false
	}

	switch entry.Type {
	case "session_meta":
		appendSessionMeta(b, compactSessionTimestamp(entry.Timestamp), entry.Payload)
	case "response_item":
		appendResponseItem(b, compactSessionTimestamp(entry.Timestamp), entry.Payload)
	case "event_msg":
		appendEventMessage(b, compactSessionTimestamp(entry.Timestamp), entry.Payload)
	default:
		return false
	}
	return true
}

func appendSessionMeta(b *strings.Builder, ts string, payload json.RawMessage) {
	var meta sessionMeta
	json.Unmarshal(payload, &meta)
	fmt.Fprintf(b, "━━━ Session ━━━\n")
	fmt.Fprintf(b, "  Time:    %s\n", ts)
	if meta.CWD != "" {
		fmt.Fprintf(b, "  Dir:     %s\n", meta.CWD)
	}
	if meta.CLIVersion != "" {
		fmt.Fprintf(b, "  CLI:     %s\n", meta.CLIVersion)
	}
	if meta.Originator != "" {
		fmt.Fprintf(b, "  Agent:   %s\n", meta.Originator)
	}
	b.WriteString("\n")
}

func appendResponseItem(b *strings.Builder, ts string, payload json.RawMessage) {
	var item responseItem
	json.Unmarshal(payload, &item)
	itemType, role, content := normalizeResponseItem(item)
	if itemType != "message" || role == "" {
		return
	}
	text := extractText(content)
	if text == "" {
		return
	}
	appendLogBlock(b, actorLabel(role), ts, text)
}

func normalizeResponseItem(item responseItem) (string, string, json.RawMessage) {
	itemType := item.Type
	role := item.Role
	content := item.Content
	if itemType == "" && item.Item.Type != "" {
		return item.Item.Type, item.Item.Role, item.Item.Content
	}
	return itemType, role, content
}

func appendEventMessage(b *strings.Builder, ts string, payload json.RawMessage) {
	var msg eventMsg
	json.Unmarshal(payload, &msg)
	text := strings.TrimSpace(msg.Message)
	if text == "" {
		text = strings.TrimSpace(msg.Msg)
	}
	if text == "" {
		return
	}
	label := "INFO"
	if msg.Phase == "commentary" {
		label = "AGENT"
	}
	appendLogBlock(b, label, ts, text)
}

func actorLabel(kind string) string {
	switch kind {
	case "user":
		return "USER"
	case "assistant", "commentary":
		return "AGENT"
	case "developer", "system":
		return "SYS"
	default:
		return ""
	}
}

func compactSessionTimestamp(ts string) string {
	if len(ts) > 19 {
		return ts[:19]
	}
	return ts
}

func compactLogTimestamp(ts string) string {
	ts = compactSessionTimestamp(ts)
	if len(ts) >= 19 {
		return ts[11:19]
	}
	return ts
}

func appendLogBlock(b *strings.Builder, label, ts, text string) {
	if label == "" || text == "" {
		return
	}
	if ts != "" {
		fmt.Fprintf(b, "── %s [%s] ──\n%s\n\n", label, ts, text)
		return
	}
	fmt.Fprintf(b, "── %s ──\n%s\n\n", label, text)
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
	name := agent.Name
	ac.app.footer.ShowStatus("Running review...", false)
	go func() {
		out, err := newCLICmd("review", name).CombinedOutput()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				msg := strings.TrimSpace(string(out))
				if msg == "" {
					msg = "Review failed"
				}
				ac.app.footer.ShowStatus(msg, true)
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
		cmd := newCLICmd("cost", name)
		out, err := cmd.Output()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				// Show stderr from the exit error if available
				msg := strings.TrimSpace(string(out))
				if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
					msg = strings.TrimSpace(string(ee.Stderr))
				}
				ac.app.footer.ShowStatus("Cost analysis failed: "+msg, true)
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
		cmd := newCLICmd("audit")
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
		cmd := newCLICmd("fleet", manifestPath)
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
		ac.app.footer.ShowStatus("Usage: :pipeline <pipeline.yaml|name>", true)
		return
	}
	ac.app.footer.ShowStatus("Running pipeline from "+pipelinePath+"...", false)
	go func() {
		cmd := newCLICmd("pipeline", pipelinePath)
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

// PRReview runs the dedicated PR review workflow.
func (ac *Actions) PRReview(arg string) {
	ac.app.footer.ShowStatus("Running pr-review "+strings.TrimSpace(arg)+"...", false)
	go func() {
		args := []string{"pr-review"}
		if strings.TrimSpace(arg) != "" {
			args = append(args, strings.Fields(arg)...)
		}
		out, err := newCLICmd(args...).CombinedOutput()
		ac.app.poller.ForceRefresh()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("pr-review failed: "+strings.TrimSpace(string(out)), true)
			} else {
				ac.app.footer.ShowStatus("pr-review complete", false)
			}
		})
	}()
}

// PRFix runs the dedicated PR fix workflow.
func (ac *Actions) PRFix(arg string) {
	ac.app.footer.ShowStatus("Running pr-fix "+strings.TrimSpace(arg)+"...", false)
	go func() {
		args := []string{"pr-fix"}
		if strings.TrimSpace(arg) != "" {
			args = append(args, strings.Fields(arg)...)
		}
		out, err := newCLICmd(args...).CombinedOutput()
		ac.app.poller.ForceRefresh()
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("pr-fix failed: "+strings.TrimSpace(string(out)), true)
			} else {
				ac.app.footer.ShowStatus("pr-fix complete", false)
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
	if err != nil || !sshLabelAllowsPush(string(sshLabel)) {
		ac.app.footer.ShowStatus(fmt.Sprintf("%s was not spawned with --ssh; push requires SSH", name), true)
		return
	}

	ac.app.footer.ShowConfirm(fmt.Sprintf("Create PR from %s?", name), func(yes bool) {
		if !yes {
			return
		}
		ac.app.footer.ShowStatus(fmt.Sprintf("Creating PR for %s...", name), false)
		go func() {
			cmd := newCLICmd("pr", name)
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
