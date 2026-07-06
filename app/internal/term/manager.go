package term

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"

	"github.com/0x666c6f/safe-agentic/app/internal/emit"
	"github.com/0x666c6f/safe-agentic/pkg/tmux"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
)

type CommandFactory func(container string) *exec.Cmd

// vmNameFromEnv mirrors the CLI's VM-name rule.
func vmNameFromEnv() string {
	if v := os.Getenv("SAFE_AGENTIC_VM_NAME"); v != "" {
		return v
	}
	return "safe-agentic"
}

func DefaultFactory(vmName string) CommandFactory {
	return func(container string) *exec.Cmd {
		// Route through the safe-ag-exec relay (base64-wrapped args) — the
		// only proven convention for arg-safe execution via `container
		// machine run`; raw args get mangled by flag parsing.
		//
		// attach -d: kick every other client. Killing the host relay does NOT
		// kill the VM-side `docker exec tmux attach`, so app restarts and
		// reattaches leak zombie clients that pin stale sizes and keep the
		// session thrash-redrawing. The app multiplexes panes onto one client
		// per container, so any other client is either a zombie or a stale
		// attach that -d sweeps (and detaching ends its docker exec).
		argv := vmexec.BuildInteractiveArgs(vmName,
			"docker", "exec", "-it", container, "tmux", "attach", "-d", "-t", tmux.SessionName())
		cmd := exec.Command("container", argv...)
		env := make([]string, 0, len(os.Environ())+1)
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(kv, "TERM=") {
				env = append(env, kv)
			}
		}
		cmd.Env = append(env, "TERM=xterm-256color")
		return cmd
	}
}

type size struct{ cols, rows int }

// session is one PTY attached to one container's tmux, shared by every pane
// viewing that container. tmux must only ever see ONE client from the app: a
// tmux window has a single size, so two clients (second app window, split on
// the same agent) resize-fight and thrash-redraw each other into garbage.
// Output fans out to all subscribers and the PTY is sized to the smallest
// subscriber so every pane renders complete rows.
type session struct {
	ptmx      *os.File
	cmd       *exec.Cmd
	container string
	subs      map[string]size // subscriber id → its requested size
	emitSeq   atomic.Int64    // orders term:data chunks (see pump below)
}

// minSize returns the smallest requested cols/rows across subscribers.
func (s *session) minSize() (cols, rows int) {
	for _, sz := range s.subs {
		if cols == 0 || sz.cols < cols {
			cols = sz.cols
		}
		if rows == 0 || sz.rows < rows {
			rows = sz.rows
		}
	}
	return cols, rows
}

type Manager struct {
	em      emit.Emitter
	factory CommandFactory
	vmName  string
	openMu  sync.Mutex // serializes Open so concurrent panes share one PTY
	mu      sync.Mutex
	seq     atomic.Int64
	byID    map[string]*session // subscriber id → shared session
	byCont  map[string]*session // container → shared session
}

func NewManager(em emit.Emitter, factory CommandFactory) *Manager {
	if factory == nil {
		factory = DefaultFactory(vmNameFromEnv())
	}
	return &Manager{em: em, factory: factory, vmName: vmNameFromEnv(),
		byID: map[string]*session{}, byCont: map[string]*session{}}
}

// waitForSession polls until the container's tmux session exists, so attaching
// to a still-starting agent (cloning a large repo before it launches the agent
// in tmux) shows "attaching…" until ready instead of failing with "no sessions".
// It bails immediately if the container is absent or the relay is unavailable —
// only a genuinely-still-starting container (exists, no tmux server yet) is
// worth waiting on.
func (m *Manager) waitForSession(container string) {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// single-token args — survives the container-machine relay
		out, err := exec.CommandContext(ctx, "container", "machine", "run", "-n", m.vmName, "-u", "root",
			"docker", "exec", container, "tmux", "has-session", "-t", tmux.SessionName()).CombinedOutput()
		cancel()
		if err == nil {
			return // session is up
		}
		s := string(out)
		if errors.Is(err, exec.ErrNotFound) ||
			strings.Contains(s, "No such container") ||
			strings.Contains(s, "is not running") {
			return // nothing to wait for — let the attach surface the real error
		}
		time.Sleep(1500 * time.Millisecond)
	}
}

func (m *Manager) Open(container string, cols, rows int) (string, error) {
	if cols <= 0 || rows <= 0 {
		cols, rows = 120, 40
	}
	// ponytail: one global open lock (waitForSession can hold it for a while);
	// per-container locks if parallel opens of different agents ever matter.
	m.openMu.Lock()
	defer m.openMu.Unlock()

	id := fmt.Sprintf("t%d", m.seq.Add(1))

	m.mu.Lock()
	if s, ok := m.byCont[container]; ok {
		// Another pane already views this agent: subscribe to the shared PTY.
		s.subs[id] = size{cols, rows}
		m.byID[id] = s
		minC, minR := s.minSize()
		ptmx := s.ptmx
		m.mu.Unlock()
		// Shrink one column and restore: there is no "request full redraw"
		// through the relay, and if the min size is unchanged tmux would never
		// repaint — the new pane would sit on a blank screen until output.
		if minC > 1 {
			pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(minC - 1), Rows: uint16(minR)})
		}
		pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(minC), Rows: uint16(minR)})
		return id, nil
	}
	m.mu.Unlock()

	m.waitForSession(container)
	cmd := m.factory(container)
	// Start the PTY at the real xterm size so `tmux attach` renders at the
	// right dimensions from the first frame.
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		return "", fmt.Errorf("start pty: %w", err)
	}
	s := &session{ptmx: ptmx, cmd: cmd, container: container, subs: map[string]size{id: {cols, rows}}}
	m.mu.Lock()
	m.byID[id] = s
	m.byCont[container] = s
	m.mu.Unlock()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				// Wails alpha dispatches every event on its own goroutine, so
				// rapid chunks race to the webview and arrive out of order,
				// interleaving escape sequences into garbage. Prefix each chunk
				// with a sequence number; the frontend reassembles strict order.
				payload := strconv.FormatInt(s.emitSeq.Add(1), 10) + "|" +
					base64.StdEncoding.EncodeToString(buf[:n])
				for _, sid := range m.subscriberIDs(s) {
					m.em.Emit("term:data:"+sid, payload)
				}
			}
			if err != nil {
				break
			}
		}
		cmd.Wait()
		m.mu.Lock()
		ids := make([]string, 0, len(s.subs))
		for sid := range s.subs {
			ids = append(ids, sid)
			delete(m.byID, sid)
		}
		s.subs = map[string]size{}
		if m.byCont[container] == s {
			delete(m.byCont, container)
		}
		m.mu.Unlock()
		for _, sid := range ids {
			m.em.Emit("term:exit:"+sid, nil)
		}
	}()
	return id, nil
}

// subscriberIDs snapshots the ids fanned out to for one output chunk.
func (m *Manager) subscriberIDs(s *session) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(s.subs))
	for sid := range s.subs {
		ids = append(ids, sid)
	}
	return ids
}

func (m *Manager) get(id string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.byID[id]
	if !ok {
		return nil, fmt.Errorf("unknown terminal session %q", id)
	}
	return s, nil
}

func (m *Manager) Write(id, data string) error {
	s, err := m.get(id)
	if err != nil {
		return err
	}
	_, err = io.WriteString(s.ptmx, data)
	return err
}

func (m *Manager) Resize(id string, cols, rows int) error {
	m.mu.Lock()
	s, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown terminal session %q", id)
	}
	s.subs[id] = size{cols, rows}
	minC, minR := s.minSize()
	m.mu.Unlock()
	// SIGWINCH propagates through the container-machine relay to tmux (verified),
	// so a plain PTY resize is enough — tmux (window-size latest) follows the
	// client and repaints. The PTY tracks the SMALLEST subscriber so every pane
	// sees complete rows; larger panes just have idle margins.
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(minC), Rows: uint16(minR)})
}

func (m *Manager) Close(id string) error {
	m.mu.Lock()
	s, ok := m.byID[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown terminal session %q", id)
	}
	delete(m.byID, id)
	delete(s.subs, id)
	last := len(s.subs) == 0
	if last && m.byCont[s.container] == s {
		delete(m.byCont, s.container)
	}
	minC, minR := s.minSize()
	m.mu.Unlock()

	if !last {
		// Remaining panes may now afford a bigger grid.
		return pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(minC), Rows: uint16(minR)})
	}
	s.ptmx.Close()
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	return nil
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.byID))
	for id := range m.byID {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Close(id)
	}
}
