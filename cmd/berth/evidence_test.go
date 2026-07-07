package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0x666c6f/berth/pkg/audit"
	"github.com/0x666c6f/berth/pkg/vmexec"
)

// setTempAuditPath redirects audit.DefaultPath() to a temp dir for the test.
func setTempAuditPath(t *testing.T) {
	t.Helper()
	t.Setenv("BERTH_STATE_HOME", t.TempDir())
}

func TestIngestEvidence_DryRun(t *testing.T) {
	setTempAuditPath(t)
	fake := vmexec.NewFake()

	origPopulate := populateEvidenceVolume
	called := false
	populateEvidenceVolume = func(vmName, volName, imageName, hostPath string) error {
		called = true
		return nil
	}
	defer func() { populateEvidenceVolume = origPopulate }()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "evidence.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	vol, err := ingestEvidence(context.Background(), fake, "berth", "agent-1", root, "berth:latest", true)
	if err != nil {
		t.Fatalf("ingestEvidence() error = %v", err)
	}
	if vol != "agent-1-evidence" {
		t.Errorf("volume name = %q, want %q", vol, "agent-1-evidence")
	}
	if called {
		t.Error("populateEvidenceVolume was called during dry run")
	}
	if matches := fake.CommandsMatching("docker volume create"); len(matches) != 0 {
		t.Errorf("docker volume create called during dry run: %v", matches)
	}

	entries, err := (&audit.Logger{Path: audit.DefaultPath()}).Read(0)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Action != "evidence-ingest" || e.Container != "agent-1" {
		t.Errorf("unexpected audit entry: %+v", e)
	}
	if e.Details["count"] != "1" {
		t.Errorf("audit count = %q, want %q", e.Details["count"], "1")
	}
	if !strings.Contains(e.Details["files"], "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824") {
		t.Errorf("audit files field missing sha256(%q): %q", "hello", e.Details["files"])
	}
}

func TestIngestEvidence_NonexistentPath(t *testing.T) {
	setTempAuditPath(t)
	fake := vmexec.NewFake()

	origPopulate := populateEvidenceVolume
	called := false
	populateEvidenceVolume = func(vmName, volName, imageName, hostPath string) error {
		called = true
		return nil
	}
	defer func() { populateEvidenceVolume = origPopulate }()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := ingestEvidence(context.Background(), fake, "berth", "agent-1", missing, "berth:latest", false)
	if err == nil {
		t.Fatal("expected error for nonexistent host path, got nil")
	}
	if called {
		t.Error("populateEvidenceVolume was called despite Build() error")
	}
	if len(fake.Log) != 0 {
		t.Errorf("expected no Docker calls, got %v", fake.Log)
	}

	entries, err := (&audit.Logger{Path: audit.DefaultPath()}).Read(0)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no audit entries, got %d", len(entries))
	}
}

// TestIngestEvidence_PopulateFailureCleansUpVolume covers Fix 1: if populate
// fails after the volume was created, ingestEvidence must best-effort remove
// the now-orphaned volume (the spawn aborts, so the stop-path cleanup never
// runs for it) and still surface the original populate error.
func TestIngestEvidence_PopulateFailureCleansUpVolume(t *testing.T) {
	setTempAuditPath(t)
	fake := vmexec.NewFake()

	origPopulate := populateEvidenceVolume
	populateErr := errors.New("populate boom")
	populateEvidenceVolume = func(vmName, volName, imageName, hostPath string) error {
		return populateErr
	}
	defer func() { populateEvidenceVolume = origPopulate }()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "evidence.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ingestEvidence(context.Background(), fake, "berth", "agent-1", root, "berth:latest", false)
	if !errors.Is(err, populateErr) {
		t.Fatalf("ingestEvidence() error = %v, want wrapped %v", err, populateErr)
	}

	if matches := fake.CommandsMatching("docker volume rm agent-1-evidence"); len(matches) != 1 {
		t.Errorf("docker volume rm agent-1-evidence calls = %d, want 1 (log: %v)", len(matches), fake.Log)
	}
}
