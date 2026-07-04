package svc

import "github.com/0x666c6f/safe-agentic/app/internal/term"

type TerminalService struct {
	Manager *term.Manager
}

func (t *TerminalService) Open(container string) (string, error) { return t.Manager.Open(container) }
func (t *TerminalService) Write(id, data string) error           { return t.Manager.Write(id, data) }
func (t *TerminalService) Resize(id string, cols, rows int) error {
	return t.Manager.Resize(id, cols, rows)
}
func (t *TerminalService) Close(id string) error { return t.Manager.Close(id) }
