package term

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/creack/pty"

	"github.com/0x666c6f/safe-agentic/app/internal/emit"
	"github.com/0x666c6f/safe-agentic/pkg/tmux"
)

type CommandFactory func(container string) *exec.Cmd

func DefaultFactory(vmName string) CommandFactory {
	return func(container string) *exec.Cmd {
		cmd := exec.Command("container",
			"machine", "run", "--interactive", "--tty", "-n", vmName, "-u", "root",
			"docker", "exec", "-it", container, "tmux", "attach", "-t", tmux.SessionName())
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		return cmd
	}
}

type session struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

type Manager struct {
	em      emit.Emitter
	factory CommandFactory
	mu      sync.Mutex
	seq     atomic.Int64
	byID    map[string]*session
}

func NewManager(em emit.Emitter, factory CommandFactory) *Manager {
	return &Manager{em: em, factory: factory, byID: map[string]*session{}}
}

func (m *Manager) Open(container string) (string, error) {
	cmd := m.factory(container)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", fmt.Errorf("start pty: %w", err)
	}
	id := fmt.Sprintf("t%d", m.seq.Add(1))
	m.mu.Lock()
	m.byID[id] = &session{ptmx: ptmx, cmd: cmd}
	m.mu.Unlock()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				m.em.Emit("term:data:"+id, base64.StdEncoding.EncodeToString(buf[:n]))
			}
			if err != nil {
				break
			}
		}
		cmd.Wait()
		m.mu.Lock()
		delete(m.byID, id)
		m.mu.Unlock()
		m.em.Emit("term:exit:"+id, nil)
	}()
	return id, nil
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
	s, err := m.get(id)
	if err != nil {
		return err
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (m *Manager) Close(id string) error {
	s, err := m.get(id)
	if err != nil {
		return err
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
