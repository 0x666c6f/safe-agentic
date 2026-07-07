package fleet

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/0x666c6f/berth/pkg/profiles"
	"gopkg.in/yaml.v3"
)

// AgentSpec describes a single agent in a fleet or pipeline stage.
// Fields mirror the flat YAML format used in examples/:
//
//	profile, name, type, repo, prompt, template, instructions, ssh, reuse_auth,
//	seed_auth, auto_trust, background, network, memory, cpus, aws, docker,
//	reuse_gh_auth, allow_setup_scripts, callbacks, pids_limit,
//	depends_on, on_failure, retry, when, outputs
type AgentSpec struct {
	Name              string   `yaml:"name"`
	Profile           string   `yaml:"profile"`
	Type              string   `yaml:"type"`
	Repo              string   `yaml:"repo"`
	Repos             []string `yaml:"repos"`
	Prompt            string   `yaml:"prompt"`
	Template          string   `yaml:"template"`
	TemplateVars      []string `yaml:"template_vars"`
	Instructions      string   `yaml:"instructions"`
	InstructionsFile  string   `yaml:"instructions_file"`
	SSH               bool     `yaml:"ssh"`
	ReuseAuth         bool     `yaml:"reuse_auth"`
	EphemeralAuth     bool     `yaml:"ephemeral_auth"`
	ReuseGHAuth       bool     `yaml:"reuse_gh_auth"`
	SeedAuth          bool     `yaml:"seed_auth"`
	AutoTrust         bool     `yaml:"auto_trust"`
	Background        bool     `yaml:"background"`
	Docker            bool     `yaml:"docker"`
	DockerSocket      bool     `yaml:"docker_socket"`
	AllowSetupScripts bool     `yaml:"allow_setup_scripts"`
	Network           string   `yaml:"network"`
	Memory            string   `yaml:"memory"`
	CPUs              string   `yaml:"cpus"`
	PIDsLimit         int      `yaml:"pids_limit"`
	Identity          string   `yaml:"identity"`
	AWS               string   `yaml:"aws"`
	MaxCost           string   `yaml:"max_cost"`
	Notify            string   `yaml:"notify"`
	OnExit            string   `yaml:"on_exit"`
	OnComplete        string   `yaml:"on_complete"`
	OnFail            string   `yaml:"on_fail"`
	// Pipeline-specific fields
	DependsOn string     `yaml:"depends_on"`
	Models    []string   `yaml:"models"` // flat-step fan-out across models/engines
	OnFailure string     `yaml:"on_failure"`
	Retry     int        `yaml:"retry"`
	When      string     `yaml:"when"`
	Outputs   string     `yaml:"outputs"`
	Judge     *JudgeSpec `yaml:"judge"`

	hasSSH               bool `yaml:"-"`
	hasReuseAuth         bool `yaml:"-"`
	hasEphemeralAuth     bool `yaml:"-"`
	hasReuseGHAuth       bool `yaml:"-"`
	hasSeedAuth          bool `yaml:"-"`
	hasAutoTrust         bool `yaml:"-"`
	hasBackground        bool `yaml:"-"`
	hasDocker            bool `yaml:"-"`
	hasDockerSocket      bool `yaml:"-"`
	hasAllowSetupScripts bool `yaml:"-"`
}

type InputSpec struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
	Default     string `yaml:"default"`
	Infer       string `yaml:"infer"`
}

// JudgeSpec configures a best-of-N "crown" judge stage. A judge stage does not
// spawn a normal agent from a repo; instead, after its dependency stages
// complete, it collects the working-tree diff and final message of each
// candidate container, runs a one-shot judge agent to select a winner, and
// records a strict-JSON verdict. It carries no prompt/template/repo of its own.
type JudgeSpec struct {
	// Criteria is optional free-text guidance for the judge (e.g.
	// "correctness first, then minimal diff"). When empty a sensible default
	// is used at execution time.
	Criteria string `yaml:"criteria"`
	// AutoPR, when true, opens a GitHub PR from the winning candidate container
	// using the judge's summary as the PR body.
	AutoPR bool `yaml:"auto_pr"`
	// Base is the PR base branch used when AutoPR is set (defaults to "main").
	Base string `yaml:"base"`
	// MaxDiff caps the number of bytes of each candidate diff embedded in the
	// judge prompt (defaults to a sane value at execution time). Truncation is
	// noted inline in the prompt.
	MaxDiff int `yaml:"max_diff"`
}

type rawAgentSpec struct {
	Name              string     `yaml:"name"`
	Profile           string     `yaml:"profile"`
	Type              string     `yaml:"type"`
	Repo              string     `yaml:"repo"`
	Repos             []string   `yaml:"repos"`
	Prompt            string     `yaml:"prompt"`
	Template          string     `yaml:"template"`
	TemplateVars      []string   `yaml:"template_vars"`
	Instructions      string     `yaml:"instructions"`
	InstructionsFile  string     `yaml:"instructions_file"`
	SSH               *bool      `yaml:"ssh"`
	ReuseAuth         *bool      `yaml:"reuse_auth"`
	EphemeralAuth     *bool      `yaml:"ephemeral_auth"`
	ReuseGHAuth       *bool      `yaml:"reuse_gh_auth"`
	SeedAuth          *bool      `yaml:"seed_auth"`
	AutoTrust         *bool      `yaml:"auto_trust"`
	Background        *bool      `yaml:"background"`
	Docker            *bool      `yaml:"docker"`
	DockerSocket      *bool      `yaml:"docker_socket"`
	AllowSetupScripts *bool      `yaml:"allow_setup_scripts"`
	Network           string     `yaml:"network"`
	Memory            string     `yaml:"memory"`
	CPUs              string     `yaml:"cpus"`
	PIDsLimit         int        `yaml:"pids_limit"`
	Identity          string     `yaml:"identity"`
	AWS               string     `yaml:"aws"`
	MaxCost           string     `yaml:"max_cost"`
	Notify            string     `yaml:"notify"`
	OnExit            string     `yaml:"on_exit"`
	OnComplete        string     `yaml:"on_complete"`
	OnFail            string     `yaml:"on_fail"`
	DependsOn         string     `yaml:"depends_on"`
	Models            []string   `yaml:"models"`
	OnFailure         string     `yaml:"on_failure"`
	Retry             int        `yaml:"retry"`
	When              string     `yaml:"when"`
	Outputs           string     `yaml:"outputs"`
	Judge             *JudgeSpec `yaml:"judge"`
}

func (a *AgentSpec) UnmarshalYAML(value *yaml.Node) error {
	var raw rawAgentSpec
	if err := value.Decode(&raw); err != nil {
		return err
	}

	*a = AgentSpec{
		Name:             raw.Name,
		Profile:          raw.Profile,
		Type:             raw.Type,
		Repo:             raw.Repo,
		Repos:            raw.Repos,
		Prompt:           raw.Prompt,
		Template:         raw.Template,
		TemplateVars:     raw.TemplateVars,
		Instructions:     raw.Instructions,
		InstructionsFile: raw.InstructionsFile,
		Network:          raw.Network,
		Memory:           raw.Memory,
		CPUs:             raw.CPUs,
		PIDsLimit:        raw.PIDsLimit,
		Identity:         raw.Identity,
		AWS:              raw.AWS,
		MaxCost:          raw.MaxCost,
		Notify:           raw.Notify,
		OnExit:           raw.OnExit,
		OnComplete:       raw.OnComplete,
		OnFail:           raw.OnFail,
		DependsOn:        raw.DependsOn,
		Models:           raw.Models,
		OnFailure:        raw.OnFailure,
		Retry:            raw.Retry,
		When:             raw.When,
		Outputs:          raw.Outputs,
		Judge:            raw.Judge,
	}
	if raw.SSH != nil {
		a.SSH = *raw.SSH
		a.hasSSH = true
	}
	if raw.ReuseAuth != nil {
		a.ReuseAuth = *raw.ReuseAuth
		a.hasReuseAuth = true
	}
	if raw.EphemeralAuth != nil {
		a.EphemeralAuth = *raw.EphemeralAuth
		a.hasEphemeralAuth = true
	}
	if raw.ReuseGHAuth != nil {
		a.ReuseGHAuth = *raw.ReuseGHAuth
		a.hasReuseGHAuth = true
	}
	if raw.SeedAuth != nil {
		a.SeedAuth = *raw.SeedAuth
		a.hasSeedAuth = true
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
	if raw.DockerSocket != nil {
		a.DockerSocket = *raw.DockerSocket
		a.hasDockerSocket = true
	}
	if raw.AllowSetupScripts != nil {
		a.AllowSetupScripts = *raw.AllowSetupScripts
		a.hasAllowSetupScripts = true
	}
	return nil
}

// FleetManifest is the top-level structure for `agent fleet <manifest.yaml>`.
type FleetManifest struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Inputs      []InputSpec       `yaml:"inputs"`
	Examples    []string          `yaml:"examples"`
	Tags        []string          `yaml:"tags"`
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
	Judge     *JudgeSpec  `yaml:"judge"`    // best-of-N judge stage (mutually exclusive with agents/pipeline)
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
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Inputs      []InputSpec       `yaml:"inputs"`
	Examples    []string          `yaml:"examples"`
	Tags        []string          `yaml:"tags"`
	Defaults    AgentSpec         `yaml:"defaults"` // inherited by all agents
	Vars        map[string]string `yaml:"vars"`     // ${key} interpolated in prompts
	Stages      []PipelineStage   `yaml:"stages"`
	Steps       []AgentSpec       `yaml:"steps"`
}

type ParseOptions struct {
	Vars         map[string]string
	DefaultRepos []string
	ProfileDirs  []string
}

// ParseFleet reads and parses a fleet YAML manifest file.
// Defaults are merged into agents, vars are interpolated in prompts.
func ParseFleet(path string) (*FleetManifest, error) {
	return ParseFleetWithOptions(path, ParseOptions{})
}

func ParseFleetWithOptions(path string, opts ParseOptions) (*FleetManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read fleet manifest %q: %w", path, err)
	}
	var m FleetManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse fleet manifest %q: %w", path, err)
	}
	vars := parseOptionVars(m.Vars, opts)
	vars, err = applyInputSpecs(m.Inputs, vars, opts.DefaultRepos)
	if err != nil {
		return nil, err
	}
	m.Defaults = interpolateAgentSpec(m.Defaults, vars)
	profileCatalog, err := loadProfileCatalog(opts.ProfileDirs)
	if err != nil {
		return nil, err
	}
	m.Defaults, err = applyProfile(m.Defaults, profileCatalog)
	if err != nil {
		return nil, err
	}
	m.Defaults = interpolateAgentSpec(m.Defaults, vars)
	for i := range m.Agents {
		m.Agents[i] = interpolateAgentSpec(m.Agents[i], vars)
		m.Agents[i], err = applyProfile(m.Agents[i], profileCatalog)
		if err != nil {
			return nil, err
		}
		m.Agents[i] = mergeDefaults(m.Defaults, m.Agents[i])
		m.Agents[i] = interpolateAgentSpec(m.Agents[i], vars)
		m.Agents[i] = applyDefaultRepos(m.Agents[i], opts.DefaultRepos)
		if err := ensureAgentSpecResolved("agent", m.Agents[i]); err != nil {
			return nil, err
		}
		if err := ensureAgentTypeValid("agent", m.Agents[i]); err != nil {
			return nil, err
		}
	}
	return &m, nil
}

// ParsePipeline reads and parses a pipeline YAML manifest file.
// Steps (flat list) are automatically normalized into Stages.
// Defaults are merged into agents, vars are interpolated in prompts.
func ParsePipeline(path string) (*PipelineManifest, error) {
	return ParsePipelineWithOptions(path, ParseOptions{})
}

func ParsePipelineWithOptions(path string, opts ParseOptions) (*PipelineManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline manifest %q: %w", path, err)
	}
	var m PipelineManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse pipeline manifest %q: %w", path, err)
	}
	if err := validateJudgeSteps(m.Steps); err != nil {
		return nil, err
	}
	normalizePipelineSteps(&m)
	m.Stages = expandPipelineModels(m.Stages)
	rewriteStageDependencies(m.Stages)
	vars := parseOptionVars(m.Vars, opts)
	vars, err = applyInputSpecs(m.Inputs, vars, opts.DefaultRepos)
	if err != nil {
		return nil, err
	}
	m.Defaults = interpolateAgentSpec(m.Defaults, vars)
	profileCatalog, err := loadProfileCatalog(opts.ProfileDirs)
	if err != nil {
		return nil, err
	}
	m.Defaults, err = applyProfile(m.Defaults, profileCatalog)
	if err != nil {
		return nil, err
	}
	m.Defaults = interpolateAgentSpec(m.Defaults, vars)
	if err := applyPipelineProfiles(&m, profileCatalog, vars); err != nil {
		return nil, err
	}
	applyPipelineDefaults(&m)
	interpolatePipelineFields(&m, vars, filepath.Dir(path))
	applyPipelineDefaultRepos(&m, opts.DefaultRepos)
	if err := validateJudgeStages(m.Stages); err != nil {
		return nil, err
	}
	if err := ensurePipelineResolved(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// mergeDefaults applies default values to an agent spec (agent values take precedence).
func mergeDefaults(defaults, agent AgentSpec) AgentSpec {
	agent.Profile = mergeStringDefault(agent.Profile, defaults.Profile)
	agent.Repo = mergeStringDefault(agent.Repo, defaults.Repo)
	agent.Repos = mergeSliceDefault(agent.Repos, defaults.Repos)
	agent.Template = mergeStringDefault(agent.Template, defaults.Template)
	agent.TemplateVars = mergeSliceDefault(agent.TemplateVars, defaults.TemplateVars)
	agent.Instructions = mergeStringDefault(agent.Instructions, defaults.Instructions)
	agent.InstructionsFile = mergeStringDefault(agent.InstructionsFile, defaults.InstructionsFile)
	agent.SSH = mergeBoolDefault(agent.SSH, agent.hasSSH, defaults.SSH)
	agent.ReuseAuth = mergeBoolDefault(agent.ReuseAuth, agent.hasReuseAuth, defaults.ReuseAuth)
	agent.EphemeralAuth = mergeBoolDefault(agent.EphemeralAuth, agent.hasEphemeralAuth, defaults.EphemeralAuth)
	agent.ReuseGHAuth = mergeBoolDefault(agent.ReuseGHAuth, agent.hasReuseGHAuth, defaults.ReuseGHAuth)
	agent.SeedAuth = mergeBoolDefault(agent.SeedAuth, agent.hasSeedAuth, defaults.SeedAuth)
	agent.AutoTrust = mergeBoolDefault(agent.AutoTrust, agent.hasAutoTrust, defaults.AutoTrust)
	agent.Background = mergeBoolDefault(agent.Background, agent.hasBackground, defaults.Background)
	agent.Docker = mergeBoolDefault(agent.Docker, agent.hasDocker, defaults.Docker)
	agent.DockerSocket = mergeBoolDefault(agent.DockerSocket, agent.hasDockerSocket, defaults.DockerSocket)
	agent.AllowSetupScripts = mergeBoolDefault(agent.AllowSetupScripts, agent.hasAllowSetupScripts, defaults.AllowSetupScripts)
	agent.Network = mergeStringDefault(agent.Network, defaults.Network)
	agent.Memory = mergeStringDefault(agent.Memory, defaults.Memory)
	agent.CPUs = mergeStringDefault(agent.CPUs, defaults.CPUs)
	if agent.PIDsLimit == 0 {
		agent.PIDsLimit = defaults.PIDsLimit
	}
	agent.Identity = mergeStringDefault(agent.Identity, defaults.Identity)
	agent.AWS = mergeStringDefault(agent.AWS, defaults.AWS)
	agent.MaxCost = mergeStringDefault(agent.MaxCost, defaults.MaxCost)
	agent.Notify = mergeStringDefault(agent.Notify, defaults.Notify)
	agent.OnExit = mergeStringDefault(agent.OnExit, defaults.OnExit)
	agent.OnComplete = mergeStringDefault(agent.OnComplete, defaults.OnComplete)
	agent.OnFail = mergeStringDefault(agent.OnFail, defaults.OnFail)
	agent.Type = mergeStringDefault(agent.Type, defaults.Type)
	agent.hasSSH = agent.hasSSH || defaults.hasSSH
	agent.hasReuseAuth = agent.hasReuseAuth || defaults.hasReuseAuth
	agent.hasEphemeralAuth = agent.hasEphemeralAuth || defaults.hasEphemeralAuth
	agent.hasReuseGHAuth = agent.hasReuseGHAuth || defaults.hasReuseGHAuth
	agent.hasSeedAuth = agent.hasSeedAuth || defaults.hasSeedAuth
	agent.hasAutoTrust = agent.hasAutoTrust || defaults.hasAutoTrust
	agent.hasBackground = agent.hasBackground || defaults.hasBackground
	agent.hasDocker = agent.hasDocker || defaults.hasDocker
	agent.hasDockerSocket = agent.hasDockerSocket || defaults.hasDockerSocket
	agent.hasAllowSetupScripts = agent.hasAllowSetupScripts || defaults.hasAllowSetupScripts
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
	stage := PipelineStage{Name: step.Name}
	if step.DependsOn != "" {
		stage.DependsOn = []string{step.DependsOn}
	}
	// A judge step carries no spawnable agent; the judge stage synthesizes and
	// runs its own one-shot agent at execution time.
	if step.Judge != nil {
		stage.Judge = step.Judge
		return stage
	}
	// A flat step may fan out across models/engines just like a stage.
	stage.Models = step.Models
	stage.Agents = []AgentSpec{step}
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
	// Judge stages are never model-expanded; leaving them intact preserves the
	// Judge spec (and any misconfigured Models) so validation can report it.
	if len(stage.Models) == 0 || stage.Judge != nil {
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

func applyPipelineProfiles(m *PipelineManifest, catalog profiles.Catalog, vars map[string]string) error {
	var err error
	for i, stage := range m.Stages {
		for j := range stage.Agents {
			m.Stages[i].Agents[j] = interpolateAgentSpec(m.Stages[i].Agents[j], vars)
			m.Stages[i].Agents[j], err = applyProfile(m.Stages[i].Agents[j], catalog)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func interpolatePipelineFields(m *PipelineManifest, vars map[string]string, baseDir string) {
	for i, stage := range m.Stages {
		if stage.Pipeline != "" {
			stage.Pipeline = interpolateVars(stage.Pipeline, vars)
			if stage.Pipeline != "" && !filepath.IsAbs(stage.Pipeline) {
				stage.Pipeline = filepath.Join(baseDir, stage.Pipeline)
			}
			m.Stages[i].Pipeline = stage.Pipeline
		}
		if stage.Judge != nil {
			m.Stages[i].Judge.Criteria = interpolateVars(stage.Judge.Criteria, vars)
			m.Stages[i].Judge.Base = interpolateVars(stage.Judge.Base, vars)
		}
		for j := range stage.Agents {
			m.Stages[i].Agents[j] = interpolateAgentSpec(m.Stages[i].Agents[j], vars)
		}
	}
}

func loadProfileCatalog(dirs []string) (profiles.Catalog, error) {
	if len(dirs) == 0 {
		return profiles.Catalog{}, nil
	}
	return profiles.LoadDirs(dirs)
}

func applyProfile(spec AgentSpec, catalog profiles.Catalog) (AgentSpec, error) {
	if spec.Profile == "" {
		return spec, nil
	}
	profile, ok := catalog.Get(spec.Profile)
	if !ok {
		return spec, fmt.Errorf("profile %q not found", spec.Profile)
	}
	base := agentSpecFromProfile(profile)
	base.Profile = spec.Profile
	return mergeDefaults(base, spec), nil
}

func agentSpecFromProfile(profile profiles.Profile) AgentSpec {
	spec := AgentSpec{
		Name:                 profile.ContainerName,
		Type:                 profile.AgentType,
		Repos:                append([]string{}, profile.Repos...),
		Prompt:               profile.Prompt,
		Template:             profile.Template,
		TemplateVars:         append([]string{}, profile.TemplateVars...),
		Instructions:         profile.Instructions,
		InstructionsFile:     profile.InstructionsFile,
		Network:              profile.Network,
		Memory:               profile.Memory,
		CPUs:                 profile.CPUs,
		PIDsLimit:            profile.PIDsLimit,
		Identity:             profile.Identity,
		AWS:                  profile.AWSProfile,
		MaxCost:              profile.MaxCost,
		Notify:               profile.Notify,
		OnExit:               profile.OnExit,
		OnComplete:           profile.OnComplete,
		OnFail:               profile.OnFail,
		SSH:                  boolValue(profile.SSH),
		ReuseAuth:            boolValue(profile.ReuseAuth),
		EphemeralAuth:        boolValue(profile.EphemeralAuth),
		ReuseGHAuth:          boolValue(profile.ReuseGHAuth),
		SeedAuth:             boolValue(profile.SeedAuth),
		Docker:               boolValue(profile.Docker),
		DockerSocket:         boolValue(profile.DockerSocket),
		AutoTrust:            boolValue(profile.AutoTrust),
		Background:           boolValue(profile.Background),
		AllowSetupScripts:    boolValue(profile.AllowSetupScripts),
		hasSSH:               profile.SSH != nil,
		hasReuseAuth:         profile.ReuseAuth != nil,
		hasEphemeralAuth:     profile.EphemeralAuth != nil,
		hasReuseGHAuth:       profile.ReuseGHAuth != nil,
		hasSeedAuth:          profile.SeedAuth != nil,
		hasAutoTrust:         profile.AutoTrust != nil,
		hasBackground:        profile.Background != nil,
		hasDocker:            profile.Docker != nil,
		hasDockerSocket:      profile.DockerSocket != nil,
		hasAllowSetupScripts: profile.AllowSetupScripts != nil,
	}
	return spec
}

func boolValue(value *bool) bool {
	return value != nil && *value
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

func mergedVars(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	vars := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		vars[k] = v
	}
	for k, v := range override {
		vars[k] = v
	}
	return vars
}

func parseOptionVars(base map[string]string, opts ParseOptions) map[string]string {
	vars := mergedVars(base, opts.Vars)
	if len(opts.DefaultRepos) > 0 {
		if vars == nil {
			vars = make(map[string]string, 1)
		}
		if _, exists := vars["repo"]; !exists {
			vars["repo"] = opts.DefaultRepos[0]
		}
	}
	return vars
}

func applyInputSpecs(inputs []InputSpec, vars map[string]string, defaultRepos []string) (map[string]string, error) {
	if len(inputs) == 0 {
		return vars, nil
	}
	if vars == nil {
		vars = make(map[string]string, len(inputs))
	}
	for _, input := range inputs {
		if vars[input.Name] == "" && input.Default != "" {
			vars[input.Name] = input.Default
		}
		if input.Name == "repo" && vars[input.Name] == "" && len(defaultRepos) > 0 {
			vars[input.Name] = defaultRepos[0]
		}
		if input.Required && vars[input.Name] == "" {
			return nil, fmt.Errorf("missing required input %q", input.Name)
		}
	}
	return vars, nil
}

func interpolateAgentSpec(spec AgentSpec, vars map[string]string) AgentSpec {
	if len(vars) == 0 {
		return spec
	}
	spec.Name = interpolateVars(spec.Name, vars)
	spec.Profile = interpolateVars(spec.Profile, vars)
	spec.Type = interpolateVars(spec.Type, vars)
	spec.Repo = interpolateVars(spec.Repo, vars)
	for i := range spec.Repos {
		spec.Repos[i] = interpolateVars(spec.Repos[i], vars)
	}
	spec.Prompt = interpolateVars(spec.Prompt, vars)
	spec.Template = interpolateVars(spec.Template, vars)
	for i := range spec.TemplateVars {
		spec.TemplateVars[i] = interpolateVars(spec.TemplateVars[i], vars)
	}
	spec.Instructions = interpolateVars(spec.Instructions, vars)
	spec.InstructionsFile = interpolateVars(spec.InstructionsFile, vars)
	spec.Network = interpolateVars(spec.Network, vars)
	spec.Memory = interpolateVars(spec.Memory, vars)
	spec.CPUs = interpolateVars(spec.CPUs, vars)
	spec.Identity = interpolateVars(spec.Identity, vars)
	spec.AWS = interpolateVars(spec.AWS, vars)
	spec.MaxCost = interpolateVars(spec.MaxCost, vars)
	spec.Notify = interpolateVars(spec.Notify, vars)
	spec.OnExit = interpolateVars(spec.OnExit, vars)
	spec.OnComplete = interpolateVars(spec.OnComplete, vars)
	spec.OnFail = interpolateVars(spec.OnFail, vars)
	return spec
}

func applyDefaultRepos(spec AgentSpec, defaultRepos []string) AgentSpec {
	if spec.Repo != "" || len(spec.Repos) > 0 || len(defaultRepos) == 0 {
		return spec
	}
	if len(defaultRepos) == 1 {
		spec.Repo = defaultRepos[0]
		return spec
	}
	spec.Repos = append([]string{}, defaultRepos...)
	return spec
}

func applyPipelineDefaultRepos(m *PipelineManifest, defaultRepos []string) {
	for i := range m.Stages {
		for j := range m.Stages[i].Agents {
			m.Stages[i].Agents[j] = applyDefaultRepos(m.Stages[i].Agents[j], defaultRepos)
		}
	}
}

var judgeBaseBranch = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

// validateJudgeSteps rejects flat-form steps that pair a judge block with
// agent-producing fields. A judge step must not carry prompt/template/repo/type.
func validateJudgeSteps(steps []AgentSpec) error {
	for _, step := range steps {
		if step.Judge == nil {
			continue
		}
		label := "judge step"
		if step.Name != "" {
			label = fmt.Sprintf("judge step %q", step.Name)
		}
		if step.Prompt != "" || step.Template != "" || step.Repo != "" || len(step.Repos) > 0 ||
			step.Type != "" || step.Instructions != "" || step.InstructionsFile != "" || step.Profile != "" || len(step.Models) > 0 {
			return fmt.Errorf("%s must not define agent fields (prompt/template/repo/type/instructions/profile/models)", label)
		}
		if err := validateJudgeSpec(label, step.Judge); err != nil {
			return err
		}
	}
	return nil
}

// validateJudgeStages verifies structural and dependency constraints on judge
// stages after model expansion and dependency rewriting: a judge stage must not
// spawn agents/sub-pipelines/models of its own, must declare depends_on, and its
// dependencies must collectively produce at least two candidate runs.
func validateJudgeStages(stages []PipelineStage) error {
	index := make(map[string]PipelineStage, len(stages))
	for _, s := range stages {
		index[s.Name] = s
	}
	for _, stage := range stages {
		if stage.Judge == nil {
			continue
		}
		label := fmt.Sprintf("judge stage %q", stage.Name)
		if len(stage.Agents) > 0 {
			return fmt.Errorf("%s must not define agents", label)
		}
		if stage.Pipeline != "" {
			return fmt.Errorf("%s must not reference a sub-pipeline", label)
		}
		if len(stage.Models) > 0 {
			return fmt.Errorf("%s must not use models fan-out", label)
		}
		if len(stage.DependsOn) == 0 {
			return fmt.Errorf("%s must declare depends_on with candidate stages", label)
		}
		if err := validateJudgeSpec(label, stage.Judge); err != nil {
			return err
		}
		if err := ensureResolvedField(label+" criteria", stage.Judge.Criteria); err != nil {
			return err
		}
		candidates := 0
		for _, dep := range stage.DependsOn {
			d, ok := index[dep]
			if !ok {
				return fmt.Errorf("%s depends on unknown stage %q", label, dep)
			}
			if d.Judge != nil {
				return fmt.Errorf("%s cannot judge another judge stage %q", label, dep)
			}
			if d.Pipeline != "" {
				return fmt.Errorf("%s cannot judge sub-pipeline stage %q; depend on concrete agent stages instead", label, dep)
			}
			candidates += len(d.Agents)
		}
		if candidates < 2 {
			return fmt.Errorf("%s must depend on stages producing at least 2 candidate runs (found %d)", label, candidates)
		}
	}
	return nil
}

func validateJudgeSpec(label string, spec *JudgeSpec) error {
	if spec.MaxDiff < 0 {
		return fmt.Errorf("%s max_diff must not be negative", label)
	}
	if spec.Base != "" && !judgeBaseBranch.MatchString(spec.Base) {
		return fmt.Errorf("%s has invalid base branch %q", label, spec.Base)
	}
	return nil
}

func ensurePipelineResolved(m *PipelineManifest) error {
	for _, stage := range m.Stages {
		if err := ensureResolvedField("stage pipeline", stage.Pipeline); err != nil {
			return err
		}
		for _, agent := range stage.Agents {
			if err := ensureSupportedPipelineControls(stage.Name, agent); err != nil {
				return err
			}
			if err := ensureAgentSpecResolved("agent", agent); err != nil {
				return err
			}
			if err := ensureAgentTypeValid("stage "+stage.Name+" agent", agent); err != nil {
				return err
			}
		}
	}
	return nil
}

func ensureSupportedPipelineControls(stageName string, spec AgentSpec) error {
	prefix := "stage " + stageName
	if spec.Name != "" {
		prefix += " agent " + spec.Name
	}
	switch {
	case spec.OnFailure != "":
		return fmt.Errorf("%s uses unsupported on_failure; use depends_on or remove it", prefix)
	case spec.Retry != 0:
		return fmt.Errorf("%s uses unsupported retry; rerun failed stages manually for now", prefix)
	case spec.When != "":
		return fmt.Errorf("%s uses unsupported when; split the manifest or remove it", prefix)
	case spec.Outputs != "":
		return fmt.Errorf("%s uses unsupported outputs; pass context through prompts/files for now", prefix)
	default:
		return nil
	}
}

func ensureAgentSpecResolved(kind string, spec AgentSpec) error {
	if err := ensureResolvedField(kind+" profile", spec.Profile); err != nil {
		return err
	}
	if err := ensureResolvedField(kind+" name", spec.Name); err != nil {
		return err
	}
	if err := ensureResolvedField(kind+" repo", spec.Repo); err != nil {
		return err
	}
	for _, repo := range spec.Repos {
		if err := ensureResolvedField(kind+" repos", repo); err != nil {
			return err
		}
	}
	if err := ensureResolvedField(kind+" prompt", spec.Prompt); err != nil {
		return err
	}
	if err := ensureResolvedField(kind+" template", spec.Template); err != nil {
		return err
	}
	for _, value := range spec.TemplateVars {
		if err := ensureResolvedField(kind+" template_vars", value); err != nil {
			return err
		}
	}
	if err := ensureResolvedField(kind+" instructions", spec.Instructions); err != nil {
		return err
	}
	if err := ensureResolvedField(kind+" instructions_file", spec.InstructionsFile); err != nil {
		return err
	}
	return nil
}

func ensureResolvedField(label, value string) error {
	if strings.Contains(value, "${") {
		return fmt.Errorf("%s contains unresolved variables: %s", label, value)
	}
	return nil
}

// validAgentTypes are the agent kinds an entry may declare. It mirrors what
// `spawn` accepts downstream so a manifest fails at parse time rather than
// deep inside a spawn.
var validAgentTypes = []string{"claude", "codex", "shell"}

// ensureAgentTypeValid rejects an agent entry whose type is missing or not one
// of validAgentTypes, naming the entry and the allowed types. Previously such
// entries were silently skipped at spawn time, hiding manifest typos.
func ensureAgentTypeValid(kind string, spec AgentSpec) error {
	for _, t := range validAgentTypes {
		if spec.Type == t {
			return nil
		}
	}
	label := spec.Name
	if label == "" {
		label = "(unnamed)"
	}
	allowed := strings.Join(validAgentTypes, ", ")
	if spec.Type == "" {
		return fmt.Errorf("%s %q is missing a type; set type to one of: %s", kind, label, allowed)
	}
	return fmt.Errorf("%s %q has unknown type %q; must be one of: %s", kind, label, spec.Type, allowed)
}
