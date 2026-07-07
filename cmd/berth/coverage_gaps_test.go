package main

import (
	"strings"
	"testing"

	"github.com/0x666c6f/berth/pkg/fleet"
)

func TestConfirmProceedYesBypass(t *testing.T) {
	ok, err := confirmProceed("risky thing", true)
	if err != nil || !ok {
		t.Fatalf("confirmProceed(yes) = %v, %v; want true, nil", ok, err)
	}
}

func TestConfirmProceedNonTTYFails(t *testing.T) {
	// Test stdin is never a terminal, so without --yes this must error
	// instead of hanging on a prompt.
	ok, err := confirmProceed("risky thing", false)
	if ok || err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("confirmProceed(non-TTY) = %v, %v; want false + --yes error", ok, err)
	}
}

func TestStageNames(t *testing.T) {
	stages := []fleet.PipelineStage{{Name: "build"}, {Name: "review"}}
	got := stageNames(stages)
	if len(got) != 2 || got[0] != "build" || got[1] != "review" {
		t.Fatalf("stageNames = %v", got)
	}
	if got := stageNames(nil); len(got) != 0 {
		t.Fatalf("stageNames(nil) = %v, want empty", got)
	}
}

func TestPipelineNameFromFile(t *testing.T) {
	cases := map[string]string{
		"review.yaml":  "review",
		"review.yml":   "review",
		"review":       "",
		"review.json":  "",
		"a/b/pipe.yml": "a/b/pipe",
	}
	for in, want := range cases {
		if got := pipelineNameFromFile(in); got != want {
			t.Fatalf("pipelineNameFromFile(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyPipelineAuthOverrides(t *testing.T) {
	origSeed, origTrust := pipelineSeedAuth, pipelineAutoTrust
	defer func() { pipelineSeedAuth, pipelineAutoTrust = origSeed, origTrust }()

	manifest := func() *fleet.PipelineManifest {
		return &fleet.PipelineManifest{Stages: []fleet.PipelineStage{
			{Agents: []fleet.AgentSpec{{Type: "claude"}, {Type: "codex"}}},
			{Agents: []fleet.AgentSpec{{Type: "claude"}}},
		}}
	}

	pipelineSeedAuth, pipelineAutoTrust = false, false
	m := manifest()
	applyPipelineAuthOverrides(m)
	if m.Stages[0].Agents[0].SeedAuth || m.Stages[0].Agents[0].AutoTrust {
		t.Fatal("overrides applied without flags set")
	}

	pipelineSeedAuth, pipelineAutoTrust = true, true
	m = manifest()
	applyPipelineAuthOverrides(m)
	for i, st := range m.Stages {
		for j, a := range st.Agents {
			if !a.SeedAuth || !a.AutoTrust {
				t.Fatalf("stage %d agent %d missing overrides: %+v", i, j, a)
			}
		}
	}
}

func TestExtractAssistantTextVariants(t *testing.T) {
	if got := extractAssistantText(map[string]interface{}{"content": "plain"}); got != "plain" {
		t.Fatalf("string content = %q", got)
	}
	blocks := map[string]interface{}{"content": []interface{}{
		map[string]interface{}{"type": "text", "text": "hello"},
		map[string]interface{}{"type": "tool_use", "name": "Bash"},
		map[string]interface{}{"type": "text", "text": "world"},
	}}
	got := extractAssistantText(blocks)
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("block content = %q", got)
	}
	if got := extractAssistantText(map[string]interface{}{}); got != "" {
		t.Fatalf("missing content = %q, want empty", got)
	}
	if got := extractAssistantText(map[string]interface{}{"content": 42}); got != "" {
		t.Fatalf("bogus content = %q, want empty", got)
	}
}
