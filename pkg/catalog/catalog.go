package catalog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/fleet"
	"github.com/0x666c6f/safe-agentic/pkg/validate"
	"gopkg.in/yaml.v3"
)

type AssetSource string

const (
	SourceUser    AssetSource = "user"
	SourceBuiltin AssetSource = "built-in"
	SourcePath    AssetSource = "path"
)

type TemplateAsset struct {
	Name        string
	Description string
	Inputs      []fleet.InputSpec
	Examples    []string
	Tags        []string
	Body        string
	Path        string
	Source      AssetSource
}

type PipelineAsset struct {
	Manifest fleet.PipelineManifest
	Path     string
	Source   AssetSource
}

type templateFrontMatter struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Inputs      []fleet.InputSpec `yaml:"inputs"`
	Examples    []string          `yaml:"examples"`
	Tags        []string          `yaml:"tags"`
}

func ListTemplates() ([]TemplateAsset, error) {
	assets, err := listTemplateAssets()
	if err != nil {
		return nil, err
	}
	return assets, nil
}

func ResolveTemplate(name string) (*TemplateAsset, error) {
	if err := ValidateAssetName(name); err != nil {
		return nil, err
	}
	assets, err := listTemplateAssets()
	if err != nil {
		return nil, err
	}
	for _, asset := range assets {
		if asset.Name == name {
			copy := asset
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("template %q not found", name)
}

func LoadTemplateFile(path string, source AssetSource) (*TemplateAsset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template %q: %w", path, err)
	}
	meta, body, err := parseFrontMatter(data)
	if err != nil {
		return nil, err
	}
	name := meta.Name
	if name == "" {
		name = trimKnownExt(filepath.Base(path))
	}
	return &TemplateAsset{
		Name:        name,
		Description: meta.Description,
		Inputs:      meta.Inputs,
		Examples:    meta.Examples,
		Tags:        meta.Tags,
		Body:        strings.TrimSpace(body),
		Path:        path,
		Source:      source,
	}, nil
}

func ListPipelines() ([]PipelineAsset, error) {
	user, err := collectPipelineAssets(config.PipelinesDir(), SourceUser)
	if err != nil {
		return nil, err
	}
	builtin, err := collectPipelineAssets(builtinPipelinesDir(), SourceBuiltin)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]PipelineAsset, len(builtin)+len(user))
	for _, asset := range builtin {
		merged[asset.Manifest.Name] = asset
	}
	for _, asset := range user {
		merged[asset.Manifest.Name] = asset
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]PipelineAsset, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out, nil
}

func ResolvePipeline(ref string) (*PipelineAsset, error) {
	if info, err := os.Stat(ref); err == nil && !info.IsDir() {
		return loadPipelineAsset(ref, SourcePath, "")
	}
	if err := ValidateAssetName(ref); err != nil {
		return nil, err
	}
	userPath, err := namedAssetPath(config.PipelinesDir(), ref, ".yaml", ".yml")
	if err != nil {
		return nil, err
	}
	if userPath != "" {
		return loadPipelineAsset(userPath, SourceUser, config.PipelinesDir())
	}
	builtinPath, err := namedAssetPath(builtinPipelinesDir(), ref, ".yaml", ".yml")
	if err != nil {
		return nil, err
	}
	if builtinPath != "" {
		return loadPipelineAsset(builtinPath, SourceBuiltin, builtinPipelinesDir())
	}
	return nil, fmt.Errorf("pipeline %q not found", ref)
}

func ResolveReviewPreset(name string) (*PipelineAsset, error) {
	if err := ValidateAssetName(name); err != nil {
		return nil, err
	}
	for _, base := range []struct {
		dir    string
		source AssetSource
	}{
		{filepath.Join(config.PipelinesDir(), "reviews"), SourceUser},
		{filepath.Join(builtinPipelinesDir(), "reviews"), SourceBuiltin},
	} {
		path, err := namedAssetPath(base.dir, name, ".yaml", ".yml")
		if err != nil {
			return nil, err
		}
		if path == "" {
			continue
		}
		return loadPipelineAsset(path, base.source, base.dir)
	}
	return nil, fmt.Errorf("review preset %q not found", name)
}

func ValidateAssetName(name string) error {
	if name == "" {
		return fmt.Errorf("asset name must not be empty")
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, ".") {
		return fmt.Errorf("asset name must be relative")
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("asset name contains invalid path segment %q", segment)
		}
		if err := validate.NameComponent(segment, "asset name"); err != nil {
			return err
		}
	}
	return nil
}

func builtinTemplatesDir() string {
	return findBuiltinDir("templates")
}

func builtinPipelinesDir() string {
	return findBuiltinDir(filepath.Join("templates", "pipelines"))
}

func findBuiltinDir(rel string) string {
	var candidates []string
	seen := map[string]bool{}
	add := func(root string) {
		if root == "" {
			return
		}
		path := filepath.Join(root, rel)
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		if seen[path] {
			return
		}
		seen[path] = true
		candidates = append(candidates, path)
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(exeDir)
		add(filepath.Dir(exeDir))
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			resolvedDir := filepath.Dir(resolved)
			add(resolvedDir)
			add(filepath.Dir(resolvedDir))
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			add(dir)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && info.IsDir() && looksLikeBuiltinDir(candidate, rel) {
			return candidate
		}
	}
	return ""
}

func listTemplateAssets() ([]TemplateAsset, error) {
	user, err := collectTemplateAssets(config.TemplatesDir(), SourceUser)
	if err != nil {
		return nil, err
	}
	builtin, err := collectTemplateAssets(builtinTemplatesDir(), SourceBuiltin)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]TemplateAsset, len(builtin)+len(user))
	for _, asset := range builtin {
		merged[asset.Name] = asset
	}
	for _, asset := range user {
		merged[asset.Name] = asset
	}
	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]TemplateAsset, 0, len(names))
	for _, name := range names {
		out = append(out, merged[name])
	}
	return out, nil
}

func collectTemplateAssets(base string, source AssetSource) ([]TemplateAsset, error) {
	if base == "" {
		return nil, nil
	}
	entries, err := collectFiles(base, ".md")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]TemplateAsset, 0, len(entries))
	for _, entry := range entries {
		asset, err := LoadTemplateFile(entry, source)
		if err != nil {
			return nil, err
		}
		if source != SourcePath {
			if name, err := relativeAssetName(base, entry); err == nil {
				asset.Name = name
			}
		}
		out = append(out, *asset)
	}
	return out, nil
}

func collectPipelineAssets(base string, source AssetSource) ([]PipelineAsset, error) {
	if base == "" {
		return nil, nil
	}
	entries, err := collectFiles(base, ".yaml", ".yml")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]PipelineAsset, 0, len(entries))
	for _, entry := range entries {
		asset, err := loadPipelineAsset(entry, source, base)
		if err != nil {
			return nil, err
		}
		out = append(out, *asset)
	}
	return out, nil
}

func loadPipelineAsset(path string, source AssetSource, base string) (*PipelineAsset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline %q: %w", path, err)
	}
	var manifest fleet.PipelineManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse pipeline %q: %w", path, err)
	}
	if source != SourcePath && base != "" {
		if name, err := relativeAssetName(base, path); err == nil {
			manifest.Name = name
		}
	}
	return &PipelineAsset{Manifest: manifest, Path: path, Source: source}, nil
}

func relativeAssetName(base, path string) (string, error) {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	return trimKnownExt(rel), nil
}

func trimKnownExt(name string) string {
	switch {
	case strings.HasSuffix(name, ".yaml"):
		return strings.TrimSuffix(name, ".yaml")
	case strings.HasSuffix(name, ".yml"):
		return strings.TrimSuffix(name, ".yml")
	case strings.HasSuffix(name, ".md"):
		return strings.TrimSuffix(name, ".md")
	default:
		return name
	}
}

func namedAssetPath(base, name string, exts ...string) (string, error) {
	if base == "" {
		return "", nil
	}
	if err := ValidateAssetName(name); err != nil {
		return "", err
	}
	for _, ext := range exts {
		path := filepath.Join(base, filepath.FromSlash(name)+ext)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", nil
}

func collectFiles(base string, exts ...string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		for _, ext := range exts {
			if strings.HasSuffix(d.Name(), ext) {
				out = append(out, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func looksLikeBuiltinDir(candidate, rel string) bool {
	switch filepath.ToSlash(rel) {
	case "templates":
		for _, name := range []string{"security-audit.md", "code-review.md"} {
			if !assetFileExists(filepath.Join(candidate, name)) {
				return false
			}
		}
		return true
	case "templates/pipelines":
		for _, name := range []string{"reviews/dual.yaml", "reviews/fix.yaml"} {
			if !assetFileExists(filepath.Join(candidate, filepath.FromSlash(name))) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func assetFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func parseFrontMatter(data []byte) (templateFrontMatter, string, error) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return templateFrontMatter{}, string(data), nil
	}
	rest := data[len("---\n"):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		return templateFrontMatter{}, "", fmt.Errorf("parse template front matter: missing closing ---")
	}
	var meta templateFrontMatter
	if err := yaml.Unmarshal(rest[:end], &meta); err != nil {
		return templateFrontMatter{}, "", fmt.Errorf("parse template front matter: %w", err)
	}
	body := string(rest[end+len("\n---\n"):])
	return meta, body, nil
}
