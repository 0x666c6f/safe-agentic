package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0x666c6f/berth/pkg/audit"
	"github.com/0x666c6f/berth/pkg/detonate"
)

// setTempAuditPath redirects audit.DefaultPath() to a temp dir, same
// convention as cmd/berth's evidence_test.go.
func setTempAuditPath(t *testing.T) {
	t.Helper()
	t.Setenv("BERTH_STATE_HOME", t.TempDir())
}

func readAudit(t *testing.T) []audit.Entry {
	t.Helper()
	entries, err := (&audit.Logger{Path: audit.DefaultPath()}).Read(0)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	return entries
}

func hasCall(log []detonate.Call, method string) bool {
	for _, c := range log {
		if c.Method == method {
			return true
		}
	}
	return false
}

// ─── route ──────────────────────────────────────────────────────────────

func TestParseStaticFindings_TolerantOfSnakeAndCamel(t *testing.T) {
	snake := []byte(`{"sha256":"abc","file_type":"PE32","arch":"arm64","format":"pe"}`)
	f, err := parseStaticFindings(snake)
	if err != nil {
		t.Fatalf("parseStaticFindings(snake): %v", err)
	}
	if f.SHA256 != "abc" || f.FileType != "PE32" || f.Arch != "arm64" || f.Format != "pe" {
		t.Errorf("snake_case parse mismatch: %+v", f)
	}

	camel := []byte(`{"sha256":"def","fileType":"ELF","arch":"x86-64","format":"pe"}`)
	f, err = parseStaticFindings(camel)
	if err != nil {
		t.Fatalf("parseStaticFindings(camel): %v", err)
	}
	if f.FileType != "ELF" {
		t.Errorf("camelCase fileType not picked up: %+v", f)
	}
}

func TestRunRoute_ExitBehaviorPerTier(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "static.json")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("local ARM: no error", func(t *testing.T) {
		p := write(t, `{"sha256":"a","arch":"arm64","format":"elf"}`)
		var buf bytes.Buffer
		if err := runRoute(p, &buf); err != nil {
			t.Fatalf("runRoute() error = %v, want nil", err)
		}
		if !strings.Contains(buf.String(), "local-arm") {
			t.Errorf("output missing tier: %s", buf.String())
		}
	})

	t.Run("cloud x86: no error, guidance printed", func(t *testing.T) {
		p := write(t, `{"sha256":"a","arch":"x86-64","format":"pe"}`)
		var buf bytes.Buffer
		if err := runRoute(p, &buf); err != nil {
			t.Fatalf("runRoute() error = %v, want nil", err)
		}
		if !strings.Contains(buf.String(), "guidance") {
			t.Errorf("output missing guidance: %s", buf.String())
		}
	})

	t.Run("refuse: non-zero (error returned)", func(t *testing.T) {
		p := write(t, `{"sha256":"a","arch":"","format":""}`)
		var buf bytes.Buffer
		err := runRoute(p, &buf)
		if err == nil {
			t.Fatal("runRoute() error = nil, want non-nil for TierRefuse")
		}
	})
}

// ─── create ─────────────────────────────────────────────────────────────

func TestRunCreate_FailsClosedWhenGoldenMissing(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetGoldenExists("golden-1", false)

	err := runCreate(context.Background(), fake, "run-1", "golden-1")
	if err == nil {
		t.Fatal("runCreate() error = nil, want error when golden missing")
	}
	if hasCall(fake.Log, "Clone") {
		t.Error("Clone was called despite GoldenExists=false")
	}
	if entries := readAudit(t); len(entries) != 0 {
		t.Errorf("audit entries = %d, want 0 on fail-closed path", len(entries))
	}
}

func TestRunCreate_Success(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetGoldenExists("golden-1", true)

	if err := runCreate(context.Background(), fake, "run-1", "golden-1"); err != nil {
		t.Fatalf("runCreate() error = %v", err)
	}
	if !hasCall(fake.Log, "Clone") {
		t.Error("Clone was not called")
	}
	entries := readAudit(t)
	if len(entries) != 1 || entries[0].Action != "detonate-create" || entries[0].Container != "run-1" {
		t.Fatalf("unexpected audit entries: %+v", entries)
	}
	if entries[0].Details["golden"] != "golden-1" {
		t.Errorf("audit golden = %q, want %q", entries[0].Details["golden"], "golden-1")
	}
}

// ─── inject ─────────────────────────────────────────────────────────────

func TestRunInject_AuditsBeforeRunnerCall(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetInjectErr("run-1", fmt.Errorf("operator-provisioned: disk-attach not configured"))

	sample := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(sample, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := runInject(context.Background(), fake, "run-1", sample)
	if err == nil {
		t.Fatal("runInject() error = nil, want the InjectOffline error surfaced")
	}

	// The audit entry must exist even though InjectOffline failed: the hash
	// is recorded BEFORE the runner call, not after a successful one.
	entries := readAudit(t)
	if len(entries) != 1 || entries[0].Action != "detonate-inject" {
		t.Fatalf("unexpected audit entries: %+v", entries)
	}
	wantHash := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if entries[0].Details["sha256"] != wantHash {
		t.Errorf("audit sha256 = %q, want %q", entries[0].Details["sha256"], wantHash)
	}
}

// ─── run ────────────────────────────────────────────────────────────────

func TestRunRun_FailsClosedWhenNotIsolated(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetNetAttachment("run-1", detonate.NetAttachment{Mode: "bridged", HasUplink: true})
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want containment error")
	}
	if hasCall(fake.Log, "Run") {
		t.Error("Runner.Run was called despite a non-isolated attachment")
	}
	if hasCall(fake.Log, "Destroy") {
		t.Error("Destroy should not be called on the isolation-check path (nothing was started)")
	}
}

func TestRunRun_AutoDestroysOnRunError(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetRunErr("run-1", fmt.Errorf("boom"))
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want the Run error surfaced")
	}

	runIdx, destroyIdx := -1, -1
	for i, c := range fake.Log {
		switch c.Method {
		case "Run":
			runIdx = i
		case "Destroy":
			destroyIdx = i
		}
	}
	if runIdx == -1 || destroyIdx == -1 || destroyIdx < runIdx {
		t.Fatalf("expected Run then Destroy in call log, got: %+v", fake.Log)
	}

	entries := readAudit(t)
	if len(entries) != 1 || entries[0].Action != "detonate-run" {
		t.Fatalf("unexpected audit entries: %+v", entries)
	}
	if entries[0].Details["error"] == "" {
		t.Error("audit entry missing error detail")
	}
	if entries[0].Details["auto_destroyed"] != "true" {
		t.Error("audit entry missing auto_destroyed=true")
	}
}

func TestRunRun_DestroyFailureIsNotReportedAsAutoDestroyed(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetRunErr("run-1", fmt.Errorf("boom"))
	fake.SetDestroyErr("run-1", fmt.Errorf("tart: vm busy"))
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want the Run error surfaced")
	}
	if strings.Contains(err.Error(), "auto-destroyed") {
		t.Errorf("error must not claim auto-destroyed when Destroy itself failed: %v", err)
	}
	if !strings.Contains(err.Error(), "may still be running") {
		t.Errorf("error must warn the VM may still be running: %v", err)
	}

	if !hasCall(fake.Log, "Run") || !hasCall(fake.Log, "Destroy") {
		t.Fatalf("expected both Run and Destroy attempted, got: %+v", fake.Log)
	}

	entries := readAudit(t)
	if len(entries) != 1 || entries[0].Action != "detonate-run" {
		t.Fatalf("unexpected audit entries: %+v", entries)
	}
	if entries[0].Details["destroy_error"] == "" {
		t.Error("audit entry missing destroy_error detail")
	}
	if entries[0].Details["auto_destroyed"] != "" {
		t.Error("audit entry must not claim auto_destroyed=true when Destroy failed")
	}
}

func TestRunRun_MissingGatewayFailsClosed(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	err := runRun(context.Background(), fake, "run-1", "", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want error when --gateway is empty")
	}
	if len(fake.Log) != 0 {
		t.Errorf("Runner should not be touched before --gateway is validated, got: %+v", fake.Log)
	}
}

func TestRunRun_ConfirmationGate(t *testing.T) {
	setTempAuditPath(t)

	t.Run("wrong phrase aborts without calling Run", func(t *testing.T) {
		fake := detonate.NewFakeRunner()
		err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, false, strings.NewReader("nope\n"))
		if err == nil {
			t.Fatal("runRun() error = nil, want confirmation mismatch error")
		}
		if hasCall(fake.Log, "Run") {
			t.Error("Runner.Run was called despite a wrong confirmation phrase")
		}
	})

	t.Run("exact phrase proceeds to Run", func(t *testing.T) {
		fake := detonate.NewFakeRunner()
		err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, false, strings.NewReader("detonate run-1\n"))
		if err != nil {
			t.Fatalf("runRun() error = %v, want nil", err)
		}
		if !hasCall(fake.Log, "Run") {
			t.Error("Runner.Run was not called despite the exact confirmation phrase")
		}
	})

	t.Run("--yes alone (no env) still prompts", func(t *testing.T) {
		fake := detonate.NewFakeRunner()
		err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader(""))
		if err == nil {
			t.Fatal("runRun() error = nil, want error: --yes without DETONATE_I_UNDERSTAND=1 must not bypass the prompt")
		}
		if hasCall(fake.Log, "Run") {
			t.Error("Runner.Run was called despite --yes without the env var")
		}
	})
}

// ─── collect ────────────────────────────────────────────────────────────

func TestRunCollect_FailsClosedWhenNotPoweredOff(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetPoweredOff("run-1", false)

	err := runCollect(context.Background(), fake, "run-1", t.TempDir())
	if err == nil {
		t.Fatal("runCollect() error = nil, want error when not powered off")
	}
	if hasCall(fake.Log, "Collect") {
		t.Error("Collect was called despite PoweredOff=false")
	}
}

func TestRunCollect_HashesArtifactsAndAudits(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetPoweredOff("run-1", true)

	dir := t.TempDir()
	f1 := filepath.Join(dir, "report.json")
	if err := os.WriteFile(f1, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake.SetCollectFiles("run-1", []string{f1})

	if err := runCollect(context.Background(), fake, "run-1", dir); err != nil {
		t.Fatalf("runCollect() error = %v", err)
	}

	entries := readAudit(t)
	if len(entries) != 1 || entries[0].Action != "detonate-collect" {
		t.Fatalf("unexpected audit entries: %+v", entries)
	}
	if entries[0].Details["count"] != "1" {
		t.Errorf("audit count = %q, want %q", entries[0].Details["count"], "1")
	}
	wantHash := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if !strings.Contains(entries[0].Details["artifacts"], wantHash) {
		t.Errorf("audit artifacts missing sha256(%q): %q", "hello", entries[0].Details["artifacts"])
	}
}

// ─── destroy ────────────────────────────────────────────────────────────

func TestRunDestroy_Idempotent(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()

	if err := runDestroy(context.Background(), fake, "run-1"); err != nil {
		t.Fatalf("runDestroy() error = %v", err)
	}
	if !hasCall(fake.Log, "Destroy") {
		t.Error("Destroy was not called")
	}
	entries := readAudit(t)
	if len(entries) != 1 || entries[0].Action != "detonate-destroy" {
		t.Fatalf("unexpected audit entries: %+v", entries)
	}

	// Idempotent: calling again should not error even though the run is
	// already gone (FakeRunner.Destroy is a no-op success by default).
	if err := runDestroy(context.Background(), fake, "run-1"); err != nil {
		t.Fatalf("second runDestroy() error = %v, want nil (idempotent)", err)
	}
}
