package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0x666c6f/safe-agentic/pkg/config"
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

func TestResolveTemplateFallsBackToLegacyUserTemplates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacyDir := filepath.Join(home, ".config", "safe-agentic", "templates")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "legacy.md"), []byte("legacy body"), 0o644); err != nil {
		t.Fatalf("write legacy template: %v", err)
	}
	asset, err := ResolveTemplate("legacy")
	if err != nil {
		t.Fatalf("ResolveTemplate() error = %v", err)
	}
	if asset.Source != SourceUser {
		t.Fatalf("Source = %q", asset.Source)
	}
	if asset.Body != "legacy body" {
		t.Fatalf("Body = %q", asset.Body)
	}
}

func TestResolvePipelineFallsBackToLegacyUserPipelines(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacyDir := filepath.Join(filepath.Dir(config.LegacyDefaultsPath()), "pipelines")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy pipelines: %v", err)
	}
	body := "name: legacy\nsteps:\n  - name: review\n    type: claude\n    prompt: test\n"
	if err := os.WriteFile(filepath.Join(legacyDir, "legacy.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write legacy pipeline: %v", err)
	}
	asset, err := ResolvePipeline("legacy")
	if err != nil {
		t.Fatalf("ResolvePipeline() error = %v", err)
	}
	if asset.Source != SourceUser {
		t.Fatalf("Source = %q", asset.Source)
	}
}
