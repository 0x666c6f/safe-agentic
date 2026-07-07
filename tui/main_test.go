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
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "'container' not found") {
		t.Fatalf("preflight missing container err = %v", err)
	}

	installFakeBinary(t, "container", `if [ "$1 $2" = "system status" ]; then exit 0; fi
if [ "$1 $2" = "machine list" ]; then exit 1; fi`)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "failed to list VMs") {
		t.Fatalf("preflight list err = %v", err)
	}

	installFakeBinary(t, "container", `if [ "$1 $2" = "system status" ]; then exit 0; fi
if [ "$1 $2" = "machine list" ]; then echo '[{"id":"other-vm"}]'; exit 0; fi`)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "VM 'berth' not found") {
		t.Fatalf("preflight missing vm err = %v", err)
	}

	installFakeBinary(t, "container", `if [ "$1 $2" = "system status" ]; then exit 0; fi
if [ "$1 $2" = "machine list" ]; then echo '[{"id":"berth"}]'; exit 0; fi`)
	if err := preflight(); err != nil {
		t.Fatalf("preflight success err = %v", err)
	}
}

func TestPreflight_ContainerSystemStopped(t *testing.T) {
	installFakeBinary(t, "container", `if [ "$1 $2" = "system status" ]; then exit 1; fi`)
	if err := preflight(); err == nil || !strings.Contains(err.Error(), "Apple container system is stopped") {
		t.Fatalf("preflight stopped err = %v", err)
	}
}

func TestPreflight_CustomVMName(t *testing.T) {
	installFakeBinary(t, "container", `if [ "$1 $2" = "system status" ]; then exit 0; fi
if [ "$1 $2" = "machine list" ]; then echo '[{"id":"custom-vm"}]'; exit 0; fi`)
	t.Setenv("BERTH_VM_NAME", "custom-vm")
	if err := preflight(); err != nil {
		t.Fatalf("preflight custom vm err = %v", err)
	}
}
