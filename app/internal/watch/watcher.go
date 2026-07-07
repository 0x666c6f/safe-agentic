package watch

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/0x666c6f/berth/app/internal/emit"
	"github.com/0x666c6f/berth/pkg/events"
)

type Watcher struct {
	path     string
	em       emit.Emitter
	interval time.Duration
	Notify   func(events.SystemNotification) error

	mu      sync.Mutex
	offset  int64
	stop    chan struct{}
	once    sync.Once
	done    chan struct{}
	started bool
}

func NewWatcher(path string, em emit.Emitter, interval time.Duration) *Watcher {
	return &Watcher{path: path, em: em, interval: interval, Notify: events.NotifySystem,
		stop: make(chan struct{}), done: make(chan struct{})}
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
		_ = fsw.Add(filepath.Dir(w.path))
		evCh, errCh = fsw.Events, fsw.Errors
	}
	w.mu.Lock()
	w.started = true
	w.mu.Unlock()
	go func() {
		defer close(w.done)
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

// Stop signals the watch loop to exit and blocks until it has actually
// returned (no-op if Start was never called, since the goroutine — and thus
// done — never existed).
func (w *Watcher) Stop() {
	w.once.Do(func() { close(w.stop) })
	w.mu.Lock()
	started := w.started
	w.mu.Unlock()
	if started {
		<-w.done
	}
}

func (w *Watcher) drain() {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := os.Open(w.path)
	if err != nil {
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return
	}
	if fi.Size() < w.offset {
		w.offset = 0 // truncated/rotated: re-read from start
	}
	if fi.Size() <= w.offset {
		return
	}
	if _, err := f.Seek(w.offset, 0); err != nil {
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return
	}
	// Only consume COMPLETE lines (terminated by '\n'). Any trailing partial
	// line is left unconsumed — w.offset stays put — so a later drain() that
	// sees the completing newline re-reads it from the same starting point
	// instead of double counting or overshooting by the missing newline byte.
	start := 0
	for {
		idx := bytes.IndexByte(data[start:], '\n')
		if idx < 0 {
			break
		}
		line := data[start : start+idx]
		consumed := idx + 1
		w.offset += int64(consumed)
		start += consumed

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
