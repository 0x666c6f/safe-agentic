package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0x666c6f/berth/pkg/config"
	"github.com/BurntSushi/toml"
)

const (
	DockerModeOff        = "off"
	DockerModeDinD       = "dind"
	DockerModeHostSocket = "host-socket"
	NetworkManaged       = "managed"
	NetworkNone          = "none"
)

type SpawnRequest struct {
	DockerMode        string
	Network           string
	AWSProfile        string
	SSH               bool
	ReuseAuth         bool
	ReuseGHAuth       bool
	SeedAuth          bool
	AllowSetupScripts bool
}

type RuleSet struct {
	Source string
	Allow  AllowRules `toml:"allow"`
}

type AllowRules struct {
	DockerModes       *[]string `toml:"docker_modes"`
	Networks          *[]string `toml:"networks"`
	AWSProfiles       *[]string `toml:"aws_profiles"`
	SSH               *bool     `toml:"ssh"`
	ReuseAuth         *bool     `toml:"reuse_auth"`
	ReuseGHAuth       *bool     `toml:"reuse_gh_auth"`
	SeedAuth          *bool     `toml:"seed_auth"`
	AllowSetupScripts *bool     `toml:"setup_scripts"`
}

func UserRulesPath() string {
	return filepath.Join(config.UserDir(), "rules.toml")
}

func DefaultRulePaths() []string {
	userPath := UserRulesPath()
	paths := []string{userPath}
	if projectPath := findNearestProjectRules(userPath); projectPath != "" {
		paths = append(paths, projectPath)
	}
	return paths
}

func LoadDefault() ([]RuleSet, error) {
	return Load(DefaultRulePaths())
}

func Load(paths []string) ([]RuleSet, error) {
	var rules []RuleSet
	for _, path := range paths {
		if path == "" {
			continue
		}
		rule, ok, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		if ok {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

func LoadFile(path string) (RuleSet, bool, error) {
	var rule RuleSet
	rule.Source = path
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return rule, false, nil
		}
		return rule, false, fmt.Errorf("open policy rules %s: %w", path, err)
	}
	md, err := toml.DecodeFile(path, &rule)
	if err != nil {
		return rule, false, fmt.Errorf("parse policy rules %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		var keys []string
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		sort.Strings(keys)
		return rule, false, fmt.Errorf("unsupported policy keys in %s: %s", path, strings.Join(keys, ", "))
	}
	if err := rule.validate(); err != nil {
		return rule, false, err
	}
	return rule, true, nil
}

func Enforce(rules []RuleSet, req SpawnRequest) error {
	for _, rule := range rules {
		if err := rule.enforce(req); err != nil {
			return err
		}
	}
	return nil
}

func (rule RuleSet) validate() error {
	if err := validateList(rule.Source, "allow.docker_modes", rule.Allow.DockerModes, map[string]bool{
		DockerModeOff:        true,
		DockerModeDinD:       true,
		DockerModeHostSocket: true,
	}); err != nil {
		return err
	}
	if err := validateStringList(rule.Source, "allow.networks", rule.Allow.Networks); err != nil {
		return err
	}
	if err := validateStringList(rule.Source, "allow.aws_profiles", rule.Allow.AWSProfiles); err != nil {
		return err
	}
	return nil
}

func (rule RuleSet) enforce(req SpawnRequest) error {
	source := rule.Source
	if source == "" {
		source = "policy"
	}
	if !containsAllowed(rule.Allow.DockerModes, req.DockerMode) {
		return fmt.Errorf("%s denies docker mode %q", source, req.DockerMode)
	}
	if !containsAllowed(rule.Allow.Networks, req.Network) {
		return fmt.Errorf("%s denies network %q", source, req.Network)
	}
	if req.AWSProfile != "" && !containsAllowed(rule.Allow.AWSProfiles, req.AWSProfile) {
		return fmt.Errorf("%s denies AWS profile %q", source, req.AWSProfile)
	}
	if deniesBool(rule.Allow.SSH, req.SSH) {
		return fmt.Errorf("%s denies SSH forwarding", source)
	}
	if deniesBool(rule.Allow.ReuseAuth, req.ReuseAuth) {
		return fmt.Errorf("%s denies shared agent auth", source)
	}
	if deniesBool(rule.Allow.ReuseGHAuth, req.ReuseGHAuth) {
		return fmt.Errorf("%s denies shared GitHub auth", source)
	}
	if deniesBool(rule.Allow.SeedAuth, req.SeedAuth) {
		return fmt.Errorf("%s denies host auth seeding", source)
	}
	if deniesBool(rule.Allow.AllowSetupScripts, req.AllowSetupScripts) {
		return fmt.Errorf("%s denies repo setup scripts", source)
	}
	return nil
}

func containsAllowed(allowed *[]string, value string) bool {
	if allowed == nil {
		return true
	}
	for _, candidate := range *allowed {
		if candidate == value {
			return true
		}
	}
	return false
}

func deniesBool(allowed *bool, enabled bool) bool {
	return allowed != nil && !*allowed && enabled
}

func validateList(source, key string, values *[]string, allowed map[string]bool) error {
	if values == nil {
		return nil
	}
	if len(*values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, value := range *values {
		if value == "" {
			return fmt.Errorf("%s has empty %s entry", source, key)
		}
		if seen[value] {
			return fmt.Errorf("%s has duplicate %s entry %q", source, key, value)
		}
		seen[value] = true
		if !allowed[value] {
			return fmt.Errorf("%s has unsupported %s entry %q", source, key, value)
		}
	}
	return nil
}

func validateStringList(source, key string, values *[]string) error {
	if values == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, value := range *values {
		if value == "" {
			return fmt.Errorf("%s has empty %s entry", source, key)
		}
		if seen[value] {
			return fmt.Errorf("%s has duplicate %s entry %q", source, key, value)
		}
		seen[value] = true
	}
	return nil
}

func findNearestProjectRules(userPath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	userPath = filepath.Clean(userPath)
	for {
		candidate := filepath.Join(cwd, ".berth", "rules.toml")
		if filepath.Clean(candidate) != userPath {
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return ""
		}
		cwd = parent
	}
}
