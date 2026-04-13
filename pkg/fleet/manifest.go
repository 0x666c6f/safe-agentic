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
	SSH         bool     `yaml:"ssh"`
	ReuseAuth   bool     `yaml:"reuse_auth"`
	ReuseGHAuth bool     `yaml:"reuse_gh_auth"`
	AutoTrust   bool     `yaml:"auto_trust"`
	Background  bool     `yaml:"background"`
	Docker      bool     `yaml:"docker"`
	Network     string   `yaml:"network"`
	Memory      string   `yaml:"memory"`
	CPUs        string   `yaml:"cpus"`
	AWS         string   `yaml:"aws"`
	// Pipeline-specific fields
	DependsOn string `yaml:"depends_on"`
	OnFailure string `yaml:"on_failure"`
	Retry     int    `yaml:"retry"`
	When      string `yaml:"when"`
	Outputs   string `yaml:"outputs"`

	hasSSH         bool `yaml:"-"`
	hasReuseAuth   bool `yaml:"-"`
	hasReuseGHAuth bool `yaml:"-"`
	hasAutoTrust   bool `yaml:"-"`
	hasBackground  bool `yaml:"-"`
	hasDocker      bool `yaml:"-"`
}

type rawAgentSpec struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"`
	Repo        string   `yaml:"repo"`
	Repos       []string `yaml:"repos"`
	Prompt      string   `yaml:"prompt"`
	SSH         *bool    `yaml:"ssh"`
	ReuseAuth   *bool    `yaml:"reuse_auth"`
	ReuseGHAuth *bool    `yaml:"reuse_gh_auth"`
	AutoTrust   *bool    `yaml:"auto_trust"`
	Background  *bool    `yaml:"background"`
	Docker      *bool    `yaml:"docker"`
	Network     string   `yaml:"network"`
	Memory      string   `yaml:"memory"`
	CPUs        string   `yaml:"cpus"`
	AWS         string   `yaml:"aws"`
	DependsOn   string   `yaml:"depends_on"`
	OnFailure   string   `yaml:"on_failure"`
	Retry       int      `yaml:"retry"`
	When        string   `yaml:"when"`
	Outputs     string   `yaml:"outputs"`
}

func (a *AgentSpec) UnmarshalYAML(value *yaml.Node) error {
	var raw rawAgentSpec
	if err := value.Decode(&raw); err != nil {
		return err
	}

	*a = AgentSpec{
		Name:      raw.Name,
		Type:      raw.Type,
		Repo:      raw.Repo,
		Repos:     raw.Repos,
		Prompt:    raw.Prompt,
		Network:   raw.Network,
		Memory:    raw.Memory,
		CPUs:      raw.CPUs,
		AWS:       raw.AWS,
		DependsOn: raw.DependsOn,
		OnFailure: raw.OnFailure,
		Retry:     raw.Retry,
		When:      raw.When,
		Outputs:   raw.Outputs,
	}
	if raw.SSH != nil {
		a.SSH = *raw.SSH
		a.hasSSH = true
	}
	if raw.ReuseAuth != nil {
		a.ReuseAuth = *raw.ReuseAuth
		a.hasReuseAuth = true
	}
	if raw.ReuseGHAuth != nil {
		a.ReuseGHAuth = *raw.ReuseGHAuth
		a.hasReuseGHAuth = true
	}
	if raw.AutoTrust != nil {
		a.AutoTrust = *raw.AutoTrust
		a.hasAutoTrust = true
	}
	if raw.Background != nil {
		a.Background = *raw.Background
		a.hasBackground = true
	}
	if raw.Docker != nil {
		a.Docker = *raw.Docker
		a.hasDocker = true
	}
	return nil
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
	Parent    string      `yaml:"-"`        // set during model expansion (original stage name)
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
	normalizePipelineSteps(&m)
	m.Stages = expandPipelineModels(m.Stages)
	rewriteStageDependencies(m.Stages)
	applyPipelineDefaults(&m)
	interpolatePipelinePrompts(&m)
	return &m, nil
}

// mergeDefaults applies default values to an agent spec (agent values take precedence).
func mergeDefaults(defaults, agent AgentSpec) AgentSpec {
	agent.Repo = mergeStringDefault(agent.Repo, defaults.Repo)
	agent.Repos = mergeSliceDefault(agent.Repos, defaults.Repos)
	agent.SSH = mergeBoolDefault(agent.SSH, agent.hasSSH, defaults.SSH)
	agent.ReuseAuth = mergeBoolDefault(agent.ReuseAuth, agent.hasReuseAuth, defaults.ReuseAuth)
	agent.ReuseGHAuth = mergeBoolDefault(agent.ReuseGHAuth, agent.hasReuseGHAuth, defaults.ReuseGHAuth)
	agent.AutoTrust = mergeBoolDefault(agent.AutoTrust, agent.hasAutoTrust, defaults.AutoTrust)
	agent.Background = mergeBoolDefault(agent.Background, agent.hasBackground, defaults.Background)
	agent.Docker = mergeBoolDefault(agent.Docker, agent.hasDocker, defaults.Docker)
	agent.Network = mergeStringDefault(agent.Network, defaults.Network)
	agent.Memory = mergeStringDefault(agent.Memory, defaults.Memory)
	agent.CPUs = mergeStringDefault(agent.CPUs, defaults.CPUs)
	agent.AWS = mergeStringDefault(agent.AWS, defaults.AWS)
	agent.Type = mergeStringDefault(agent.Type, defaults.Type)
	agent.hasSSH = agent.hasSSH || defaults.hasSSH
	agent.hasReuseAuth = agent.hasReuseAuth || defaults.hasReuseAuth
	agent.hasReuseGHAuth = agent.hasReuseGHAuth || defaults.hasReuseGHAuth
	agent.hasAutoTrust = agent.hasAutoTrust || defaults.hasAutoTrust
	agent.hasBackground = agent.hasBackground || defaults.hasBackground
	agent.hasDocker = agent.hasDocker || defaults.hasDocker
	return agent
}

func normalizePipelineSteps(m *PipelineManifest) {
	if len(m.Steps) == 0 || len(m.Stages) > 0 {
		return
	}
	for _, step := range m.Steps {
		m.Stages = append(m.Stages, stageFromStep(step))
	}
}

func stageFromStep(step AgentSpec) PipelineStage {
	stage := PipelineStage{
		Name:   step.Name,
		Agents: []AgentSpec{step},
	}
	if step.DependsOn != "" {
		stage.DependsOn = []string{step.DependsOn}
	}
	return stage
}

func expandPipelineModels(stages []PipelineStage) []PipelineStage {
	var expanded []PipelineStage
	for _, stage := range stages {
		expanded = append(expanded, expandStageModels(stage)...)
	}
	return expanded
}

func expandStageModels(stage PipelineStage) []PipelineStage {
	if len(stage.Models) == 0 {
		return []PipelineStage{stage}
	}
	var expanded []PipelineStage
	for _, model := range stage.Models {
		expanded = append(expanded, stageForModel(stage, model))
	}
	return expanded
}

func stageForModel(stage PipelineStage, model string) PipelineStage {
	expanded := PipelineStage{
		Name:      stage.Name + "-" + model,
		DependsOn: stage.DependsOn,
		Parent:    stage.Name,
	}
	for _, agent := range stage.Agents {
		expanded.Agents = append(expanded.Agents, agentForModel(agent, model))
	}
	return expanded
}

func agentForModel(agent AgentSpec, model string) AgentSpec {
	expanded := agent
	expanded.Type = model
	expanded.Name = model + "-" + agent.Name
	expanded.Prompt = strings.ReplaceAll(agent.Prompt, "${model}", model)
	return expanded
}

func rewriteStageDependencies(stages []PipelineStage) {
	stageNames := stageNameSet(stages)
	for i, stage := range stages {
		stages[i].DependsOn = expandedDependencies(stage.DependsOn, stageNames)
	}
}

func stageNameSet(stages []PipelineStage) map[string]bool {
	stageNames := make(map[string]bool, len(stages))
	for _, s := range stages {
		stageNames[s.Name] = true
	}
	return stageNames
}

func expandedDependencies(deps []string, stageNames map[string]bool) []string {
	var expanded []string
	for _, dep := range deps {
		expanded = append(expanded, expandDependency(dep, stageNames)...)
	}
	return expanded
}

func expandDependency(dep string, stageNames map[string]bool) []string {
	if stageNames[dep] {
		return []string{dep}
	}
	var expanded []string
	for name := range stageNames {
		if strings.HasPrefix(name, dep+"-") {
			expanded = append(expanded, name)
		}
	}
	if len(expanded) == 0 {
		return []string{dep}
	}
	return expanded
}

func applyPipelineDefaults(m *PipelineManifest) {
	for i, stage := range m.Stages {
		for j := range stage.Agents {
			m.Stages[i].Agents[j] = mergeDefaults(m.Defaults, m.Stages[i].Agents[j])
		}
	}
}

func interpolatePipelinePrompts(m *PipelineManifest) {
	if len(m.Vars) == 0 {
		return
	}
	for i, stage := range m.Stages {
		for j := range stage.Agents {
			m.Stages[i].Agents[j].Prompt = interpolateVars(m.Stages[i].Agents[j].Prompt, m.Vars)
		}
	}
}

func mergeStringDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func mergeSliceDefault(value, fallback []string) []string {
	if len(value) > 0 {
		return value
	}
	return fallback
}

func mergeBoolDefault(value, valueSet, fallback bool) bool {
	if valueSet {
		return value
	}
	return fallback
}

// interpolateVars replaces ${key} in a string with values from vars map.
func interpolateVars(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}
