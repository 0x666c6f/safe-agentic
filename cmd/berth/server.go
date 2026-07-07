package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	actionpkg "github.com/0x666c6f/berth/pkg/actions"
	"github.com/0x666c6f/berth/pkg/docker"
	"github.com/0x666c6f/berth/pkg/labels"
	"github.com/spf13/cobra"
)

type serverRequest struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type serverResponse struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *serverError    `json:"error,omitempty"`
}

type serverError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type timelineParams struct {
	Lines int  `json:"lines"`
	All   bool `json:"all"`
}

type agentParams struct {
	Target string `json:"target"`
	Lines  int    `json:"lines"`
}

type actionRunParams struct {
	Target string `json:"target"`
	Action string `json:"action"`
}

type serverAgent struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Repo      string `json:"repo"`
	Fleet     string `json:"fleet,omitempty"`
	Status    string `json:"status"`
	Worktree  string `json:"worktree,omitempty"`
	Resources string `json:"resources,omitempty"`
}

var serverStdio bool
var serverListen string
var serverToken string

var serverCmd = &cobra.Command{
	Use:     "server",
	Short:   "Serve berth state over a JSON protocol",
	GroupID: groupSetup,
	Args:    cobra.NoArgs,
	RunE:    runServer,
}

func init() {
	serverCmd.Flags().BoolVar(&serverStdio, "stdio", true, "Serve newline-delimited JSON over stdio")
	serverCmd.Flags().StringVar(&serverListen, "listen", "", "Serve HTTP JSON on localhost address, e.g. 127.0.0.1:8765")
	serverCmd.Flags().StringVar(&serverToken, "token", "", "Bearer token for --listen; defaults to BERTH_SERVER_TOKEN")
	rootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) error {
	if serverListen != "" {
		if err := validateServerListenAddress(serverListen); err != nil {
			return err
		}
		token := serverToken
		if token == "" {
			token = os.Getenv("BERTH_SERVER_TOKEN")
		}
		if token == "" {
			return fmt.Errorf("--listen requires --token or BERTH_SERVER_TOKEN")
		}
		fmt.Printf("berth server listening on %s\n", serverListen)
		srv := &http.Server{
			Addr:         serverListen,
			Handler:      serverHTTPHandler(token),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		return srv.ListenAndServe()
	}
	if !serverStdio {
		return fmt.Errorf("set --stdio or --listen")
	}
	return serveJSON(os.Stdin, os.Stdout)
}

func validateServerListenAddress(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid --listen address %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("--listen must bind to localhost or a loopback IP, got %q", addr)
	}
	return nil
}

func serveJSON(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	encoder := json.NewEncoder(w)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		resp := handleServerLine(line)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func handleServerLine(line []byte) serverResponse {
	var req serverRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return serverResponse{
			JSONRPC: "2.0",
			Error:   &serverError{Code: -32700, Message: "parse error: " + err.Error()},
		}
	}
	result, err := handleServerRequest(req)
	if err != nil {
		return serverResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &serverError{Code: -32000, Message: err.Error()},
		}
	}
	return serverResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
}

func serverHTTPHandler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rpc" {
			http.NotFound(w, r)
			return
		}
		if got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		defer r.Body.Close()
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(handleServerLine(body))
	})
}

func handleServerRequest(req serverRequest) (any, error) {
	switch req.Method {
	case "schema":
		return serverSchema(), nil
	case "ping":
		return map[string]any{"ok": true, "version": Version}, nil
	case "timeline":
		params, err := decodeTimelineParams(req.Params)
		if err != nil {
			return nil, err
		}
		if params.Lines <= 0 {
			params.Lines = 50
		}
		return loadTimelineEntries(params.Lines)
	case "inbox":
		params, err := decodeTimelineParams(req.Params)
		if err != nil {
			return nil, err
		}
		entries, err := loadTimelineEntries(0)
		if err != nil {
			return nil, err
		}
		var items []timelineEntry
		for _, entry := range entries {
			if params.All || needsAttention(entry) {
				items = append(items, entry)
			}
		}
		return items, nil
	case "actions.list":
		return serverListActions()
	case "agents.list":
		return serverListAgents()
	case "agent.diff":
		params, err := decodeAgentParams(req.Params)
		if err != nil {
			return nil, err
		}
		return serverAgentDiff(params.Target)
	case "agent.logs":
		params, err := decodeAgentParams(req.Params)
		if err != nil {
			return nil, err
		}
		if params.Lines <= 0 {
			params.Lines = 50
		}
		return loadRenderedLogs(context.Background(), newExecutor(), params.Target, params.Lines)
	case "actions.run":
		params, err := decodeActionRunParams(req.Params)
		if err != nil {
			return nil, err
		}
		return serverRunAction(params)
	default:
		return nil, fmt.Errorf("unknown method %q", req.Method)
	}
}

func decodeTimelineParams(raw json.RawMessage) (timelineParams, error) {
	if len(raw) == 0 {
		return timelineParams{}, nil
	}
	var params timelineParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("invalid params: %w", err)
	}
	return params, nil
}

func serverSchema() map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"methods": map[string]any{
			"ping": map[string]any{
				"params": map[string]string{},
				"result": "object",
			},
			"timeline": map[string]any{
				"params": map[string]string{"lines": "integer"},
				"result": "timelineEntry[]",
			},
			"inbox": map[string]any{
				"params": map[string]string{"all": "boolean"},
				"result": "timelineEntry[]",
			},
			"agents.list": map[string]any{
				"params": map[string]string{},
				"result": "serverAgent[]",
			},
			"agent.logs": map[string]any{
				"params": map[string]string{"target": "string", "lines": "integer"},
				"result": "string[]",
			},
			"agent.diff": map[string]any{
				"params": map[string]string{"target": "string"},
				"result": "string",
			},
			"actions.list": map[string]any{
				"params": map[string]string{},
				"result": "Action[]",
			},
			"actions.run": map[string]any{
				"params": map[string]string{"target": "string", "action": "string"},
				"result": "object",
			},
		},
	}
}

func decodeAgentParams(raw json.RawMessage) (agentParams, error) {
	if len(raw) == 0 {
		return agentParams{}, nil
	}
	var params agentParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("invalid params: %w", err)
	}
	return params, nil
}

func decodeActionRunParams(raw json.RawMessage) (actionRunParams, error) {
	var params actionRunParams
	if len(raw) == 0 {
		return params, fmt.Errorf("params.action is required")
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return params, fmt.Errorf("invalid params: %w", err)
	}
	if params.Action == "" {
		return params, fmt.Errorf("params.action is required")
	}
	return params, nil
}

func serverListActions() ([]actionpkg.Action, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get cwd: %w", err)
	}
	catalog, err := actionpkg.LoadDefault(cwd)
	if err != nil {
		return nil, err
	}
	return catalog.Actions, nil
}

func serverListAgents() ([]serverAgent, error) {
	ctx := context.Background()
	exec := newExecutor()
	format := "{{.Names}}\t{{.Label \"" + labels.AgentType + "\"}}\t" +
		"{{.Label \"" + labels.RepoDisplay + "\"}}\t" +
		"{{.Label \"" + labels.Fleet + "\"}}\t" +
		"{{.Status}}\t" +
		"{{.Label \"" + labels.Worktree + "\"}}\t" +
		"{{.Label \"" + labels.Resources + "\"}}"
	out, err := exec.Run(ctx, "docker", "ps", "-a", "--filter", "name=^agent-", "--format", format)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	var agents []serverAgent
	for _, line := range splitLines(string(out)) {
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}
		agents = append(agents, serverAgent{
			Name:      parts[0],
			Type:      parts[1],
			Repo:      parts[2],
			Fleet:     parts[3],
			Status:    parts[4],
			Worktree:  parts[5],
			Resources: parts[6],
		})
	}
	return agents, nil
}

func serverAgentDiff(target string) (string, error) {
	ctx := context.Background()
	exec := newExecutor()
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return "", err
	}
	running, _ := docker.IsRunning(ctx, exec, name)
	var out []byte
	if running {
		out, err = exec.Run(ctx, workspaceExec(name, "git diff")...)
	} else {
		out, err = runGitOnStoppedWorkspace(ctx, exec, name, "git diff")
	}
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(out), nil
}

func serverRunAction(params actionRunParams) (map[string]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get cwd: %w", err)
	}
	catalog, err := actionpkg.LoadDefault(cwd)
	if err != nil {
		return nil, err
	}
	action, ok := catalog.Get(params.Action)
	if !ok {
		return nil, fmt.Errorf("action %q not found", params.Action)
	}
	ctx := context.Background()
	exec := newExecutor()
	name, err := docker.ResolveTarget(ctx, exec, params.Target)
	if err != nil {
		return nil, err
	}
	command := action.Command
	if action.CWD != "" {
		command = "cd " + shellQuote(cleanActionCWD(action.CWD)) + " && " + command
	}
	out, err := exec.Run(ctx, workspaceExecCommand(name, "bash", "-lc", command)...)
	if err != nil {
		return nil, fmt.Errorf("run action %q in %s: %w", action.Name, name, err)
	}
	return map[string]string{
		"container": name,
		"action":    action.Name,
		"output":    string(out),
	}, nil
}
