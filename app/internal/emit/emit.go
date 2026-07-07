package emit

import "sync"

type Emitter interface {
	Emit(name string, data any)
}

type Recorded struct {
	Name string
	Data any
}

type Recorder struct {
	mu     sync.Mutex
	Events []Recorded
}

func (r *Recorder) Emit(name string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Events = append(r.Events, Recorded{Name: name, Data: data})
}

func (r *Recorder) Named(name string) []Recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Recorded
	for _, e := range r.Events {
		if e.Name == name {
			out = append(out, e)
		}
	}
	return out
}
