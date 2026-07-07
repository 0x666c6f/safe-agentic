package inject

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
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

func ReadClaudeAuth(homeDir string) (map[string]string, error) {
	envs := make(map[string]string)
	authPath := filepath.Join(homeDir, ".claude.json")
	data, err := os.ReadFile(authPath)
	if os.IsNotExist(err) {
		return envs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read claude auth: %w", err)
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		return nil, fmt.Errorf("gzip claude auth: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("gzip claude auth close: %w", err)
	}
	envs["SAFE_AGENTIC_CLAUDE_AUTH_B64"] = base64.StdEncoding.EncodeToString(buf.Bytes())
	return envs, nil
}

// ReadGHToken reads the host's GitHub CLI token and returns it as GH_TOKEN
// (and GITHUB_TOKEN) so `gh auth setup-git` inside the container can
// authenticate HTTPS git operations against private repos.
func ReadGHToken() (map[string]string, error) {
	envs := make(map[string]string)
	if t := os.Getenv("GH_TOKEN"); t != "" {
		envs["GH_TOKEN"] = t
		envs["GITHUB_TOKEN"] = t
		return envs, nil
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return envs, nil
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return envs, nil
	}
	envs["GH_TOKEN"] = token
	envs["GITHUB_TOKEN"] = token
	return envs, nil
}

// ReadClaudeCredentialsFile reads ~/.claude/.credentials.json (the live OAuth
// credential store) and returns it as SAFE_AGENTIC_CLAUDE_CREDS_B64. This is
// the authoritative login for modern Claude Code; the keychain token below is
// a fallback for hosts that store credentials only in the macOS keychain.
func ReadClaudeCredentialsFile(configDir string) (map[string]string, error) {
	envs := make(map[string]string)
	data, err := os.ReadFile(filepath.Join(configDir, ".credentials.json"))
	if err != nil {
		return envs, nil
	}
	envs["SAFE_AGENTIC_CLAUDE_CREDS_B64"] = base64.StdEncoding.EncodeToString(data)
	return envs, nil
}

func ReadClaudeOAuthToken() (map[string]string, error) {
	envs := make(map[string]string)
	if token := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); token != "" {
		envs["CLAUDE_CODE_OAUTH_TOKEN"] = token
		return envs, nil
	}

	// -w prints the raw password to stdout with no escaping (the -g form
	// hex-encodes JSON secrets, which broke extraction).
	out, err := exec.Command("security", "find-generic-password",
		"-s", "Claude Code-credentials", "-w").Output()
	if err != nil {
		return envs, nil
	}
	if accessToken := extractClaudeAccessToken(string(out)); accessToken != "" {
		envs["CLAUDE_CODE_OAUTH_TOKEN"] = accessToken
	}
	return envs, nil
}

func extractClaudeAccessToken(secret string) string {
	// The keychain item holds more than the account login these days: Claude
	// Code stores MCP server OAuth grants under "mcpOAuth" in the same JSON,
	// serialized before "claudeAiOauth". A first-match substring scan picks an
	// MCP token (often empty) instead of the account token, so parse properly.
	var payload struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal([]byte(secret), &payload); err == nil && payload.ClaudeAiOauth.AccessToken != "" {
		return payload.ClaudeAiOauth.AccessToken
	}
	// Fallback for unparseable payloads: scope to claudeAiOauth when present,
	// then take the first non-empty accessToken.
	rest := secret
	if i := strings.Index(rest, `"claudeAiOauth"`); i != -1 {
		rest = rest[i:]
	}
	const marker = `"accessToken":"`
	for {
		start := strings.Index(rest, marker)
		if start == -1 {
			return ""
		}
		rest = rest[start+len(marker):]
		end := strings.Index(rest, `"`)
		if end == -1 {
			return ""
		}
		if end > 0 {
			return rest[:end]
		}
	}
}

// ReadClaudeSupportFiles tars CLAUDE.md, hooks/, commands/, statusline-command.sh
// from configDir and returns them as SAFE_AGENTIC_CLAUDE_SUPPORT_B64.
// The entrypoint extracts this into ~/.claude/ inside the container.
func ReadClaudeSupportFiles(configDir string) (map[string]string, error) {
	envs := make(map[string]string)

	type entry struct {
		path  string
		isDir bool
	}
	var entries []entry

	for _, name := range []string{"CLAUDE.md", "statusline-command.sh"} {
		p := filepath.Join(configDir, name)
		if info, err := os.Lstat(p); err == nil && info.Mode().IsRegular() {
			entries = append(entries, entry{name, false})
		}
	}
	for _, name := range []string{"hooks", "commands"} {
		p := filepath.Join(configDir, name)
		if info, err := os.Lstat(p); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			entries = append(entries, entry{name, true})
		}
	}

	if len(entries) == 0 {
		return envs, nil
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		fullPath := filepath.Join(configDir, e.path)
		if e.isDir {
			if err := tarDir(tw, configDir, e.path); err != nil {
				tw.Close()
				gw.Close()
				return nil, fmt.Errorf("tar %s: %w", e.path, err)
			}
		} else {
			if err := tarFile(tw, fullPath, e.path); err != nil {
				tw.Close()
				gw.Close()
				return nil, fmt.Errorf("tar %s: %w", e.path, err)
			}
		}
	}

	tw.Close()
	gw.Close()
	envs["SAFE_AGENTIC_CLAUDE_SUPPORT_B64"] = base64.StdEncoding.EncodeToString(buf.Bytes())
	return envs, nil
}

func tarFile(tw *tar.Writer, fullPath, name string) error {
	info, err := os.Lstat(fullPath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to include symlink %s", name)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to include non-regular file %s", name)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}

func tarDir(tw *tar.Writer, baseDir, dirName string) error {
	root := filepath.Join(baseDir, dirName)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, _ := filepath.Rel(baseDir, path)
		if info.IsDir() {
			hdr := &tar.Header{
				Typeflag: tar.TypeDir,
				Name:     rel + "/",
				Mode:     0755,
			}
			return tw.WriteHeader(hdr)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hdr := &tar.Header{
			Name: rel,
			Mode: 0644,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = io.Copy(tw, bytes.NewReader(data))
		return err
	})
}

func ReadCodexConfig(codexHome string) (map[string]string, error) {
	envs := make(map[string]string)
	configPath := filepath.Join(codexHome, "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read codex config: %w", err)
	}
	_, agentsErr := os.Stat(filepath.Join(codexHome, "agents"))
	hasAgentsDir := agentsErr == nil
	sanitized := sanitizeCodexConfig(string(data), codexHome, hasAgentsDir)
	envs["SAFE_AGENTIC_CODEX_CONFIG_B64"] = base64.StdEncoding.EncodeToString([]byte(sanitized))
	return envs, nil
}

const containerCodexHome = "/home/agent/.codex"

func sanitizeCodexConfig(content, codexHome string, includeAgents bool) string {
	if codexHome != "" {
		content = strings.ReplaceAll(content, codexHome, containerCodexHome)
	}

	blockedTables := []string{"mcp_servers", "plugins", "marketplaces", "desktop", "projects"}
	if !includeAgents {
		blockedTables = append(blockedTables, "agents")
	}

	var out []string
	skip := false
	scope := "" // "" = top-level, else current table name
	hooksInScope := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if table, ok := tomlTableName(trimmed); ok {
			scope = table
			skip = tableIsBlocked(table, blockedTables)
		}
		if skip {
			continue
		}
		if strings.HasPrefix(trimmed, "notify =") {
			continue
		}
		if strings.HasPrefix(trimmed, "check_for_update_on_startup =") {
			continue
		}
		// codex renamed codex_hooks → hooks; a config carrying both (or the
		// rewrite colliding with an existing hooks) yields a duplicate-key
		// error that aborts codex. Rewrite codex_hooks → hooks, but emit at
		// most one hooks key per scope (top-level or table).
		if strings.HasPrefix(trimmed, "codex_hooks =") || strings.HasPrefix(trimmed, "hooks =") {
			if hooksInScope[scope] {
				continue
			}
			hooksInScope[scope] = true
			out = append(out, strings.Replace(line, "codex_hooks", "hooks", 1))
			continue
		}
		out = append(out, line)
	}
	result := strings.TrimRight(strings.Join(out, "\n"), "\n")
	if result != "" {
		result = "check_for_update_on_startup = false\n" + result
	} else {
		result = "check_for_update_on_startup = false\n"
	}
	if !strings.Contains(result, `[projects."/workspace"]`) {
		if result != "" {
			result += "\n\n"
		}
		result += "[projects.\"/workspace\"]\ntrust_level = \"trusted\"\n"
	} else {
		result += "\n"
	}
	return result
}

func tomlTableName(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	return strings.Trim(line, "[]"), true
}

func tableIsBlocked(table string, blocked []string) bool {
	for _, prefix := range blocked {
		if table == prefix || strings.HasPrefix(table, prefix+".") {
			return true
		}
	}
	return false
}

func ReadCodexSupportFiles(codexHome string) (map[string]string, error) {
	envs := make(map[string]string)
	agentsPath := filepath.Join(codexHome, "agents")
	if info, err := os.Lstat(agentsPath); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return envs, nil
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tarDir(tw, codexHome, "agents"); err != nil {
		tw.Close()
		gw.Close()
		return nil, fmt.Errorf("tar codex agents: %w", err)
	}
	if err := tw.Close(); err != nil {
		gw.Close()
		return nil, fmt.Errorf("tar codex agents close: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("gzip codex agents close: %w", err)
	}
	envs["SAFE_AGENTIC_CODEX_SUPPORT_B64"] = base64.StdEncoding.EncodeToString(buf.Bytes())
	return envs, nil
}

func ReadCodexAuth(codexHome string) (map[string]string, error) {
	envs := make(map[string]string)
	authPath := filepath.Join(codexHome, "auth.json")
	data, err := os.ReadFile(authPath)
	if os.IsNotExist(err) {
		return envs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read codex auth: %w", err)
	}
	envs["SAFE_AGENTIC_CODEX_AUTH_B64"] = base64.StdEncoding.EncodeToString(data)
	return envs, nil
}

func ReadAWSCredentials(credPath, profile string) (map[string]string, error) {
	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read AWS credentials: %w", err)
	}
	content, err := extractAWSProfile(string(data), profile)
	if err != nil {
		return nil, fmt.Errorf("AWS profile %q not found in %s", profile, credPath)
	}
	envs := map[string]string{
		"SAFE_AGENTIC_AWS_CREDS_B64": base64.StdEncoding.EncodeToString([]byte(content)),
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

func extractAWSProfile(content, profile string) (string, error) {
	lines := strings.Split(content, "\n")
	var current string
	var builder strings.Builder
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			current = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		}
		if current != profile {
			continue
		}
		builder.WriteString(line)
		builder.WriteString("\n")
		found = true
	}

	if !found {
		return "", fmt.Errorf("profile not found")
	}
	return builder.String(), nil
}
