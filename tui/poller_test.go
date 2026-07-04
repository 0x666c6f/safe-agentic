package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func installFakeContainer(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "container")
	script := `#!/bin/sh
cmd=""
decode=0
for arg in "$@"; do
  if [ "$decode" = "1" ]; then
    cmd="$arg"
    decode=2
    continue
  fi
  if [ "$decode" = "2" ]; then
    decoded=$(printf '%s' "$arg" | base64 -d)
    cmd="$cmd $decoded"
    continue
  fi
  if [ "$arg" = "/usr/local/bin/safe-ag-exec" ]; then
    decode=1
  fi
done
[ -n "$cmd" ] || cmd="$*"
case "$cmd" in
  *"docker ps -a"*)
    cat <<'EOF'
agent-beta	codex	org/private-api	yes	session	app	sandbox	bridge			tmux	Up 2 minutes
agent-alpha	claude	org/docs	no								Exited (0) 1 minute ago
EOF
    ;;
  *"docker stats --no-stream"*)
    cat <<'EOF'
{"Name":"agent-beta","CPUPerc":"25.5%","MemUsage":"512MiB / 8GiB","NetIO":"2MB / 1MB","PIDs":"7"}
EOF
    ;;
  *"docker exec agent-beta bash -c"*)
    echo working
    ;;
  *"docker exec agent-beta tmux capture-pane"*)
    printf 'line one\nline two\n'
    ;;
  *"docker exec agent-beta tmux has-session -t safe-agentic"*)
    exit 0
    ;;
  *"docker inspect --format {{index .Config.Labels \"safe-agentic.terminal\"}} agent-beta"*)
    echo tmux
    ;;
  *"docker stop agent-beta"*)
    echo "$cmd" >> "$SAFE_AGENTIC_TEST_CONTAINER_LOG"
    ;;
  *)
    exit 1
    ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake container: %v", err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SAFE_AGENTIC_TEST_CONTAINER_LOG", filepath.Join(dir, "container.log"))
}

func TestParsePSOutputAndHelpers(t *testing.T) {
	psData := []byte("agent-beta\tcodex\torg/private-api\tyes\tsession\tapp\tsandbox\tbridge\t\t\ttmux\tUp 1 second\n" +
		"agent-alpha\tclaude\torg/docs\tno\t\t\t\t\t\t\t\tExited (0) 1 second ago\n" +
		"short\trow\n")
	agents := parsePSOutput(psData)
	if len(agents) != 2 {
		t.Fatalf("len(parsePSOutput) = %d, want 2", len(agents))
	}
	if !agents[0].Running || agents[1].Running {
		t.Fatalf("running flags = %#v", agents)
	}
	if !agents[1].Finished {
		t.Fatalf("finished flag = false, want true: %#v", agents[1])
	}
	if agents[1].Status != "Finished 1 second ago" {
		t.Fatalf("status = %q, want Finished 1 second ago", agents[1].Status)
	}
	if agents[0].Repo != "org/private-api" || agents[0].NetworkMode != "bridge" {
		t.Fatalf("parsed agent = %#v", agents[0])
	}
	if agents[0].Terminal != "tmux" {
		t.Fatalf("parsed terminal = %q, want tmux", agents[0].Terminal)
	}

	labels := parseLabels("a=1, b = 2, malformed")
	if !reflect.DeepEqual(labels, map[string]string{"a": "1", "b": "2"}) {
		t.Fatalf("parseLabels() = %#v", labels)
	}

	lines := splitLines([]byte("\nalpha\nbeta\n"))
	if len(lines) != 2 || string(lines[0]) != "alpha" || string(lines[1]) != "beta" {
		t.Fatalf("splitLines() = %#v", lines)
	}

	mergeStats(agents, []byte(`{"Name":"agent-beta","CPUPerc":"3%","MemUsage":"10MiB","NetIO":"1B / 2B","PIDs":"4"}`))
	if agents[0].CPU != "3%" || agents[0].PIDs != "4" {
		t.Fatalf("mergeStats() = %#v", agents[0])
	}
}

func TestFetchAgentsProbeActivitiesAndContainerTmux(t *testing.T) {
	installFakeContainer(t)

	agents, err := fetchAgents()
	if err != nil {
		t.Fatalf("fetchAgents() error = %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("len(fetchAgents) = %d, want 2", len(agents))
	}
	if agents[0].CPU != "25.5%" || agents[0].Memory != "512MiB / 8GiB" || agents[0].NetIO != "2MB / 1MB" || agents[0].PIDs != "7" {
		t.Fatalf("running agent stats = %#v", agents[0])
	}

	probeActivities(agents)
	if agents[0].Activity != "Working" {
		t.Fatalf("running activity = %q, want Working", agents[0].Activity)
	}
	if agents[1].Activity != "Stopped" {
		t.Fatalf("stopped activity = %q, want Stopped", agents[1].Activity)
	}
	// State: the running agent's pane has no markers → unknown; the stopped
	// agent exited cleanly (code 0) → done.
	if agents[0].State != "unknown" {
		t.Fatalf("running state = %q, want unknown", agents[0].State)
	}
	if agents[1].State != "done" {
		t.Fatalf("stopped state = %q, want done", agents[1].State)
	}
	if got := probeProcessActivity("agent-beta", "codex"); got != "Working" {
		t.Fatalf("probeProcessActivity(codex) = %q", got)
	}
	if got := probeProcessActivity("agent-alpha", "claude"); got != "Idle" {
		t.Fatalf("probeProcessActivity(claude) = %q", got)
	}
	if !containerUsesTmux("agent-beta") {
		t.Fatal("containerUsesTmux() = false, want true")
	}
}

// A running container whose tmux pane can't be captured (headless/background
// mode, a bash-direct shell, or a session still starting) must probe as
// working, not unknown.
func TestProbeAgentStateNoPaneIsWorking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "container")
	script := `#!/bin/sh
cmd=""
decode=0
for arg in "$@"; do
  if [ "$decode" = "1" ]; then cmd="$arg"; decode=2; continue; fi
  if [ "$decode" = "2" ]; then decoded=$(printf '%s' "$arg" | base64 -d); cmd="$cmd $decoded"; continue; fi
  if [ "$arg" = "/usr/local/bin/safe-ag-exec" ]; then decode=1; fi
done
[ -n "$cmd" ] || cmd="$*"
case "$cmd" in
  *"tmux capture-pane"*) exit 1 ;;
  *) exit 0 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake container: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	a := &Agent{Name: "agent-x", Type: "claude", Running: true, Terminal: "tmux"}
	probeAgentState(a)
	if a.State != "working" {
		t.Fatalf("state = %q, want working", a.State)
	}
	if !strings.Contains(a.StateReason, "no tmux pane") {
		t.Fatalf("reason = %q, want 'no tmux pane'", a.StateReason)
	}
}

func TestPollerPollGetAgentsAndErrorStale(t *testing.T) {
	installFakeContainer(t)

	var (
		gotAgents []Agent
		gotStale  bool
		calls     int
	)
	p := NewPoller(func(agents []Agent, stale bool) {
		gotAgents = append([]Agent(nil), agents...)
		gotStale = stale
		calls++
	})
	p.poll()

	if calls != 1 || gotStale {
		t.Fatalf("poll callback calls=%d stale=%v", calls, gotStale)
	}
	if len(gotAgents) != 2 {
		t.Fatalf("callback agents len = %d", len(gotAgents))
	}

	copyAgents := p.GetAgents()
	copyAgents[0].Name = "mutated"
	if p.GetAgents()[0].Name == "mutated" {
		t.Fatal("GetAgents() should return a copy")
	}

	p.Stop()

	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)
	calls = 0
	gotStale = false
	gotAgents = nil
	p = NewPoller(func(agents []Agent, stale bool) {
		gotAgents = append([]Agent(nil), agents...)
		gotStale = stale
		calls++
	})
	p.poll()
	if calls != 1 || !gotStale || len(gotAgents) != 0 {
		t.Fatalf("stale callback calls=%d stale=%v agents=%d", calls, gotStale, len(gotAgents))
	}
}

func TestPollerRestartAndStopSafe(t *testing.T) {
	installFakeContainer(t)

	p := NewPoller(nil)
	p.Start()
	time.Sleep(50 * time.Millisecond)
	p.Restart()
	time.Sleep(50 * time.Millisecond)
	p.Stop()
	p.Stop()
}

func TestPollerForceRefresh(t *testing.T) {
	installFakeContainer(t)

	ch := make(chan string, 1)
	p := NewPoller(func(agents []Agent, stale bool) {
		ch <- fmt.Sprintf("%d:%v", len(agents), stale)
	})
	p.ForceRefresh()

	select {
	case got := <-ch:
		if got != "2:false" {
			t.Fatalf("ForceRefresh callback = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ForceRefresh callback timed out")
	}
}

func TestSetStoppedState(t *testing.T) {
	done := &Agent{Finished: true, Status: "Finished"}
	setStoppedState(done)
	if done.State != "done" {
		t.Fatalf("clean-exit state = %q, want done", done.State)
	}

	crashed := &Agent{Finished: false, Status: "Exited (137) 3s ago"}
	setStoppedState(crashed)
	if crashed.State != "exited" {
		t.Fatalf("non-zero-exit state = %q, want exited", crashed.State)
	}
	if crashed.StateReason != "Exited (137) 3s ago" {
		t.Fatalf("exited reason = %q", crashed.StateReason)
	}
}

func TestCapturePreview(t *testing.T) {
	installFakeContainer(t)

	got, err := capturePreview("agent-beta", 30)
	if err != nil {
		t.Fatalf("capturePreview() error = %v", err)
	}
	if !strings.Contains(got, "line one") {
		t.Fatalf("capturePreview() = %q", got)
	}

	if _, err := capturePreview("agent-alpha", 30); err == nil || !strings.Contains(err.Error(), "No tmux session") {
		t.Fatalf("capturePreview() missing tmux err = %v", err)
	}
}
