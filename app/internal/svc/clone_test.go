package svc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalContainerName(t *testing.T) {
	c, n := localContainerName("claude", "/Users/me/My Project/")
	if c != "agent-claude-My-Project" || n != "My-Project" {
		t.Fatalf("got %q / %q", c, n)
	}
}

func TestWriteDirTarSkipsHeavyDirs(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o644)
	os.MkdirAll(filepath.Join(root, "node_modules", "x"), 0o755)
	os.WriteFile(filepath.Join(root, "node_modules", "x", "big.js"), []byte("junk"), 0o644)
	os.MkdirAll(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "src", "a.ts"), []byte("x"), 0o644)

	var buf bytes.Buffer
	if err := writeDirTar(&buf, root); err != nil {
		t.Fatal(err)
	}
	gz, _ := gzip.NewReader(&buf)
	tr := tar.NewReader(gz)
	names := map[string]bool{}
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		names[h.Name] = true
	}
	if !names["main.go"] || !names["src/a.ts"] {
		t.Fatalf("expected files missing: %v", names)
	}
	if names["node_modules/x/big.js"] {
		t.Fatalf("node_modules should be skipped: %v", names)
	}
}
