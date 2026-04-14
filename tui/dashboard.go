package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type dashboardPageData struct {
	InitialAgent     string
	InitialAgentsJS  template.JS
	RefreshIntervalS int
}

type dashboardAPIResponse struct {
	OK          bool   `json:"ok"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
	Command     string `json:"command,omitempty"`
	Interactive bool   `json:"interactive,omitempty"`
}

type dashboardCheckpointRequest struct {
	Label string `json:"label"`
}

type dashboardCheckpointRevertRequest struct {
	Ref string `json:"ref"`
}

type dashboardTransferRequest struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type dashboardPathRequest struct {
	Path string `json:"path"`
}

type dashboardSpawnRequest struct {
	AgentType   string `json:"agentType"`
	RepoURL     string `json:"repoURL"`
	Name        string `json:"name"`
	Prompt      string `json:"prompt"`
	SSH         bool   `json:"ssh"`
	ReuseAuth   bool   `json:"reuseAuth"`
	ReuseGHAuth bool   `json:"reuseGHAuth"`
	AWSProfile  string `json:"awsProfile"`
	Docker      bool   `json:"docker"`
	Identity    string `json:"identity"`
}

type dashboardCommandRequest struct {
	Args []string `json:"args"`
}

// Dashboard is a browser UI backed by the same poller and CLI commands as the TUI.
type Dashboard struct {
	bind         string
	poller       *Poller
	tmpl         *template.Template
	runCLI       func(args ...string) (string, error)
	runOrb       func(args ...string) ([]byte, error)
	openTerminal func(args []string) error
}

// NewDashboard creates a Dashboard bound to the given address (e.g. "localhost:8420").
func NewDashboard(bind string) *Dashboard {
	return &Dashboard{
		bind:   bind,
		poller: NewPoller(nil),
		tmpl:   template.Must(template.New("dashboard").Parse(dashboardHTML)),
		runCLI: func(args ...string) (string, error) {
			out, err := newCLICmd(args...).CombinedOutput()
			return strings.TrimSpace(string(out)), err
		},
		runOrb:       execOrbLong,
		openTerminal: openTerminalCommand,
	}
}

// Start begins polling and serves HTTP until the process exits.
func (d *Dashboard) Start() error {
	d.poller.Start()
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.handleIndex)
	mux.HandleFunc("/agents/", d.handleAgentDetail)
	mux.HandleFunc("/events", d.handleSSE)
	mux.HandleFunc("/api/agents", d.handleAPIAgents)
	mux.HandleFunc("/api/agents/stop/", d.handleAPIStop)
	mux.HandleFunc("/api/agents/", d.handleAPIAgent)
	mux.HandleFunc("/api/global/content/", d.handleAPIGlobalContent)
	mux.HandleFunc("/api/global/action/", d.handleAPIGlobalAction)
	mux.HandleFunc("/api/command", d.handleAPICommand)
	if assetDir := d.assetDir(); assetDir != "" {
		mux.Handle("/dashboard-assets/", http.StripPrefix("/dashboard-assets/", http.FileServer(http.Dir(assetDir))))
	}
	log.Printf("Dashboard running at http://%s", d.bind)
	return http.ListenAndServe(d.bind, mux)
}

func (d *Dashboard) assetDir() string {
	exe, _ := os.Executable()
	wd, _ := os.Getwd()
	return findDashboardAssetDir(exe, wd)
}

func findDashboardAssetDir(exePath, wd string) string {
	if exePath != "" {
		candidate := filepath.Clean(filepath.Join(filepath.Dir(exePath), "..", "docs", "assets"))
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	if wd != "" {
		candidate := filepath.Join(wd, "docs", "assets")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func dashboardWorkspaceRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Clean(wd), nil
}

func resolveDashboardWorkspacePath(raw string, requireYAML bool) (string, error) {
	root, err := dashboardWorkspaceRoot()
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("path is required")
	}

	var candidate string
	if filepath.IsAbs(value) {
		candidate = filepath.Clean(value)
	} else {
		candidate = filepath.Join(root, value)
	}
	candidate = filepath.Clean(candidate)

	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must stay within %s", root)
	}
	if requireYAML {
		ext := strings.ToLower(filepath.Ext(candidate))
		if ext != ".yaml" && ext != ".yml" {
			return "", fmt.Errorf("manifest path must end in .yaml or .yml")
		}
	}
	return candidate, nil
}

func (d *Dashboard) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	d.renderShell(w, "")
}

func (d *Dashboard) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/agents/"), "/")
	if strings.HasSuffix(name, "/logs") {
		d.handleAgentLogs(w, r, strings.TrimSuffix(name, "/logs"))
		return
	}
	if name == "" {
		http.NotFound(w, r)
		return
	}
	if _, ok := d.lookupAgent(name); !ok {
		http.NotFound(w, r)
		return
	}
	d.renderShell(w, name)
}

func (d *Dashboard) renderShell(w http.ResponseWriter, initialAgent string) {
	agents := d.poller.GetAgents()
	data := dashboardPageData{
		InitialAgent:     initialAgent,
		InitialAgentsJS:  mustJSONJS(agents),
		RefreshIntervalS: pollInterval,
	}
	if err := d.tmpl.ExecuteTemplate(w, "app", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func mustJSONJS(v any) template.JS {
	data, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return template.JS(data)
}

func (d *Dashboard) lookupAgent(name string) (Agent, bool) {
	for _, agent := range d.poller.GetAgents() {
		if agent.Name == name {
			return agent, true
		}
	}
	return Agent{}, false
}

func (d *Dashboard) handleAgentLogs(w http.ResponseWriter, r *http.Request, name string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		out, err := dashboardPreviewContent(name)
		if err != nil {
			fmt.Fprintf(w, "data: [agent stopped]\n\n")
			flusher.Flush()
			return
		}
		escaped := strings.ReplaceAll(out, "\n", "\\n")
		fmt.Fprintf(w, "data: %s\n\n", escaped)
		flusher.Flush()
		time.Sleep(2 * time.Second)
	}
}

func (d *Dashboard) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		agents := d.poller.GetAgents()
		data, _ := json.Marshal(agents)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		time.Sleep(2 * time.Second)
	}
}

func (d *Dashboard) handleAPIAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agents := d.poller.GetAgents()
	d.writeJSON(w, agents)
}

func (d *Dashboard) handleAPIStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/agents/stop/"), "/")
	if name == "" {
		http.Error(w, "Missing agent name", http.StatusBadRequest)
		return
	}
	out, err := d.runCLI("stop", name)
	d.writeCommandResult(w, out, err, "", false)
}

func (d *Dashboard) handleAPIAgent(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/agents/"), "/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	name, err := url.PathUnescape(parts[0])
	if err != nil || name == "" {
		http.Error(w, "Invalid agent name", http.StatusBadRequest)
		return
	}

	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		agent, ok := d.lookupAgent(name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		d.writeJSON(w, agent)
	case len(parts) >= 3 && parts[1] == "content":
		d.handleAPIAgentContent(w, r, name, parts[2])
	case len(parts) >= 3 && parts[1] == "action":
		d.handleAPIAgentAction(w, r, name, parts[2])
	case len(parts) >= 3 && parts[1] == "interactive":
		d.handleAPIAgentInteractive(w, r, name, parts[2])
	default:
		http.NotFound(w, r)
	}
}

func (d *Dashboard) handleAPIAgentContent(w http.ResponseWriter, r *http.Request, name, kind string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := d.lookupAgent(name); !ok {
		http.NotFound(w, r)
		return
	}

	var (
		content string
		err     error
	)

	switch kind {
	case "summary":
		content, err = d.runCLI("summary", name)
	case "preview":
		content, err = dashboardPreviewContent(name)
	case "logs":
		tail := r.URL.Query().Get("tail")
		if tail == "" {
			tail = "500"
		}
		content, err = dashboardLogsContent(name, tail)
	case "describe":
		content, err = d.prettyInspect(name)
	case "diff":
		content, err = d.runCLI("diff", name)
	case "todo":
		content, err = d.runCLI("todo", "list", name)
	case "review":
		content, err = d.runCLI("review", name)
	case "cost":
		content, err = d.runCLI("cost", name)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(content) == "" {
		content = "(no output)"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(content))
}

func (d *Dashboard) handleAPIAgentAction(w http.ResponseWriter, r *http.Request, name, action string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := d.lookupAgent(name); !ok {
		http.NotFound(w, r)
		return
	}

	switch action {
	case "stop":
		out, err := d.runCLI("stop", name)
		d.writeCommandResult(w, out, err, "", false)
	case "checkpoint":
		var req dashboardCheckpointRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		label := strings.TrimSpace(req.Label)
		if label == "" {
			label = fmt.Sprintf("checkpoint-%d", time.Now().Unix())
		}
		out, err := d.runCLI("checkpoint", "create", name, label)
		d.writeCommandResult(w, out, err, "", false)
	case "checkpoint-list":
		out, err := d.runCLI("checkpoint", "list", name)
		d.writeCommandResult(w, out, err, "", false)
	case "checkpoint-restore":
		var req dashboardCheckpointRevertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		ref := strings.TrimSpace(req.Ref)
		if ref == "" {
			http.Error(w, "ref is required", http.StatusBadRequest)
			return
		}
		out, err := d.runCLI("checkpoint", "restore", name, ref)
		d.writeCommandResult(w, out, err, "", false)
	case "sessions":
		out, err := d.runCLI("sessions", name)
		d.writeCommandResult(w, out, err, "", false)
	case "pr":
		out, err := d.runCLI("pr", name)
		d.writeCommandResult(w, out, err, "", false)
	case "copy":
		var req dashboardTransferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		source := strings.TrimSpace(req.Source)
		destination, err := resolveDashboardWorkspacePath(req.Destination, false)
		if source == "" || strings.TrimSpace(req.Destination) == "" {
			http.Error(w, "source and destination are required", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out, err := d.runOrb("docker", "cp", name+":"+source, destination)
		d.writeCommandResult(w, strings.TrimSpace(string(out)), err, "", false)
	case "push":
		var req dashboardTransferRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		source, err := resolveDashboardWorkspacePath(req.Source, false)
		destination := strings.TrimSpace(req.Destination)
		if strings.TrimSpace(req.Source) == "" || destination == "" {
			http.Error(w, "source and destination are required", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out, err := d.runOrb("docker", "cp", source, name+":"+destination)
		d.writeCommandResult(w, strings.TrimSpace(string(out)), err, "", false)
	default:
		http.NotFound(w, r)
	}
}

func (d *Dashboard) handleAPIAgentInteractive(w http.ResponseWriter, r *http.Request, name, kind string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if name != "--latest" {
		if _, ok := d.lookupAgent(name); !ok {
			http.NotFound(w, r)
			return
		}
	}
	args, err := d.interactiveArgs(name, kind, r.URL.Query().Get("server"))
	if err != nil {
		d.writeCommandResult(w, "", err, "", false)
		return
	}
	command := interactiveDisplayCommand(args)
	if r.Method == http.MethodPost {
		out := "Opened Terminal for: " + command
		err = d.openTerminal(args)
		d.writeCommandResult(w, out, err, command, true)
		return
	}
	d.writeCommandResult(w, "", nil, command, true)
}

func (d *Dashboard) handleAPIGlobalContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	kind := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/global/content/"), "/")
	switch kind {
	case "audit":
		out, err := d.runCLI("audit")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(out) == "" {
			out = "(no output)"
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(out))
	default:
		http.NotFound(w, r)
	}
}

func (d *Dashboard) handleAPIGlobalAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/global/action/"), "/")
	switch action {
	case "kill-all":
		out, err := d.runCLI("stop", "--all")
		d.writeCommandResult(w, out, err, "", false)
	case "fleet":
		var req dashboardPathRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		path, err := resolveDashboardWorkspacePath(req.Path, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out, err := d.runCLI("fleet", path)
		d.writeCommandResult(w, out, err, "", false)
	case "pipeline":
		var req dashboardPathRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		path, err := resolveDashboardWorkspacePath(req.Path, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out, err := d.runCLI("pipeline", path)
		d.writeCommandResult(w, out, err, "", false)
	case "spawn":
		var req dashboardSpawnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		spec := spawnFormSpec{
			agentType:   strings.TrimSpace(req.AgentType),
			repoURL:     strings.TrimSpace(req.RepoURL),
			name:        strings.TrimSpace(req.Name),
			prompt:      req.Prompt,
			ssh:         req.SSH,
			reuseAuth:   req.ReuseAuth,
			reuseGHAuth: req.ReuseGHAuth,
			awsProfile:  strings.TrimSpace(req.AWSProfile),
			docker:      req.Docker,
			identity:    strings.TrimSpace(req.Identity),
		}
		if spec.agentType == "" {
			spec.agentType = "claude"
		}
		args := append(buildSpawnFormArgs(spec), "--background")
		out, err := d.runCLI(args...)
		d.writeCommandResult(w, out, err, "", false)
	default:
		http.NotFound(w, r)
	}
}

func (d *Dashboard) handleAPICommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req dashboardCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.Args) == 0 {
		http.Error(w, "args are required", http.StatusBadRequest)
		return
	}
	if command, ok, err := d.interactiveCommandFromArgs(req.Args); ok {
		d.writeCommandResult(w, "", err, command, true)
		return
	}
	if err := validateDashboardCommandArgs(req.Args); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out, err := d.runCLI(req.Args...)
	d.writeCommandResult(w, out, err, "", false)
}

func (d *Dashboard) interactiveArgs(name, kind, server string) ([]string, error) {
	switch kind {
	case "attach":
		target := strings.TrimSpace(name)
		if target == "" {
			return nil, fmt.Errorf("attach target is required")
		}
		return []string{cliBinary, "attach", target}, nil
	case "resume":
		agent, ok := d.lookupAgent(name)
		if !ok {
			return nil, fmt.Errorf("agent %s not found", name)
		}
		if containerUsesTmux(name) {
			return buildTmuxAttachArgs(name), nil
		}
		if !resumeSupported(agent.Type) {
			return nil, fmt.Errorf("resume not supported for %s containers", agent.Type)
		}
		if agent.Running {
			return buildAttachArgs(name), nil
		}
		cwd := defaultResumeCWD
		meta, err := loadLatestSessionMeta(name, agent.Type, false)
		if err == nil && meta.CWD != "" {
			cwd = meta.CWD
		}
		args, err := buildResumeExecArgs(agent.Type, name, cwd)
		if err != nil {
			return nil, err
		}
		return args, nil
	case "mcp-login":
		server = strings.TrimSpace(server)
		if server == "" {
			return nil, fmt.Errorf("server is required for mcp-login")
		}
		return append([]string{cliBinary}, mcpLoginArgs(server, name)...), nil
	default:
		return nil, fmt.Errorf("unsupported interactive command: %s", kind)
	}
}

func interactiveDisplayCommand(args []string) string {
	if len(args) == 0 {
		return ""
	}
	display := append([]string(nil), args...)
	if display[0] == cliBinary {
		display[0] = cliBinaryName
	}
	return shellQuoteArgs(display)
}

func openTerminalCommand(args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("opening interactive commands from the dashboard is only supported on macOS")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	command := "cd " + shellQuoteArgs([]string{cwd}) + " && exec " + shellQuoteArgs(args)
	script := []string{
		"-e", `tell application "Terminal"`,
		"-e", `activate`,
		"-e", `do script ` + strconv.Quote(command),
		"-e", `end tell`,
	}
	return exec.Command("osascript", script...).Run()
}

func (d *Dashboard) interactiveCommand(name, kind, server string) (string, error) {
	args, err := d.interactiveArgs(name, kind, server)
	if err != nil {
		return "", err
	}
	return interactiveDisplayCommand(args), nil
}

func (d *Dashboard) interactiveCommandFromArgs(args []string) (string, bool, error) {
	switch args[0] {
	case "attach":
		if len(args) < 2 {
			return "", true, fmt.Errorf("attach requires a target")
		}
		cmd, err := d.interactiveCommand(args[1], "attach", "")
		return cmd, true, err
	case "resume":
		if len(args) < 2 {
			return "", true, fmt.Errorf("resume requires a target")
		}
		cmd, err := d.interactiveCommand(args[1], "resume", "")
		return cmd, true, err
	case "mcp-login":
		if len(args) < 2 {
			return "", true, fmt.Errorf("mcp-login requires a server")
		}
		target := ""
		if len(args) >= 3 {
			target = args[2]
		}
		if target == "" {
			return shellQuoteArgs(append([]string{"safe-ag"}, "mcp-login", args[1])), true, nil
		}
		cmd, err := d.interactiveCommand(target, "mcp-login", args[1])
		return cmd, true, err
	case "vm":
		if len(args) >= 2 && args[1] == "ssh" {
			return shellQuoteArgs(append([]string{"safe-ag"}, args...)), true, nil
		}
	case "tui", "dashboard":
		return shellQuoteArgs(append([]string{"safe-ag"}, args...)), true, nil
	case "template":
		if len(args) >= 2 && args[1] == "create" {
			return shellQuoteArgs(append([]string{"safe-ag"}, args...)), true, nil
		}
	case "cron":
		if len(args) >= 2 && args[1] == "daemon" {
			return shellQuoteArgs(append([]string{"safe-ag"}, args...)), true, nil
		}
	case "run":
		if !containsArg(args[1:], "--background") {
			return shellQuoteArgs(append([]string{"safe-ag"}, args...)), true, nil
		}
	case "spawn":
		if !containsArg(args[1:], "--background") {
			return shellQuoteArgs(append([]string{"safe-ag"}, args...)), true, nil
		}
	}
	return "", false, nil
}

func validateDashboardCommandArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("args are required")
	}
	switch args[0] {
	case "list", "audit", "cleanup", "setup", "update", "diagnose",
		"stop", "peek", "logs", "summary", "output", "diff", "review",
		"replay", "retry", "sessions", "aws-refresh", "cost", "pr":
		return nil
	case "checkpoint":
		if len(args) < 2 {
			return fmt.Errorf("checkpoint subcommand is required")
		}
		switch args[1] {
		case "create", "list", "restore", "revert":
			return nil
		}
		return fmt.Errorf("checkpoint subcommand %q is not allowed from the dashboard", args[1])
	case "todo":
		if len(args) < 2 {
			return fmt.Errorf("todo subcommand is required")
		}
		switch args[1] {
		case "add", "list", "check", "uncheck":
			return nil
		}
		return fmt.Errorf("todo subcommand %q is not allowed from the dashboard", args[1])
	case "fleet":
		if len(args) >= 2 && args[1] == "status" {
			return nil
		}
		return validateDashboardManifestArgs("fleet", args[1:])
	case "pipeline":
		return validateDashboardManifestArgs("pipeline", args[1:])
	case "config":
		if len(args) < 2 {
			return fmt.Errorf("config subcommand is required")
		}
		switch args[1] {
		case "show", "get", "set", "reset":
			return nil
		}
		return fmt.Errorf("config subcommand %q is not allowed from the dashboard", args[1])
	case "template":
		if len(args) < 2 {
			return fmt.Errorf("template subcommand is required")
		}
		switch args[1] {
		case "list", "show":
			return nil
		}
		return fmt.Errorf("template subcommand %q is not allowed from the dashboard", args[1])
	case "cron":
		if len(args) < 2 {
			return fmt.Errorf("cron subcommand is required")
		}
		switch args[1] {
		case "add", "list", "remove", "enable", "disable", "run":
			return nil
		}
		return fmt.Errorf("cron subcommand %q is not allowed from the dashboard", args[1])
	case "vm":
		if len(args) < 2 {
			return fmt.Errorf("vm subcommand is required")
		}
		switch args[1] {
		case "start", "stop":
			return nil
		}
		return fmt.Errorf("vm subcommand %q is not allowed from the dashboard", args[1])
	case "spawn", "run":
		if !containsArg(args[1:], "--background") {
			return fmt.Errorf("%s requires --background when run from the dashboard", args[0])
		}
		return nil
	default:
		return fmt.Errorf("command %q is not allowed from the dashboard", args[0])
	}
}

func validateDashboardManifestArgs(kind string, args []string) error {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		_, err := resolveDashboardWorkspacePath(arg, true)
		if err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("%s manifest path is required", kind)
}

func containsArg(args []string, needle string) bool {
	for _, arg := range args {
		if arg == needle {
			return true
		}
	}
	return false
}

func (d *Dashboard) prettyInspect(name string) (string, error) {
	data, err := d.runOrb("docker", "inspect", name)
	if err != nil {
		return "", err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		return string(data), nil
	}
	return pretty.String(), nil
}

func (d *Dashboard) writeCommandResult(w http.ResponseWriter, out string, err error, command string, interactive bool) {
	resp := dashboardAPIResponse{
		OK:          err == nil,
		Output:      strings.TrimSpace(out),
		Command:     command,
		Interactive: interactive,
	}
	if err != nil {
		resp.Error = err.Error()
		status := http.StatusInternalServerError
		if interactive {
			status = http.StatusBadRequest
		}
		w.WriteHeader(status)
	}
	d.writeJSON(w, resp)
}

func (d *Dashboard) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func dashboardPreviewContent(name string) (string, error) {
	if content, err := capturePreview(name, 30); err == nil && strings.TrimSpace(content) != "" {
		return content, nil
	}
	content, err := dashboardLogsContent(name, "30")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("no preview available")
	}
	return content, nil
}

func dashboardLogsContent(name, tailLines string) (string, error) {
	state := &logsState{tailLines: tailLines}
	data := (&Actions{}).fetchDockerLogs(name, tailLines)
	rendered := renderSessionLog(data)
	if len(data) > 0 && rendered != "(empty session log)" {
		state.rawMode = false
		return rendered, nil
	}
	agentRendered := fetchAgentLogsCLI(name, tailLines)
	if strings.TrimSpace(agentRendered) != "" {
		state.rawMode = false
		return agentRendered, nil
	}
	raw := fetchPlainDockerLogs(name, tailLines)
	if len(raw) == 0 {
		return "", fmt.Errorf("no logs found")
	}
	state.rawMode = true
	return strings.TrimSpace(string(raw)), nil
}
