package main

import (
	"testing"

	"github.com/0x666c6f/safe-agentic/pkg/events"
)

func newTestNotifier() *stateNotifier {
	return &stateNotifier{
		last:     make(map[string]string),
		enabled:  true,
		dispatch: func(events.SystemNotification) error { return nil },
	}
}

// The first observation of any container seeds its state silently, so neither
// startup nor a freshly spawned agent produces a burst of notifications.
func TestNotifierSeedsFirstObservationSilently(t *testing.T) {
	n := newTestNotifier()
	got := n.transitions([]Agent{
		{Name: "a", State: "blocked"},
		{Name: "b", State: "working"},
		{Name: "c", State: "done"},
	})
	if len(got) != 0 {
		t.Fatalf("first observation fired %d notifications, want 0", len(got))
	}
}

// A transition into blocked/done/exited fires exactly once and is debounced on
// repeat observations of the same state.
func TestNotifierFiresOncePerTransition(t *testing.T) {
	n := newTestNotifier()
	n.transitions([]Agent{{Name: "a", State: "working"}}) // seed

	got := n.transitions([]Agent{{Name: "a", State: "blocked", StateReason: "permission prompt"}})
	if len(got) != 1 {
		t.Fatalf("into-blocked fired %d, want 1", len(got))
	}
	if got[0].Container != "a" || got[0].Sound != events.SoundAttention {
		t.Fatalf("blocked notification = %+v", got[0])
	}

	// Same state again → debounced.
	if got := n.transitions([]Agent{{Name: "a", State: "blocked"}}); len(got) != 0 {
		t.Fatalf("repeat blocked fired %d, want 0 (debounced)", len(got))
	}
}

// done and exited fire with their respective sounds; working/idle never fire.
func TestNotifierDoneExitedAndPassiveStates(t *testing.T) {
	n := newTestNotifier()
	n.transitions([]Agent{{Name: "d", State: "working"}, {Name: "e", State: "working"}, {Name: "p", State: "working"}})

	got := n.transitions([]Agent{
		{Name: "d", State: "done"},
		{Name: "e", State: "exited", StateReason: "code 137"},
		{Name: "p", State: "idle"}, // passive → no notification
	})
	byContainer := map[string]events.SystemNotification{}
	for _, note := range got {
		byContainer[note.Container] = note
	}
	if len(got) != 2 {
		t.Fatalf("fired %d, want 2 (done, exited): %+v", len(got), got)
	}
	if byContainer["d"].Sound != events.SoundSuccess {
		t.Fatalf("done sound = %q, want success", byContainer["d"].Sound)
	}
	if byContainer["e"].Sound != events.SoundAttention {
		t.Fatalf("exited sound = %q, want attention", byContainer["e"].Sound)
	}
}

// A container first observed as "unknown" is seeded silently, but its FIRST
// real state is not a blip: unknown→blocked on that container must fire.
func TestNotifierFirstUnknownThenBlockedFires(t *testing.T) {
	n := newTestNotifier()
	n.transitions([]Agent{{Name: "a", State: "unknown"}}) // first sighting, seeded silently
	got := n.transitions([]Agent{{Name: "a", State: "blocked", StateReason: "login"}})
	if len(got) != 1 {
		t.Fatalf("first unknown→blocked fired %d, want 1", len(got))
	}
	if got[0].Container != "a" || got[0].Sound != events.SoundAttention {
		t.Fatalf("blocked notification = %+v", got[0])
	}
}

// A transient "unknown" between two identical passive states fires nothing and
// preserves the last meaningful state.
func TestNotifierKnownWorkingUnknownWorkingSilent(t *testing.T) {
	n := newTestNotifier()
	n.transitions([]Agent{{Name: "a", State: "working"}}) // seed
	if got := n.transitions([]Agent{{Name: "a", State: "unknown"}}); len(got) != 0 {
		t.Fatalf("working→unknown fired %d, want 0", len(got))
	}
	if got := n.transitions([]Agent{{Name: "a", State: "working"}}); len(got) != 0 {
		t.Fatalf("unknown→working fired %d, want 0", len(got))
	}
}

// A known blocked agent that blips to unknown and back must not re-notify.
func TestNotifierKnownBlockedUnknownBlockedNoRefire(t *testing.T) {
	n := newTestNotifier()
	n.transitions([]Agent{{Name: "a", State: "working"}}) // seed
	if got := n.transitions([]Agent{{Name: "a", State: "blocked"}}); len(got) != 1 {
		t.Fatalf("into-blocked want 1, got %d", len(got))
	}
	n.transitions([]Agent{{Name: "a", State: "unknown"}}) // blip
	if got := n.transitions([]Agent{{Name: "a", State: "blocked"}}); len(got) != 0 {
		t.Fatalf("blocked after unknown blip re-fired %d, want 0", len(got))
	}
}

// A transient "unknown" reading must not re-arm an already-fired transition.
func TestNotifierUnknownDoesNotRearm(t *testing.T) {
	n := newTestNotifier()
	n.transitions([]Agent{{Name: "a", State: "working"}}) // seed
	if got := n.transitions([]Agent{{Name: "a", State: "blocked"}}); len(got) != 1 {
		t.Fatalf("into-blocked want 1, got %d", len(got))
	}
	n.transitions([]Agent{{Name: "a", State: "unknown"}}) // blip, ignored
	if got := n.transitions([]Agent{{Name: "a", State: "blocked"}}); len(got) != 0 {
		t.Fatalf("blocked after unknown blip fired %d, want 0", len(got))
	}
}

// A container that disappears and returns re-seeds instead of firing.
func TestNotifierPrunesVanishedContainers(t *testing.T) {
	n := newTestNotifier()
	n.transitions([]Agent{{Name: "a", State: "working"}})
	n.transitions(nil) // "a" gone → pruned
	if got := n.transitions([]Agent{{Name: "a", State: "blocked"}}); len(got) != 0 {
		t.Fatalf("returned container should re-seed, fired %d", len(got))
	}
}

func TestNotifierDisabledFiresNothing(t *testing.T) {
	n := newTestNotifier()
	n.enabled = false
	n.transitions([]Agent{{Name: "a", State: "working"}})
	if got := n.transitions([]Agent{{Name: "a", State: "blocked"}}); got != nil {
		t.Fatalf("disabled notifier fired %+v", got)
	}
}

func TestNotifyEnabledEnvToggle(t *testing.T) {
	for _, off := range []string{"off", "0", "false", "no", "OFF"} {
		t.Setenv("SAFE_AG_TUI_NOTIFY", off)
		if notifyEnabled() {
			t.Fatalf("SAFE_AG_TUI_NOTIFY=%q should disable", off)
		}
	}
	t.Setenv("SAFE_AG_TUI_NOTIFY", "")
	if !notifyEnabled() {
		t.Fatal("unset SAFE_AG_TUI_NOTIFY should default to enabled")
	}
}
