package detonate

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0x666c6f/berth/pkg/config"
)

// StateDir is the directory holding one JSON file per run. DETONATE_STATE_DIR
// overrides it entirely (tests point it at a temp dir); otherwise it's
// ~/.berth/detonate, alongside berth's other per-user state.
func StateDir() string {
	if dir := os.Getenv("DETONATE_STATE_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(config.UserDir(), "detonate")
}

// Run is a detonation run's persisted state — the record that lets every
// verb invocation, even in a fresh process, know whether this run has
// already been detonated. Without this on disk, "no reuse" is only a
// comment; with it, CanTransition can actually be enforced across
// invocations.
type Run struct {
	Name    string `json:"name"`
	Golden  string `json:"golden"`
	Gateway string `json:"gateway,omitempty"`
	State   State  `json:"state"`
	CloneID string `json:"clone_id,omitempty"`
	// Nonce is a random token minted once at create. It lets a verb that
	// captured the nonce at load time detect that the run it started acting on
	// was destroyed and recreated out from under it (a lockless `destroy`
	// followed by a fresh `create` reuses the name but mints a new nonce). The
	// *If Nonce store ops refuse to overwrite or delete state whose on-disk
	// nonce no longer matches — so a stale invocation can never corrupt the
	// brand-new run that took its place.
	Nonce     string    `json:"nonce,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewNonce mints a random per-run token (see Run.Nonce). crypto/rand failing
// is near-impossible on the supported platforms; if it ever does, fall back to
// a high-resolution timestamp so create still yields a distinct-per-run value
// rather than a shared empty string that would defeat the guard.
func NewNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func runStatePath(name string) string {
	return filepath.Join(StateDir(), name+".json")
}

func lockPath(name string) string {
	return filepath.Join(StateDir(), name+".lock")
}

// LockRun acquires an exclusive, advisory lock for a run name, closing the
// TOCTOU a plain Load-then-Save can't: without it, two concurrent
// invocations against the same run (e.g. two `run` calls fired close
// together) could each load state=Injected and both reach the runner's Run
// before either persists Detonated. Callers should hold the lock for the
// whole check-then-act section (load, decide, side effect, save).
//
// ponytail: stale-lock ceiling — if the holding process is killed before
// calling unlock, the lock file is left behind and every later verb on this
// run name fails with "locked" until an operator removes it by hand.
// Acceptable for a manually-operated CLI; add a PID+liveness check if this
// ever runs unattended.
func LockRun(name string) (unlock func(), err error) {
	if err := os.MkdirAll(StateDir(), 0o700); err != nil {
		return nil, fmt.Errorf("creating state dir: %w", err)
	}
	path := lockPath(name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("run %q is locked by another in-progress operation (remove %s if you're sure nothing is running)", name, path)
		}
		return nil, fmt.Errorf("locking run %q: %w", name, err)
	}
	f.Close()
	return func() { os.Remove(path) }, nil
}

// LoadRun reads a run's persisted state. The error is os.IsNotExist-checkable
// when no such run has ever been created.
func LoadRun(name string) (*Run, error) {
	data, err := os.ReadFile(runStatePath(name))
	if err != nil {
		return nil, err
	}
	var r Run
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing state for run %q: %w", name, err)
	}
	return &r, nil
}

// SaveRun persists a run's state, creating the state dir on first use.
// UpdatedAt is stamped here; CreatedAt is the caller's responsibility (set
// once, on the Created save, and carried forward on every later save since
// callers load-modify-save the same struct).
func SaveRun(r *Run) error {
	r.UpdatedAt = time.Now()
	if err := os.MkdirAll(StateDir(), 0o700); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state for run %q: %w", r.Name, err)
	}
	if err := os.WriteFile(runStatePath(r.Name), data, 0o600); err != nil {
		return fmt.Errorf("writing state for run %q: %w", r.Name, err)
	}
	return nil
}

// DeleteRun removes a run's persisted state and any leftover lock file.
// Deleting a run with no state file is not an error: destroy is idempotent,
// and clearing state is the only way to make a run name available for a
// fresh create. Clearing the lock file too matters for the same reason: a
// stale lock left behind by a killed process must not survive a destroy and
// keep blocking every later verb on this run name.
func DeleteRun(name string) error {
	err := os.Remove(runStatePath(name))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting state for run %q: %w", name, err)
	}
	if err := os.Remove(lockPath(name)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting lock for run %q: %w", name, err)
	}
	return nil
}

// SaveRunIfNonce persists r only if the on-disk state still carries the nonce
// the caller loaded (expectedNonce). It re-reads state right before writing:
// if the run was destroyed (state gone) or destroyed-and-recreated (nonce
// changed) since the caller loaded it, the save is refused rather than
// resurrecting a dead run or clobbering the brand-new one that reused the name.
func SaveRunIfNonce(r *Run, expectedNonce string) error {
	cur, err := LoadRun(r.Name)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("run %q was destroyed concurrently (nonce %q gone); not recreating its state", r.Name, expectedNonce)
		}
		return fmt.Errorf("re-reading state for run %q before save: %w", r.Name, err)
	}
	if cur.Nonce != expectedNonce {
		return fmt.Errorf("run %q was recreated concurrently (nonce mismatch: loaded %q, on-disk %q); not touching the new run", r.Name, expectedNonce, cur.Nonce)
	}
	return SaveRun(r)
}

// DeleteRunIfNonce clears a run's state only if the on-disk nonce still matches
// expectedNonce. A stale invocation (e.g. a `run` whose boot finally errored
// long after an operator destroyed and recreated the run) must not delete the
// new run's state. If the run is already gone it's a no-op success — there is
// nothing left to protect — but any leftover lock file is still cleared.
func DeleteRunIfNonce(name, expectedNonce string) error {
	cur, err := LoadRun(name)
	if err != nil {
		if os.IsNotExist(err) {
			if lerr := os.Remove(lockPath(name)); lerr != nil && !os.IsNotExist(lerr) {
				return fmt.Errorf("clearing lock for run %q: %w", name, lerr)
			}
			return nil
		}
		return fmt.Errorf("re-reading state for run %q before delete: %w", name, err)
	}
	if cur.Nonce != expectedNonce {
		return fmt.Errorf("run %q was recreated concurrently (nonce mismatch: loaded %q, on-disk %q); not deleting the new run's state", name, expectedNonce, cur.Nonce)
	}
	return DeleteRun(name)
}
