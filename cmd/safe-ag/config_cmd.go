package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/inject"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/validate"

	"github.com/spf13/cobra"
)

// ─── config ────────────────────────────────────────────────────────────────

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI defaults",
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configResetCmd)
	rootCmd.AddCommand(configCmd)
}

// config show

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print current defaults file",
	RunE:  runConfigShow,
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	path := config.DefaultsPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		fmt.Println("# No defaults file found at", path)
		fmt.Println("# Use: safe-ag config set <key> <value>")
		return nil
	}
	if err != nil {
		return fmt.Errorf("read defaults: %w", err)
	}
	fmt.Printf("# %s\n", path)
	fmt.Print(string(data))
	return nil
}

// config set

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a default value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	// Validate key using the same allowlist as the config package.
	if !config.KeyAllowed(key) {
		return fmt.Errorf("unsupported key %q\n\nValid keys:\n%s", key, configAllowedKeysList())
	}

	path := config.DefaultsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// Read existing lines.
	var lines []string
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read defaults: %w", err)
	}
	if err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
	}

	// Find and replace or append.
	newLine := key + "=" + quoteValue(value)
	found := false
	for i, line := range lines {
		stripped := strings.TrimSpace(line)
		stripped = strings.TrimPrefix(stripped, "export ")
		stripped = strings.TrimSpace(stripped)
		if strings.HasPrefix(stripped, key+"=") {
			lines[i] = newLine
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, newLine)
	}

	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write defaults: %w", err)
	}

	fmt.Printf("Set %s=%s in %s\n", key, value, path)
	return nil
}

// quoteValue wraps a value in double quotes if it contains whitespace.
func quoteValue(v string) string {
	if strings.ContainsAny(v, " \t") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}

// config get

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a single default value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]
	if !config.KeyAllowed(key) {
		return fmt.Errorf("unsupported key %q", key)
	}

	path := config.DefaultsPath()
	cfg, err := config.LoadDefaults(path)
	if err != nil {
		return fmt.Errorf("load defaults: %w", err)
	}

	value := configGetField(cfg, key)
	if value == "" {
		fmt.Printf("%s is not set\n", key)
	} else {
		fmt.Printf("%s=%s\n", key, value)
	}
	return nil
}

// config reset

var configResetCmd = &cobra.Command{
	Use:   "reset <key>",
	Short: "Remove a default value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigReset,
}

func runConfigReset(cmd *cobra.Command, args []string) error {
	key := args[0]
	if !config.KeyAllowed(key) {
		return fmt.Errorf("unsupported key %q", key)
	}

	path := config.DefaultsPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		fmt.Printf("No defaults file at %s — nothing to reset.\n", path)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read defaults: %w", err)
	}

	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		stripped := strings.TrimSpace(line)
		stripped = strings.TrimPrefix(stripped, "export ")
		stripped = strings.TrimSpace(stripped)
		if strings.HasPrefix(stripped, key+"=") {
			continue // drop this line
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	if len(lines) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write defaults: %w", err)
	}

	fmt.Printf("Removed %s from %s\n", key, path)
	return nil
}

func configAllowedKeysList() string {
	keys := []string{
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
		"GIT_COMMITTER_EMAIL",
	}
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString("  ")
		sb.WriteString(k)
		sb.WriteString("\n")
	}
	return sb.String()
}

// configGetField maps an allowed key back to the Config field value.
func configGetField(cfg config.Config, key string) string {
	switch key {
	case "SAFE_AGENTIC_DEFAULT_CPUS":
		return cfg.CPUs
	case "SAFE_AGENTIC_DEFAULT_MEMORY":
		return cfg.Memory
	case "SAFE_AGENTIC_DEFAULT_PIDS_LIMIT":
		return cfg.PIDsLimit
	case "SAFE_AGENTIC_DEFAULT_SSH":
		return cfg.SSH
	case "SAFE_AGENTIC_DEFAULT_DOCKER":
		return cfg.Docker
	case "SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET":
		return cfg.DockerSocket
	case "SAFE_AGENTIC_DEFAULT_REUSE_AUTH":
		return cfg.ReuseAuth
	case "SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH":
		return cfg.ReuseGHAuth
	case "SAFE_AGENTIC_DEFAULT_NETWORK":
		return cfg.Network
	case "SAFE_AGENTIC_DEFAULT_IDENTITY":
		return cfg.Identity
	case "GIT_AUTHOR_NAME":
		return cfg.GitAuthorName
	case "GIT_AUTHOR_EMAIL":
		return cfg.GitAuthorEmail
	case "GIT_COMMITTER_NAME":
		return cfg.GitCommitterName
	case "GIT_COMMITTER_EMAIL":
		return cfg.GitCommitterEmail
	}
	return ""
}

// ─── template ──────────────────────────────────────────────────────────────

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage prompt templates",
}

func init() {
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateCreateCmd)
	rootCmd.AddCommand(templateCmd)
}

func looksLikeBuiltInTemplates(dir string) bool {
	for _, name := range []string{"security-audit.md", "code-review.md"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

func addTemplateCandidates(candidates *[]string, seen map[string]bool, roots ...string) {
	for _, root := range roots {
		if root == "" {
			continue
		}
		candidate := filepath.Join(root, "templates")
		abs, err := filepath.Abs(candidate)
		if err == nil {
			candidate = abs
		}
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		*candidates = append(*candidates, candidate)
	}
}

// repoTemplatesDir returns the built-in templates directory for repo and packaged installs.
func repoTemplatesDir() string {
	var candidates []string
	seen := map[string]bool{}

	// Start from the current executable. This covers direct repo binaries and
	// packaged installs where templates sit next to the real binary.
	exe, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exe)
		addTemplateCandidates(&candidates, seen, exeDir, filepath.Dir(exeDir))
		if resolved, resolveErr := filepath.EvalSymlinks(exe); resolveErr == nil {
			resolvedDir := filepath.Dir(resolved)
			addTemplateCandidates(&candidates, seen, resolvedDir, filepath.Dir(resolvedDir))
		}
	}

	// Fall back to walking upward from cwd. This keeps `go run ./cmd/safe-ag`
	// usable from a source checkout.
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			addTemplateCandidates(&candidates, seen, dir)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}

	for _, candidate := range candidates {
		if looksLikeBuiltInTemplates(candidate) {
			return candidate
		}
	}
	return ""
}

// userTemplatesDir returns ~/.config/safe-agentic/templates.
func userTemplatesDir() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, _ := os.UserHomeDir()
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "safe-agentic", "templates")
}

// template list

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available templates",
	RunE:  runTemplateList,
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	type tplEntry struct {
		name   string
		source string
	}
	var templates []tplEntry

	// Collect built-in templates.
	if dir := repoTemplatesDir(); dir != "" {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					name := strings.TrimSuffix(e.Name(), ".md")
					templates = append(templates, tplEntry{name, "built-in"})
				}
			}
		}
	}

	// Collect user templates.
	if dir := userTemplatesDir(); dir != "" {
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
					name := strings.TrimSuffix(e.Name(), ".md")
					templates = append(templates, tplEntry{name, "user"})
				}
			}
		}
	}

	if len(templates) == 0 {
		fmt.Println("No templates found.")
		return nil
	}

	fmt.Printf("%-30s  %s\n", "NAME", "SOURCE")
	fmt.Println(strings.Repeat("─", 45))
	for _, t := range templates {
		fmt.Printf("%-30s  %s\n", t.name, t.source)
	}
	return nil
}

// template show

var templateShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Display template content",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateShow,
}

func runTemplateShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	path, err := findTemplate(name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

// findTemplate searches user then built-in dirs for a template by name.
func findTemplate(name string) (string, error) {
	if err := validate.NameComponent(name, "template name"); err != nil {
		return "", err
	}

	candidates := []string{}

	// User dir takes precedence.
	userDir := userTemplatesDir()
	candidates = append(candidates,
		filepath.Join(userDir, name+".md"),
		filepath.Join(userDir, name),
	)

	// Built-in dir.
	if repoDir := repoTemplatesDir(); repoDir != "" {
		candidates = append(candidates,
			filepath.Join(repoDir, name+".md"),
			filepath.Join(repoDir, name),
		)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("template %q not found (checked user and built-in dirs)", name)
}

// template create

var templateCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new user template",
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateCreate,
}

func runTemplateCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	dir := userTemplatesDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create templates dir: %w", err)
	}

	path := filepath.Join(dir, name+".md")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("template %q already exists at %s", name, path)
	}

	// Write a starter template.
	starter := fmt.Sprintf("# %s\n\nDescribe what this template should do.\n", name)
	if err := os.WriteFile(path, []byte(starter), 0644); err != nil {
		return fmt.Errorf("create template file: %w", err)
	}
	fmt.Printf("Created template: %s\n", path)

	// Open in $EDITOR if set.
	editor := os.Getenv("EDITOR")
	if editor != "" {
		editorCmd := exec.Command(editor, path)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr
		_ = editorCmd.Run()
	}
	return nil
}

// ─── mcp-login ─────────────────────────────────────────────────────────────

var mcpLoginCmd = &cobra.Command{
	Use:   "mcp-login <service> [container]",
	Short: "Authenticate an MCP service",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runMCPLogin,
}

func init() {
	rootCmd.AddCommand(mcpLoginCmd)
}

func runMCPLogin(cmd *cobra.Command, args []string) error {
	service := args[0]

	if len(args) == 2 {
		container := args[1]
		orbRunner := newExecutor()
		return orbRunner.RunInteractive("docker", "exec", "-it", container, "mcp-login", service)
	}

	// Resolve the most recent running container, if any.
	ctx := context.Background()
	orbRunner := newExecutor()
	name, err := docker.ResolveTarget(ctx, orbRunner, "--latest")
	if err == nil {
		fmt.Printf("Logging in to MCP service %q in container %s…\n", service, name)
		return orbRunner.RunInteractive("docker", "exec", "-it", name, "mcp-login", service)
	}

	// No running container — print instructions.
	fmt.Printf("To authenticate MCP service %q:\n", service)
	fmt.Println()
	fmt.Println("  1. Start a container: safe-ag spawn claude --repo <url>")
	fmt.Printf("  2. Then run:          safe-ag mcp-login %s <container-name>\n", service)
	return nil
}

// ─── aws-refresh ───────────────────────────────────────────────────────────

var awsRefreshLatest bool

var awsRefreshCmd = &cobra.Command{
	Use:   "aws-refresh [name|--latest] [profile]",
	Short: "Refresh AWS credentials in a running container",
	Args:  cobra.RangeArgs(0, 2),
	RunE:  runAWSRefresh,
}

func init() {
	awsRefreshCmd.Flags().BoolVar(&awsRefreshLatest, "latest", false, "Target the most recently started container")
	rootCmd.AddCommand(awsRefreshCmd)
}

func runAWSRefresh(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	orbRunner := newExecutor()

	target := ""
	profileArg := ""

	switch len(args) {
	case 0:
		target = "--latest"
	case 1:
		if awsRefreshLatest || args[0] == "--latest" {
			target = "--latest"
		} else {
			// Could be a container name or a profile name; treat as container name.
			target = args[0]
		}
	case 2:
		target = args[0]
		profileArg = args[1]
	}

	name, err := docker.ResolveTarget(ctx, orbRunner, target)
	if err != nil {
		return err
	}

	// Determine AWS profile: explicit arg > container label.
	profile := profileArg
	if profile == "" {
		profile, _ = docker.InspectLabel(ctx, orbRunner, name, labels.AWS)
	}
	if profile == "" {
		return fmt.Errorf("no AWS profile specified and container %s has no %s label", name, labels.AWS)
	}

	// Read credentials from host.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home dir: %w", err)
	}
	credPath := filepath.Join(home, ".aws", "credentials")
	envs, err := inject.ReadAWSCredentials(credPath, profile)
	if err != nil {
		return fmt.Errorf("read AWS credentials: %w", err)
	}
	credsContent, err := inject.DecodeB64(envs["SAFE_AGENTIC_AWS_CREDS_B64"])
	if err != nil {
		return fmt.Errorf("decode AWS credentials: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "safe-agentic-aws-creds-*")
	if err != nil {
		return fmt.Errorf("create temp credentials file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	if _, err := tmpFile.WriteString(credsContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp credentials file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp credentials file: %w", err)
	}

	if _, err := orbRunner.Run(ctx, "docker", "exec", name, "mkdir", "-p", "/home/agent/.aws"); err != nil {
		return fmt.Errorf("prepare AWS directory: %w", err)
	}
	if _, err := orbRunner.Run(ctx, "docker", "cp", tmpPath, name+":/home/agent/.aws/credentials"); err != nil {
		return fmt.Errorf("copy credentials into container: %w", err)
	}
	if _, err := orbRunner.Run(ctx, "docker", "exec", name, "chmod", "600", "/home/agent/.aws/credentials"); err != nil {
		return fmt.Errorf("chmod container credentials: %w", err)
	}

	// Set the profile env var via docker exec.
	if p, ok := envs["AWS_PROFILE"]; ok {
		exportCmd := fmt.Sprintf("printf '\\nexport AWS_PROFILE=%q\\n' %q >> ~/.bashrc", p, p)
		if _, err := orbRunner.Run(ctx, "docker", "exec", name, "bash", "-lc", exportCmd); err != nil {
			return fmt.Errorf("persist AWS profile: %w", err)
		}
	}

	fmt.Printf("AWS credentials for profile %q refreshed in %s\n", profile, name)
	return nil
}
