package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	browserpkg "github.com/0x666c6f/safe-agentic/pkg/browser"
	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/spf13/cobra"
)

var browserOutDir string
var browserTimeout time.Duration
var browserMode string
var browserChromePath string
var browserNodePath string
var browserAnnotations []string

var browserCmd = &cobra.Command{
	Use:     "browser",
	Short:   "Capture browser verification artifacts",
	GroupID: groupWorkflow,
}

var browserCaptureCmd = &cobra.Command{
	Use:   "capture <url>",
	Short: "Capture DOM and response headers for a URL",
	Args:  cobra.ExactArgs(1),
	RunE:  runBrowserCapture,
}

func init() {
	browserCaptureCmd.Flags().StringVar(&browserOutDir, "out", "", "Output directory; defaults to ~/.safe-ag/state/browser/<timestamp>")
	browserCaptureCmd.Flags().DurationVar(&browserTimeout, "timeout", 30*time.Second, "HTTP capture timeout")
	browserCaptureCmd.Flags().StringVar(&browserMode, "mode", "auto", "Capture mode: auto, http, or chrome")
	browserCaptureCmd.Flags().StringVar(&browserChromePath, "chrome-path", "", "Chrome/Chromium executable for --mode chrome")
	browserCaptureCmd.Flags().StringVar(&browserNodePath, "node-path", "", "Node executable for --mode chrome")
	browserCaptureCmd.Flags().StringArrayVar(&browserAnnotations, "annotation", nil, "Annotation to store with the browser artifact; repeatable")
	browserCmd.AddCommand(browserCaptureCmd)
	rootCmd.AddCommand(browserCmd)
}

func runBrowserCapture(cmd *cobra.Command, args []string) error {
	outDir := browserOutDir
	if outDir == "" {
		outDir = filepath.Join(config.StateDir(), "browser", time.Now().Format("20060102-150405"))
	}
	artifact, err := captureBrowserArtifact(context.Background(), args[0], outDir)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func captureBrowserArtifact(ctx context.Context, targetURL, outDir string) (browserpkg.Artifact, error) {
	switch browserMode {
	case "http":
		return browserpkg.CaptureHTTP(ctx, browserpkg.CaptureOptions{
			URL:         targetURL,
			OutDir:      outDir,
			Timeout:     browserTimeout,
			Annotations: browserAnnotations,
		})
	case "chrome":
		return browserpkg.CaptureChrome(ctx, browserpkg.ChromeCaptureOptions{
			URL:         targetURL,
			OutDir:      outDir,
			Timeout:     browserTimeout,
			ChromePath:  browserChromePath,
			NodePath:    browserNodePath,
			Annotations: browserAnnotations,
		})
	case "auto":
		if canRunChromeCapture() {
			artifact, err := browserpkg.CaptureChrome(ctx, browserpkg.ChromeCaptureOptions{
				URL:         targetURL,
				OutDir:      outDir,
				Timeout:     browserTimeout,
				ChromePath:  browserChromePath,
				NodePath:    browserNodePath,
				Annotations: browserAnnotations,
			})
			if err == nil {
				return artifact, nil
			}
		}
		return browserpkg.CaptureHTTP(ctx, browserpkg.CaptureOptions{
			URL:         targetURL,
			OutDir:      outDir,
			Timeout:     browserTimeout,
			Annotations: browserAnnotations,
		})
	default:
		return browserpkg.Artifact{}, fmt.Errorf("unsupported browser mode %q", browserMode)
	}
}

func canRunChromeCapture() bool {
	chromePath := browserChromePath
	if chromePath == "" {
		chromePath = browserpkg.DetectChrome()
	}
	if chromePath == "" {
		return false
	}
	nodePath := browserNodePath
	if nodePath == "" {
		nodePath = "node"
	}
	_, err := exec.LookPath(nodePath)
	return err == nil
}
