package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installFakeBinary(t *testing.T, name, body string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestPreflight(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "'orb' not found") {
		t.Fatalf("preflight missing orb err = %v", err)
	}

	installFakeBinary(t, "orb", `if [ "$1" = "list" ]; then exit 1; fi`)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "failed to list VMs") {
		t.Fatalf("preflight list err = %v", err)
	}

	installFakeBinary(t, "orb", `if [ "$1" = "list" ]; then echo "other-vm"; exit 0; fi`)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "VM 'safe-agentic' not found") {
		t.Fatalf("preflight missing vm err = %v", err)
	}

	installFakeBinary(t, "orb", `if [ "$1" = "list" ]; then echo "safe-agentic"; exit 0; fi`)
	if err := preflight(); err != nil {
		t.Fatalf("preflight success err = %v", err)
	}
}

func TestPreflight_OrbStackStopped(t *testing.T) {
	installFakeBinary(t, "orb", `if [ "$1" = "list" ]; then echo "safe-agentic"; exit 0; fi`)
	installFakeBinary(t, "orbctl", `if [ "$1" = "status" ]; then echo "Stopped"; exit 1; fi`)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "OrbStack is stopped") {
		t.Fatalf("preflight stopped err = %v", err)
	}
}

func TestPreflight_CustomVMName(t *testing.T) {
	installFakeBinary(t, "orb", `if [ "$1" = "list" ]; then echo "custom-vm"; exit 0; fi`)
	installFakeBinary(t, "orbctl", `if [ "$1" = "status" ]; then echo "Running"; exit 0; fi`)
	t.Setenv("SAFE_AGENTIC_VM_NAME", "custom-vm")
	if err := preflight(); err != nil {
		t.Fatalf("preflight custom vm err = %v", err)
	}
}
