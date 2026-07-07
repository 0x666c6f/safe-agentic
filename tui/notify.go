package main

import (
	"os"
	"strings"
	"sync"

	"github.com/0x666c6f/berth/pkg/agentstate"
	"github.com/0x666c6f/berth/pkg/events"
)

// stateNotifier fires a native desktop notification when an agent transitions
// INTO a state that wants the operator's attention (blocked) or has finished
// (done / exited). It debounces per container: the first time a container is
// seen its state is recorded silently, so neither TUI startup nor a freshly
// spawned agent produces a burst; only genuine state changes notify.
//
// events.NotifySystem is a no-op off darwin, so this is safe everywhere.
type stateNotifier struct {
	mu       sync.Mutex
	last     map[string]string
	enabled  bool
	dispatch func(events.SystemNotification) error
}

func newStateNotifier() *stateNotifier {
	return &stateNotifier{
		last:     make(map[string]string),
		enabled:  notifyEnabled(),
		dispatch: events.NotifySystem,
	}
}

// notifyEnabled reports whether transition notifications are on. They default to
// on; set BERTH_TUI_NOTIFY=off (or 0/false/no) to silence them.
func notifyEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BERTH_TUI_NOTIFY"))) {
	case "off", "0", "false", "no":
		return false
	default:
		return true
	}
}

// observe records the latest states and dispatches a notification for every
// transition into a notify-worthy state. Dispatch is asynchronous so a slow
// notifier binary never stalls the poll loop. Called from the poller goroutine
// (off the tview event loop).
func (n *stateNotifier) observe(agents []Agent) {
	for _, note := range n.transitions(agents) {
		note := note
		go n.dispatch(note)
	}
}

// transitions updates the per-container state map and returns the notifications
// to fire. It is the synchronous, deterministic core of observe: the first time
// a container is seen its state is recorded silently (seeding), and only genuine
// changes into blocked/done/exited produce a notification. Returns nil when
// notifications are disabled.
func (n *stateNotifier) transitions(agents []Agent) []events.SystemNotification {
	if !n.enabled {
		return nil
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	var fire []events.SystemNotification
	seen := make(map[string]bool, len(agents))
	for _, a := range agents {
		seen[a.Name] = true
		cur := a.State
		prev, known := n.last[a.Name]
		if !known {
			// First sighting: seed silently, even when the reading is ""/unknown.
			// Recording it here is what lets a later unknown→blocked fire — a
			// genuine first blocked state must not be mistaken for a seed.
			n.last[a.Name] = cur
			continue
		}
		// Already known: treat "no detection" as no new information. Keep the
		// last meaningful state (don't overwrite) so a transient blip cannot
		// re-arm an already-fired transition.
		if cur == "" || cur == string(agentstate.StateUnknown) {
			continue
		}
		n.last[a.Name] = cur
		if prev == cur {
			continue // no change
		}
		if note, ok := transitionNotification(a); ok {
			fire = append(fire, note)
		}
	}
	// Forget vanished containers so a reused name re-seeds instead of firing.
	for name := range n.last {
		if !seen[name] {
			delete(n.last, name)
		}
	}
	return fire
}

// transitionNotification builds the notification for a state worth surfacing, or
// reports ok=false for passive states (working/idle) that never notify.
func transitionNotification(a Agent) (events.SystemNotification, bool) {
	var status, msg string
	switch a.State {
	case string(agentstate.StateBlocked):
		status, msg = events.StatusBlocked, "Blocked — needs your input"
	case string(agentstate.StateDone):
		status, msg = "done", "Finished"
	case string(agentstate.StateExited):
		status, msg = events.StatusFailed, "Exited with error"
	default:
		return events.SystemNotification{}, false
	}
	if a.StateReason != "" {
		msg += " (" + a.StateReason + ")"
	}
	return events.SystemNotification{
		Container: a.Name,
		Message:   msg,
		Sound:     events.SoundForStatus(status),
	}, true
}
