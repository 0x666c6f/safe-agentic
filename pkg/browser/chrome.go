package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type ChromeCaptureOptions struct {
	URL         string
	OutDir      string
	ChromePath  string
	NodePath    string
	Timeout     time.Duration
	Annotations []string
}

type chromeResult struct {
	StatusCode  int    `json:"statusCode"`
	ContentType string `json:"contentType"`
}

func CaptureChrome(ctx context.Context, opts ChromeCaptureOptions) (Artifact, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if err := validateURL(opts.URL); err != nil {
		return Artifact{}, err
	}
	outDir := opts.OutDir
	if outDir == "" {
		outDir = filepath.Join(os.TempDir(), "safe-ag-browser")
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("create output dir: %w", err)
	}
	nodePath := opts.NodePath
	if nodePath == "" {
		nodePath = "node"
	}
	chromePath := opts.ChromePath
	if chromePath == "" {
		chromePath = DetectChrome()
	}
	if chromePath == "" {
		return Artifact{}, fmt.Errorf("chrome executable not found; pass --chrome-path or set SAFE_AGENTIC_CHROME")
	}

	scriptPath := filepath.Join(outDir, "capture.mjs")
	annotationsPath := filepath.Join(outDir, "annotations.json")
	if err := os.WriteFile(scriptPath, []byte(chromeCaptureScript), 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write capture script: %w", err)
	}
	if err := writeAnnotations(annotationsPath, opts.Annotations); err != nil {
		return Artifact{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout+5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, nodePath, scriptPath, chromePath, opts.URL, outDir, fmt.Sprintf("%d", opts.Timeout.Milliseconds()))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Artifact{}, fmt.Errorf("chrome capture: %w\nstderr: %s", err, stderr.String())
	}
	var result chromeResult
	if err := json.Unmarshal(bytes.TrimSpace(out), &result); err != nil {
		return Artifact{}, fmt.Errorf("parse chrome capture result: %w\nstdout: %s\nstderr: %s", err, out, stderr.String())
	}

	artifactPath := filepath.Join(outDir, "artifact.json")
	artifact := Artifact{
		URL:         opts.URL,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
		StatusCode:  result.StatusCode,
		ContentType: result.ContentType,
		Mode:        "chrome-cdp",
		Files: map[string]string{
			"dom":         filepath.Join(outDir, "dom.html"),
			"screenshot":  filepath.Join(outDir, "screenshot.png"),
			"console":     filepath.Join(outDir, "console.json"),
			"network":     filepath.Join(outDir, "network.json"),
			"annotations": annotationsPath,
			"artifact":    artifactPath,
		},
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

func DetectChrome() string {
	if path := strings.TrimSpace(os.Getenv("SAFE_AGENTIC_CHROME")); path != "" {
		return path
	}
	candidates := []string{
		"google-chrome-stable",
		"google-chrome",
		"chromium",
		"chromium-browser",
	}
	if runtime.GOOS == "darwin" {
		candidates = append([]string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}, candidates...)
	}
	for _, candidate := range candidates {
		if filepath.IsAbs(candidate) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}

const chromeCaptureScript = `
import { spawn } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const [chromePath, targetURL, outDir, timeoutMsRaw] = process.argv.slice(2);
const timeoutMs = Number(timeoutMsRaw || "30000");
await mkdir(outDir, { recursive: true, mode: 0o700 });
const profileDir = await mkdtemp(join(tmpdir(), "safe-ag-chrome-"));

let browser;
let wsURL = "";
let stderr = "";
try {
  browser = spawn(chromePath, [
    "--headless=new",
    "--disable-gpu",
    "--no-first-run",
    "--no-default-browser-check",
    "--disable-extensions",
    "--remote-debugging-port=0",
    "--user-data-dir=" + profileDir,
    "about:blank",
  ], { stdio: ["ignore", "ignore", "pipe"] });

  wsURL = await new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("timed out waiting for DevTools endpoint: " + stderr)), timeoutMs);
    browser.stderr.on("data", (chunk) => {
      stderr += chunk.toString();
      const match = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
      if (match) {
        clearTimeout(timer);
        resolve(match[1]);
      }
    });
    browser.on("exit", (code) => {
      clearTimeout(timer);
      reject(new Error("chrome exited before DevTools endpoint, code=" + code + ": " + stderr));
    });
  });

  const ws = new WebSocket(wsURL);
  await onceOpen(ws, timeoutMs);
  let id = 0;
  const pending = new Map();
  let sessionId = "";
  const consoleEvents = [];
  const networkEvents = [];
  let mainStatus = 0;
  let mainContentType = "";

  ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);
    if (msg.id && pending.has(msg.id)) {
      pending.get(msg.id)(msg);
      pending.delete(msg.id);
      return;
    }
    if (msg.method === "Runtime.consoleAPICalled") {
      consoleEvents.push({
        type: msg.params.type,
        text: (msg.params.args || []).map((arg) => arg.value ?? arg.description ?? "").join(" "),
      });
    }
    if (msg.method === "Network.responseReceived") {
      const response = msg.params.response;
      networkEvents.push({
        url: response.url,
        status: response.status,
        mimeType: response.mimeType,
      });
      if (response.url === targetURL || msg.params.type === "Document") {
        mainStatus = response.status;
        mainContentType = response.mimeType || "";
      }
    }
  };

  const send = (method, params = {}, sid = "") => new Promise((resolve, reject) => {
    const request = { id: ++id, method, params };
    if (sid) request.sessionId = sid;
    pending.set(request.id, (msg) => {
      if (msg.error) reject(new Error(JSON.stringify(msg.error)));
      else resolve(msg.result || {});
    });
    ws.send(JSON.stringify(request));
  });

  const target = await send("Target.createTarget", { url: "about:blank" });
  const attached = await send("Target.attachToTarget", { targetId: target.targetId, flatten: true });
  sessionId = attached.sessionId;
  await send("Page.enable", {}, sessionId);
  await send("Runtime.enable", {}, sessionId);
  await send("Network.enable", {}, sessionId);

  const loaded = new Promise((resolve) => {
    const oldHandler = ws.onmessage;
    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data);
      if (msg.method === "Page.loadEventFired" && msg.sessionId === sessionId) resolve();
      oldHandler(event);
    };
  });
  await send("Page.navigate", { url: targetURL }, sessionId);
  await Promise.race([loaded, delay(timeoutMs)]);
  await delay(500);

  const dom = await send("Runtime.evaluate", {
    expression: "document.documentElement.outerHTML",
    returnByValue: true,
  }, sessionId);
  await writeFile(join(outDir, "dom.html"), dom.result?.value || "", { mode: 0o600 });

  const screenshot = await send("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: true,
    fromSurface: true,
  }, sessionId);
  await writeFile(join(outDir, "screenshot.png"), Buffer.from(screenshot.data, "base64"), { mode: 0o600 });
  await writeFile(join(outDir, "console.json"), JSON.stringify(consoleEvents, null, 2), { mode: 0o600 });
  await writeFile(join(outDir, "network.json"), JSON.stringify(networkEvents, null, 2), { mode: 0o600 });
  await send("Browser.close");
  console.log(JSON.stringify({ statusCode: mainStatus, contentType: mainContentType }));
} finally {
  if (browser && !browser.killed) browser.kill("SIGTERM");
  await rm(profileDir, { recursive: true, force: true });
}

function onceOpen(ws, timeoutMs) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("websocket open timeout")), timeoutMs);
    ws.onopen = () => {
      clearTimeout(timer);
      resolve();
    };
    ws.onerror = (err) => {
      clearTimeout(timer);
      reject(err);
    };
  });
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
`
