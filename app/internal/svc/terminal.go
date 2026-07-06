package svc

import "github.com/0x666c6f/safe-agentic/app/internal/term"

type TerminalService struct {
	Manager *term.Manager
}

func (t *TerminalService) Open(container string, cols, rows int) (string, error) {
	return t.Manager.Open(container, cols, rows)
}
func (t *TerminalService) Write(id, data string) error           { return t.Manager.Write(id, data) }
func (t *TerminalService) Resize(id string, cols, rows int) error {
	return t.Manager.Resize(id, cols, rows)
}
func (t *TerminalService) Close(id string) error  { return t.Manager.Close(id) }
func (t *TerminalService) Redraw(id string) error { return t.Manager.Redraw(id) }
