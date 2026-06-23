package browser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"
)

func TestCaptureHTTPWritesArtifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer srv.Close()

	outDir := t.TempDir()
	artifact, err := CaptureHTTP(context.Background(), CaptureOptions{
		URL:         srv.URL,
		OutDir:      outDir,
		Timeout:     5 * time.Second,
		Annotations: []string{"check header"},
	})
	if err != nil {
		t.Fatalf("CaptureHTTP() error = %v", err)
	}
	if artifact.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want 200", artifact.StatusCode)
	}
	if artifact.Mode != "http-dom" {
		t.Fatalf("Mode = %q, want http-dom", artifact.Mode)
	}
	dom, err := os.ReadFile(filepath.Join(outDir, "dom.html"))
	if err != nil {
		t.Fatalf("read dom: %v", err)
	}
	if !strings.Contains(string(dom), "<body>ok</body>") {
		t.Fatalf("dom = %q", dom)
	}
	for _, name := range []string{"dom.html", "headers.json", "annotations.json", "artifact.json"} {
		info, err := os.Stat(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o, want 600", name, got)
		}
	}
	if len(artifact.Annotations) != 1 || artifact.Annotations[0] != "check header" {
		t.Fatalf("Annotations = %#v", artifact.Annotations)
	}
}

func TestCaptureHTTPRejectsNonHTTPURL(t *testing.T) {
	_, err := CaptureHTTP(context.Background(), CaptureOptions{URL: "file:///tmp/x"})
	if err == nil || !strings.Contains(err.Error(), "unsupported url scheme") {
		t.Fatalf("CaptureHTTP() error = %v, want unsupported scheme", err)
	}
}

func TestDetectChromeUsesEnv(t *testing.T) {
	t.Setenv("SAFE_AGENTIC_CHROME", "/tmp/chrome")
	if got := DetectChrome(); got != "/tmp/chrome" {
		t.Fatalf("DetectChrome() = %q, want env path", got)
	}
}

func TestCaptureChromeRejectsNonHTTPURL(t *testing.T) {
	_, err := CaptureChrome(context.Background(), ChromeCaptureOptions{
		URL: "file:///tmp/x",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported url scheme") {
		t.Fatalf("CaptureChrome() error = %v, want unsupported scheme", err)
	}
}
