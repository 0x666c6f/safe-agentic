package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPipeline_DryRunNestedExample(t *testing.T) {
	pipelineDryRun = true
	defer func() { pipelineDryRun = false }()

	path := filepath.Join("..", "..", "examples", "pipeline-display-nested.yaml")
	output := captureOutput(func() {
		if err := runPipeline(pipelineCmd, []string{path}); err != nil {
			t.Fatalf("runPipeline() error = %v", err)
		}
	})

	if !strings.Contains(output, "display-nested") {
		t.Fatalf("dry-run output missing pipeline name:\n%s", output)
	}
	if !strings.Contains(output, "level-1") {
		t.Fatalf("dry-run output missing top stage:\n%s", output)
	}
	if !strings.Contains(output, "root-leaf") {
		t.Fatalf("dry-run output missing root leaf stage:\n%s", output)
	}
	if !strings.Contains(output, "examples/pipeline-display-nested-child.yaml") {
		t.Fatalf("dry-run output missing child pipeline path:\n%s", output)
	}
	if strings.Contains(output, "Agent codex started") {
		t.Fatalf("dry-run unexpectedly started agent:\n%s", output)
	}
}

func TestRunPipeline_BackgroundDelegatesToLauncher(t *testing.T) {
	origBackground := pipelineBackground
	origLauncher := launchDetachedPipeline
	defer func() {
		pipelineBackground = origBackground
		launchDetachedPipeline = origLauncher
	}()

	pipelineBackground = true
	var gotManifest string
	launchDetachedPipeline = func(manifestPath string) error {
		gotManifest = manifestPath
		return nil
	}

	path := filepath.Join("..", "..", "examples", "pipeline-display-nested.yaml")
	output := captureOutput(func() {
		if err := runPipeline(pipelineCmd, []string{path}); err != nil {
			t.Fatalf("runPipeline() error = %v", err)
		}
	})

	if gotManifest != path {
		t.Fatalf("launcher manifest = %q, want %q", gotManifest, path)
	}
	if output != "" {
		t.Fatalf("expected no direct pipeline output, got:\n%s", output)
	}
}
