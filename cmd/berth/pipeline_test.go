package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0x666c6f/berth/pkg/catalog"
	"github.com/0x666c6f/berth/pkg/config"
	"github.com/0x666c6f/berth/pkg/fleet"
	"github.com/0x666c6f/berth/pkg/vmexec"
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

func TestOpenDetachedPipelineLogCreatesPrivateLog(t *testing.T) {
	stateHome := t.TempDir()

	logPath, logFile, err := openDetachedPipelineLog(stateHome, "pipeline.yaml")
	if err != nil {
		t.Fatalf("openDetachedPipelineLog() error = %v", err)
	}
	if err := logFile.Close(); err != nil {
		t.Fatalf("close log file: %v", err)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("pipeline log mode = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(logPath))
	if err != nil {
		t.Fatalf("stat log dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("pipeline log dir mode = %o, want 700", got)
	}
}

func TestSpawnPipelineAgentIncludesStageInHierarchy(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	stage := fleet.PipelineStage{Name: "claude-reviews"}
	spec := fleet.AgentSpec{
		Name:   "code-review-claude",
		Type:   "claude",
		Repo:   "https://github.com/org/repo.git",
		Prompt: "review",
	}

	output := captureOutput(func() {
		opts := specToSpawnOpts(spec, "pipeline-123")
		opts.Hierarchy = pipelineStageHierarchy([]string{"double-review-reconcile"}, stage.Name)
		opts.DryRun = true
		err := executeSpawn(opts)
		if err != nil {
			t.Fatalf("executeSpawn error = %v", err)
		}
	})

	if !strings.Contains(output, "berth.hierarchy=double-review-reconcile/claude-reviews") {
		t.Fatalf("dry-run missing stage hierarchy label:\n%s", output)
	}
}

func TestPipelineContainerSuffixIsSafeAndStable(t *testing.T) {
	got := pipelineContainerSuffix(
		fleet.PipelineStage{Name: "review/stage"},
		fleet.AgentSpec{Name: "code review $(touch /tmp/pwned)", Type: "claude"},
		[]string{"root pipeline"},
		"20260410-120000",
	)
	want := "root-pipeline-review-stage-code-review-touch-tmp-pwned-20260410-120000"
	if got != want {
		t.Fatalf("pipelineContainerSuffix() = %q, want %q", got, want)
	}
}

func TestSpawnPipelineAgentReturnsActualContainerName(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	stage := fleet.PipelineStage{Name: "review"}
	spec := fleet.AgentSpec{
		Name:   "code-review",
		Type:   "claude",
		Repo:   "https://github.com/org/repo.git",
		Prompt: "review",
	}

	got, err := spawnPipelineAgent(stage, spec, "pipeline-123", []string{"root"}, "20260410-120000")
	if err != nil {
		t.Fatalf("spawnPipelineAgent() error = %v", err)
	}
	want := "agent-claude-root-review-code-review-20260410-120000"
	if got != want {
		t.Fatalf("spawnPipelineAgent() name = %q, want %q", got, want)
	}
	cmds := fake.CommandsMatching("docker run")
	if len(cmds) == 0 {
		t.Fatal("expected docker run command")
	}
	joined := strings.Join(cmds[0], " ")
	if !strings.Contains(joined, "--name "+want) {
		t.Fatalf("docker run used different container name:\n%s", joined)
	}
}

func TestWaitForContainers_FetchesExitCodeOnceOnSuccess(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker inspect --format {{.State.Status}} agent-1", "exited")
	fake.SetResponse("docker inspect --format {{.State.ExitCode}} agent-1", "0")

	output := captureOutput(func() {
		if err := waitForContainers(context.Background(), fake, []string{"agent-1"}); err != nil {
			t.Fatalf("waitForContainers() error = %v", err)
		}
	})

	if !strings.Contains(output, "✓ agent-1 exited") {
		t.Fatalf("expected success output, got:\n%s", output)
	}
	if got := len(fake.CommandsMatching("docker inspect --format {{.State.ExitCode}} agent-1")); got != 1 {
		t.Fatalf("exit code inspected %d times, want 1", got)
	}
}

func TestResolvePipelineManifestFromUserCatalog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(config.PipelinesDir(), 0o755); err != nil {
		t.Fatalf("mkdir pipelines dir: %v", err)
	}
	want := filepath.Join(config.PipelinesDir(), "review.yaml")
	if err := os.WriteFile(want, []byte("steps: []\n"), 0o644); err != nil {
		t.Fatalf("write pipeline: %v", err)
	}
	got, err := resolvePipelineManifest("review")
	if err != nil {
		t.Fatalf("resolvePipelineManifest() error = %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRunPipelineCreate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	output := captureOutput(func() {
		if err := runPipelineCreate(pipelineCreateCmd, []string{"review"}); err != nil {
			t.Fatalf("runPipelineCreate() error = %v", err)
		}
	})
	if !strings.Contains(output, "Created pipeline:") {
		t.Fatalf("unexpected output: %s", output)
	}
	data, err := os.ReadFile(filepath.Join(config.PipelinesDir(), "review.yaml"))
	if err != nil {
		t.Fatalf("read created pipeline: %v", err)
	}
	if !strings.Contains(string(data), "repo: ${repo}") {
		t.Fatalf("starter pipeline missing ${repo}:\n%s", data)
	}
}

func TestSpawnTemplateInterpolatesVars(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(config.TemplatesDir(), 0o755); err != nil {
		t.Fatalf("mkdir templates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(config.TemplatesDir(), "review.md"), []byte("Review ${repo} for ${topic}."), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	output := captureOutput(func() {
		err := executeSpawn(SpawnOpts{
			AgentType:    "claude",
			Repos:        []string{"https://github.com/org/repo.git"},
			Template:     "review",
			TemplateVars: []string{"topic=security"},
			DryRun:       true,
		})
		if err != nil {
			t.Fatalf("executeSpawn() error = %v", err)
		}
	})
	if !strings.Contains(output, `berth.prompt=Review https://github.com/org/repo.git for security.`) {
		t.Fatalf("dry-run missing interpolated prompt:\n%s", output)
	}
}

func TestParseTemplateVarsInfersPR(t *testing.T) {
	origRepo := inferRepoFromCurrent
	origPR := inferPRFromCurrent
	defer func() {
		inferRepoFromCurrent = origRepo
		inferPRFromCurrent = origPR
	}()

	inferRepoFromCurrent = func() (string, error) {
		return "https://github.com/org/repo.git", nil
	}
	inferPRFromCurrent = func() (string, error) {
		return "16", nil
	}

	vars, repos, err := parseTemplateVars(nil, nil, true)
	if err != nil {
		t.Fatalf("parseTemplateVars() error = %v", err)
	}
	if got := repos[0]; got != "https://github.com/org/repo.git" {
		t.Fatalf("repos[0] = %q", got)
	}
	if got := vars["pr"]; got != "16" {
		t.Fatalf("vars[pr] = %q", got)
	}
}

func TestParseTemplateVarsExplicitPROverridesInference(t *testing.T) {
	origPR := inferPRFromCurrent
	defer func() { inferPRFromCurrent = origPR }()
	inferPRFromCurrent = func() (string, error) {
		return "16", nil
	}

	vars, _, err := parseTemplateVars([]string{"pr=99"}, nil, false)
	if err != nil {
		t.Fatalf("parseTemplateVars() error = %v", err)
	}
	if got := vars["pr"]; got != "99" {
		t.Fatalf("vars[pr] = %q", got)
	}
}

func TestParsePRReviewArgs(t *testing.T) {
	mode, pr, err := parsePRReviewArgs(nil)
	if err != nil || mode != "dual" || pr != "" {
		t.Fatalf("default parse = (%q,%q,%v)", mode, pr, err)
	}
	mode, pr, err = parsePRReviewArgs([]string{"claude", "16"})
	if err != nil || mode != "claude" || pr != "16" {
		t.Fatalf("explicit parse = (%q,%q,%v)", mode, pr, err)
	}
	mode, pr, err = parsePRReviewArgs([]string{"17"})
	if err != nil || mode != "dual" || pr != "17" {
		t.Fatalf("implicit pr parse = (%q,%q,%v)", mode, pr, err)
	}
}

func TestResolveBuiltinReviewPreset(t *testing.T) {
	asset, err := catalog.ResolveReviewPreset("dual")
	if err != nil {
		t.Fatalf("ResolveReviewPreset() error = %v", err)
	}
	if asset.Manifest.Name != "dual" {
		t.Fatalf("preset name = %q", asset.Manifest.Name)
	}
	if asset.Source != catalog.SourceBuiltin {
		t.Fatalf("preset source = %q", asset.Source)
	}
}

// ─── judge stage ───────────────────────────────────────────────────────────────

func TestRunPipeline_DryRunJudgeFanoutExample(t *testing.T) {
	pipelineDryRun = true
	defer func() { pipelineDryRun = false }()

	path := filepath.Join("..", "..", "examples", "pipeline-judge-fanout.yaml")
	output := captureOutput(func() {
		if err := runPipeline(pipelineCmd, []string{path}); err != nil {
			t.Fatalf("runPipeline() error = %v", err)
		}
	})
	for _, want := range []string{"judge-fanout", "implement-claude", "implement-codex", "pick-winner", "🏆 judge", "auto PR"} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run tree missing %q:\n%s", want, output)
		}
	}
}

func TestRunPipelineManifest_JudgeStageEndToEnd(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	// Every spawned candidate + judge container reports a clean exit.
	fake.SetResponse("docker inspect --format {{.State.Status}}", "exited")
	fake.SetResponse("docker inspect --format {{.State.ExitCode}}", "0")

	// Stub the judge seams: capture the real candidate names discovered from the
	// preceding stage, and vote the first one the winner.
	var capturedNames []string
	var spawnCount int
	origSpawn := judgeSpawn
	origRead := judgeReadOutput
	origCollect := judgeCollectCandidates
	defer func() {
		judgeSpawn = origSpawn
		judgeReadOutput = origRead
		judgeCollectCandidates = origCollect
	}()
	judgeCollectCandidates = func(_ context.Context, _ vmexec.Executor, names []string, _ string) ([]JudgeCandidate, error) {
		capturedNames = names
		cands := make([]JudgeCandidate, len(names))
		for i, n := range names {
			cands[i] = JudgeCandidate{Name: n}
		}
		return cands, nil
	}
	judgeSpawn = func(opts SpawnOpts) error {
		spawnCount++
		if opts.AgentType != "claude" || !opts.Background {
			t.Errorf("unexpected judge spawn opts: %#v", opts)
		}
		return nil
	}
	judgeReadOutput = func(_ context.Context, _ vmexec.Executor, _ string) string {
		return `{"winner":"` + capturedNames[0] + `","reason":"r","summary":"s"}`
	}

	m := &fleet.PipelineManifest{
		Name: "p",
		Stages: []fleet.PipelineStage{
			{
				Name: "implement",
				Agents: []fleet.AgentSpec{
					{Name: "a", Type: "claude", Repo: "https://github.com/org/r.git", Prompt: "A"},
					{Name: "b", Type: "codex", Repo: "https://github.com/org/r.git", Prompt: "B"},
				},
			},
			{
				Name:      "pick",
				DependsOn: []string{"implement"},
				Judge:     &fleet.JudgeSpec{Criteria: "best"},
			},
		},
	}

	output := captureOutput(func() {
		if err := runPipelineManifest(context.Background(), fake, m, fleet.ParseOptions{}, false, "20260101-000000", "", nil); err != nil {
			t.Fatalf("runPipelineManifest() error = %v", err)
		}
	})

	if spawnCount != 1 {
		t.Fatalf("judge spawn count = %d, want 1", spawnCount)
	}
	if len(capturedNames) != 2 {
		t.Fatalf("judge saw %d candidate containers, want 2 (from implement stage): %v", len(capturedNames), capturedNames)
	}
	if !strings.Contains(output, "selected winner: "+capturedNames[0]) {
		t.Fatalf("expected winner announcement, got:\n%s", output)
	}
	if !strings.Contains(output, `Pipeline "p" complete.`) {
		t.Fatalf("pipeline did not complete cleanly:\n%s", output)
	}
	// The two candidates spawn real containers; the judge stage does not spawn a
	// normal agent (it goes through the stubbed judge seam).
	if got := len(fake.CommandsMatching("docker run")); got != 2 {
		t.Fatalf("docker run count = %d, want 2 (candidates only)", got)
	}
	verdictPath := filepath.Join(os.Getenv("HOME"), ".berth", "state", "judge", "p-pick-20260101-000000.json")
	if _, err := os.Stat(verdictPath); err != nil {
		t.Fatalf("verdict not persisted: %v", err)
	}
}
