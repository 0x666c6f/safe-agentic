package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/catalog"
	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/inject"
	"github.com/0x666c6f/safe-agentic/pkg/labels"

	"github.com/spf13/cobra"
)

// ─── config ────────────────────────────────────────────────────────────────

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage ~/.safe-ag/config.toml defaults",
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
	path := config.ConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		fmt.Println("# No config file found at", path)
		fmt.Println("# Use: safe-ag config set <key> <value>")
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
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

	canonical, err := config.ResolveKey(key)
	if err != nil {
		return fmt.Errorf("%w\n\nValid keys:\n%s", err, configAllowedKeysList())
	}
	path := config.ConfigPath()
	raw, err := config.LoadRawConfig(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := config.SetValue(&raw, canonical, value); err != nil {
		return err
	}
	if err := config.SaveRawConfig(path, raw); err != nil {
		return err
	}
	fmt.Printf("Set %s=%s in %s\n", canonical, value, path)
	return nil
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
	canonical, err := config.ResolveKey(key)
	if err != nil {
		return err
	}

	path := config.ConfigPath()
	cfg, err := config.LoadDefaults(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	value, err := config.GetValue(cfg, canonical)
	if err != nil {
		return err
	}
	if value == "" {
		fmt.Printf("%s is not set\n", canonical)
	} else {
		fmt.Printf("%s=%s\n", canonical, value)
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
	canonical, err := config.ResolveKey(key)
	if err != nil {
		return err
	}

	path := config.ConfigPath()
	raw, err := config.LoadRawConfig(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if raw.IsZero() {
		fmt.Printf("No config file at %s — nothing to reset.\n", path)
		return nil
	}
	if err := config.ResetValue(&raw, canonical); err != nil {
		return err
	}
	if err := config.SaveRawConfig(path, raw); err != nil {
		return err
	}

	fmt.Printf("Removed %s from %s\n", canonical, path)
	return nil
}

func configAllowedKeysList() string {
	var sb strings.Builder
	for _, k := range config.AllowedKeys() {
		sb.WriteString("  ")
		sb.WriteString(k)
		sb.WriteString("\n")
	}
	return sb.String()
}

// ─── template ──────────────────────────────────────────────────────────────

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage prompt templates",
	Long: `Manage prompt templates.

Templates are loaded from ~/.safe-ag/templates/ and can override built-in
templates shipped with safe-agentic. Templates may include YAML front matter
for description, inputs, examples, and tags.`,
}

func init() {
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateRenderCmd)
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

// userTemplatesDir returns ~/.safe-ag/templates.
func userTemplatesDir() string {
	return config.TemplatesDir()
}

// template list

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available templates",
	RunE:  runTemplateList,
}

func runTemplateList(cmd *cobra.Command, args []string) error {
	templates, err := catalog.ListTemplates()
	if err != nil {
		return err
	}
	if len(templates) == 0 {
		fmt.Println("No templates found.")
		return nil
	}

	fmt.Printf("%-30s  %-10s  %s\n", "NAME", "SOURCE", "DESCRIPTION")
	fmt.Println(strings.Repeat("─", 80))
	for _, t := range templates {
		fmt.Printf("%-30s  %-10s  %s\n", t.Name, t.Source, t.Description)
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
	asset, err := catalog.ResolveTemplate(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Name: %s\nSource: %s\nPath: %s\n", asset.Name, asset.Source, asset.Path)
	if asset.Description != "" {
		fmt.Printf("Description: %s\n", asset.Description)
	}
	if len(asset.Inputs) > 0 {
		fmt.Println("Inputs:")
		for _, input := range asset.Inputs {
			status := "optional"
			if input.Required {
				status = "required"
			}
			if input.Infer != "" {
				status += ", infer=" + input.Infer
			}
			fmt.Printf("  - %s (%s)\n", input.Name, status)
		}
	}
	fmt.Println()
	fmt.Print(asset.Body)
	return nil
}

// findTemplate searches user then built-in dirs for a template by name.
func findTemplate(name string) (string, error) {
	asset, err := catalog.ResolveTemplate(name)
	if err != nil {
		return "", err
	}
	return asset.Path, nil
}

var templateRenderVars []string
var templateRenderRepos []string

var templateRenderCmd = &cobra.Command{
	Use:   "render <name>",
	Short: "Render a template with inferred and explicit variables",
	Long: `Render a template with inferred and explicit variables.

${repo} is inferred from the current checkout when possible or can be
provided explicitly with --repo. Declared template inputs are validated
before rendering.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runTemplateRender,
}

func init() {
	templateRenderCmd.Flags().StringSliceVar(&templateRenderVars, "var", nil, "Template variable assignment (key=value)")
	templateRenderCmd.Flags().StringSliceVar(&templateRenderRepos, "repo", nil, "Repo URL for ${repo}; repeatable")
}

func runTemplateRender(cmd *cobra.Command, args []string) error {
	asset, err := catalog.ResolveTemplate(args[0])
	if err != nil {
		return err
	}
	vars, repos, err := parseTemplateVars(templateRenderVars, templateRenderRepos, true)
	if err != nil {
		return err
	}
	if err := applyInputValues(asset.Inputs, vars, repos); err != nil {
		return err
	}
	body := interpolateString(asset.Body, vars)
	if err := ensureNoUnresolvedVars("template "+asset.Name, body); err != nil {
		return err
	}
	fmt.Print(body)
	return nil
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
	if err := catalog.ValidateAssetName(name); err != nil {
		return err
	}
	dir := userTemplatesDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create templates dir: %w", err)
	}

	path := filepath.Join(dir, name+".md")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("template %q already exists at %s", name, path)
	}

	// Write a starter template with explicit metadata.
	starter := fmt.Sprintf(`---
name: %s
description: Describe what this template should do.
inputs:
  - name: repo
    description: Repository URL or current checkout repo.
    infer: repo
examples:
  - safe-ag spawn claude --template %s
---

Write the reusable prompt body here.
`, name, name)
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
	writeCmd := fmt.Sprintf(
		"umask 177; mkdir -p /home/agent/.aws && printf %%s %s | base64 -d > /home/agent/.aws/credentials",
		shellQuote(inject.EncodeB64(credsContent)),
	)
	if _, err := orbRunner.Run(ctx, "docker", "exec", name, "bash", "-lc", writeCmd); err != nil {
		return fmt.Errorf("write container credentials: %w", err)
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
