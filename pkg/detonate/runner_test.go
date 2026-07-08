package detonate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- FakeRunner: call recording ---

func TestFakeRunner_RecordsCallsInOrder(t *testing.T) {
	f := NewFakeRunner()
	ctx := context.Background()

	_, _ = f.GoldenExists(ctx, "golden-1")
	_ = f.Clone(ctx, "golden-1", "run-1")
	_ = f.Destroy(ctx, "run-1")

	if len(f.Log) != 3 {
		t.Fatalf("expected 3 log entries, got %d: %+v", len(f.Log), f.Log)
	}
	wantMethods := []string{"GoldenExists", "Clone", "Destroy"}
	for i, want := range wantMethods {
		if f.Log[i].Method != want {
			t.Errorf("Log[%d].Method = %q, want %q", i, f.Log[i].Method, want)
		}
	}
	if f.Log[1].Args[0] != "golden-1" || f.Log[1].Args[1] != "run-1" {
		t.Errorf("Clone call args = %v, want [golden-1 run-1]", f.Log[1].Args)
	}
}

func TestFakeRunner_GoldenExists_ConfigurableReturn(t *testing.T) {
	f := NewFakeRunner()
	f.SetGoldenExists("known-golden", true)

	got, err := f.GoldenExists(context.Background(), "known-golden")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected GoldenExists(known-golden) = true")
	}

	got, err = f.GoldenExists(context.Background(), "unknown-golden")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected GoldenExists(unknown-golden) = false (default)")
	}
}

func TestFakeRunner_ErrorPropagation(t *testing.T) {
	f := NewFakeRunner()
	wantErr := errors.New("clone blew up")
	f.SetCloneErr("run-1", wantErr)

	err := f.Clone(context.Background(), "golden-1", "run-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Clone error = %v, want %v", err, wantErr)
	}
	// The call must still be recorded even though it errored.
	if len(f.Log) != 1 || f.Log[0].Method != "Clone" {
		t.Errorf("expected Clone call to be logged despite error, got %+v", f.Log)
	}
}

func TestFakeRunner_CollectAndPoweredOff(t *testing.T) {
	f := NewFakeRunner()
	f.SetPoweredOff("run-1", true)
	f.SetCollectFiles("run-1", []string{"/dest/a.pcap", "/dest/b.log"})

	off, err := f.PoweredOff(context.Background(), "run-1")
	if err != nil || !off {
		t.Fatalf("PoweredOff = (%v, %v), want (true, nil)", off, err)
	}

	files, err := f.Collect(context.Background(), "run-1", "/dest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 collected files, got %v", files)
	}
}

// TestFakeRunner_DrivesFullLifecycle proves a higher-level caller (Task 3's
// CLI) can drive the fake through the whole forward-only lifecycle:
// golden check -> clone -> net -> run -> collect -> destroy.
func TestFakeRunner_DrivesFullLifecycle(t *testing.T) {
	f := NewFakeRunner()
	ctx := context.Background()
	const golden, run = "golden-1", "run-1"

	f.SetGoldenExists(golden, true)
	f.SetNetAttachment(run, NetAttachment{Mode: "isolated", HasUplink: false})
	f.SetPoweredOff(run, true)
	f.SetCollectFiles(run, []string{"/dest/report.json"})

	exists, err := f.GoldenExists(ctx, golden)
	if err != nil || !exists {
		t.Fatalf("GoldenExists = (%v, %v), want (true, nil)", exists, err)
	}
	if err := f.Clone(ctx, golden, run); err != nil {
		t.Fatalf("Clone: %v", err)
	}
	net, err := f.ConfigureIsolatedNet(ctx, run, "gw0")
	if err != nil {
		t.Fatalf("ConfigureIsolatedNet: %v", err)
	}
	if err := ValidateIsolated(net); err != nil {
		t.Fatalf("ValidateIsolated on fake-configured net: %v", err)
	}
	if err := f.InjectOffline(ctx, run, "/tmp/sample.bin"); err != nil {
		t.Fatalf("InjectOffline: %v", err)
	}
	if err := f.Run(ctx, run, 30*time.Second); err != nil {
		t.Fatalf("Run: %v", err)
	}
	off, err := f.PoweredOff(ctx, run)
	if err != nil || !off {
		t.Fatalf("PoweredOff = (%v, %v), want (true, nil)", off, err)
	}
	files, err := f.Collect(ctx, run, "/dest")
	if err != nil || len(files) != 1 {
		t.Fatalf("Collect = (%v, %v), want 1 file, nil", files, err)
	}
	if err := f.Destroy(ctx, run); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	if len(f.Log) != 8 {
		t.Fatalf("expected 8 recorded calls, got %d: %+v", len(f.Log), f.Log)
	}
}

// --- TartRunner: pure arg-building (no live exec) ---

func TestBuildTartCloneArgs(t *testing.T) {
	got := buildTartCloneArgs("golden-1", "run-1")
	want := []string{"clone", "golden-1", "run-1"}
	if !equalStrings(got, want) {
		t.Errorf("buildTartCloneArgs = %v, want %v", got, want)
	}
}

func TestBuildTartDeleteArgs(t *testing.T) {
	got := buildTartDeleteArgs("run-1")
	want := []string{"delete", "run-1"}
	if !equalStrings(got, want) {
		t.Errorf("buildTartDeleteArgs = %v, want %v", got, want)
	}
}

func TestBuildTartListArgs(t *testing.T) {
	got := buildTartListArgs()
	want := []string{"list", "--format", "json"}
	if !equalStrings(got, want) {
		t.Errorf("buildTartListArgs = %v, want %v", got, want)
	}
}

func TestBuildTartRunArgs(t *testing.T) {
	got := buildTartRunArgs("run-1", "/tmp/sample.iso", "10.0.0.0/24", "/tmp/artifacts")
	want := []string{
		"run", "run-1",
		"--no-graphics",
		"--net-softnet",
		"--net-softnet-allow=10.0.0.0/24",
		"--disk=/tmp/sample.iso:ro",
		"--dir=out:/tmp/artifacts",
	}
	if !equalStrings(got, want) {
		t.Errorf("buildTartRunArgs = %v, want %v", got, want)
	}
	// Containment: bridged networking is forbidden — it can reach the host
	// LAN/internet. There must be no --net-bridged in the argv, ever.
	for _, a := range got {
		if strings.HasPrefix(a, "--net-bridged") {
			t.Fatalf("buildTartRunArgs emitted forbidden bridged networking: %v", got)
		}
	}
}

func TestBuildHdiutilCreateArgs(t *testing.T) {
	// Must build an ISO (Tart attaches ISOs; it rejects a UDRO .dmg), with the
	// source dir as the trailing positional that makehybrid expects.
	got := buildHdiutilCreateArgs("/tmp/src", "run-1-sample", "/tmp/sample.iso")
	want := []string{"makehybrid", "-iso", "-joliet", "-default-volume-name", "run-1-sample", "-ov", "-o", "/tmp/sample.iso", "/tmp/src"}
	if !equalStrings(got, want) {
		t.Errorf("buildHdiutilCreateArgs = %v, want %v", got, want)
	}
}

func TestParseTartList(t *testing.T) {
	input := []byte(`[{"Name":"golden-1","State":"stopped"},{"Name":"run-1","State":"running"}]`)
	entries, err := parseTartList(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "golden-1" || entries[0].State != "stopped" {
		t.Errorf("unexpected entry[0]: %+v", entries[0])
	}
	if entries[1].Name != "run-1" || entries[1].State != "running" {
		t.Errorf("unexpected entry[1]: %+v", entries[1])
	}
}

func TestParseTartList_InvalidJSON(t *testing.T) {
	if _, err := parseTartList([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// --- TartRunner: fail-closed containment guard on Run ---

// spyCmdRunner is a cmdRunner test double that records every invocation so
// tests can assert whether the underlying `tart`/`hdiutil` command was ever
// invoked. It returns listJSON for `tart list` calls (empty array by
// default) and nil output otherwise.
type spyCmdRunner struct {
	calls    [][]string
	listJSON []byte
}

func (s *spyCmdRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	s.calls = append(s.calls, append([]string{name}, args...))
	if name == "tart" && len(args) > 0 && args[0] == "list" {
		if s.listJSON != nil {
			return s.listJSON, nil
		}
		return []byte("[]"), nil
	}
	return nil, nil
}

func TestTartRunner_Run_RejectsNonIsolatedAttachmentWithoutInvokingCommand(t *testing.T) {
	spy := &spyCmdRunner{}
	r := NewTartRunner(t.TempDir())
	r.cmd = spy
	// Bypass ConfigureIsolatedNet (which only ever stores safe attachments)
	// to prove Run() re-validates the stored attachment itself, defending
	// against a future ConfigureIsolatedNet regression.
	r.nets["run-1"] = NetAttachment{Mode: "nat", HasUplink: false}

	err := r.Run(context.Background(), "run-1", time.Second)
	if err == nil {
		t.Fatal("Run with non-isolated attachment = nil error, want containment rejection")
	}
	if len(spy.calls) != 0 {
		t.Fatalf("Run must not invoke the command runner when containment fails, got calls: %v", spy.calls)
	}
}

func TestTartRunner_Run_RejectsWhenNoAttachmentConfigured(t *testing.T) {
	spy := &spyCmdRunner{}
	r := NewTartRunner(t.TempDir())
	r.cmd = spy

	err := r.Run(context.Background(), "never-configured", time.Second)
	if err == nil {
		t.Fatal("Run with no configured attachment = nil error, want rejection")
	}
	if len(spy.calls) != 0 {
		t.Fatalf("Run must not invoke the command runner without a configured attachment, got calls: %v", spy.calls)
	}
}

func TestTartRunner_ConfigureIsolatedNet_FailsClosedOnNonCIDRGateway(t *testing.T) {
	r := NewTartRunner(t.TempDir())
	// A non-CIDR allow-list (e.g. an interface name) must fail closed:
	// ValidateSoftnetAllow only accepts a parseable private CIDR.
	_, err := r.ConfigureIsolatedNet(context.Background(), "run-1", "en0")
	if err == nil {
		t.Fatal("ConfigureIsolatedNet with a non-CIDR allow-list = nil error, want rejection")
	}
}

func TestTartRunner_ConfigureIsolatedNet_SucceedsWithMatchingOperatorGateway(t *testing.T) {
	r := NewTartRunner(t.TempDir())
	r.AllowedIsolatedGateway = "10.0.0.0/24"

	n, err := r.ConfigureIsolatedNet(context.Background(), "run-1", "10.0.0.0/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateIsolated(n); err != nil {
		t.Fatalf("attachment returned by ConfigureIsolatedNet failed its own guard: %v", err)
	}
}

// TestTartRunner_InjectOffline_BuildsImageAndRecordsPath: hdiutil builds the
// read-only sample image (spied here — the real-hdiutil path is exercised in
// tart_test.go), and the built image path is recorded per-run so Run can
// attach it read-only. Attaching happens at `tart run --disk` time, so
// InjectOffline succeeds rather than failing closed.
func TestTartRunner_InjectOffline_BuildsImageAndRecordsPath(t *testing.T) {
	spy := &spyCmdRunner{listJSON: []byte(`[{"Name":"run-1","State":"stopped"}]`)}
	r := NewTartRunner(t.TempDir())
	r.cmd = spy

	samplePath := writeTempSample(t)

	if err := r.InjectOffline(context.Background(), "run-1", samplePath); err != nil {
		t.Fatalf("InjectOffline = %v, want nil (image built + path recorded)", err)
	}

	foundHdiutil := false
	for _, c := range spy.calls {
		if c[0] == "hdiutil" {
			foundHdiutil = true
		}
	}
	if !foundHdiutil {
		t.Errorf("expected hdiutil to be invoked to build the read-only sample image, calls: %v", spy.calls)
	}
	if r.samples["run-1"] == "" {
		t.Error("InjectOffline must record the built sample image path for run-1")
	}
}

func TestTartRunner_InjectOffline_RefusesWhenRunning(t *testing.T) {
	spy := &spyCmdRunner{listJSON: []byte(`[{"Name":"run-1","State":"running"}]`)}
	r := NewTartRunner(t.TempDir())
	r.cmd = spy

	samplePath := writeTempSample(t)
	if err := r.InjectOffline(context.Background(), "run-1", samplePath); err == nil {
		t.Fatal("InjectOffline while guest running = nil error, want rejection")
	}
	for _, c := range spy.calls {
		if c[0] == "hdiutil" {
			t.Fatalf("must not build sample image while guest is running, calls: %v", spy.calls)
		}
	}
}

// --- TartRunner: Collect must not follow guest-planted symlinks ---

// TestTartRunner_Collect_SkipsSymlinksAndNeverReadsThroughThem is the key
// containment test for this fix: the artifacts dir is guest-writable (the
// --dir=out: mount), so a detonating sample can plant a symlink there that
// points at an arbitrary host path. Collect must skip it entirely — never
// dereference it — or the guest gets a host file-read primitive.
func TestTartRunner_Collect_SkipsSymlinksAndNeverReadsThroughThem(t *testing.T) {
	spy := &spyCmdRunner{listJSON: []byte(`[{"Name":"run-1","State":"stopped"}]`)}
	r := NewTartRunner(t.TempDir())
	r.cmd = spy

	artifactsDir := filepath.Join(r.WorkDir, "run-1", "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o700); err != nil {
		t.Fatalf("mkdir artifacts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(artifactsDir, "report.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("write report.json: %v", err)
	}

	// Host secret that lives OUTSIDE the artifacts dir entirely.
	secretPath := filepath.Join(t.TempDir(), "secret.txt")
	const secretContents = "top-secret-host-data"
	if err := os.WriteFile(secretPath, []byte(secretContents), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	// Guest-planted symlink, named like a plausible artifact, pointing at
	// the host secret above.
	if err := os.Symlink(secretPath, filepath.Join(artifactsDir, "evil.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	destDir := t.TempDir()
	files, err := r.Collect(context.Background(), "run-1", destDir)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if len(files) != 1 || filepath.Base(files[0]) != "report.json" {
		t.Fatalf("expected only report.json collected, got %v", files)
	}
	if _, err := os.Stat(filepath.Join(destDir, "evil.json")); !os.IsNotExist(err) {
		t.Fatalf("evil.json symlink must not be collected, stat err = %v", err)
	}
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatalf("reading destDir: %v", err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(destDir, e.Name()))
		if err != nil {
			t.Fatalf("reading collected %s: %v", e.Name(), err)
		}
		if strings.Contains(string(data), secretContents) {
			t.Fatalf("symlink target was followed: secret leaked into collected artifact %s", e.Name())
		}
	}
}

// --- TartRunner: Run must use the gateway captured at ConfigureIsolatedNet time ---

func TestTartRunner_Run_UsesGatewayCapturedAtConfigureTime(t *testing.T) {
	spy := &spyCmdRunner{}
	r := NewTartRunner(t.TempDir())
	r.cmd = spy
	r.AllowedIsolatedGateway = "10.0.0.0/24"

	if _, err := r.ConfigureIsolatedNet(context.Background(), "run-1", "10.0.0.0/24"); err != nil {
		t.Fatalf("ConfigureIsolatedNet: %v", err)
	}
	// A sample must be injected before Run will boot; Run fails closed unless
	// the image exists on disk, so stage a real file and record it directly.
	sampleImg := filepath.Join(t.TempDir(), "run-1-sample.iso")
	if err := os.WriteFile(sampleImg, []byte("iso"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.samples["run-1"] = sampleImg

	// Regression: mutate the shared field after ConfigureIsolatedNet. Run
	// must not pick this up — it must use the CIDR captured at
	// ConfigureIsolatedNet time, so the value ValidateSoftnetAllow guarded and
	// the value `tart run` consumes can never diverge.
	r.AllowedIsolatedGateway = "192.168.99.0/24"

	if err := r.Run(context.Background(), "run-1", time.Second); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var runCall []string
	for _, c := range spy.calls {
		if len(c) > 1 && c[0] == "tart" && c[1] == "run" {
			runCall = c
		}
	}
	if runCall == nil {
		t.Fatalf("expected a `tart run` invocation, calls: %v", spy.calls)
	}
	wantArg := "--net-softnet-allow=10.0.0.0/24"
	badArg := "--net-softnet-allow=192.168.99.0/24"
	wantDisk := "--disk=" + sampleImg + ":ro"
	found, foundDisk := false, false
	for _, a := range runCall {
		if a == badArg {
			t.Fatalf("Run used the mutated shared gateway field instead of the captured value: %v", runCall)
		}
		if strings.HasPrefix(a, "--net-bridged") {
			t.Fatalf("Run emitted forbidden bridged networking: %v", runCall)
		}
		if a == wantArg {
			found = true
		}
		if a == wantDisk {
			foundDisk = true
		}
	}
	if !found {
		t.Fatalf("expected tart run args to contain %q (captured CIDR), got %v", wantArg, runCall)
	}
	if !foundDisk {
		t.Fatalf("expected tart run args to attach the injected sample read-only %q, got %v", wantDisk, runCall)
	}
}

func writeTempSample(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(path, []byte("not actually malware"), 0o600); err != nil {
		t.Fatalf("writing temp sample: %v", err)
	}
	return path
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
