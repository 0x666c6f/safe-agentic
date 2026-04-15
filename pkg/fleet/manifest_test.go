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
  ssh: true
agents:
  - name: worker-a
    type: claude
    repo: https://github.com/org/api.git
    reuse_auth: false
    ssh: false
`)
	m, err := ParseFleet(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := m.Agents[0].ReuseAuth; got {
		t.Fatalf("ReuseAuth = %v, want false", got)
	}
	if got := m.Agents[0].SSH; got {
		t.Fatalf("SSH = %v, want false", got)
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
    on_failure: fix-tests
  - name: fix-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: Fix failing tests
    retry: 2
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
	if s0.Agents[0].OnFailure != "fix-tests" {
		t.Errorf("stage[0].Agents[0].OnFailure = %q", s0.Agents[0].OnFailure)
	}

	s1 := m.Stages[1]
	if s1.Agents[0].Retry != 2 {
		t.Errorf("stage[1].Agents[0].Retry = %d, want 2", s1.Agents[0].Retry)
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

func TestParsePipeline_WhenOutputs(t *testing.T) {
	p := writeTemp(t, `
name: test-pipeline
steps:
  - name: check
    type: claude
    repo: https://github.com/org/r.git
    prompt: test
    outputs: "echo pass"
  - name: act
    type: claude
    repo: https://github.com/org/r.git
    prompt: test
    depends_on: check
    when: pass
`)
	m, err := ParsePipeline(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Stages[0].Agents[0].Outputs != "echo pass" {
		t.Errorf("Outputs = %q", m.Stages[0].Agents[0].Outputs)
	}
	if m.Stages[1].Agents[0].When != "pass" {
		t.Errorf("When = %q", m.Stages[1].Agents[0].When)
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
