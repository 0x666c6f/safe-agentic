package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type DefaultsSection struct {
	CPUs         string `toml:"cpus"`
	Memory       string `toml:"memory"`
	PIDsLimit    int    `toml:"pids_limit"`
	SSH          bool   `toml:"ssh"`
	Docker       bool   `toml:"docker"`
	DockerSocket bool   `toml:"docker_socket"`
	ReuseAuth    bool   `toml:"reuse_auth"`
	ReuseGHAuth  bool   `toml:"reuse_gh_auth"`
	Network      string `toml:"network"`
	Identity     string `toml:"identity"`
}

type GitSection struct {
	AuthorName     string `toml:"author_name"`
	AuthorEmail    string `toml:"author_email"`
	CommitterName  string `toml:"committer_name"`
	CommitterEmail string `toml:"committer_email"`
}

type Config struct {
	Version  int             `toml:"version"`
	Defaults DefaultsSection `toml:"defaults"`
	Git      GitSection      `toml:"git"`
}

type fileDefaultsSection struct {
	CPUs         *string `toml:"cpus"`
	Memory       *string `toml:"memory"`
	PIDsLimit    *int    `toml:"pids_limit"`
	SSH          *bool   `toml:"ssh"`
	Docker       *bool   `toml:"docker"`
	DockerSocket *bool   `toml:"docker_socket"`
	ReuseAuth    *bool   `toml:"reuse_auth"`
	ReuseGHAuth  *bool   `toml:"reuse_gh_auth"`
	Network      *string `toml:"network"`
	Identity     *string `toml:"identity"`
}

type fileGitSection struct {
	AuthorName     *string `toml:"author_name"`
	AuthorEmail    *string `toml:"author_email"`
	CommitterName  *string `toml:"committer_name"`
	CommitterEmail *string `toml:"committer_email"`
}

type FileConfig struct {
	Version  int                  `toml:"version"`
	Defaults *fileDefaultsSection `toml:"defaults"`
	Git      *fileGitSection      `toml:"git"`
}

var keyAliases = map[string]string{
	"defaults.cpus":                      "defaults.cpus",
	"defaults.memory":                    "defaults.memory",
	"defaults.pids_limit":                "defaults.pids_limit",
	"defaults.ssh":                       "defaults.ssh",
	"defaults.docker":                    "defaults.docker",
	"defaults.docker_socket":             "defaults.docker_socket",
	"defaults.reuse_auth":                "defaults.reuse_auth",
	"defaults.reuse_gh_auth":             "defaults.reuse_gh_auth",
	"defaults.network":                   "defaults.network",
	"defaults.identity":                  "defaults.identity",
	"git.author_name":                    "git.author_name",
	"git.author_email":                   "git.author_email",
	"git.committer_name":                 "git.committer_name",
	"git.committer_email":                "git.committer_email",
	"SAFE_AGENTIC_DEFAULT_CPUS":          "defaults.cpus",
	"SAFE_AGENTIC_DEFAULT_MEMORY":        "defaults.memory",
	"SAFE_AGENTIC_DEFAULT_PIDS_LIMIT":    "defaults.pids_limit",
	"SAFE_AGENTIC_DEFAULT_SSH":           "defaults.ssh",
	"SAFE_AGENTIC_DEFAULT_DOCKER":        "defaults.docker",
	"SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET": "defaults.docker_socket",
	"SAFE_AGENTIC_DEFAULT_REUSE_AUTH":    "defaults.reuse_auth",
	"SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH": "defaults.reuse_gh_auth",
	"SAFE_AGENTIC_DEFAULT_NETWORK":       "defaults.network",
	"SAFE_AGENTIC_DEFAULT_IDENTITY":      "defaults.identity",
	"GIT_AUTHOR_NAME":                    "git.author_name",
	"GIT_AUTHOR_EMAIL":                   "git.author_email",
	"GIT_COMMITTER_NAME":                 "git.committer_name",
	"GIT_COMMITTER_EMAIL":                "git.committer_email",
}

// Defaults returns the compiled-in fallback config used when config.toml is absent.
func Defaults() Config {
	return Config{
		Version: 1,
		Defaults: DefaultsSection{
			CPUs:        "4",
			Memory:      "8g",
			PIDsLimit:   512,
			ReuseAuth:   false,
			ReuseGHAuth: false,
		},
	}
}

func UserDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			fmt.Fprintf(os.Stderr, "[safe-ag] warning: resolve home dir: %v; using %s\n", err, filepath.Join(cwd, ".safe-ag"))
			return filepath.Join(cwd, ".safe-ag")
		}
		fmt.Fprintf(os.Stderr, "[safe-ag] warning: resolve home dir: %v; using /.safe-ag\n", err)
		return filepath.Join(string(os.PathSeparator), ".safe-ag")
	}
	return filepath.Join(home, ".safe-ag")
}

func ConfigPath() string {
	return filepath.Join(UserDir(), "config.toml")
}

func TemplatesDir() string {
	return filepath.Join(UserDir(), "templates")
}

func PipelinesDir() string {
	return filepath.Join(UserDir(), "pipelines")
}

func CronPath() string {
	return filepath.Join(UserDir(), "cron.json")
}

// DefaultsPath kept as a compatibility name for existing callers.
func DefaultsPath() string {
	return ConfigPath()
}

func LoadDefaults(path string) (Config, error) {
	raw, err := LoadRawConfig(path)
	if err != nil {
		return Config{}, err
	}
	return raw.Effective(), nil
}

func LoadRawConfig(path string) (FileConfig, error) {
	var raw FileConfig
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return raw, nil
		}
		return raw, fmt.Errorf("open config file: %w", err)
	}
	md, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return raw, fmt.Errorf("parse config file: %w", err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		var keys []string
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		sort.Strings(keys)
		return raw, fmt.Errorf("unsupported config keys: %s", strings.Join(keys, ", "))
	}
	return raw, nil
}

func SaveRawConfig(path string, raw FileConfig) error {
	raw.normalize()
	if raw.IsZero() {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove config file: %w", err)
		}
		return nil
	}
	raw.Version = 1
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return fmt.Errorf("encode config file: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

func (raw FileConfig) Effective() Config {
	cfg := Defaults()
	if raw.Version != 0 {
		cfg.Version = raw.Version
	}
	if raw.Defaults != nil {
		if raw.Defaults.CPUs != nil {
			cfg.Defaults.CPUs = *raw.Defaults.CPUs
		}
		if raw.Defaults.Memory != nil {
			cfg.Defaults.Memory = *raw.Defaults.Memory
		}
		if raw.Defaults.PIDsLimit != nil {
			cfg.Defaults.PIDsLimit = *raw.Defaults.PIDsLimit
		}
		if raw.Defaults.SSH != nil {
			cfg.Defaults.SSH = *raw.Defaults.SSH
		}
		if raw.Defaults.Docker != nil {
			cfg.Defaults.Docker = *raw.Defaults.Docker
		}
		if raw.Defaults.DockerSocket != nil {
			cfg.Defaults.DockerSocket = *raw.Defaults.DockerSocket
		}
		if raw.Defaults.ReuseAuth != nil {
			cfg.Defaults.ReuseAuth = *raw.Defaults.ReuseAuth
		}
		if raw.Defaults.ReuseGHAuth != nil {
			cfg.Defaults.ReuseGHAuth = *raw.Defaults.ReuseGHAuth
		}
		if raw.Defaults.Network != nil {
			cfg.Defaults.Network = *raw.Defaults.Network
		}
		if raw.Defaults.Identity != nil {
			cfg.Defaults.Identity = *raw.Defaults.Identity
		}
	}
	if raw.Git != nil {
		if raw.Git.AuthorName != nil {
			cfg.Git.AuthorName = *raw.Git.AuthorName
		}
		if raw.Git.AuthorEmail != nil {
			cfg.Git.AuthorEmail = *raw.Git.AuthorEmail
		}
		if raw.Git.CommitterName != nil {
			cfg.Git.CommitterName = *raw.Git.CommitterName
		}
		if raw.Git.CommitterEmail != nil {
			cfg.Git.CommitterEmail = *raw.Git.CommitterEmail
		}
	}
	return cfg
}

func (raw FileConfig) IsZero() bool {
	return raw.Version == 0 && raw.Defaults == nil && raw.Git == nil
}

func (raw *FileConfig) normalize() {
	if raw.Defaults != nil && raw.Defaults.isZero() {
		raw.Defaults = nil
	}
	if raw.Git != nil && raw.Git.isZero() {
		raw.Git = nil
	}
	if raw.Defaults == nil && raw.Git == nil {
		raw.Version = 0
	}
}

func (s *fileDefaultsSection) isZero() bool {
	return s == nil ||
		(s.CPUs == nil &&
			s.Memory == nil &&
			s.PIDsLimit == nil &&
			s.SSH == nil &&
			s.Docker == nil &&
			s.DockerSocket == nil &&
			s.ReuseAuth == nil &&
			s.ReuseGHAuth == nil &&
			s.Network == nil &&
			s.Identity == nil)
}

func (s *fileGitSection) isZero() bool {
	return s == nil ||
		(s.AuthorName == nil &&
			s.AuthorEmail == nil &&
			s.CommitterName == nil &&
			s.CommitterEmail == nil)
}

func ResolveKey(key string) (string, error) {
	canonical, ok := keyAliases[key]
	if !ok {
		return "", fmt.Errorf("unsupported key %q", key)
	}
	return canonical, nil
}

func AllowedKeys() []string {
	keys := make([]string, 0, len(keyAliases))
	for key := range keyAliases {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func KeyAllowed(key string) bool {
	_, err := ResolveKey(key)
	return err == nil
}

func GetValue(cfg Config, key string) (string, error) {
	canonical, err := ResolveKey(key)
	if err != nil {
		return "", err
	}
	switch canonical {
	case "defaults.cpus":
		return cfg.Defaults.CPUs, nil
	case "defaults.memory":
		return cfg.Defaults.Memory, nil
	case "defaults.pids_limit":
		return strconv.Itoa(cfg.Defaults.PIDsLimit), nil
	case "defaults.ssh":
		return strconv.FormatBool(cfg.Defaults.SSH), nil
	case "defaults.docker":
		return strconv.FormatBool(cfg.Defaults.Docker), nil
	case "defaults.docker_socket":
		return strconv.FormatBool(cfg.Defaults.DockerSocket), nil
	case "defaults.reuse_auth":
		return strconv.FormatBool(cfg.Defaults.ReuseAuth), nil
	case "defaults.reuse_gh_auth":
		return strconv.FormatBool(cfg.Defaults.ReuseGHAuth), nil
	case "defaults.network":
		return cfg.Defaults.Network, nil
	case "defaults.identity":
		return cfg.Defaults.Identity, nil
	case "git.author_name":
		return cfg.Git.AuthorName, nil
	case "git.author_email":
		return cfg.Git.AuthorEmail, nil
	case "git.committer_name":
		return cfg.Git.CommitterName, nil
	case "git.committer_email":
		return cfg.Git.CommitterEmail, nil
	default:
		return "", fmt.Errorf("unsupported key %q", key)
	}
}

func SetValue(raw *FileConfig, key, value string) error {
	canonical, err := ResolveKey(key)
	if err != nil {
		return err
	}
	switch canonical {
	case "defaults.cpus":
		raw.ensureDefaults().CPUs = stringPtr(value)
	case "defaults.memory":
		raw.ensureDefaults().Memory = stringPtr(value)
	case "defaults.pids_limit":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer for %s: %w", canonical, err)
		}
		raw.ensureDefaults().PIDsLimit = intPtr(parsed)
	case "defaults.ssh":
		return setBool(&raw.ensureDefaults().SSH, canonical, value)
	case "defaults.docker":
		return setBool(&raw.ensureDefaults().Docker, canonical, value)
	case "defaults.docker_socket":
		return setBool(&raw.ensureDefaults().DockerSocket, canonical, value)
	case "defaults.reuse_auth":
		return setBool(&raw.ensureDefaults().ReuseAuth, canonical, value)
	case "defaults.reuse_gh_auth":
		return setBool(&raw.ensureDefaults().ReuseGHAuth, canonical, value)
	case "defaults.network":
		raw.ensureDefaults().Network = stringPtr(value)
	case "defaults.identity":
		raw.ensureDefaults().Identity = stringPtr(value)
	case "git.author_name":
		raw.ensureGit().AuthorName = stringPtr(value)
	case "git.author_email":
		raw.ensureGit().AuthorEmail = stringPtr(value)
	case "git.committer_name":
		raw.ensureGit().CommitterName = stringPtr(value)
	case "git.committer_email":
		raw.ensureGit().CommitterEmail = stringPtr(value)
	}
	return nil
}

func ResetValue(raw *FileConfig, key string) error {
	canonical, err := ResolveKey(key)
	if err != nil {
		return err
	}
	switch canonical {
	case "defaults.cpus":
		if raw.Defaults != nil {
			raw.Defaults.CPUs = nil
		}
	case "defaults.memory":
		if raw.Defaults != nil {
			raw.Defaults.Memory = nil
		}
	case "defaults.pids_limit":
		if raw.Defaults != nil {
			raw.Defaults.PIDsLimit = nil
		}
	case "defaults.ssh":
		if raw.Defaults != nil {
			raw.Defaults.SSH = nil
		}
	case "defaults.docker":
		if raw.Defaults != nil {
			raw.Defaults.Docker = nil
		}
	case "defaults.docker_socket":
		if raw.Defaults != nil {
			raw.Defaults.DockerSocket = nil
		}
	case "defaults.reuse_auth":
		if raw.Defaults != nil {
			raw.Defaults.ReuseAuth = nil
		}
	case "defaults.reuse_gh_auth":
		if raw.Defaults != nil {
			raw.Defaults.ReuseGHAuth = nil
		}
	case "defaults.network":
		if raw.Defaults != nil {
			raw.Defaults.Network = nil
		}
	case "defaults.identity":
		if raw.Defaults != nil {
			raw.Defaults.Identity = nil
		}
	case "git.author_name":
		if raw.Git != nil {
			raw.Git.AuthorName = nil
		}
	case "git.author_email":
		if raw.Git != nil {
			raw.Git.AuthorEmail = nil
		}
	case "git.committer_name":
		if raw.Git != nil {
			raw.Git.CommitterName = nil
		}
	case "git.committer_email":
		if raw.Git != nil {
			raw.Git.CommitterEmail = nil
		}
	}
	raw.normalize()
	return nil
}

func (raw *FileConfig) ensureDefaults() *fileDefaultsSection {
	if raw.Defaults == nil {
		raw.Defaults = &fileDefaultsSection{}
	}
	return raw.Defaults
}

func (raw *FileConfig) ensureGit() *fileGitSection {
	if raw.Git == nil {
		raw.Git = &fileGitSection{}
	}
	return raw.Git
}

func setBool(dst **bool, key, value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("invalid boolean for %s: %w", key, err)
	}
	*dst = boolPtr(parsed)
	return nil
}

func stringPtr(value string) *string { return &value }
func intPtr(value int) *int          { return &value }
func boolPtr(value bool) *bool       { return &value }
