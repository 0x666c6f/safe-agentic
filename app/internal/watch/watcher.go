package watch

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/0x666c6f/safe-agentic/app/internal/emit"
	"github.com/0x666c6f/safe-agentic/pkg/events"
)

type Watcher struct {
	path     string
	em       emit.Emitter
	interval time.Duration
	Notify   func(events.SystemNotification) error

	mu     sync.Mutex
	offset int64
	stop   chan struct{}
	once   sync.Once
}

func NewWatcher(path string, em emit.Emitter, interval time.Duration) *Watcher {
	return &Watcher{path: path, em: em, interval: interval, Notify: events.NotifySystem,
		stop: make(chan struct{})}
}

func (w *Watcher) Start() error {
	// Start at current EOF: only NEW events notify (no replay storm on app start).
	if fi, err := os.Stat(w.path); err == nil {
		w.offset = fi.Size()
	}
	// nil channels block forever in select — safe fallback to ticker-only mode
	// when fsnotify is unavailable.
	var evCh chan fsnotify.Event
	var errCh chan error
	fsw, err := fsnotify.NewWatcher()
	if err == nil {
		// Watch the file's directory: JSONL appends fire Write events on most volumes.
		_ = fsw.Add(dirOf(w.path))
		evCh, errCh = fsw.Events, fsw.Errors
	}
	go func() {
		t := time.NewTicker(w.interval)
		defer t.Stop()
		if fsw != nil {
			defer fsw.Close()
		}
		for {
			select {
			case <-w.stop:
				return
			case <-t.C:
				w.drain()
			case ev, ok := <-evCh:
				if ok && ev.Name == w.path {
					w.drain()
				}
			case <-errCh:
			}
		}
	}()
	return nil
}

func dirOf(p string) string {
	if i := len(p) - 1; i > 0 {
		for ; i >= 0; i-- {
			if p[i] == '/' {
				return p[:i]
			}
		}
	}
	return "."
}

func (w *Watcher) Stop() { w.once.Do(func() { close(w.stop) }) }

func (w *Watcher) drain() {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := os.Open(w.path)
	if err != nil {
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.Size() <= w.offset {
		if err == nil && fi.Size() < w.offset {
			w.offset = 0 // truncated/rotated: re-read from start
		} else {
			return
		}
	}
	if _, err := f.Seek(w.offset, 0); err != nil {
		return
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		w.offset += int64(len(line)) + 1
		var e events.Event
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		status := events.ClassifyFields(e.Type, e.Payload)
		w.em.Emit("event.new", map[string]any{"event": e, "status": status})
		if events.NeedsAttentionStatus(status) && w.Notify != nil {
			msg := e.Payload["message"]
			if msg == "" {
				msg = e.Type
			}
			w.Notify(events.SystemNotification{
				Container: e.Payload["container"],
				Message:   msg,
				Sound:     events.SoundForStatus(status),
			})
		}
	}
}
