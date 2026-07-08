package detonate

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadRun_NotExistIsCheckable(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())
	_, err := LoadRun("no-such-run")
	if err == nil {
		t.Fatal("LoadRun() error = nil, want an error for a run that was never created")
	}
	if !os.IsNotExist(err) {
		t.Errorf("LoadRun() error = %v, want an os.IsNotExist error", err)
	}
}

func TestSaveRun_RoundTrips(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())
	now := time.Now().Truncate(time.Second)
	r := &Run{Name: "run-1", Golden: "golden-1", State: StateCreated, CreatedAt: now}
	if err := SaveRun(r); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	got, err := LoadRun("run-1")
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if got.Name != "run-1" || got.Golden != "golden-1" || got.State != StateCreated {
		t.Errorf("LoadRun() = %+v, want name/golden/state to round-trip", got)
	}
	if !got.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, now)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt not stamped by SaveRun")
	}
}

func TestSaveRun_PreservesCreatedAtAcrossUpdates(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())
	created := time.Now().Add(-time.Hour).Truncate(time.Second)
	r := &Run{Name: "run-1", State: StateCreated, CreatedAt: created}
	if err := SaveRun(r); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	loaded, err := LoadRun("run-1")
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	loaded.State = StateInjected
	if err := SaveRun(loaded); err != nil {
		t.Fatalf("SaveRun() (advance) error = %v", err)
	}

	got, err := LoadRun("run-1")
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if got.State != StateInjected {
		t.Errorf("State = %v, want Injected", got.State)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want unchanged %v", got.CreatedAt, created)
	}
}

func TestNewNonce_UniqueNonEmpty(t *testing.T) {
	a, b := NewNonce(), NewNonce()
	if a == "" || b == "" {
		t.Fatalf("NewNonce() returned empty: %q %q", a, b)
	}
	if a == b {
		t.Errorf("NewNonce() not unique across calls: %q == %q", a, b)
	}
}

// TestSaveRunIfNonce_AbortsWhenRecreated is the Fix-2 guard on the save path:
// after a caller loads a run (capturing its nonce), an out-of-band
// destroy+recreate mints a new nonce; the stale caller's SaveRunIfNonce must
// refuse rather than overwrite the brand-new run's state.
func TestSaveRunIfNonce_AbortsWhenRecreated(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())
	if err := SaveRun(&Run{Name: "run-1", State: StateCreated, Nonce: "nonce-A"}); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}
	loaded, err := LoadRun("run-1") // stale caller captures nonce-A
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}

	// Out-of-band: destroy and recreate the same name with a fresh nonce.
	if err := DeleteRun("run-1"); err != nil {
		t.Fatalf("DeleteRun() error = %v", err)
	}
	if err := SaveRun(&Run{Name: "run-1", State: StateCreated, Nonce: "nonce-B"}); err != nil {
		t.Fatalf("recreate SaveRun() error = %v", err)
	}

	loaded.State = StateInjected // the stale caller tries to advance state
	err = SaveRunIfNonce(loaded, loaded.Nonce)
	if err == nil {
		t.Fatal("SaveRunIfNonce() error = nil, want a nonce-mismatch abort")
	}
	if !strings.Contains(err.Error(), "nonce mismatch") {
		t.Errorf("SaveRunIfNonce() error = %v, want it to name the nonce mismatch", err)
	}

	// The recreated run must be untouched.
	got, err := LoadRun("run-1")
	if err != nil {
		t.Fatalf("LoadRun() after aborted save error = %v", err)
	}
	if got.Nonce != "nonce-B" || got.State != StateCreated {
		t.Errorf("recreated run was overwritten: state=%s nonce=%q, want Created/nonce-B", got.State, got.Nonce)
	}
}

// TestSaveRunIfNonce_AbortsWhenDestroyed covers the recreate's earlier half:
// if the run was destroyed and NOT yet recreated, a stale save must not
// resurrect a dead run's state.
func TestSaveRunIfNonce_AbortsWhenDestroyed(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())
	if err := SaveRun(&Run{Name: "run-1", State: StateCreated, Nonce: "nonce-A"}); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}
	loaded, err := LoadRun("run-1")
	if err != nil {
		t.Fatalf("LoadRun() error = %v", err)
	}
	if err := DeleteRun("run-1"); err != nil {
		t.Fatalf("DeleteRun() error = %v", err)
	}
	loaded.State = StateInjected
	if err := SaveRunIfNonce(loaded, loaded.Nonce); err == nil {
		t.Fatal("SaveRunIfNonce() error = nil, want abort on a destroyed run")
	}
	if _, err := LoadRun("run-1"); !os.IsNotExist(err) {
		t.Errorf("LoadRun() after aborted save = %v, want os.IsNotExist (state not resurrected)", err)
	}
}

// TestDeleteRunIfNonce_AbortsWhenRecreated is the Fix-2 guard on the delete
// path: a stale invocation (nonce-A) must not wipe the state of a run that was
// destroyed and recreated (nonce-B) since it loaded.
func TestDeleteRunIfNonce_AbortsWhenRecreated(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())
	if err := SaveRun(&Run{Name: "run-1", State: StateDetonated, Nonce: "nonce-A"}); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}
	staleNonce := "nonce-A" // captured at the start of the stale invocation

	if err := DeleteRun("run-1"); err != nil {
		t.Fatalf("DeleteRun() error = %v", err)
	}
	if err := SaveRun(&Run{Name: "run-1", State: StateCreated, Nonce: "nonce-B"}); err != nil {
		t.Fatalf("recreate SaveRun() error = %v", err)
	}

	err := DeleteRunIfNonce("run-1", staleNonce)
	if err == nil {
		t.Fatal("DeleteRunIfNonce() error = nil, want a nonce-mismatch abort")
	}
	if !strings.Contains(err.Error(), "nonce mismatch") {
		t.Errorf("DeleteRunIfNonce() error = %v, want it to name the nonce mismatch", err)
	}

	got, err := LoadRun("run-1")
	if err != nil {
		t.Fatalf("recreated run's state was wiped: %v", err)
	}
	if got.Nonce != "nonce-B" {
		t.Errorf("wrong run left on disk: nonce=%q, want nonce-B", got.Nonce)
	}
}

// TestDeleteRunIfNonce_MatchDeletes confirms the guard doesn't over-block: a
// matching nonce (or a legacy/ghost run with an empty nonce) still clears.
func TestDeleteRunIfNonce_MatchDeletes(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())
	if err := SaveRun(&Run{Name: "run-1", State: StateCreated, Nonce: "nonce-A"}); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}
	if err := DeleteRunIfNonce("run-1", "nonce-A"); err != nil {
		t.Fatalf("DeleteRunIfNonce() error = %v, want nil on a matching nonce", err)
	}
	if _, err := LoadRun("run-1"); !os.IsNotExist(err) {
		t.Errorf("LoadRun() after matching delete = %v, want os.IsNotExist", err)
	}

	// A run that's already gone is a no-op success (nothing to protect).
	if err := DeleteRunIfNonce("ghost", ""); err != nil {
		t.Errorf("DeleteRunIfNonce() on a ghost run = %v, want nil", err)
	}
}

func TestLockRun_SecondAcquireFailsUntilUnlocked(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())

	unlock, err := LockRun("run-1")
	if err != nil {
		t.Fatalf("LockRun() error = %v", err)
	}

	if _, err := LockRun("run-1"); err == nil {
		t.Fatal("second LockRun() error = nil, want error while the first lock is held")
	}

	unlock()

	unlock2, err := LockRun("run-1")
	if err != nil {
		t.Fatalf("LockRun() after unlock error = %v, want nil", err)
	}
	unlock2()
}

func TestLockRun_DifferentRunsDoNotContend(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())

	unlockA, err := LockRun("run-a")
	if err != nil {
		t.Fatalf("LockRun(run-a) error = %v", err)
	}
	defer unlockA()

	unlockB, err := LockRun("run-b")
	if err != nil {
		t.Fatalf("LockRun(run-b) error = %v, want nil (different run names must not contend)", err)
	}
	unlockB()
}

func TestDeleteRun_IdempotentAndClearsState(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())

	// Deleting a run that was never created is not an error.
	if err := DeleteRun("ghost"); err != nil {
		t.Fatalf("DeleteRun() on nonexistent run error = %v, want nil", err)
	}

	if err := SaveRun(&Run{Name: "run-1", State: StateCreated}); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}
	if err := DeleteRun("run-1"); err != nil {
		t.Fatalf("DeleteRun() error = %v", err)
	}
	if _, err := LoadRun("run-1"); !os.IsNotExist(err) {
		t.Errorf("LoadRun() after DeleteRun error = %v, want os.IsNotExist", err)
	}

	// Idempotent: deleting again is still not an error.
	if err := DeleteRun("run-1"); err != nil {
		t.Fatalf("second DeleteRun() error = %v, want nil (idempotent)", err)
	}
}

// TestDeleteRun_ClearsLockFile guards against a stale lock (left behind by a
// killed process) surviving a destroy and permanently blocking every later
// verb on that run name — destroy must be a full reset, not just of state.
func TestDeleteRun_ClearsLockFile(t *testing.T) {
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())

	unlock, err := LockRun("run-1")
	if err != nil {
		t.Fatalf("LockRun() error = %v", err)
	}
	unlock() // simulate a clean unlock; the file is gone

	// Re-acquire and simulate a crash: leak the lock file by not unlocking.
	if _, err := LockRun("run-1"); err != nil {
		t.Fatalf("LockRun() error = %v", err)
	}

	if err := DeleteRun("run-1"); err != nil {
		t.Fatalf("DeleteRun() error = %v", err)
	}

	if _, err := LockRun("run-1"); err != nil {
		t.Fatalf("LockRun() after DeleteRun error = %v, want nil (stale lock must be cleared)", err)
	}
}
