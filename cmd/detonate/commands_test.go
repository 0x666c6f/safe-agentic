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
// convention as cmd/berth's evidence_test.go, and points the detonate state
// store (pkg/detonate.StateDir) at its own temp dir so tests never touch a
// real ~/.berth/detonate.
func setTempAuditPath(t *testing.T) {
	t.Helper()
	t.Setenv("BERTH_STATE_HOME", t.TempDir())
	t.Setenv("DETONATE_STATE_DIR", t.TempDir())
}

func readAudit(t *testing.T) []audit.Entry {
	t.Helper()
	entries, err := (&audit.Logger{Path: audit.DefaultPath()}).Read(0)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	return entries
}

// auditEntriesByAction filters the audit log to one action, since tests that
// seed a run through create/inject/run now produce audit entries for those
// steps too, not just the one under test.
func auditEntriesByAction(t *testing.T, action string) []audit.Entry {
	t.Helper()
	var out []audit.Entry
	for _, e := range readAudit(t) {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

func hasCall(log []detonate.Call, method string) bool {
	for _, c := range log {
		if c.Method == method {
			return true
		}
	}
	return false
}

// seedCreated drives runCreate for a fresh run against a golden that's
// configured to exist, leaving state=Created.
func seedCreated(t *testing.T, fake *detonate.FakeRunner, run string) {
	t.Helper()
	fake.SetGoldenExists("golden-seed", true)
	if err := runCreate(context.Background(), fake, run, "golden-seed"); err != nil {
		t.Fatalf("seed create(%q): %v", run, err)
	}
}

// seedInjected drives create then inject, leaving state=Injected.
func seedInjected(t *testing.T, fake *detonate.FakeRunner, run string) {
	t.Helper()
	seedCreated(t, fake, run)
	sample := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(sample, []byte("sample"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runInject(context.Background(), fake, run, sample); err != nil {
		t.Fatalf("seed inject(%q): %v", run, err)
	}
}

// seedDetonated drives create, inject, then a successful run, leaving
// state=Detonated. Requires DETONATE_I_UNDERSTAND=1 (set here) since it must
// pass the confirmation gate non-interactively.
func seedDetonated(t *testing.T, fake *detonate.FakeRunner, run string) {
	t.Helper()
	seedInjected(t, fake, run)
	t.Setenv("DETONATE_I_UNDERSTAND", "1")
	if err := runRun(context.Background(), fake, run, "gw-seed", time.Second, true, strings.NewReader("")); err != nil {
		t.Fatalf("seed run(%q): %v", run, err)
	}
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

// TestParseStaticFindings_TolerantOfNumericFields covers berth's forensic
// JSON, which carries numeric fields (size, entropy) alongside the string
// ones route cares about. Unmarshaling into map[string]string used to fail
// wholesale the moment any field wasn't a string.
func TestParseStaticFindings_TolerantOfNumericFields(t *testing.T) {
	data := []byte(`{"sha256":"abc","arch":"arm64","format":"elf","size":1234,"entropy":7.9}`)
	f, err := parseStaticFindings(data)
	if err != nil {
		t.Fatalf("parseStaticFindings() with numeric fields: %v", err)
	}
	if f.Arch != "arm64" || f.Format != "elf" || f.SHA256 != "abc" {
		t.Errorf("numeric-field JSON parse mismatch: %+v", f)
	}
}

// TestRunRoute_AcceptsNumericFields is the end-to-end check: route must not
// refuse plausible input just because static.json has numeric fields.
func TestRunRoute_AcceptsNumericFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "static.json")
	body := `{"sha256":"a","arch":"arm64","format":"elf","size":1234,"entropy":7.9}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runRoute(p, &buf); err != nil {
		t.Fatalf("runRoute() error = %v, want nil (numeric fields must not cause a refusal)", err)
	}
	if !strings.Contains(buf.String(), "local-arm") {
		t.Errorf("output missing tier: %s", buf.String())
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

func TestRunCreate_RejectsInvalidGolden(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()

	err := runCreate(context.Background(), fake, "run-1", "-not-a-name")
	if err == nil {
		t.Fatal("runCreate() error = nil, want error for a leading-dash --golden value")
	}
	if hasCall(fake.Log, "GoldenExists") || hasCall(fake.Log, "Clone") {
		t.Error("Runner should not be touched before --golden is validated")
	}
}

// TestRunCreate_FailsClosedOnExistingRun is the reuse-enforcement test for
// create: a state file already on disk (meaning some earlier run by this
// name hasn't been destroyed) must block a second create outright, even
// against a perfectly valid golden.
func TestRunCreate_FailsClosedOnExistingRun(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetGoldenExists("golden-1", true)

	if err := runCreate(context.Background(), fake, "run-1", "golden-1"); err != nil {
		t.Fatalf("first runCreate() error = %v, want nil", err)
	}
	before := len(fake.Log)

	err := runCreate(context.Background(), fake, "run-1", "golden-1")
	if err == nil {
		t.Fatal("second runCreate() error = nil, want error: run already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second runCreate() error = %v, want it to say the run already exists", err)
	}
	if len(fake.Log) != before {
		t.Errorf("Runner was touched on the already-exists path: new calls %+v", fake.Log[before:])
	}
}

// TestRunDestroyThenCreate_AllowsFreshRun proves destroy is the only escape
// hatch: after destroy clears state, create with the same run name succeeds
// again — a genuinely fresh run, not a reused clone.
func TestRunDestroyThenCreate_AllowsFreshRun(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetGoldenExists("golden-1", true)

	if err := runCreate(context.Background(), fake, "run-1", "golden-1"); err != nil {
		t.Fatalf("first runCreate() error = %v", err)
	}
	if err := runDestroy(context.Background(), fake, "run-1"); err != nil {
		t.Fatalf("runDestroy() error = %v", err)
	}
	if err := runCreate(context.Background(), fake, "run-1", "golden-1"); err != nil {
		t.Fatalf("runCreate() after destroy error = %v, want nil (fresh run allowed)", err)
	}

	st, err := detonate.LoadRun("run-1")
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if st.State != detonate.StateCreated {
		t.Errorf("state = %s, want Created for the fresh run", st.State)
	}
}

// TestFullLifecycle_AdvancesStateAtEveryStep drives create -> inject -> run
// -> collect -> destroy and checks the persisted state after each step,
// verifying both the state-file read-back and the FakeRunner call log.
func TestFullLifecycle_AdvancesStateAtEveryStep(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	fake.SetGoldenExists("golden-1", true)
	fake.SetPoweredOff("run-1", true)
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	requireState := func(t *testing.T, want detonate.State) {
		t.Helper()
		st, err := detonate.LoadRun("run-1")
		if err != nil {
			t.Fatalf("LoadRun: %v", err)
		}
		if st.State != want {
			t.Fatalf("state = %s, want %s", st.State, want)
		}
	}

	if err := runCreate(context.Background(), fake, "run-1", "golden-1"); err != nil {
		t.Fatalf("runCreate() error = %v", err)
	}
	requireState(t, detonate.StateCreated)
	if !hasCall(fake.Log, "Clone") {
		t.Error("Clone was not called")
	}

	sample := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(sample, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runInject(context.Background(), fake, "run-1", sample); err != nil {
		t.Fatalf("runInject() error = %v", err)
	}
	requireState(t, detonate.StateInjected)
	if !hasCall(fake.Log, "InjectOffline") {
		t.Error("InjectOffline was not called")
	}

	if err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader("")); err != nil {
		t.Fatalf("runRun() error = %v", err)
	}
	requireState(t, detonate.StateDetonated)
	if !hasCall(fake.Log, "Run") {
		t.Error("Run was not called")
	}

	outDir := t.TempDir()
	if err := runCollect(context.Background(), fake, "run-1", outDir); err != nil {
		t.Fatalf("runCollect() error = %v", err)
	}
	requireState(t, detonate.StateCollected)
	if !hasCall(fake.Log, "Collect") {
		t.Error("Collect was not called")
	}

	if err := runDestroy(context.Background(), fake, "run-1"); err != nil {
		t.Fatalf("runDestroy() error = %v", err)
	}
	if !hasCall(fake.Log, "Destroy") {
		t.Error("Destroy was not called")
	}
	if _, err := detonate.LoadRun("run-1"); !os.IsNotExist(err) {
		t.Errorf("LoadRun after destroy = err %v, want os.IsNotExist (state cleared)", err)
	}

	// And a fresh create is now possible again.
	if err := runCreate(context.Background(), fake, "run-1", "golden-1"); err != nil {
		t.Fatalf("runCreate() after full lifecycle destroy error = %v, want nil", err)
	}
	requireState(t, detonate.StateCreated)
}

// ─── inject ─────────────────────────────────────────────────────────────

func TestRunInject_AuditsBeforeRunnerCall(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedCreated(t, fake, "run-1")
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
	entries := auditEntriesByAction(t, "detonate-inject")
	if len(entries) != 1 {
		t.Fatalf("unexpected detonate-inject audit entries: %+v", entries)
	}
	wantHash := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if entries[0].Details["sha256"] != wantHash {
		t.Errorf("audit sha256 = %q, want %q", entries[0].Details["sha256"], wantHash)
	}

	// State must not advance past Created when InjectOffline fails closed.
	st, err := detonate.LoadRun("run-1")
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if st.State != detonate.StateCreated {
		t.Errorf("state = %s, want still Created after a failed inject", st.State)
	}
}

// ─── inject: state gate ─────────────────────────────────────────────────

func TestRunInject_FailsClosedBeforeCreate(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	sample := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(sample, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := runInject(context.Background(), fake, "never-created", sample)
	if err == nil {
		t.Fatal("runInject() error = nil, want error when run was never created")
	}
	if hasCall(fake.Log, "InjectOffline") {
		t.Error("InjectOffline was called despite no prior create")
	}
}

// ─── run ────────────────────────────────────────────────────────────────

func TestRunRun_FailsClosedWhenNotIsolated(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedInjected(t, fake, "run-1")
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
	seedInjected(t, fake, "run-1")
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

	entries := auditEntriesByAction(t, "detonate-run")
	if len(entries) != 1 {
		t.Fatalf("unexpected detonate-run audit entries: %+v", entries)
	}
	if entries[0].Details["error"] == "" {
		t.Error("audit entry missing error detail")
	}
	if entries[0].Details["auto_destroyed"] != "true" {
		t.Error("audit entry missing auto_destroyed=true")
	}

	// Auto-destroy succeeded, so state must be cleared too — the run name is
	// free for a fresh create.
	if _, err := detonate.LoadRun("run-1"); !os.IsNotExist(err) {
		t.Errorf("LoadRun after auto-destroy = err %v, want os.IsNotExist (state should be cleared)", err)
	}
}

func TestRunRun_DestroyFailureIsNotReportedAsAutoDestroyed(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedInjected(t, fake, "run-1")
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

	entries := auditEntriesByAction(t, "detonate-run")
	if len(entries) != 1 {
		t.Fatalf("unexpected detonate-run audit entries: %+v", entries)
	}
	if entries[0].Details["destroy_error"] == "" {
		t.Error("audit entry missing destroy_error detail")
	}
	if entries[0].Details["auto_destroyed"] != "" {
		t.Error("audit entry must not claim auto_destroyed=true when Destroy failed")
	}

	// State was marked Detonated before boot (not after success), precisely
	// so this case — Run fails AND auto-destroy also fails — still leaves
	// the run blocked from reuse rather than sitting at Injected (which
	// would still permit another `run`). Only an explicit destroy (always
	// allowed) can clear it from here.
	st, err := detonate.LoadRun("run-1")
	if err != nil {
		t.Fatalf("LoadRun after failed auto-destroy: %v", err)
	}
	if st.State != detonate.StateDetonated {
		t.Errorf("state = %s, want Detonated (marked before boot) when auto-destroy itself failed", st.State)
	}
}

// TestRunRun_FailsClosedWhenLockHeld covers the TOCTOU closure: a run name
// with its lock already held (standing in for a second, concurrent `run`
// invocation) must fail closed rather than race the lock holder to
// Runner.Run.
func TestRunRun_FailsClosedWhenLockHeld(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedInjected(t, fake, "run-1")
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	unlock, err := detonate.LockRun("run-1")
	if err != nil {
		t.Fatalf("LockRun() error = %v", err)
	}
	defer unlock()

	before := len(fake.Log)
	err = runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want error while the run's lock is already held")
	}
	if len(fake.Log) != before {
		t.Errorf("Runner was touched despite the lock being held: new calls %+v", fake.Log[before:])
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

func TestRunRun_RejectsInvalidGateway(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	err := runRun(context.Background(), fake, "run-1", "-not-a-name", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want error for a leading-dash --gateway value")
	}
	if len(fake.Log) != 0 {
		t.Errorf("Runner should not be touched before --gateway is validated, got: %+v", fake.Log)
	}
}

func TestRunRun_ConfirmationGate(t *testing.T) {
	setTempAuditPath(t)

	// Each subtest uses its own run name: they share the parent's
	// DETONATE_STATE_DIR (t.Setenv applies for the whole parent test), so
	// reusing "run-1" across subtests would trip the new create/run state
	// gate against a leftover state file from an earlier subtest.

	t.Run("wrong phrase aborts without calling Run", func(t *testing.T) {
		fake := detonate.NewFakeRunner()
		seedInjected(t, fake, "gate-wrong")
		err := runRun(context.Background(), fake, "gate-wrong", "gw0", time.Second, false, strings.NewReader("nope\n"))
		if err == nil {
			t.Fatal("runRun() error = nil, want confirmation mismatch error")
		}
		if hasCall(fake.Log, "Run") {
			t.Error("Runner.Run was called despite a wrong confirmation phrase")
		}
	})

	t.Run("exact phrase proceeds to Run", func(t *testing.T) {
		fake := detonate.NewFakeRunner()
		seedInjected(t, fake, "gate-exact")
		err := runRun(context.Background(), fake, "gate-exact", "gw0", time.Second, false, strings.NewReader("detonate gate-exact\n"))
		if err != nil {
			t.Fatalf("runRun() error = %v, want nil", err)
		}
		if !hasCall(fake.Log, "Run") {
			t.Error("Runner.Run was not called despite the exact confirmation phrase")
		}
	})

	t.Run("--yes alone (no env) still prompts", func(t *testing.T) {
		fake := detonate.NewFakeRunner()
		seedInjected(t, fake, "gate-yes")
		err := runRun(context.Background(), fake, "gate-yes", "gw0", time.Second, true, strings.NewReader(""))
		if err == nil {
			t.Fatal("runRun() error = nil, want error: --yes without DETONATE_I_UNDERSTAND=1 must not bypass the prompt")
		}
		if hasCall(fake.Log, "Run") {
			t.Error("Runner.Run was called despite --yes without the env var")
		}
	})
}

// ─── run: state gate (the no-reuse enforcement) ────────────────────────

func TestRunRun_FailsClosedBeforeInject(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedCreated(t, fake, "run-1") // created, but never injected
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want error when run was created but never injected")
	}
	if !strings.Contains(err.Error(), "not been injected") {
		t.Errorf("runRun() error = %v, want it to name the missing inject step", err)
	}
	if hasCall(fake.Log, "Run") {
		t.Error("Runner.Run was called despite state=Created (not Injected)")
	}
}

// TestRunRun_RefusesReuseOfAlreadyDetonatedRun is the core no-reuse
// enforcement test: a second `run` on a run that already completed
// successfully (and was never destroyed) must fail closed rather than
// re-boot the same clone against the live sample a second time.
func TestRunRun_RefusesReuseOfAlreadyDetonatedRun(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedDetonated(t, fake, "run-1")

	before := len(fake.Log)
	err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want error on a second run of an already-detonated clone")
	}
	if !strings.Contains(err.Error(), "already detonated") {
		t.Errorf("runRun() error = %v, want it to say the run is already detonated", err)
	}
	if len(fake.Log) != before {
		t.Errorf("Runner was touched on the reuse-blocked path: new calls %+v", fake.Log[before:])
	}

	// State must still be Detonated — the blocked second run must not have
	// perturbed it.
	st, loadErr := detonate.LoadRun("run-1")
	if loadErr != nil {
		t.Fatalf("LoadRun: %v", loadErr)
	}
	if st.State != detonate.StateDetonated {
		t.Errorf("state = %s, want still Detonated after the blocked reuse attempt", st.State)
	}
}

// TestRunRun_RefusesReuseOfCollectedRun exercises the CanTransition gate on a
// state the old `!= StateInjected` check also covered, but here the point is
// the policy is now the tested State.CanTransition, not an ad-hoc compare: a
// Collected run cannot transition to Detonated, so `run` must refuse it.
func TestRunRun_RefusesReuseOfCollectedRun(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedDetonated(t, fake, "run-1")
	fake.SetPoweredOff("run-1", true)
	if err := runCollect(context.Background(), fake, "run-1", t.TempDir()); err != nil {
		t.Fatalf("seed collect: %v", err)
	}

	before := len(fake.Log)
	err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want refusal for a run already in Collected state")
	}
	if !strings.Contains(err.Error(), "already detonated") {
		t.Errorf("runRun() error = %v, want the no-reuse refusal (CanTransition path)", err)
	}
	if len(fake.Log) != before {
		t.Errorf("Runner was touched on the CanTransition-blocked path: new calls %+v", fake.Log[before:])
	}
}

// TestRunRun_SkipsCleanupWhenRecreatedDuringRun is the Fix-2 integration test:
// while r.Run is blocked, an out-of-band destroy+recreate replaces run-1 with a
// brand-new run (new nonce). When r.Run then errors, runRun must NOT auto-
// destroy or wipe the new run — the nonce guard aborts the cleanup.
func TestRunRun_SkipsCleanupWhenRecreatedDuringRun(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedInjected(t, fake, "run-1")
	t.Setenv("DETONATE_I_UNDERSTAND", "1")

	fake.SetRunErr("run-1", fmt.Errorf("boom"))
	// Simulate the operator racing us mid-detonation: destroy this run and
	// create a fresh one under the same name with a different nonce.
	fake.SetRunHook("run-1", func() {
		if err := detonate.DeleteRun("run-1"); err != nil {
			t.Errorf("out-of-band DeleteRun: %v", err)
		}
		recreated := &detonate.Run{Name: "run-1", Golden: "golden-seed", State: detonate.StateCreated, Nonce: "nonce-C", CreatedAt: time.Now()}
		if err := detonate.SaveRun(recreated); err != nil {
			t.Errorf("out-of-band recreate: %v", err)
		}
	})

	err := runRun(context.Background(), fake, "run-1", "gw0", time.Second, true, strings.NewReader(""))
	if err == nil {
		t.Fatal("runRun() error = nil, want the run error surfaced")
	}
	if !strings.Contains(err.Error(), "recreated concurrently") {
		t.Errorf("runRun() error = %v, want it to report the concurrent recreate", err)
	}

	// The brand-new run must be untouched: still Created, still nonce-C.
	st, err := detonate.LoadRun("run-1")
	if err != nil {
		t.Fatalf("recreated run's state was wiped: %v", err)
	}
	if st.State != detonate.StateCreated || st.Nonce != "nonce-C" {
		t.Errorf("recreated run corrupted: state=%s nonce=%q, want Created/nonce-C", st.State, st.Nonce)
	}
	// And we must NOT have destroyed the new run's clone.
	if hasCall(fake.Log, "Destroy") {
		t.Error("auto-destroy ran against the recreated run's clone — nonce guard failed")
	}
}

// ─── collect ────────────────────────────────────────────────────────────

func TestRunCollect_FailsClosedWhenNotPoweredOff(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedDetonated(t, fake, "run-1")
	fake.SetPoweredOff("run-1", false)

	err := runCollect(context.Background(), fake, "run-1", t.TempDir())
	if err == nil {
		t.Fatal("runCollect() error = nil, want error when not powered off")
	}
	if hasCall(fake.Log, "Collect") {
		t.Error("Collect was called despite PoweredOff=false")
	}
}

// ─── collect: state gate ───────────────────────────────────────────────

func TestRunCollect_FailsClosedBeforeDetonation(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedInjected(t, fake, "run-1") // injected, never run
	fake.SetPoweredOff("run-1", true)

	err := runCollect(context.Background(), fake, "run-1", t.TempDir())
	if err == nil {
		t.Fatal("runCollect() error = nil, want error when run was never detonated")
	}
	if hasCall(fake.Log, "Collect") {
		t.Error("Collect was called despite state=Injected (not Detonated)")
	}
}

func TestRunCollect_HashesArtifactsAndAudits(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedDetonated(t, fake, "run-1")
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

	entries := auditEntriesByAction(t, "detonate-collect")
	if len(entries) != 1 {
		t.Fatalf("unexpected detonate-collect audit entries: %+v", entries)
	}
	if entries[0].Details["count"] != "1" {
		t.Errorf("audit count = %q, want %q", entries[0].Details["count"], "1")
	}
	wantHash := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if !strings.Contains(entries[0].Details["artifacts"], wantHash) {
		t.Errorf("audit artifacts missing sha256(%q): %q", "hello", entries[0].Details["artifacts"])
	}

	// Collect fully succeeded, so state must advance to Collected.
	st, err := detonate.LoadRun("run-1")
	if err != nil {
		t.Fatalf("LoadRun: %v", err)
	}
	if st.State != detonate.StateCollected {
		t.Errorf("state = %s, want Collected after a successful collect", st.State)
	}
}

// TestRunCollect_AuditsPartialFilesWhenCollectItselfErrors covers the case
// TartRunner.Collect actually hits: a mid-loop failure that still returns
// the artifacts already copied to outDir. runCollect must hash and audit
// those before surfacing the collect error — otherwise the already-copied
// files sit on disk with zero chain-of-custody trail.
func TestRunCollect_AuditsPartialFilesWhenCollectItselfErrors(t *testing.T) {
	setTempAuditPath(t)
	fake := detonate.NewFakeRunner()
	seedDetonated(t, fake, "run-1")
	fake.SetPoweredOff("run-1", true)

	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.log")
	f2 := filepath.Join(dir, "b.log")
	if err := os.WriteFile(f1, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("world"), 0o600); err != nil {
		t.Fatal(err)
	}
	collectErr := fmt.Errorf("writing artifact c.log: disk full")
	fake.SetCollectResult("run-1", []string{f1, f2}, collectErr)

	err := runCollect(context.Background(), fake, "run-1", dir)
	if err == nil {
		t.Fatal("runCollect() error = nil, want the Collect error surfaced")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("runCollect() error = %v, want it to wrap the Collect error", err)
	}

	entries := auditEntriesByAction(t, "detonate-collect")
	if len(entries) != 1 {
		t.Fatalf("unexpected detonate-collect audit entries: %+v — already-copied artifacts must still get a chain-of-custody entry", entries)
	}
	if entries[0].Details["count"] != "2" {
		t.Errorf("audit count = %q, want %q", entries[0].Details["count"], "2")
	}
	wantHash1 := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	wantHash2 := "486ea46224d1bb4fb680f34f7c9ad96a8f24ec88be73ea8e5a6c65260e9cb8a7"
	if !strings.Contains(entries[0].Details["artifacts"], wantHash1) {
		t.Errorf("audit artifacts missing sha256(a.log): %q", entries[0].Details["artifacts"])
	}
	if !strings.Contains(entries[0].Details["artifacts"], wantHash2) {
		t.Errorf("audit artifacts missing sha256(b.log): %q", entries[0].Details["artifacts"])
	}

	// A mid-loop collect error must leave state at Detonated so collect can
	// be retried, rather than advancing to Collected.
	st, loadErr := detonate.LoadRun("run-1")
	if loadErr != nil {
		t.Fatalf("LoadRun: %v", loadErr)
	}
	if st.State != detonate.StateDetonated {
		t.Errorf("state = %s, want still Detonated after a mid-loop collect error", st.State)
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
