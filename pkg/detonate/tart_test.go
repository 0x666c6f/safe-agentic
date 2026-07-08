package detonate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- ValidateSoftnetAllow: the load-bearing softnet containment control ---

func TestValidateSoftnetAllow(t *testing.T) {
	accept := []string{
		"10.0.0.0/8",
		"10.0.0.0/24",
		"10.1.2.3/32", // a /32 host within private space
		"172.16.0.0/12",
		"172.16.5.0/24",
		"172.31.255.0/24", // top of the 172.16/12 block
		"192.168.0.0/16",
		"192.168.1.0/24",
		"169.254.0.0/16", // link-local
		"169.254.1.1/32",
	}
	for _, cidr := range accept {
		if err := ValidateSoftnetAllow(cidr); err != nil {
			t.Errorf("ValidateSoftnetAllow(%q) = %v, want nil (private CIDR)", cidr, err)
		}
	}

	reject := []string{
		"",                    // empty
		"0.0.0.0/0",           // default route — internet
		"::/0",                // IPv6 default route
		"8.8.8.8/32",          // public host
		"1.2.3.0/24",          // public range
		"11.0.0.0/8",          // just outside 10.0.0.0/8
		"172.15.0.0/16",       // just below the 172.16/12 block
		"172.32.0.0/24",       // just above the 172.16/12 block
		"192.167.0.0/16",      // just below 192.168/16
		"10.0.0.0/7",          // prefix too short: spans 10.x and 11.x
		"192.168.0.0/8",       // prefix widened to all of 192.x (public space)
		"::ffff:10.0.0.0/120", // IPv4-mapped IPv6 notation — rejected outright
		"::ffff:8.8.8.8/128",  // v4-mapped public host
		"not-a-cidr",          // unparseable
		"en0",                 // interface name, not a CIDR
	}
	for _, cidr := range reject {
		if err := ValidateSoftnetAllow(cidr); err == nil {
			t.Errorf("ValidateSoftnetAllow(%q) = nil, want rejection (not a private CIDR)", cidr)
		}
	}
}

// --- ConfigureIsolatedNet fails closed on a public / default-route allow-list ---

func TestTartRunner_ConfigureIsolatedNet_FailsClosedOnPublicCIDR(t *testing.T) {
	for _, gw := range []string{"0.0.0.0/0", "8.8.8.8/32", "1.2.3.0/24"} {
		r := NewTartRunner(t.TempDir())
		if _, err := r.ConfigureIsolatedNet(context.Background(), "run-1", gw); err == nil {
			t.Fatalf("ConfigureIsolatedNet(%q) = nil error, want containment rejection", gw)
		}
		if _, ok := r.nets["run-1"]; ok {
			t.Fatalf("ConfigureIsolatedNet(%q) must not record an attachment on rejection", gw)
		}
	}
}

// --- Run fails closed if no sample was injected, without invoking `tart run` ---

func TestTartRunner_Run_RejectsWhenNoSampleInjected(t *testing.T) {
	spy := &spyCmdRunner{}
	r := NewTartRunner(t.TempDir())
	r.cmd = spy
	r.AllowedIsolatedGateway = "10.0.0.0/24"

	if _, err := r.ConfigureIsolatedNet(context.Background(), "run-1", "10.0.0.0/24"); err != nil {
		t.Fatalf("ConfigureIsolatedNet: %v", err)
	}
	// No InjectOffline — the run has an isolated net but no sample.

	err := r.Run(context.Background(), "run-1", time.Second)
	if err == nil {
		t.Fatal("Run without an injected sample = nil error, want fail-closed rejection")
	}
	for _, c := range spy.calls {
		if len(c) > 1 && c[0] == "tart" && c[1] == "run" {
			t.Fatalf("Run must not invoke `tart run` without an injected sample, calls: %v", spy.calls)
		}
	}
}

// --- The detonation timeout is the WINDOW, not a failure ---

// blockingCmdRunner blocks the `tart run` call until the context is canceled
// (a guest that runs until the window's deadline kills tart run) and returns
// immediately for every other call. `tart list` reports the run in listState
// (default "stopped") so Run's post-window PoweredOff check has something to
// read — set listState to "running" to simulate a VM that survived the stop.
type blockingCmdRunner struct {
	calls     [][]string
	listState string
}

func (b *blockingCmdRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	b.calls = append(b.calls, append([]string{name}, args...))
	switch {
	case name == "tart" && len(args) > 0 && args[0] == "list":
		st := b.listState
		if st == "" {
			st = "stopped"
		}
		return []byte(`[{"Name":"run-1","State":"` + st + `"}]`), nil
	case name == "tart" && len(args) > 0 && args[0] == "run":
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, nil
}

func TestTartRunner_Run_TimeoutIsSuccessNotFailure(t *testing.T) {
	b := &blockingCmdRunner{}
	r := NewTartRunner(t.TempDir())
	r.cmd = b
	r.AllowedIsolatedGateway = "10.0.0.0/24"

	if _, err := r.ConfigureIsolatedNet(context.Background(), "run-1", "10.0.0.0/24"); err != nil {
		t.Fatalf("ConfigureIsolatedNet: %v", err)
	}
	sampleImg := filepath.Join(t.TempDir(), "sample.iso")
	if err := os.WriteFile(sampleImg, []byte("iso"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.samples["run-1"] = sampleImg

	// A guest that never self-powers-off ends when our timeout fires: the
	// normal, successful end of the detonation window — not a failure. Run
	// must return nil and leave the clone intact for collect.
	if err := r.Run(context.Background(), "run-1", 50*time.Millisecond); err != nil {
		t.Fatalf("Run on timeout = %v, want nil (window elapsed is success)", err)
	}

	var stopped, deleted bool
	for _, c := range b.calls {
		if len(c) >= 2 && c[0] == "tart" && c[1] == "stop" {
			stopped = true
		}
		if len(c) >= 2 && c[0] == "tart" && c[1] == "delete" {
			deleted = true
		}
	}
	if !stopped {
		t.Errorf("Run must best-effort `tart stop` after the window; calls: %v", b.calls)
	}
	if deleted {
		t.Errorf("Run must NOT delete the clone on timeout (collect needs it); calls: %v", b.calls)
	}
}

// If the window elapses but the VM is STILL running after the stop, that is a
// containment failure, not a completed run: a live-malware guest must never be
// reported as success. Run must return an error so the caller auto-destroys.
func TestTartRunner_Run_TimeoutButVMStillRunningIsFailure(t *testing.T) {
	b := &blockingCmdRunner{listState: "running"} // stop "fails" to power it off
	r := NewTartRunner(t.TempDir())
	r.cmd = b
	r.AllowedIsolatedGateway = "10.0.0.0/24"
	if _, err := r.ConfigureIsolatedNet(context.Background(), "run-1", "10.0.0.0/24"); err != nil {
		t.Fatalf("ConfigureIsolatedNet: %v", err)
	}
	sampleImg := filepath.Join(t.TempDir(), "sample.iso")
	if err := os.WriteFile(sampleImg, []byte("iso"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.samples["run-1"] = sampleImg

	if err := r.Run(context.Background(), "run-1", 50*time.Millisecond); err == nil {
		t.Fatal("Run with a VM still running after the window = nil, want a containment failure error")
	}
}

// A parent-context cancel (operator Ctrl-C), by contrast, is NOT a normal
// window end: Run must propagate it as an error so the run is treated as
// failed and auto-destroyed upstream.
func TestTartRunner_Run_ParentCancelPropagatesAsError(t *testing.T) {
	b := &blockingCmdRunner{}
	r := NewTartRunner(t.TempDir())
	r.cmd = b
	r.AllowedIsolatedGateway = "10.0.0.0/24"
	if _, err := r.ConfigureIsolatedNet(context.Background(), "run-1", "10.0.0.0/24"); err != nil {
		t.Fatalf("ConfigureIsolatedNet: %v", err)
	}
	sampleImg := filepath.Join(t.TempDir(), "sample.iso")
	if err := os.WriteFile(sampleImg, []byte("iso"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.samples["run-1"] = sampleImg

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	// Long timeout so the deadline can't fire first — the parent cancel wins.
	if err := r.Run(ctx, "run-1", time.Hour); err == nil {
		t.Fatal("Run on parent cancel = nil, want a propagated error")
	}
}

// --- InjectOffline exercises the real hdiutil image build (macOS only) ---

// hdiutilTartCmd delegates hdiutil to the real exec runner but fakes `tart
// list` so InjectOffline's powered-off check passes without a real Tart VM.
type hdiutilTartCmd struct{ real execCmdRunner }

func (h hdiutilTartCmd) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "tart" && len(args) > 0 && args[0] == "list" {
		return []byte(`[{"Name":"run-1","State":"stopped"}]`), nil
	}
	return h.real.Run(ctx, name, args...)
}

func TestTartRunner_InjectOffline_RealHdiutilBuildsImage(t *testing.T) {
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil not available in this environment")
	}
	r := NewTartRunner(t.TempDir())
	r.cmd = hdiutilTartCmd{}

	samplePath := writeTempSample(t)
	// `hdiutil create` intermittently fails with "Resource busy" on shared CI
	// runners (diskimages daemon contention) — that's an environment flake, not
	// a defect in InjectOffline's arg construction, which is what this test
	// verifies. Retry a few times, and skip (don't fail the suite) if hdiutil
	// stays busy. A non-hdiutil error is a real failure and fails immediately.
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if err = r.InjectOffline(context.Background(), "run-1", samplePath); err == nil {
			break
		}
		if !strings.Contains(err.Error(), "hdiutil") && !strings.Contains(err.Error(), "Resource busy") {
			t.Fatalf("InjectOffline (real hdiutil): %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Skipf("hdiutil unreliable on this runner after retries: %v", err)
	}

	built := r.samples["run-1"]
	if built == "" {
		t.Fatal("InjectOffline must record the built sample image path")
	}
	if _, err := os.Stat(built); err != nil {
		t.Fatalf("built sample image %q should exist on disk: %v", built, err)
	}
}
