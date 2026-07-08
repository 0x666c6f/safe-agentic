package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0x666c6f/berth/pkg/detonate"
)

// Known guest-dropped artifact bytes. The mock `tart run` writes these into
// the --dir=out: share, standing in for a detonation that dropped results;
// runCollect must copy+hash exactly these.
var (
	e2eReportBytes = []byte("detonate e2e report: sample executed, no egress observed\n")
	e2ePcapBytes   = []byte("\xd4\xc3\xb2\xa1fake-pcap-capture-bytes")
)

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// mockTart is a CmdRunner that simulates the tart/hdiutil processes for a full
// detonation lifecycle. It never shells out: it tracks simulated VM state and
// records every invocation so the test can assert the exact `tart run` argv the
// real TartRunner built.
type mockTart struct {
	vms   map[string]string // name -> State ("stopped"); absent = VM does not exist
	calls [][]string
}

func newMockTart(golden string) *mockTart {
	return &mockTart{vms: map[string]string{golden: "stopped"}}
}

// tartRunCall returns the recorded `tart run ...` argv, or nil if none.
func (m *mockTart) tartRunCall() []string {
	for _, c := range m.calls {
		if len(c) > 1 && c[0] == "tart" && c[1] == "run" {
			return c
		}
	}
	return nil
}

func (m *mockTart) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, append([]string{name}, args...))

	switch {
	case name == "tart" && len(args) > 0 && args[0] == "list":
		// Shape must match parseTartList's tartListEntry{Name,State}.
		type entry struct {
			Name  string `json:"Name"`
			State string `json:"State"`
		}
		var entries []entry
		for n, s := range m.vms {
			entries = append(entries, entry{Name: n, State: s})
		}
		out, err := json.Marshal(entries)
		return out, err

	case name == "tart" && len(args) > 0 && args[0] == "clone":
		// args: clone <golden> <run> — the clone boots powered off.
		if len(args) >= 3 {
			m.vms[args[2]] = "stopped"
		}
		return nil, nil

	case name == "tart" && len(args) > 0 && args[0] == "run":
		// Detonation: drop fake artifacts into the guest's --dir=out: share,
		// then "power off" (state stays stopped) so collect's PoweredOff passes.
		for _, a := range args {
			if dir, ok := strings.CutPrefix(a, "--dir=out:"); ok {
				_ = os.WriteFile(filepath.Join(dir, "report.txt"), e2eReportBytes, 0o600)
				_ = os.WriteFile(filepath.Join(dir, "capture.pcap"), e2ePcapBytes, 0o600)
			}
		}
		return nil, nil

	case name == "tart" && len(args) > 0 && args[0] == "delete":
		// args: delete <run>
		if len(args) >= 2 {
			delete(m.vms, args[1])
		}
		return nil, nil

	case name == "hdiutil":
		// InjectOffline stages the sample for real; only the image build is faked.
		return nil, nil
	}
	return nil, nil
}

// TestDetonate_EndToEnd_MockTart drives the FULL detonation lifecycle through
// the real CLI verb handlers and the real detonate.TartRunner, faking only the
// tart/hdiutil process exec. It proves the whole orchestration — arg
// construction, output parsing, the isolation guard, the state store, the
// chain-of-custody audit, and symlink-safe collect — end to end, portably (no
// real tart/hdiutil, passes on linux and darwin).
func TestDetonate_EndToEnd_MockTart(t *testing.T) {
	setTempAuditPath(t) // temp DETONATE_STATE_DIR + temp audit path
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	const (
		golden  = "golden-e2e"
		run     = "e2e-run"
		gateway = "10.0.0.0/24"
	)
	ctx := context.Background()

	mock := newMockTart(golden)
	r := detonate.NewTartRunner(t.TempDir())
	r.AllowedIsolatedGateway = gateway // exercise the operator pin too
	r.SetCmdRunner(mock)

	requireState := func(want detonate.State) {
		t.Helper()
		st, err := detonate.LoadRun(run)
		if err != nil {
			t.Fatalf("LoadRun(%q): %v", run, err)
		}
		if st.State != want {
			t.Fatalf("state = %s, want %s", st.State, want)
		}
	}

	// ── 1. create (golden present) → Created ──────────────────────────────
	if err := runCreate(ctx, r, run, golden); err != nil {
		t.Fatalf("runCreate: %v", err)
	}
	requireState(detonate.StateCreated)

	// ── 2. inject: hash + audit the sample, InjectOffline records image → Injected
	sample := filepath.Join(t.TempDir(), "malware.bin")
	sampleBytes := []byte("totally-not-malware payload")
	if err := os.WriteFile(sample, sampleBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runInject(ctx, r, run, sample); err != nil {
		t.Fatalf("runInject: %v", err)
	}
	requireState(detonate.StateInjected)

	injEntries := auditEntriesByAction(t, "detonate-inject")
	if len(injEntries) != 1 || injEntries[0].Details["sha256"] != sha256Hex(sampleBytes) {
		t.Fatalf("detonate-inject audit = %+v, want one entry with sha256 %s", injEntries, sha256Hex(sampleBytes))
	}

	// ── 3. run: passes the isolation guard, invokes mocked `tart run` with the
	//        softnet allow-list + read-only sample disk → Detonated ──────────
	if err := runRun(ctx, r, run, gateway, time.Second, true, strings.NewReader("")); err != nil {
		t.Fatalf("runRun: %v", err)
	}
	requireState(detonate.StateDetonated)

	runCall := mock.tartRunCall()
	if runCall == nil {
		t.Fatalf("expected a `tart run` invocation, calls: %v", mock.calls)
	}
	assertArg := func(want string) {
		t.Helper()
		for _, a := range runCall {
			if a == want {
				return
			}
		}
		t.Fatalf("tart run args missing %q, got %v", want, runCall)
	}
	assertArg("--net-softnet")
	assertArg("--net-softnet-allow=" + gateway)
	foundRODisk := false
	for _, a := range runCall {
		if strings.HasPrefix(a, "--net-bridged") {
			t.Fatalf("tart run emitted forbidden bridged networking: %v", runCall)
		}
		if strings.HasPrefix(a, "--disk=") && strings.HasSuffix(a, ":ro") {
			foundRODisk = true
		}
	}
	if !foundRODisk {
		t.Fatalf("tart run args missing a read-only --disk=...:ro sample attach, got %v", runCall)
	}

	// ── 5. no-reuse: a second run on the same (Detonated, not-destroyed) run
	//        FAILS closed, without invoking `tart run` again ────────────────
	callsBefore := len(mock.calls)
	err := runRun(ctx, r, run, gateway, time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("second runRun = nil, want fail-closed no-reuse error")
	}
	if !strings.Contains(err.Error(), "already detonated") {
		t.Errorf("second runRun error = %v, want it to say already detonated", err)
	}
	if len(mock.calls) != callsBefore {
		t.Errorf("no-reuse path touched the runner: new calls %v", mock.calls[callsBefore:])
	}
	requireState(detonate.StateDetonated) // blocked reuse must not perturb state

	// ── 4. collect: finds the guest-dropped artifacts, copies + hashes them,
	//        and audits their sha256s → Collected ───────────────────────────
	outDir := t.TempDir()
	if err := runCollect(ctx, r, run, outDir); err != nil {
		t.Fatalf("runCollect: %v", err)
	}
	requireState(detonate.StateCollected)

	for name, want := range map[string][]byte{"report.txt": e2eReportBytes, "capture.pcap": e2ePcapBytes} {
		got, readErr := os.ReadFile(filepath.Join(outDir, name))
		if readErr != nil {
			t.Fatalf("collected %s: %v", name, readErr)
		}
		if string(got) != string(want) {
			t.Errorf("collected %s bytes mismatch", name)
		}
	}
	colEntries := auditEntriesByAction(t, "detonate-collect")
	if len(colEntries) != 1 {
		t.Fatalf("detonate-collect audit = %+v, want exactly one entry", colEntries)
	}
	if colEntries[0].Details["count"] != "2" {
		t.Errorf("collect audit count = %q, want 2", colEntries[0].Details["count"])
	}
	for _, h := range []string{sha256Hex(e2eReportBytes), sha256Hex(e2ePcapBytes)} {
		if !strings.Contains(colEntries[0].Details["artifacts"], h) {
			t.Errorf("collect audit artifacts missing sha256 %s: %q", h, colEntries[0].Details["artifacts"])
		}
	}

	// ── 6. destroy: invokes mocked `tart delete`, clears state; a fresh create
	//        of the same name then succeeds ──────────────────────────────────
	if err := runDestroy(ctx, r, run); err != nil {
		t.Fatalf("runDestroy: %v", err)
	}
	sawDelete := false
	for _, c := range mock.calls {
		if len(c) > 1 && c[0] == "tart" && c[1] == "delete" {
			sawDelete = true
		}
	}
	if !sawDelete {
		t.Errorf("runDestroy did not invoke `tart delete`, calls: %v", mock.calls)
	}
	if _, err := detonate.LoadRun(run); !os.IsNotExist(err) {
		t.Errorf("LoadRun after destroy = %v, want os.IsNotExist (state cleared)", err)
	}
	if err := runCreate(ctx, r, run, golden); err != nil {
		t.Fatalf("fresh runCreate after destroy: %v", err)
	}
	requireState(detonate.StateCreated)

	// ── 7. chain of custody: the audit log records the full lifecycle in order.
	//        The blocked second run writes no entry; the trailing create is the
	//        fresh run from step 6. ───────────────────────────────────────────
	var actions []string
	for _, e := range readAudit(t) {
		actions = append(actions, e.Action)
	}
	wantChain := []string{
		"detonate-create", "detonate-inject", "detonate-run",
		"detonate-collect", "detonate-destroy", "detonate-create",
	}
	if strings.Join(actions, ",") != strings.Join(wantChain, ",") {
		t.Fatalf("audit chain = %v, want %v", actions, wantChain)
	}
}
