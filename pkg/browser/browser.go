package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const maxCaptureBytes = 10 << 20

type CaptureOptions struct {
	URL         string
	OutDir      string
	Timeout     time.Duration
	Annotations []string
}

type Artifact struct {
	URL         string              `json:"url"`
	FetchedAt   string              `json:"fetched_at"`
	StatusCode  int                 `json:"status_code"`
	ContentType string              `json:"content_type"`
	Mode        string              `json:"mode"`
	Files       map[string]string   `json:"files"`
	Headers     map[string][]string `json:"headers"`
	Annotations []string            `json:"annotations,omitempty"`
}

func CaptureHTTP(ctx context.Context, opts CaptureOptions) (Artifact, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if err := validateURL(opts.URL); err != nil {
		return Artifact{}, err
	}
	outDir := opts.OutDir
	if outDir == "" {
		outDir = filepath.Join(os.TempDir(), "berth-browser")
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("create output dir: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, opts.URL, nil)
	if err != nil {
		return Artifact{}, err
	}
	req.Header.Set("User-Agent", "berth-browser-capture/1")

	client := &http.Client{Timeout: opts.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return Artifact{}, fmt.Errorf("fetch %s: %w", opts.URL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCaptureBytes+1))
	if err != nil {
		return Artifact{}, fmt.Errorf("read body: %w", err)
	}
	if len(body) > maxCaptureBytes {
		return Artifact{}, fmt.Errorf("response too large: exceeds %d bytes", maxCaptureBytes)
	}

	domPath := filepath.Join(outDir, "dom.html")
	headersPath := filepath.Join(outDir, "headers.json")
	annotationsPath := filepath.Join(outDir, "annotations.json")
	artifactPath := filepath.Join(outDir, "artifact.json")
	if err := os.WriteFile(domPath, body, 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write dom: %w", err)
	}
	headersJSON, err := json.MarshalIndent(resp.Header, "", "  ")
	if err != nil {
		return Artifact{}, fmt.Errorf("marshal headers: %w", err)
	}
	if err := os.WriteFile(headersPath, headersJSON, 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write headers: %w", err)
	}
	if err := writeAnnotations(annotationsPath, opts.Annotations); err != nil {
		return Artifact{}, err
	}

	artifact := Artifact{
		URL:         opts.URL,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Mode:        "http-dom",
		Files: map[string]string{
			"dom":         domPath,
			"headers":     headersPath,
			"annotations": annotationsPath,
			"artifact":    artifactPath,
		},
		Headers:     resp.Header,
		Annotations: opts.Annotations,
	}
	artifactJSON, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return Artifact{}, fmt.Errorf("marshal artifact: %w", err)
	}
	if err := os.WriteFile(artifactPath, artifactJSON, 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write artifact: %w", err)
	}
	return artifact, nil
}

func writeAnnotations(path string, annotations []string) error {
	if annotations == nil {
		annotations = []string{}
	}
	data, err := json.MarshalIndent(annotations, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal annotations: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write annotations: %w", err)
	}
	return nil
}

func validateURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported url scheme %q: use http or https", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("url host is required")
	}
	return nil
}
