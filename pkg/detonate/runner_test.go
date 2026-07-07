package detonate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	got := buildTartRunArgs("run-1", "isolated-bridge0", "/tmp/artifacts")
	want := []string{"run", "run-1", "--no-graphics", "--net-bridged=isolated-bridge0", "--dir=out:/tmp/artifacts"}
	if !equalStrings(got, want) {
		t.Errorf("buildTartRunArgs = %v, want %v", got, want)
	}
}

func TestBuildHdiutilCreateArgs(t *testing.T) {
	got := buildHdiutilCreateArgs("/tmp/src", "run-1-sample", "/tmp/sample.dmg")
	want := []string{"create", "-srcfolder", "/tmp/src", "-volname", "run-1-sample", "-format", "UDRO", "-ov", "-o", "/tmp/sample.dmg"}
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

func TestTartRunner_ConfigureIsolatedNet_FailsClosedWithoutOperatorGateway(t *testing.T) {
	r := NewTartRunner(t.TempDir())
	// AllowedIsolatedGateway is unset (zero value) — must fail closed.
	_, err := r.ConfigureIsolatedNet(context.Background(), "run-1", "en0")
	if err == nil {
		t.Fatal("ConfigureIsolatedNet without an operator-provisioned gateway = nil error, want rejection")
	}
}

func TestTartRunner_ConfigureIsolatedNet_SucceedsWithMatchingOperatorGateway(t *testing.T) {
	r := NewTartRunner(t.TempDir())
	r.AllowedIsolatedGateway = "isolated-bridge0"

	n, err := r.ConfigureIsolatedNet(context.Background(), "run-1", "isolated-bridge0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := ValidateIsolated(n); err != nil {
		t.Fatalf("attachment returned by ConfigureIsolatedNet failed its own guard: %v", err)
	}
}

// TestTartRunner_InjectOffline_BuildsImageButFailsClosedOnAttach documents
// the deliberate stub: hdiutil image creation is a real, nameable command
// and is exercised; attaching that image to a running Tart guest is not a
// capability this code invents, so it must fail closed rather than claim
// success.
func TestTartRunner_InjectOffline_BuildsImageButFailsClosedOnAttach(t *testing.T) {
	spy := &spyCmdRunner{listJSON: []byte(`[{"Name":"run-1","State":"stopped"}]`)}
	r := NewTartRunner(t.TempDir())
	r.cmd = spy

	samplePath := writeTempSample(t)

	err := r.InjectOffline(context.Background(), "run-1", samplePath)
	if err == nil {
		t.Fatal("InjectOffline attach step = nil error, want fail-closed operator-provisioned error")
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
