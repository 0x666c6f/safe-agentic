package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"
)

// Dashboard is a lightweight HTTP server that presents an agent fleet overview
// in a browser. It reuses the same Poller and Agent types as the TUI.
type Dashboard struct {
	bind   string
	poller *Poller
	tmpl   *template.Template
}

// NewDashboard creates a Dashboard bound to the given address (e.g. "localhost:8420").
func NewDashboard(bind string) *Dashboard {
	d := &Dashboard{bind: bind}
	d.poller = NewPoller(nil) // no TUI callback needed
	d.tmpl = template.Must(template.New("").Parse(dashboardHTML))
	return d
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
	log.Printf("Dashboard running at http://%s", d.bind)
	return http.ListenAndServe(d.bind, mux)
}

func (d *Dashboard) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	agents := d.poller.GetAgents()
	if err := d.tmpl.ExecuteTemplate(w, "index", agents); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (d *Dashboard) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/agents/")
	name = strings.TrimSuffix(name, "/")

	// /agents/<name>/logs -> SSE log stream
	if strings.HasSuffix(name, "/logs") {
		d.handleAgentLogs(w, r, strings.TrimSuffix(name, "/logs"))
		return
	}

	agents := d.poller.GetAgents()
	for _, a := range agents {
		if a.Name == name {
			if err := d.tmpl.ExecuteTemplate(w, "detail", a); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}
	http.NotFound(w, r)
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

		out, err := execOrb("docker", "exec", name,
			"tmux", "capture-pane", "-t", tmuxSessionName, "-p", "-S", "-30")
		if err != nil {
			fmt.Fprintf(w, "data: [agent stopped]\n\n")
			flusher.Flush()
			return
		}
		escaped := strings.ReplaceAll(string(out), "\n", "\\n")
		fmt.Fprintf(w, "data: %s\n\n", escaped)
		flusher.Flush()
		time.Sleep(2 * time.Second)
	}
}

// handleSSE streams the full agent list as JSON every 2 seconds.
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
	agents := d.poller.GetAgents()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

func (d *Dashboard) handleAPIStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/agents/stop/")
	if name == "" {
		http.Error(w, "Missing agent name", http.StatusBadRequest)
		return
	}
	execOrb("docker", "stop", name)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"stopped","name":%q}`, name)
}
