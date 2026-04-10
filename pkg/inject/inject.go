package inject

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func EncodeB64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func DecodeB64(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func EncodeFileB64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func ReadClaudeConfig(configDir string) (map[string]string, error) {
	envs := make(map[string]string)
	settingsPath := filepath.Join(configDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return envs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read claude settings: %w", err)
	}
	envs["SAFE_AGENTIC_CLAUDE_CONFIG_B64"] = base64.StdEncoding.EncodeToString(data)
	return envs, nil
}

func ReadCodexConfig(codexHome string) (map[string]string, error) {
	envs := make(map[string]string)
	configPath := filepath.Join(codexHome, "config.toml")
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return envs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read codex config: %w", err)
	}
	envs["SAFE_AGENTIC_CODEX_CONFIG_B64"] = base64.StdEncoding.EncodeToString([]byte(string(data)))
	return envs, nil
}

func ReadAWSCredentials(credPath, profile string) (map[string]string, error) {
	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read AWS credentials: %w", err)
	}
	content := string(data)
	if !strings.Contains(content, "["+profile+"]") {
		return nil, fmt.Errorf("AWS profile %q not found in %s", profile, credPath)
	}
	envs := map[string]string{
		"SAFE_AGENTIC_AWS_CREDS_B64": base64.StdEncoding.EncodeToString(data),
		"AWS_PROFILE":                profile,
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		envs["AWS_DEFAULT_REGION"] = r
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		envs["AWS_REGION"] = r
	}
	return envs, nil
}
