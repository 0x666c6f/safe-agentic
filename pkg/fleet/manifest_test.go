package fleet

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeProfile(t *testing.T, dir, name, content string) string {
	t.Helper()
	profileDir := filepath.Join(dir, ".berth", "agents")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	path := filepath.Join(profileDir, name+".toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	return profileDir
}

// ─── agent type validation ───────────────────────────────────────────────────

func TestParseFleet_MissingTypeIsHardError(t *testing.T) {
	p := writeTemp(t, `agents:
  - name: worker
    repo: git@github.com:o/r.git
`)
	_, err := ParseFleet(p)
	if err == nil {
		t.Fatal("expected a hard error for an agent missing type")
	}
	for _, want := range []string{"worker", "missing a type", "claude, codex, shell"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestParseFleet_UnknownTypeIsHardError(t *testing.T) {
	p := writeTemp(t, `agents:
  - name: worker
    type: gpt
    repo: git@github.com:o/r.git
`)
	_, err := ParseFleet(p)
	if err == nil {
		t.Fatal("expected a hard error for an unknown agent type")
	}
	for _, want := range []string{"worker", "unknown type", "gpt", "claude, codex, shell"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err.Error(), want)
		}
	}
}

func TestParsePipeline_MissingTypeIsHardError(t *testing.T) {
	p := writeTemp(t, `name: p
steps:
  - name: build
    repo: git@github.com:o/r.git
    prompt: do it
`)
	_, err := ParsePipeline(p)
	if err == nil {
		t.Fatal("expected a hard error for a stage agent missing type")
	}
	if !strings.Contains(err.Error(), "build") || !strings.Contains(err.Error(), "type") {
		t.Errorf("error should name the stage and mention type: %v", err)
	}
}

// ─── FleetManifest ──────────────────────────────────────────────────────────

func TestParseFleet_TwoAgents(t *testing.T) {
	p := writeTemp(t, `
agents:
  - name: worker-a
    type: claude
    repo: https://github.com/org/api.git
    ssh: true
    reuse_auth: true
    prompt: Fix the CI tests
  - name: worker-b
    type: codex
    repo: https://github.com/org/frontend.git
    memory: 16g
    cpus: "8"
`)
	m, err := ParseFleet(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Agents) != 2 {
		t.Fatalf("want 2 agents, got %d", len(m.Agents))
	}

	a := m.Agents[0]
	if a.Name != "worker-a" {
		t.Errorf("agent[0].Name = %q, want worker-a", a.Name)
	}
	if a.Type != "claude" {
		t.Errorf("agent[0].Type = %q, want claude", a.Type)
	}
	if a.Repo != "https://github.com/org/api.git" {
		t.Errorf("agent[0].Repo = %q", a.Repo)
	}
	if !a.SSH {
		t.Error("agent[0].SSH should be true")
	}
	if !a.ReuseAuth {
		t.Error("agent[0].ReuseAuth should be true")
	}
	if a.Prompt != "Fix the CI tests" {
		t.Errorf("agent[0].Prompt = %q", a.Prompt)
	}

	b := m.Agents[1]
	if b.Type != "codex" {
		t.Errorf("agent[1].Type = %q, want codex", b.Type)
	}
	if b.Memory != "16g" {
		t.Errorf("agent[1].Memory = %q, want 16g", b.Memory)
	}
	if b.SSH {
		t.Error("agent[1].SSH should be false")
	}
}

func TestParseFleet_SharedTasks(t *testing.T) {
	p := writeTemp(t, `
name: test-fleet
shared_tasks: true
agents:
  - name: a
    type: claude
    repo: https://github.com/org/r.git
    prompt: test
`)
	m, err := ParseFleet(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "test-fleet" {
		t.Errorf("Name = %q, want test-fleet", m.Name)
	}
	if !m.SharedTasks {
		t.Error("SharedTasks should be true")
	}
	if len(m.Agents) != 1 {
		t.Fatalf("want 1 agent, got %d", len(m.Agents))
	}
}

func TestParseFleet_EmptyAgents(t *testing.T) {
	p := writeTemp(t, "agents:\n")
	m, err := ParseFleet(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Agents) != 0 {
		t.Errorf("want 0 agents, got %d", len(m.Agents))
	}
}

func TestParseFleet_ExplicitFalseOverridesDefaultTrue(t *testing.T) {
	p := writeTemp(t, `
defaults:
  reuse_auth: true
  seed_auth: true
  ssh: true
  allow_setup_scripts: true
agents:
  - name: worker-a
    type: claude
    repo: https://github.com/org/api.git
    reuse_auth: false
    seed_auth: false
    ssh: false
    allow_setup_scripts: false
`)
	m, err := ParseFleet(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := m.Agents[0].ReuseAuth; got {
		t.Fatalf("ReuseAuth = %v, want false", got)
	}
	if got := m.Agents[0].SeedAuth; got {
		t.Fatalf("SeedAuth = %v, want false", got)
	}
	if got := m.Agents[0].SSH; got {
		t.Fatalf("SSH = %v, want false", got)
	}
	if got := m.Agents[0].AllowSetupScripts; got {
		t.Fatalf("AllowSetupScripts = %v, want false", got)
	}
}

func TestParseFleet_ProfileInheritance(t *testing.T) {
	dir := t.TempDir()
	profileDir := writeProfile(t, dir, "reviewer", `
agent_type = "codex"
repo = ["git@github.com:org/profile.git"]
container_name = "profile-name"
prompt = "Profile prompt ${topic}"
template = "code-review"
template_vars = ["repo=${repo}"]
instructions = "Profile instructions ${topic}"
ssh = true
reuse_auth = true
reuse_gh_auth = true
memory = "12g"
pids_limit = 256
`)
	manifest := filepath.Join(dir, "manifest.yaml")
	body := `
vars:
  topic: auth
defaults:
  profile: reviewer
agents:
  - name: worker
    prompt: "Override ${topic}"
    reuse_auth: false
`
	if err := os.WriteFile(manifest, []byte(body), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	m, err := ParseFleetWithOptions(manifest, ParseOptions{
		ProfileDirs:  []string{profileDir},
		DefaultRepos: []string{"git@github.com:org/default.git"},
	})
	if err != nil {
		t.Fatalf("ParseFleetWithOptions() error = %v", err)
	}
	got := m.Agents[0]
	if got.Type != "codex" {
		t.Fatalf("Type = %q, want codex", got.Type)
	}
	if got.Name != "worker" {
		t.Fatalf("Name = %q, want worker", got.Name)
	}
	if got.Repos[0] != "git@github.com:org/profile.git" {
		t.Fatalf("Repos = %v", got.Repos)
	}
	if got.Prompt != "Override auth" {
		t.Fatalf("Prompt = %q, want Override auth", got.Prompt)
	}
	if got.Template != "code-review" || got.Instructions != "Profile instructions auth" {
		t.Fatalf("template/instructions not inherited: %#v", got)
	}
	if got.TemplateVars[0] != "repo=git@github.com:org/default.git" {
		t.Fatalf("TemplateVars = %v", got.TemplateVars)
	}
	if !got.SSH || !got.ReuseGHAuth {
		t.Fatalf("expected SSH and ReuseGHAuth from profile: %#v", got)
	}
	if got.ReuseAuth {
		t.Fatalf("ReuseAuth = true, want explicit manifest false")
	}
	if got.Memory != "12g" || got.PIDsLimit != 256 {
		t.Fatalf("resources not inherited: mem=%q pids=%d", got.Memory, got.PIDsLimit)
	}
}

func TestParseFleet_ProfileMissing(t *testing.T) {
	p := writeTemp(t, `
agents:
  - profile: nope
`)
	_, err := ParseFleetWithOptions(p, ParseOptions{ProfileDirs: []string{t.TempDir()}})
	if err == nil || !strings.Contains(err.Error(), `profile "nope" not found`) {
		t.Fatalf("ParseFleetWithOptions() error = %v, want missing profile", err)
	}
}

func TestParseFleet_MissingFile(t *testing.T) {
	_, err := ParseFleet("/nonexistent/file.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ─── PipelineManifest (flat steps) ──────────────────────────────────────────

func TestParsePipeline_FlatSteps(t *testing.T) {
	p := writeTemp(t, `
name: test-and-fix
steps:
  - name: run-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: Run all tests
  - name: fix-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: Fix failing tests
  - name: create-pr
    type: claude
    repo: git@github.com:org/api.git
    prompt: Create a PR
    depends_on: fix-tests
`)
	m, err := ParsePipeline(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "test-and-fix" {
		t.Errorf("Name = %q, want test-and-fix", m.Name)
	}
	if len(m.Stages) != 3 {
		t.Fatalf("want 3 stages, got %d", len(m.Stages))
	}

	s0 := m.Stages[0]
	if s0.Name != "run-tests" {
		t.Errorf("stage[0].Name = %q", s0.Name)
	}
	if len(s0.DependsOn) != 0 {
		t.Errorf("stage[0] should have no deps, got %v", s0.DependsOn)
	}

	s1 := m.Stages[1]
	if s1.Name != "fix-tests" {
		t.Errorf("stage[1].Name = %q, want fix-tests", s1.Name)
	}

	s2 := m.Stages[2]
	if len(s2.DependsOn) != 1 || s2.DependsOn[0] != "fix-tests" {
		t.Errorf("stage[2].DependsOn = %v, want [fix-tests]", s2.DependsOn)
	}
}

func TestParsePipeline_Stages(t *testing.T) {
	p := writeTemp(t, `
stages:
  - name: analyze
    agents:
      - name: analyzer
        type: claude
        repo: https://github.com/org/repo.git
        prompt: Analyze the codebase
  - name: implement
    depends_on:
      - analyze
    agents:
      - name: implementer
        type: claude
        repo: https://github.com/org/repo.git
        prompt: Implement the feature
`)
	m, err := ParsePipeline(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Stages) != 2 {
		t.Fatalf("want 2 stages, got %d", len(m.Stages))
	}
	if m.Stages[1].DependsOn[0] != "analyze" {
		t.Errorf("stage[1].DependsOn[0] = %q, want analyze", m.Stages[1].DependsOn[0])
	}
}

func TestParsePipeline_ProfileInheritance(t *testing.T) {
	dir := t.TempDir()
	profileDir := writeProfile(t, dir, "fixer", `
agent_type = "claude"
repo = ["git@github.com:org/api.git"]
ssh = true
seed_auth = true
docker = true
`)
	manifest := filepath.Join(dir, "pipeline.yaml")
	body := `
steps:
  - name: fix
    profile: fixer
    prompt: Fix it
`
	if err := os.WriteFile(manifest, []byte(body), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, err := ParsePipelineWithOptions(manifest, ParseOptions{ProfileDirs: []string{profileDir}})
	if err != nil {
		t.Fatalf("ParsePipelineWithOptions() error = %v", err)
	}
	got := m.Stages[0].Agents[0]
	if got.Type != "claude" || got.Repos[0] != "git@github.com:org/api.git" || !got.SSH || !got.SeedAuth || !got.Docker {
		t.Fatalf("profile fields not inherited: %#v", got)
	}
}

func TestParsePipeline_RejectsUnsupportedControls(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "on_failure",
			yaml: "on_failure: fix",
			want: "unsupported on_failure",
		},
		{
			name: "retry",
			yaml: "retry: 2",
			want: "unsupported retry",
		},
		{
			name: "when",
			yaml: "when: pass",
			want: "unsupported when",
		},
		{
			name: "outputs",
			yaml: `outputs: "echo pass"`,
			want: "unsupported outputs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeTemp(t, `
name: test-pipeline
steps:
  - name: check
    type: claude
    repo: https://github.com/org/r.git
    prompt: test
    `+tt.yaml+`
`)
			_, err := ParsePipeline(p)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParsePipeline() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestParsePipeline_ModelExpansionRewritesDependencies(t *testing.T) {
	p := writeTemp(t, `
stages:
  - name: review
    models: [claude, codex]
    agents:
      - name: checker
        prompt: Review with ${model}
  - name: followup
    depends_on:
      - review
    agents:
      - name: fixer
        type: claude
        prompt: Fix it
`)
	m, err := ParsePipeline(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Stages) != 3 {
		t.Fatalf("want 3 stages after model expansion, got %d", len(m.Stages))
	}
	if m.Stages[0].Name != "review-claude" || m.Stages[1].Name != "review-codex" {
		t.Fatalf("unexpected expanded stage names: %q, %q", m.Stages[0].Name, m.Stages[1].Name)
	}
	if got := m.Stages[0].Agents[0].Prompt; got != "Review with claude" {
		t.Fatalf("expanded prompt = %q", got)
	}
	deps := m.Stages[2].DependsOn
	if len(deps) != 2 {
		t.Fatalf("want 2 rewritten deps, got %v", deps)
	}
	depSet := map[string]bool{deps[0]: true, deps[1]: true}
	if !depSet["review-claude"] || !depSet["review-codex"] {
		t.Fatalf("unexpected rewritten deps: %v", deps)
	}
}

func TestParsePipeline_DoubleReviewReconcileExample(t *testing.T) {
	m, err := ParsePipeline(filepath.Join("..", "..", "examples", "pipeline-double-review-reconcile.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "double-review-reconcile" {
		t.Fatalf("Name = %q, want double-review-reconcile", m.Name)
	}
	if len(m.Stages) != 3 {
		t.Fatalf("want 3 stages, got %d", len(m.Stages))
	}

	if got := m.Stages[0].Name; got != "claude-reviews" {
		t.Fatalf("stage[0].Name = %q", got)
	}
	if got := m.Stages[1].Name; got != "codex-reviews" {
		t.Fatalf("stage[1].Name = %q", got)
	}
	if got := m.Stages[2].Name; got != "reconcile-fix-pr" {
		t.Fatalf("stage[2].Name = %q", got)
	}

	if len(m.Stages[0].Agents) != 5 {
		t.Fatalf("stage[0] agent count = %d, want 5", len(m.Stages[0].Agents))
	}
	if len(m.Stages[1].Agents) != 5 {
		t.Fatalf("stage[1] agent count = %d, want 5", len(m.Stages[1].Agents))
	}
	if len(m.Stages[2].Agents) != 1 {
		t.Fatalf("stage[2] agent count = %d, want 1", len(m.Stages[2].Agents))
	}

	for i, agent := range m.Stages[0].Agents {
		if agent.Type != "claude" {
			t.Fatalf("stage[0].Agents[%d].Type = %q", i, agent.Type)
		}
	}
	for i, agent := range m.Stages[1].Agents {
		if agent.Type != "codex" {
			t.Fatalf("stage[1].Agents[%d].Type = %q", i, agent.Type)
		}
	}
	if got := m.Stages[2].Agents[0].Type; got != "codex" {
		t.Fatalf("stage[2].Agents[0].Type = %q", got)
	}

	if deps := m.Stages[1].DependsOn; len(deps) != 0 {
		t.Fatalf("stage[1].DependsOn = %v, want []", deps)
	}
	deps := m.Stages[2].DependsOn
	if len(deps) != 2 {
		t.Fatalf("stage[2].DependsOn = %v, want [claude-reviews codex-reviews]", deps)
	}
	depSet := map[string]bool{deps[0]: true, deps[1]: true}
	if !depSet["claude-reviews"] || !depSet["codex-reviews"] {
		t.Fatalf("unexpected stage[2] deps: %v", deps)
	}
}

func TestParsePipeline_MissingFile(t *testing.T) {
	_, err := ParsePipeline("/nonexistent/file.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParsePipelineWithOptions_InterpolatesReposAndPrompt(t *testing.T) {
	p := writeTemp(t, `
steps:
  - name: review
    type: claude
    repo: ${repo}
    prompt: Review ${topic}
`)
	m, err := ParsePipelineWithOptions(p, ParseOptions{
		Vars:         map[string]string{"topic": "the API"},
		DefaultRepos: []string{"https://github.com/org/repo.git"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := m.Stages[0].Agents[0].Repo; got != "https://github.com/org/repo.git" {
		t.Fatalf("Repo = %q", got)
	}
	if got := m.Stages[0].Agents[0].Prompt; got != "Review the API" {
		t.Fatalf("Prompt = %q", got)
	}
}

func TestParsePipelineWithOptions_AppliesDefaultRepos(t *testing.T) {
	p := writeTemp(t, `
steps:
  - name: review
    type: claude
    prompt: Review
`)
	m, err := ParsePipelineWithOptions(p, ParseOptions{
		DefaultRepos: []string{"https://github.com/org/repo.git"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := m.Stages[0].Agents[0].Repo; got != "https://github.com/org/repo.git" {
		t.Fatalf("Repo = %q", got)
	}
}

func TestParsePipelineWithOptions_ResolvesNestedPipelineRelativePath(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "child.yaml")
	if err := os.WriteFile(child, []byte("steps:\n  - name: noop\n    type: claude\n    prompt: ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(dir, "parent.yaml")
	if err := os.WriteFile(parent, []byte("stages:\n  - name: nested\n    pipeline: child.yaml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := ParsePipelineWithOptions(parent, ParseOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := m.Stages[0].Pipeline; got != child {
		t.Fatalf("Pipeline = %q, want %q", got, child)
	}
}

func TestParsePipelineWithOptions_UnresolvedVarFails(t *testing.T) {
	p := writeTemp(t, `
steps:
  - name: review
    type: claude
    prompt: Review ${missing}
`)
	_, err := ParsePipelineWithOptions(p, ParseOptions{})
	if err == nil || !strings.Contains(err.Error(), "unresolved variables") {
		t.Fatalf("err = %v, want unresolved variables", err)
	}
}

func TestParsePipelineWithOptions_PreservesManifestRepoVarOnInference(t *testing.T) {
	p := writeTemp(t, `
vars:
  repo: https://github.com/org/from-manifest.git
steps:
  - name: review
    type: claude
    repo: ${repo}
    prompt: Review
`)
	m, err := ParsePipelineWithOptions(p, ParseOptions{
		DefaultRepos: []string{"https://github.com/org/inferred.git"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := m.Stages[0].Agents[0].Repo; got != "https://github.com/org/from-manifest.git" {
		t.Fatalf("Repo = %q, want manifest repo", got)
	}
}

// ─── Judge stages ─────────────────────────────────────────────────────────────

func TestParsePipeline_JudgeFlatStep(t *testing.T) {
	// Flat-steps form: a single `implement` step fans out across models and the
	// judge step depends on that parent (rewritten to the expanded candidates).
	p := writeTemp(t, `
name: best-of-two
defaults:
  repo: https://github.com/org/repo.git
steps:
  - name: implement
    models: [claude, codex]
    prompt: Implement it with ${model}
  - name: pick-winner
    judge:
      criteria: correctness first, then minimal diff
      auto_pr: true
      base: main
    depends_on: implement
`)
	m, err := ParsePipeline(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// implement-claude, implement-codex, pick-winner
	if len(m.Stages) != 3 {
		t.Fatalf("want 3 stages, got %d", len(m.Stages))
	}
	judge := m.Stages[2]
	if judge.Name != "pick-winner" {
		t.Fatalf("stage[2].Name = %q, want pick-winner", judge.Name)
	}
	if judge.Judge == nil {
		t.Fatal("stage[2].Judge is nil, want populated JudgeSpec")
	}
	if len(judge.Agents) != 0 {
		t.Fatalf("judge stage must have no agents, got %d", len(judge.Agents))
	}
	if !judge.Judge.AutoPR || judge.Judge.Base != "main" {
		t.Fatalf("judge spec = %#v", judge.Judge)
	}
	if judge.Judge.Criteria != "correctness first, then minimal diff" {
		t.Fatalf("criteria = %q", judge.Judge.Criteria)
	}
	deps := map[string]bool{}
	for _, d := range judge.DependsOn {
		deps[d] = true
	}
	if !deps["implement-claude"] || !deps["implement-codex"] {
		t.Fatalf("judge deps = %v, want expanded implement-claude/implement-codex", judge.DependsOn)
	}
}

func TestParsePipeline_JudgeModelFanoutCandidateCount(t *testing.T) {
	p := writeTemp(t, `
name: fanout-judge
defaults:
  repo: https://github.com/org/repo.git
stages:
  - name: implement
    models: [claude, codex]
    agents:
      - name: impl
        prompt: Implement with ${model}
  - name: pick
    judge:
      criteria: pick the best
    depends_on:
      - implement
`)
	m, err := ParsePipeline(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// implement-claude, implement-codex, pick
	if len(m.Stages) != 3 {
		t.Fatalf("want 3 stages, got %d", len(m.Stages))
	}
	judge := m.Stages[2]
	if judge.Judge == nil {
		t.Fatal("expected judge stage")
	}
	deps := map[string]bool{}
	for _, d := range judge.DependsOn {
		deps[d] = true
	}
	if !deps["implement-claude"] || !deps["implement-codex"] {
		t.Fatalf("judge deps not expanded across models: %v", judge.DependsOn)
	}
}

func TestParsePipeline_JudgeCriteriaInterpolation(t *testing.T) {
	p := writeTemp(t, `
name: interp-judge
vars:
  focus: security
stages:
  - name: implement
    agents:
      - name: a
        type: claude
        repo: https://github.com/org/repo.git
        prompt: A
      - name: b
        type: codex
        repo: https://github.com/org/repo.git
        prompt: B
  - name: pick
    judge:
      criteria: prioritize ${focus}
    depends_on:
      - implement
`)
	m, err := ParsePipeline(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := m.Stages[1].Judge.Criteria; got != "prioritize security" {
		t.Fatalf("criteria = %q, want interpolated", got)
	}
}

func TestParsePipeline_JudgeValidationRejections(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "judge with prompt",
			yaml: `
steps:
  - name: a
    type: claude
    repo: https://github.com/org/r.git
    prompt: A
  - name: b
    type: codex
    repo: https://github.com/org/r.git
    prompt: B
  - name: pick
    prompt: should not be here
    judge:
      criteria: x
    depends_on: a
`,
			want: "must not define agent fields",
		},
		{
			name: "judge with only one candidate run",
			yaml: `
stages:
  - name: implement
    agents:
      - name: only
        type: claude
        repo: https://github.com/org/r.git
        prompt: X
  - name: pick
    judge:
      criteria: x
    depends_on:
      - implement
`,
			want: "at least 2 candidate runs",
		},
		{
			name: "judge without depends_on",
			yaml: `
steps:
  - name: a
    type: claude
    repo: https://github.com/org/r.git
    prompt: A
  - name: pick
    judge:
      criteria: x
`,
			want: "must declare depends_on",
		},
		{
			name: "judge depends on unknown stage",
			yaml: `
stages:
  - name: implement
    agents:
      - name: a
        type: claude
        repo: https://github.com/org/r.git
        prompt: A
      - name: b
        type: codex
        repo: https://github.com/org/r.git
        prompt: B
  - name: pick
    judge:
      criteria: x
    depends_on:
      - nonexistent
`,
			want: `unknown stage "nonexistent"`,
		},
		{
			name: "judge stage with agents",
			yaml: `
stages:
  - name: implement
    agents:
      - name: a
        type: claude
        repo: https://github.com/org/r.git
        prompt: A
      - name: b
        type: codex
        repo: https://github.com/org/r.git
        prompt: B
  - name: pick
    judge:
      criteria: x
    depends_on:
      - implement
    agents:
      - name: bogus
        type: claude
        repo: https://github.com/org/r.git
        prompt: nope
`,
			want: "must not define agents",
		},
		{
			name: "judge with invalid base branch",
			yaml: `
stages:
  - name: implement
    agents:
      - name: a
        type: claude
        repo: https://github.com/org/r.git
        prompt: A
      - name: b
        type: codex
        repo: https://github.com/org/r.git
        prompt: B
  - name: pick
    judge:
      criteria: x
      base: "bad;branch"
    depends_on:
      - implement
`,
			want: "invalid base branch",
		},
		{
			name: "judge cannot judge another judge",
			yaml: `
stages:
  - name: implement
    agents:
      - name: a
        type: claude
        repo: https://github.com/org/r.git
        prompt: A
      - name: b
        type: codex
        repo: https://github.com/org/r.git
        prompt: B
  - name: pick
    judge:
      criteria: x
    depends_on:
      - implement
  - name: pick2
    judge:
      criteria: y
    depends_on:
      - pick
`,
			want: "cannot judge another judge stage",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeTemp(t, tt.yaml)
			_, err := ParsePipeline(p)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParsePipeline() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestParsePipeline_JudgeTwoAgentSingleStageIsValid(t *testing.T) {
	p := writeTemp(t, `
name: single-stage-fanout
stages:
  - name: implement
    agents:
      - name: a
        type: claude
        repo: https://github.com/org/r.git
        prompt: A
      - name: b
        type: codex
        repo: https://github.com/org/r.git
        prompt: B
  - name: pick
    judge:
      criteria: best
    depends_on:
      - implement
`)
	if _, err := ParsePipeline(p); err != nil {
		t.Fatalf("expected valid pipeline, got error: %v", err)
	}
}

func TestParsePipeline_JudgeFanoutExample(t *testing.T) {
	m, err := ParsePipeline(filepath.Join("..", "..", "examples", "pipeline-judge-fanout.yaml"))
	if err != nil {
		t.Fatalf("unexpected error parsing shipped example: %v", err)
	}
	if m.Name != "judge-fanout" {
		t.Fatalf("Name = %q, want judge-fanout", m.Name)
	}
	// implement-claude, implement-codex, pick-winner
	if len(m.Stages) != 3 {
		t.Fatalf("want 3 stages, got %d", len(m.Stages))
	}
	judge := m.Stages[2]
	if judge.Judge == nil || !judge.Judge.AutoPR {
		t.Fatalf("pick-winner should be an auto_pr judge stage: %#v", judge)
	}
	for _, s := range m.Stages[:2] {
		if len(s.Agents) != 1 {
			t.Fatalf("candidate stage %q should have exactly 1 agent, got %d", s.Name, len(s.Agents))
		}
		if s.Agents[0].Repo == "" {
			t.Fatalf("candidate stage %q agent has no repo after input default", s.Name)
		}
	}
}
