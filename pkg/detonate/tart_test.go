package detonate

import (
	"context"
	"os"
	"os/exec"
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
	if err := r.InjectOffline(context.Background(), "run-1", samplePath); err != nil {
		t.Fatalf("InjectOffline (real hdiutil): %v", err)
	}

	built := r.samples["run-1"]
	if built == "" {
		t.Fatal("InjectOffline must record the built sample image path")
	}
	if _, err := os.Stat(built); err != nil {
		t.Fatalf("built sample image %q should exist on disk: %v", built, err)
	}
}
