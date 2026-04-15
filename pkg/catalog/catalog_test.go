package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindBuiltinDirRejectsArbitraryCWDTemplates(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	defer os.Chdir(orig) //nolint:errcheck
	if err := os.MkdirAll(filepath.Join(cwd, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir cwd templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cwd, "templates", "shadow.md"), []byte("shadow"), 0o644); err != nil {
		t.Fatalf("write shadow template: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	if got := builtinTemplatesDir(); got == filepath.Join(cwd, "templates") {
		t.Fatalf("builtinTemplatesDir() should not use arbitrary cwd templates dir")
	}
}
