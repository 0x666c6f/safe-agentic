package fleet

import (
	"fmt"
	"os"

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
	Name        string      `yaml:"name"`
	SharedTasks bool        `yaml:"shared_tasks"`
	Agents      []AgentSpec `yaml:"agents"`
}

// PipelineStage is one stage in a pipeline manifest.
// A stage holds one or more agents that run in parallel; stages run
// sequentially according to depends_on ordering.
// A stage can also reference a sub-pipeline file instead of inline agents.
type PipelineStage struct {
	Name      string      `yaml:"name"`
	DependsOn []string    `yaml:"depends_on"`
	Agents    []AgentSpec `yaml:"agents"`
	Pipeline  string      `yaml:"pipeline"` // path to sub-pipeline YAML (mutually exclusive with agents)
}

// PipelineManifest is the top-level structure for `agent pipeline <manifest.yaml>`.
// It supports two layouts:
//  1. stages: []PipelineStage  — dependency groups (as described in the spec)
//  2. steps:  []AgentSpec      — flat sequential list (as used in examples/)
//
// When steps are used each step is promoted to its own single-agent stage.
// A step's depends_on (string) becomes the DependsOn slice of its stage.
type PipelineManifest struct {
	Name   string          `yaml:"name"`
	Stages []PipelineStage `yaml:"stages"`
	Steps  []AgentSpec     `yaml:"steps"` // flat alternative
}

// ParseFleet reads and parses a fleet YAML manifest file.
func ParseFleet(path string) (*FleetManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fleet manifest %q: %w", path, err)
	}
	var m FleetManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse fleet manifest %q: %w", path, err)
	}
	return &m, nil
}

// ParsePipeline reads and parses a pipeline YAML manifest file.
// Steps (flat list) are automatically normalized into Stages.
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
	return &m, nil
}
