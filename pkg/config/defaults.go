package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the parsed values from a safe-agentic defaults file. Each field
// corresponds to one of the allowed SAFE_AGENTIC_DEFAULT_* or GIT_*
// environment variable keys.
type Config struct {
	// SAFE_AGENTIC_DEFAULT_* keys
	CPUs         string
	Memory       string
	PIDsLimit    string
	SSH          string
	Docker       string
	DockerSocket string
	ReuseAuth    string
	ReuseGHAuth  string
	Network      string
	Identity     string

	// GIT identity keys
	GitAuthorName     string
	GitAuthorEmail    string
	GitCommitterName  string
	GitCommitterEmail string
}

// Defaults returns a Config with the CLI hardcoded defaults:
// DEFAULT_CPUS=4, DEFAULT_MEMORY=8g, DEFAULT_PIDS_LIMIT=512.
func Defaults() Config {
	return Config{
		CPUs:        "4",
		Memory:      "8g",
		PIDsLimit:   "512",
		ReuseAuth:   "true",
		ReuseGHAuth: "true",
	}
}

// DefaultsPath returns the path to the defaults file, respecting
// XDG_CONFIG_HOME.
func DefaultsPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "safe-agentic", "defaults.sh")
}

// LoadDefaults reads and parses a defaults file at path, returning a populated
// Config. If the file does not exist, it returns Defaults() with no error. It
// mirrors load_user_defaults() / parse_defaults_line() / parse_defaults_value()
// in the former shell implementation.
func LoadDefaults(path string) (Config, error) {
	cfg := Defaults()

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("open defaults file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		// Strip CR for Windows-style line endings.
		line = strings.TrimRight(line, "\r")

		if err := parseLine(line, lineNo, &cfg); err != nil {
			return cfg, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, fmt.Errorf("read defaults file: %w", err)
	}
	return cfg, nil
}

// parseLine processes a single line from the defaults file. It ignores blank
// lines and comments, strips an optional "export " prefix, then parses the
// KEY=value pair and stores it in cfg.
func parseLine(line string, lineNo int, cfg *Config) error {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}

	// Strip optional "export " prefix (with one or more spaces).
	if strings.HasPrefix(line, "export") {
		rest := strings.TrimPrefix(line, "export")
		if len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
			line = strings.TrimSpace(rest)
		}
	}

	eqIdx := strings.Index(line, "=")
	if eqIdx < 0 {
		return fmt.Errorf("unsupported line: use simple KEY=value assignments only")
	}

	key := strings.TrimSpace(line[:eqIdx])
	rawValue := line[eqIdx+1:]

	if !KeyAllowed(key) {
		return fmt.Errorf("unsupported defaults key %q", key)
	}

	value, err := parseValue(rawValue)
	if err != nil {
		return fmt.Errorf("unsupported value for %s: use KEY=value or quote the full value", key)
	}

	setField(cfg, key, value)
	return nil
}

// parseValue parses a raw value string. It handles double-quoted values
// (with \" and \\ escape sequences), single-quoted values (no escapes), and
// bare words (rejected if they contain whitespace). It mirrors
// from the legacy shell parser.
func parseValue(raw string) (string, error) {
	raw = strings.TrimSpace(raw)

	switch {
	case strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`):
		inner := raw[1 : len(raw)-1]
		// Process escape sequences: \" -> " and \\ -> \.
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		inner = strings.ReplaceAll(inner, `\\`, `\`)
		return inner, nil

	case strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'"):
		inner := raw[1 : len(raw)-1]
		return inner, nil

	default:
		// Bare word: reject if it contains any whitespace.
		if strings.ContainsAny(raw, " \t") {
			return "", fmt.Errorf("value contains whitespace")
		}
		return raw, nil
	}
}

// KeyAllowed returns true if key is in the allowlist of supported keys. It
// from the legacy shell parser.
func KeyAllowed(key string) bool {
	switch key {
	case
		"SAFE_AGENTIC_DEFAULT_CPUS",
		"SAFE_AGENTIC_DEFAULT_MEMORY",
		"SAFE_AGENTIC_DEFAULT_PIDS_LIMIT",
		"SAFE_AGENTIC_DEFAULT_SSH",
		"SAFE_AGENTIC_DEFAULT_DOCKER",
		"SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET",
		"SAFE_AGENTIC_DEFAULT_REUSE_AUTH",
		"SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH",
		"SAFE_AGENTIC_DEFAULT_NETWORK",
		"SAFE_AGENTIC_DEFAULT_IDENTITY",
		"GIT_AUTHOR_NAME",
		"GIT_AUTHOR_EMAIL",
		"GIT_COMMITTER_NAME",
		"GIT_COMMITTER_EMAIL":
		return true
	}
	return false
}

// setField maps an allowed key to the corresponding Config field.
func setField(cfg *Config, key, value string) {
	switch key {
	case "SAFE_AGENTIC_DEFAULT_CPUS":
		cfg.CPUs = value
	case "SAFE_AGENTIC_DEFAULT_MEMORY":
		cfg.Memory = value
	case "SAFE_AGENTIC_DEFAULT_PIDS_LIMIT":
		cfg.PIDsLimit = value
	case "SAFE_AGENTIC_DEFAULT_SSH":
		cfg.SSH = value
	case "SAFE_AGENTIC_DEFAULT_DOCKER":
		cfg.Docker = value
	case "SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET":
		cfg.DockerSocket = value
	case "SAFE_AGENTIC_DEFAULT_REUSE_AUTH":
		cfg.ReuseAuth = value
	case "SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH":
		cfg.ReuseGHAuth = value
	case "SAFE_AGENTIC_DEFAULT_NETWORK":
		cfg.Network = value
	case "SAFE_AGENTIC_DEFAULT_IDENTITY":
		cfg.Identity = value
	case "GIT_AUTHOR_NAME":
		cfg.GitAuthorName = value
	case "GIT_AUTHOR_EMAIL":
		cfg.GitAuthorEmail = value
	case "GIT_COMMITTER_NAME":
		cfg.GitCommitterName = value
	case "GIT_COMMITTER_EMAIL":
		cfg.GitCommitterEmail = value
	}
}
