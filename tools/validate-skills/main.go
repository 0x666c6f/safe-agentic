package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	pairs, err := skillPairs(".claude/skills", ".codex/skills")
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		if err := compareDirs(pair.claude, pair.codex); err != nil {
			return err
		}
		for _, dir := range []string{pair.claude, pair.codex} {
			if err := validateSkillDir(dir); err != nil {
				return err
			}
		}
	}
	fmt.Printf("validated %d skill pair(s)\n", len(pairs))
	return nil
}

type skillPair struct {
	name   string
	claude string
	codex  string
}

func skillPairs(claudeRoot, codexRoot string) ([]skillPair, error) {
	names, err := skillNames(claudeRoot)
	if err != nil {
		return nil, err
	}
	codexNames, err := skillNames(codexRoot)
	if err != nil {
		return nil, err
	}
	if strings.Join(names, "\n") != strings.Join(codexNames, "\n") {
		return nil, fmt.Errorf("skill roots differ: %v != %v", names, codexNames)
	}
	pairs := make([]skillPair, 0, len(names))
	for _, name := range names {
		pairs = append(pairs, skillPair{
			name:   name,
			claude: filepath.Join(claudeRoot, name),
			codex:  filepath.Join(codexRoot, name),
		})
	}
	return pairs, nil
}

func skillNames(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func compareDirs(left, right string) error {
	leftFiles, err := regularFiles(left)
	if err != nil {
		return err
	}
	rightFiles, err := regularFiles(right)
	if err != nil {
		return err
	}
	if strings.Join(leftFiles, "\n") != strings.Join(rightFiles, "\n") {
		return fmt.Errorf("skill file lists differ for %s and %s", left, right)
	}
	for _, rel := range leftFiles {
		leftData, err := os.ReadFile(filepath.Join(left, rel))
		if err != nil {
			return err
		}
		rightData, err := os.ReadFile(filepath.Join(right, rel))
		if err != nil {
			return err
		}
		if !bytes.Equal(leftData, rightData) {
			return fmt.Errorf("skill mirror drift: %s differs between %s and %s", rel, left, right)
		}
	}
	return nil
}

func regularFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular skill file: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func validateSkillDir(dir string) error {
	skillPath := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", skillPath, err)
	}
	if err := validateFrontmatter(skillPath, data); err != nil {
		return err
	}
	openAIPath := filepath.Join(dir, "agents", "openai.yaml")
	if _, err := os.Stat(openAIPath); err == nil {
		if err := validateYAML(openAIPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func validateFrontmatter(path string, data []byte) error {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		return fmt.Errorf("%s missing YAML frontmatter", path)
	}
	parts := strings.SplitN(text, "---\n", 3)
	if len(parts) < 3 {
		return fmt.Errorf("%s has unterminated YAML frontmatter", path)
	}
	var meta map[string]any
	if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
		return fmt.Errorf("parse %s frontmatter: %w", path, err)
	}
	for _, key := range []string{"name", "description"} {
		value, ok := meta[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s missing frontmatter key %q", path, key)
		}
	}
	return nil
}

func validateYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var decoded any
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}
