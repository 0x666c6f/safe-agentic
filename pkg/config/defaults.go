package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/0x666c6f/berth/pkg/validate"
	"github.com/BurntSushi/toml"
)

type DefaultsSection struct {
	CPUs           string `toml:"cpus"`
	Memory         string `toml:"memory"`
	PIDsLimit      int    `toml:"pids_limit"`
	SSH            bool   `toml:"ssh"`
	Docker         bool   `toml:"docker"`
	DockerSocket   bool   `toml:"docker_socket"`
	ReuseAuth      bool   `toml:"reuse_auth"`
	ReuseGHAuth    bool   `toml:"reuse_gh_auth"`
	SeedAuth       bool   `toml:"seed_auth"`
	Network        string `toml:"network"`
	Identity       string `toml:"identity"`
	WorktreesDir   string `toml:"worktrees_dir"`
	WorktreesMount bool   `toml:"worktrees_mount"`
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
	CPUs           *string `toml:"cpus"`
	Memory         *string `toml:"memory"`
	PIDsLimit      *int    `toml:"pids_limit"`
	SSH            *bool   `toml:"ssh"`
	Docker         *bool   `toml:"docker"`
	DockerSocket   *bool   `toml:"docker_socket"`
	ReuseAuth      *bool   `toml:"reuse_auth"`
	ReuseGHAuth    *bool   `toml:"reuse_gh_auth"`
	SeedAuth       *bool   `toml:"seed_auth"`
	Network        *string `toml:"network"`
	Identity       *string `toml:"identity"`
	WorktreesDir   *string `toml:"worktrees_dir"`
	WorktreesMount *bool   `toml:"worktrees_mount"`
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
	"defaults.cpus":                        "defaults.cpus",
	"defaults.memory":                      "defaults.memory",
	"defaults.pids_limit":                  "defaults.pids_limit",
	"defaults.ssh":                         "defaults.ssh",
	"defaults.docker":                      "defaults.docker",
	"defaults.docker_socket":               "defaults.docker_socket",
	"defaults.reuse_auth":                  "defaults.reuse_auth",
	"defaults.reuse_gh_auth":               "defaults.reuse_gh_auth",
	"defaults.seed_auth":                   "defaults.seed_auth",
	"defaults.network":                     "defaults.network",
	"defaults.identity":                    "defaults.identity",
	"defaults.worktrees_dir":               "defaults.worktrees_dir",
	"defaults.worktrees_mount":             "defaults.worktrees_mount",
	"git.author_name":                      "git.author_name",
	"git.author_email":                     "git.author_email",
	"git.committer_name":                   "git.committer_name",
	"git.committer_email":                  "git.committer_email",
	"BERTH_DEFAULT_CPUS":            "defaults.cpus",
	"BERTH_DEFAULT_MEMORY":          "defaults.memory",
	"BERTH_DEFAULT_PIDS_LIMIT":      "defaults.pids_limit",
	"BERTH_DEFAULT_SSH":             "defaults.ssh",
	"BERTH_DEFAULT_DOCKER":          "defaults.docker",
	"BERTH_DEFAULT_DOCKER_SOCKET":   "defaults.docker_socket",
	"BERTH_DEFAULT_REUSE_AUTH":      "defaults.reuse_auth",
	"BERTH_DEFAULT_REUSE_GH_AUTH":   "defaults.reuse_gh_auth",
	"BERTH_DEFAULT_SEED_AUTH":       "defaults.seed_auth",
	"BERTH_DEFAULT_NETWORK":         "defaults.network",
	"BERTH_DEFAULT_IDENTITY":        "defaults.identity",
	"BERTH_DEFAULT_WORKTREES_DIR":   "defaults.worktrees_dir",
	"BERTH_DEFAULT_WORKTREES_MOUNT": "defaults.worktrees_mount",
	"GIT_AUTHOR_NAME":                      "git.author_name",
	"GIT_AUTHOR_EMAIL":                     "git.author_email",
	"GIT_COMMITTER_NAME":                   "git.committer_name",
	"GIT_COMMITTER_EMAIL":                  "git.committer_email",
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
			SeedAuth:    false,
		},
	}
}

func UserDir() string {
	return berthDir("BERTH_CONFIG_HOME")
}

func berthDir(envKey string) string {
	if base := os.Getenv(envKey); base != "" {
		return filepath.Join(base, ".berth")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			fmt.Fprintf(os.Stderr, "[berth] warning: resolve home dir: %v; using %s\n", err, filepath.Join(cwd, ".berth"))
			return filepath.Join(cwd, ".berth")
		}
		fmt.Fprintf(os.Stderr, "[berth] warning: resolve home dir: %v; using /.berth\n", err)
		return filepath.Join(string(os.PathSeparator), ".berth")
	}
	return filepath.Join(home, ".berth")
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

func StateDir() string {
	if base := os.Getenv("BERTH_STATE_HOME"); base != "" {
		return filepath.Join(base, ".berth", "state")
	}
	return filepath.Join(UserDir(), "state")
}

// WorktreesDir returns the host directory that berth mounts into the VM
// for managed git worktrees. Everything under this directory is exposed to the
// VM at the stable mount point /worktrees; nothing else on the host is. It must
// live under the invoking user's home directory because the Apple container
// machine can only mount the user's home. Precedence: defaults.worktrees_dir in
// config.toml, then the default ~/.berth/worktrees.
func WorktreesDir() string {
	if cfg, err := LoadDefaults(ConfigPath()); err == nil && cfg.Defaults.WorktreesDir != "" {
		return cfg.Defaults.WorktreesDir
	}
	return filepath.Join(UserDir(), "worktrees")
}

// WorktreesMountEnabled reports whether the operator has opted into the worktree
// mount. It is OFF by default: enabling it switches the VM to home-mount=rw,
// which shares the host home with the machine (a weaker boundary than the
// default home-mount=none). Callers must treat this as an explicit trust choice.
func WorktreesMountEnabled() bool {
	cfg, err := LoadDefaults(ConfigPath())
	if err != nil {
		return false
	}
	return cfg.Defaults.WorktreesMount
}

func validateWorktreesDir(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("worktrees_dir must not be empty")
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("worktrees_dir must be an absolute path: %s", value)
	}
	if strings.ContainsAny(value, ",\n\r\x00") {
		return fmt.Errorf("worktrees_dir must not contain commas or newlines: %q", value)
	}
	return nil
}

func AuditPath() string {
	return filepath.Join(StateDir(), "audit.jsonl")
}

func EventsPath() string {
	return filepath.Join(StateDir(), "events.jsonl")
}

func PipelineLogsDir() string {
	return filepath.Join(StateDir(), "pipelines")
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
		if raw.Defaults.SeedAuth != nil {
			cfg.Defaults.SeedAuth = *raw.Defaults.SeedAuth
		}
		if raw.Defaults.Network != nil {
			cfg.Defaults.Network = *raw.Defaults.Network
		}
		if raw.Defaults.Identity != nil {
			cfg.Defaults.Identity = *raw.Defaults.Identity
		}
		if raw.Defaults.WorktreesDir != nil {
			cfg.Defaults.WorktreesDir = *raw.Defaults.WorktreesDir
		}
		if raw.Defaults.WorktreesMount != nil {
			cfg.Defaults.WorktreesMount = *raw.Defaults.WorktreesMount
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
			s.SeedAuth == nil &&
			s.Network == nil &&
			s.Identity == nil &&
			s.WorktreesDir == nil &&
			s.WorktreesMount == nil)
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
	case "defaults.seed_auth":
		return strconv.FormatBool(cfg.Defaults.SeedAuth), nil
	case "defaults.network":
		return cfg.Defaults.Network, nil
	case "defaults.identity":
		return cfg.Defaults.Identity, nil
	case "defaults.worktrees_dir":
		return cfg.Defaults.WorktreesDir, nil
	case "defaults.worktrees_mount":
		return strconv.FormatBool(cfg.Defaults.WorktreesMount), nil
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
		if err := validate.CPUs(value); err != nil {
			return err
		}
		raw.ensureDefaults().CPUs = stringPtr(value)
	case "defaults.memory":
		if err := validate.MemoryLimit(value); err != nil {
			return err
		}
		raw.ensureDefaults().Memory = stringPtr(value)
	case "defaults.pids_limit":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid integer for %s: %w", canonical, err)
		}
		if parsed != 0 {
			if err := validate.PIDsLimit(parsed); err != nil {
				return err
			}
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
	case "defaults.seed_auth":
		return setBool(&raw.ensureDefaults().SeedAuth, canonical, value)
	case "defaults.network":
		if err := validate.NetworkName(value); err != nil {
			return err
		}
		raw.ensureDefaults().Network = stringPtr(value)
	case "defaults.identity":
		if _, _, err := ParseIdentity(value); err != nil {
			return err
		}
		raw.ensureDefaults().Identity = stringPtr(value)
	case "defaults.worktrees_dir":
		if err := validateWorktreesDir(value); err != nil {
			return err
		}
		raw.ensureDefaults().WorktreesDir = stringPtr(value)
	case "defaults.worktrees_mount":
		return setBool(&raw.ensureDefaults().WorktreesMount, canonical, value)
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
	case "defaults.seed_auth":
		if raw.Defaults != nil {
			raw.Defaults.SeedAuth = nil
		}
	case "defaults.network":
		if raw.Defaults != nil {
			raw.Defaults.Network = nil
		}
	case "defaults.identity":
		if raw.Defaults != nil {
			raw.Defaults.Identity = nil
		}
	case "defaults.worktrees_dir":
		if raw.Defaults != nil {
			raw.Defaults.WorktreesDir = nil
		}
	case "defaults.worktrees_mount":
		if raw.Defaults != nil {
			raw.Defaults.WorktreesMount = nil
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
