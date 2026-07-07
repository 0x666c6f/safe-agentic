package main

import (
	"strings"
	"testing"
)

func TestColorize_DisabledReturnsPlain(t *testing.T) {
	orig := observeColorEnabled
	observeColorEnabled = func() bool { return false }
	defer func() { observeColorEnabled = orig }()

	if got := colorize("0;36", "hello"); got != "hello" {
		t.Fatalf("expected plain text with color disabled, got %q", got)
	}
}

func TestColorize_EnabledWrapsANSI(t *testing.T) {
	orig := observeColorEnabled
	observeColorEnabled = func() bool { return true }
	defer func() { observeColorEnabled = orig }()

	got := colorize("0;36", "hello")
	if !strings.HasPrefix(got, "\033[0;36m") || !strings.HasSuffix(got, "\033[0m") || !strings.Contains(got, "hello") {
		t.Fatalf("expected ANSI-wrapped text, got %q", got)
	}
}

func TestRenderingWriter_RendersAcrossChunkBoundaries(t *testing.T) {
	orig := observeColorEnabled
	observeColorEnabled = func() bool { return false }
	defer func() { observeColorEnabled = orig }()

	var out strings.Builder
	rw := &renderingWriter{out: &out}

	line := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello world"}]}}` + "\n"
	// Feed the JSONL entry split across two writes to exercise partial-line buffering.
	mid := len(line) / 2
	if _, err := rw.Write([]byte(line[:mid])); err != nil {
		t.Fatalf("write: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no output before newline arrives, got %q", out.String())
	}
	if _, err := rw.Write([]byte(line[mid:])); err != nil {
		t.Fatalf("write: %v", err)
	}
	rw.flush()

	if !strings.Contains(out.String(), "hello world") {
		t.Fatalf("expected rendered entry, got %q", out.String())
	}
}
