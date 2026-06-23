package profiles

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/BurntSushi/toml"
)

type Profile struct {
	Name              string
	Source            string
	AgentType         string
	Repos             []string
	ContainerName     string
	Prompt            string
	Template          string
	TemplateVars      []string
	Instructions      string
	InstructionsFile  string
	Network           string
	Memory            string
	CPUs              string
	PIDsLimit         int
	Identity          string
	AWSProfile        string
	MaxCost           string
	Notify            string
	OnExit            string
	OnComplete        string
	OnFail            string
	SSH               *bool
	ReuseAuth         *bool
	EphemeralAuth     *bool
	ReuseGHAuth       *bool
	SeedAuth          *bool
	Docker            *bool
	DockerSocket      *bool
	AutoTrust         *bool
	Background        *bool
	AllowSetupScripts *bool
}

type rawProfile struct {
	Name              string   `toml:"name"`
	AgentType         string   `toml:"agent_type"`
	Repos             []string `toml:"repos"`
	Repo              []string `toml:"repo"`
	ContainerName     string   `toml:"container_name"`
	Prompt            string   `toml:"prompt"`
	Template          string   `toml:"template"`
	TemplateVars      []string `toml:"template_vars"`
	Instructions      string   `toml:"instructions"`
	InstructionsFile  string   `toml:"instructions_file"`
	Network           string   `toml:"network"`
	Memory            string   `toml:"memory"`
	CPUs              string   `toml:"cpus"`
	PIDsLimit         int      `toml:"pids_limit"`
	Identity          string   `toml:"identity"`
	AWSProfile        string   `toml:"aws"`
	MaxCost           string   `toml:"max_cost"`
	Notify            string   `toml:"notify"`
	OnExit            string   `toml:"on_exit"`
	OnComplete        string   `toml:"on_complete"`
	OnFail            string   `toml:"on_fail"`
	SSH               *bool    `toml:"ssh"`
	ReuseAuth         *bool    `toml:"reuse_auth"`
	EphemeralAuth     *bool    `toml:"ephemeral_auth"`
	ReuseGHAuth       *bool    `toml:"reuse_gh_auth"`
	SeedAuth          *bool    `toml:"seed_auth"`
	Docker            *bool    `toml:"docker"`
	DockerSocket      *bool    `toml:"docker_socket"`
	AutoTrust         *bool    `toml:"auto_trust"`
	Background        *bool    `toml:"background"`
	AllowSetupScripts *bool    `toml:"allow_setup_scripts"`
}

type Catalog struct {
	Profiles []Profile
	byName   map[string]Profile
}

var profileNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func UserDir() string {
	return filepath.Join(config.UserDir(), "agents")
}

func ProjectDir(cwd string) string {
	return filepath.Join(cwd, ".safe-ag", "agents")
}

func DefaultDirs(cwd string) []string {
	return []string{UserDir(), ProjectDir(cwd)}
}

func LoadDirs(dirs []string) (Catalog, error) {
	merged := make(map[string]Profile)
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		profiles, err := loadDir(dir)
		if err != nil {
			return Catalog{}, err
		}
		for _, profile := range profiles {
			merged[profile.Name] = profile
		}
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)

	catalog := Catalog{byName: merged}
	for _, name := range names {
		catalog.Profiles = append(catalog.Profiles, merged[name])
	}
	return catalog, nil
}

func (c Catalog) Get(name string) (Profile, bool) {
	profile, ok := c.byName[name]
	return profile, ok
}

func loadDir(dir string) ([]Profile, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read profile dir %q: %w", dir, err)
	}
	var result []Profile
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		profile, err := LoadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		result = append(result, profile)
	}
	return result, nil
}

func LoadFile(path string) (Profile, error) {
	var raw rawProfile
	md, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return Profile{}, fmt.Errorf("parse profile %q: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		var keys []string
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		sort.Strings(keys)
		return Profile{}, fmt.Errorf("unsupported profile keys in %q: %s", path, strings.Join(keys, ", "))
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if raw.Name != "" {
		name = raw.Name
	}
	profile := Profile{
		Name:              name,
		Source:            path,
		AgentType:         raw.AgentType,
		Repos:             append([]string{}, raw.Repos...),
		ContainerName:     raw.ContainerName,
		Prompt:            raw.Prompt,
		Template:          raw.Template,
		TemplateVars:      append([]string{}, raw.TemplateVars...),
		Instructions:      raw.Instructions,
		InstructionsFile:  raw.InstructionsFile,
		Network:           raw.Network,
		Memory:            raw.Memory,
		CPUs:              raw.CPUs,
		PIDsLimit:         raw.PIDsLimit,
		Identity:          raw.Identity,
		AWSProfile:        raw.AWSProfile,
		MaxCost:           raw.MaxCost,
		Notify:            raw.Notify,
		OnExit:            raw.OnExit,
		OnComplete:        raw.OnComplete,
		OnFail:            raw.OnFail,
		SSH:               raw.SSH,
		ReuseAuth:         raw.ReuseAuth,
		EphemeralAuth:     raw.EphemeralAuth,
		ReuseGHAuth:       raw.ReuseGHAuth,
		SeedAuth:          raw.SeedAuth,
		Docker:            raw.Docker,
		DockerSocket:      raw.DockerSocket,
		AutoTrust:         raw.AutoTrust,
		Background:        raw.Background,
		AllowSetupScripts: raw.AllowSetupScripts,
	}
	if len(raw.Repo) > 0 {
		profile.Repos = append(profile.Repos, raw.Repo...)
	}
	if err := validate(profile); err != nil {
		return Profile{}, fmt.Errorf("%s: %w", path, err)
	}
	return profile, nil
}

func validate(profile Profile) error {
	if !profileNameRE.MatchString(profile.Name) {
		return fmt.Errorf("invalid profile name %q", profile.Name)
	}
	switch profile.AgentType {
	case "", "claude", "codex", "shell":
	default:
		return fmt.Errorf("agent_type must be claude, codex, or shell (got %q)", profile.AgentType)
	}
	for field, value := range map[string]string{
		"name":              profile.Name,
		"container_name":    profile.ContainerName,
		"network":           profile.Network,
		"memory":            profile.Memory,
		"cpus":              profile.CPUs,
		"identity":          profile.Identity,
		"aws":               profile.AWSProfile,
		"max_cost":          profile.MaxCost,
		"notify":            profile.Notify,
		"on_exit":           profile.OnExit,
		"on_complete":       profile.OnComplete,
		"on_fail":           profile.OnFail,
		"prompt":            profile.Prompt,
		"instructions":      profile.Instructions,
		"instructions_file": profile.InstructionsFile,
	} {
		if strings.Contains(value, "\x00") {
			return fmt.Errorf("%s contains NUL byte", field)
		}
	}
	for _, repo := range profile.Repos {
		if strings.Contains(repo, "\x00") {
			return fmt.Errorf("repo contains NUL byte")
		}
	}
	return nil
}
