package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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
	Use:     "config",
	Short:   "View and change persistent CLI defaults",
	Long:    "Manage defaults in ~/.safe-ag/config.toml so future spawns pick them up without repeating flags (e.g. default to --ssh or enable the worktree mount).",
	GroupID: groupConfig,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configKeysCmd)
	rootCmd.AddCommand(configCmd)
}

// config keys

var configKeysCmd = &cobra.Command{
	Use:   "keys",
	Short: "List config keys with their current and default values",
	Args:  cobra.NoArgs,
	RunE:  runConfigKeys,
}

func runConfigKeys(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadDefaults(config.ConfigPath())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	defaults := config.Defaults()

	// Reduce the alias table to its canonical dotted keys so env-var aliases
	// (SAFE_AGENTIC_*, GIT_*) don't appear as duplicate rows.
	seen := map[string]bool{}
	var keys []string
	for _, k := range config.AllowedKeys() {
		canonical, err := config.ResolveKey(k)
		if err != nil || seen[canonical] {
			continue
		}
		seen[canonical] = true
		keys = append(keys, canonical)
	}
	sort.Strings(keys)

	type row struct{ key, cur, def string }
	rows := make([]row, 0, len(keys))
	keyW, curW := len("KEY"), len("CURRENT")
	for _, k := range keys {
		cur, _ := config.GetValue(cfg, k)
		def, _ := config.GetValue(defaults, k)
		r := row{k, orDash(cur), orDash(def)}
		rows = append(rows, r)
		if len(r.key) > keyW {
			keyW = len(r.key)
		}
		if len(r.cur) > curW {
			curW = len(r.cur)
		}
	}

	fmt.Printf("%-*s  %-*s  %s\n", keyW, "KEY", curW, "CURRENT", "DEFAULT")
	for _, r := range rows {
		fmt.Printf("%-*s  %-*s  %s\n", keyW, r.key, curW, r.cur, r.def)
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
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
	Use:     "template",
	Short:   "List, show, and create reusable prompt templates",
	GroupID: groupConfig,
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
	Short: "Run an MCP service's OAuth login inside an agent",
	Long: `Run the interactive OAuth login for an MCP service (e.g. linear, notion) inside
a container. Without [container] it targets the most recent running agent, or
prints how to start one.`,
	Args:    cobra.RangeArgs(1, 2),
	GroupID: groupConfig,
	RunE:    runMCPLogin,
}

func init() {
	rootCmd.AddCommand(mcpLoginCmd)
}

func runMCPLogin(cmd *cobra.Command, args []string) error {
	service := args[0]

	if len(args) == 2 {
		container := args[1]
		vmRunner := newExecutor()
		return vmRunner.RunInteractive("docker", "exec", "-it", container, "mcp-login", service)
	}

	// Resolve the most recent running container, if any.
	ctx := context.Background()
	vmRunner := newExecutor()
	name, err := docker.ResolveTarget(ctx, vmRunner, "--latest")
	if err == nil {
		fmt.Printf("Logging in to MCP service %q in container %s…\n", service, name)
		return vmRunner.RunInteractive("docker", "exec", "-it", name, "mcp-login", service)
	}

	// No running container — print instructions.
	fmt.Printf("To authenticate MCP service %q:\n", service)
	fmt.Println()
	fmt.Println("  1. Start a container: safe-ag spawn claude --repo <url>")
	fmt.Printf("  2. Then run:          safe-ag mcp-login %s <container-name>\n", service)
	return nil
}

// ─── aws-refresh ───────────────────────────────────────────────────────────

var awsRefreshCmd = &cobra.Command{
	Use:   "aws-refresh [name|--latest] [profile]",
	Short: "Re-inject fresh AWS credentials into a running agent",
	Long: `Re-read ~/.aws credentials and push them into a running container whose creds
have expired. Pass [profile] to switch to a different AWS profile than the one
the agent was spawned with.`,
	Args:    cobra.RangeArgs(0, 2),
	GroupID: groupManage,
	RunE:    runAWSRefresh,
}

func init() {
	addLatestFlag(awsRefreshCmd)
	rootCmd.AddCommand(awsRefreshCmd)
}

func runAWSRefresh(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	vmRunner := newExecutor()

	latest, _ := cmd.Flags().GetBool("latest")
	target := ""
	profileArg := ""
	switch {
	case latest:
		// --latest targets the newest container; a lone positional is the profile.
		target = "--latest"
		if len(args) >= 1 {
			profileArg = args[0]
		}
	case len(args) == 0:
		target = "--latest"
	case len(args) == 1:
		target = args[0]
	default:
		target = args[0]
		profileArg = args[1]
	}

	name, err := resolveTargetCoded(ctx, vmRunner, target)
	if err != nil {
		return err
	}

	// Determine AWS profile: explicit arg > container label.
	profile := profileArg
	if profile == "" {
		profile, _ = docker.InspectLabel(ctx, vmRunner, name, labels.AWS)
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
	if _, err := vmRunner.Run(ctx, "docker", "exec", name, "bash", "-lc", writeCmd); err != nil {
		return fmt.Errorf("write container credentials: %w", err)
	}

	// Set the profile env var via docker exec.
	if p, ok := envs["AWS_PROFILE"]; ok {
		exportLine := "export AWS_PROFILE=" + shellQuote(p)
		exportCmd := fmt.Sprintf("printf '\\n%%s\\n' %s >> ~/.bashrc", shellQuote(exportLine))
		if _, err := vmRunner.Run(ctx, "docker", "exec", name, "bash", "-lc", exportCmd); err != nil {
			return fmt.Errorf("persist AWS profile: %w", err)
		}
	}

	fmt.Printf("AWS credentials for profile %q refreshed in %s\n", profile, name)
	return nil
}
