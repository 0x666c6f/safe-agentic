package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installFakeOrbList(t *testing.T, body string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "orb")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake orb: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestPreflight(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "'orb' not found") {
		t.Fatalf("preflight missing orb err = %v", err)
	}

	installFakeOrbList(t, `if [ "$1" = "list" ]; then exit 1; fi`)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "failed to list VMs") {
		t.Fatalf("preflight list err = %v", err)
	}

	installFakeOrbList(t, `if [ "$1" = "list" ]; then echo "other-vm"; exit 0; fi`)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "VM 'safe-agentic' not found") {
		t.Fatalf("preflight missing vm err = %v", err)
	}

	installFakeOrbList(t, `if [ "$1" = "list" ]; then echo "safe-agentic"; exit 0; fi`)
	if err := preflight(); err != nil {
		t.Fatalf("preflight success err = %v", err)
	}
}
