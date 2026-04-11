package fleet

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentSpec describes a single agent in a fleet or pipeline stage.
// Fields mirror the flat YAML format used in examples/:
//
//	name, type, repo, prompt, ssh, reuse_auth, auto_trust, background,
//	network, memory, cpus, aws, docker, reuse_gh_auth,
//	depends_on, on_failure, retry, when, outputs
type AgentSpec struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"`
	Repo        string   `yaml:"repo"`
	Repos       []string `yaml:"repos"`
	Prompt      string   `yaml:"prompt"`
	SSH         bool   `yaml:"ssh"`
	ReuseAuth   bool   `yaml:"reuse_auth"`
	ReuseGHAuth bool   `yaml:"reuse_gh_auth"`
	AutoTrust   bool   `yaml:"auto_trust"`
	Background  bool   `yaml:"background"`
	Docker      bool   `yaml:"docker"`
	Network     string `yaml:"network"`
	Memory      string `yaml:"memory"`
	CPUs        string `yaml:"cpus"`
	AWS         string `yaml:"aws"`
	// Pipeline-specific fields
	DependsOn string `yaml:"depends_on"`
	OnFailure string `yaml:"on_failure"`
	Retry     int    `yaml:"retry"`
	When      string `yaml:"when"`
	Outputs   string `yaml:"outputs"`
}

// FleetManifest is the top-level structure for `agent fleet <manifest.yaml>`.
type FleetManifest struct {
	Name        string            `yaml:"name"`
	SharedTasks bool              `yaml:"shared_tasks"`
	Defaults    AgentSpec         `yaml:"defaults"` // inherited by all agents
	Vars        map[string]string `yaml:"vars"`     // ${key} interpolated in prompts
	Agents      []AgentSpec       `yaml:"agents"`
}

// PipelineStage is one stage in a pipeline manifest.
// A stage holds one or more agents that run in parallel; stages run
// sequentially according to depends_on ordering.
// A stage can also reference a sub-pipeline file instead of inline agents.
// When `models` is set, agents are duplicated per model (e.g., models: [claude, codex]).
type PipelineStage struct {
	Name      string      `yaml:"name"`
	DependsOn []string    `yaml:"depends_on"`
	Agents    []AgentSpec `yaml:"agents"`
	Models    []string    `yaml:"models"`   // expand agents across multiple models
	Pipeline  string      `yaml:"pipeline"` // path to sub-pipeline YAML (mutually exclusive with agents)
}

// PipelineManifest is the top-level structure for `agent pipeline <manifest.yaml>`.
// It supports two layouts:
//  1. stages: []PipelineStage  — dependency groups (as described in the spec)
//  2. steps:  []AgentSpec      — flat sequential list (as used in examples/)
//
// When steps are used each step is promoted to its own single-agent stage.
// A step's depends_on (string) becomes the DependsOn slice of its stage.
//
// The `defaults` section provides inherited values for all agents.
// The `vars` section defines variables that are interpolated in prompts (${var}).
type PipelineManifest struct {
	Name     string            `yaml:"name"`
	Defaults AgentSpec         `yaml:"defaults"` // inherited by all agents
	Vars     map[string]string `yaml:"vars"`     // ${key} interpolated in prompts
	Stages   []PipelineStage   `yaml:"stages"`
	Steps    []AgentSpec       `yaml:"steps"`
}


// ParseFleet reads and parses a fleet YAML manifest file.
// Defaults are merged into agents, vars are interpolated in prompts.
func ParseFleet(path string) (*FleetManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fleet manifest %q: %w", path, err)
	}
	var m FleetManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse fleet manifest %q: %w", path, err)
	}
	// Apply defaults and interpolate vars
	for i := range m.Agents {
		m.Agents[i] = mergeDefaults(m.Defaults, m.Agents[i])
		if len(m.Vars) > 0 {
			m.Agents[i].Prompt = interpolateVars(m.Agents[i].Prompt, m.Vars)
		}
	}
	return &m, nil
}

// ParsePipeline reads and parses a pipeline YAML manifest file.
// Steps (flat list) are automatically normalized into Stages.
// Defaults are merged into agents, vars are interpolated in prompts.
func ParsePipeline(path string) (*PipelineManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline manifest %q: %w", path, err)
	}
	var m PipelineManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse pipeline manifest %q: %w", path, err)
	}
	// Normalize flat steps → stages
	if len(m.Steps) > 0 && len(m.Stages) == 0 {
		for _, step := range m.Steps {
			stage := PipelineStage{
				Name:   step.Name,
				Agents: []AgentSpec{step},
			}
			if step.DependsOn != "" {
				stage.DependsOn = []string{step.DependsOn}
			}
			m.Stages = append(m.Stages, stage)
		}
	}
	// Expand models: if a stage has models: [claude, codex], duplicate agents per model
	var expanded []PipelineStage
	for _, stage := range m.Stages {
		if len(stage.Models) > 0 {
			for _, model := range stage.Models {
				newStage := PipelineStage{
					Name:      stage.Name + "-" + model,
					DependsOn: stage.DependsOn,
				}
				for _, agent := range stage.Agents {
					a := agent
					a.Type = model
					a.Name = model + "-" + agent.Name
					// Add ${model} to vars for prompt interpolation
					a.Prompt = strings.ReplaceAll(a.Prompt, "${model}", model)
					newStage.Agents = append(newStage.Agents, a)
				}
				expanded = append(expanded, newStage)
			}
		} else {
			expanded = append(expanded, stage)
		}
	}
	m.Stages = expanded

	// Fix depends_on references that point to model-expanded stage names
	// e.g., depends_on: [review] → depends_on: [review-claude, review-codex]
	stageNames := make(map[string]bool)
	for _, s := range m.Stages {
		stageNames[s.Name] = true
	}
	for i, stage := range m.Stages {
		var newDeps []string
		for _, dep := range stage.DependsOn {
			if stageNames[dep] {
				newDeps = append(newDeps, dep)
			} else {
				// Expand: "review" → "review-claude", "review-codex" (any matching prefix)
				found := false
				for name := range stageNames {
					if strings.HasPrefix(name, dep+"-") {
						newDeps = append(newDeps, name)
						found = true
					}
				}
				if !found {
					newDeps = append(newDeps, dep) // keep as-is
				}
			}
		}
		m.Stages[i].DependsOn = newDeps
	}

	// Apply defaults to all agents
	for i, stage := range m.Stages {
		for j := range stage.Agents {
			m.Stages[i].Agents[j] = mergeDefaults(m.Defaults, m.Stages[i].Agents[j])
		}
	}
	// Interpolate vars in all prompts
	if len(m.Vars) > 0 {
		for i, stage := range m.Stages {
			for j := range stage.Agents {
				m.Stages[i].Agents[j].Prompt = interpolateVars(m.Stages[i].Agents[j].Prompt, m.Vars)
			}
		}
	}
	return &m, nil
}

// mergeDefaults applies default values to an agent spec (agent values take precedence).
func mergeDefaults(defaults, agent AgentSpec) AgentSpec {
	if agent.Repo == "" && defaults.Repo != "" {
		agent.Repo = defaults.Repo
	}
	if len(agent.Repos) == 0 && len(defaults.Repos) > 0 {
		agent.Repos = defaults.Repos
	}
	if !agent.SSH && defaults.SSH {
		agent.SSH = true
	}
	if !agent.ReuseAuth && defaults.ReuseAuth {
		agent.ReuseAuth = true
	}
	if !agent.ReuseGHAuth && defaults.ReuseGHAuth {
		agent.ReuseGHAuth = true
	}
	if !agent.AutoTrust && defaults.AutoTrust {
		agent.AutoTrust = true
	}
	if !agent.Background && defaults.Background {
		agent.Background = true
	}
	if !agent.Docker && defaults.Docker {
		agent.Docker = true
	}
	if agent.Network == "" && defaults.Network != "" {
		agent.Network = defaults.Network
	}
	if agent.Memory == "" && defaults.Memory != "" {
		agent.Memory = defaults.Memory
	}
	if agent.CPUs == "" && defaults.CPUs != "" {
		agent.CPUs = defaults.CPUs
	}
	if agent.AWS == "" && defaults.AWS != "" {
		agent.AWS = defaults.AWS
	}
	if agent.Type == "" && defaults.Type != "" {
		agent.Type = defaults.Type
	}
	return agent
}

// interpolateVars replaces ${key} in a string with values from vars map.
func interpolateVars(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}
