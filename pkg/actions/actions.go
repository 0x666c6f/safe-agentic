package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/BurntSushi/toml"
)

type Action struct {
	Name        string `toml:"-"`
	Description string `toml:"description"`
	Command     string `toml:"command"`
	CWD         string `toml:"cwd"`
	Source      string `toml:"-"`
}

type File struct {
	Actions map[string]Action `toml:"actions"`
}

type Catalog struct {
	Actions []Action
	byName  map[string]Action
}

var actionNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func UserPath() string {
	return filepath.Join(config.UserDir(), "actions.toml")
}

func ProjectPath(cwd string) string {
	return filepath.Join(cwd, ".safe-ag", "actions.toml")
}

func DefaultPaths(cwd string) []string {
	return []string{UserPath(), ProjectPath(cwd)}
}

func LoadDefault(cwd string) (Catalog, error) {
	return LoadFiles(DefaultPaths(cwd))
}

func LoadFiles(paths []string) (Catalog, error) {
	merged := make(map[string]Action)
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		actions, err := loadFile(path)
		if err != nil {
			return Catalog{}, err
		}
		for _, action := range actions {
			merged[action.Name] = action
		}
	}

	names := make([]string, 0, len(merged))
	for name := range merged {
		names = append(names, name)
	}
	sort.Strings(names)

	catalog := Catalog{byName: merged}
	for _, name := range names {
		catalog.Actions = append(catalog.Actions, merged[name])
	}
	return catalog, nil
}

func loadFile(path string) ([]Action, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open actions file %q: %w", path, err)
	}

	var file File
	if _, err := toml.DecodeFile(path, &file); err != nil {
		return nil, fmt.Errorf("parse actions file %q: %w", path, err)
	}

	var result []Action
	for name, action := range file.Actions {
		action.Name = name
		action.Source = path
		if err := validateAction(action); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		result = append(result, action)
	}
	return result, nil
}

func validateAction(action Action) error {
	if !actionNameRE.MatchString(action.Name) {
		return fmt.Errorf("invalid action name %q", action.Name)
	}
	if strings.TrimSpace(action.Command) == "" {
		return fmt.Errorf("action %q has empty command", action.Name)
	}
	if strings.Contains(action.CWD, "\x00") {
		return fmt.Errorf("action %q has invalid cwd", action.Name)
	}
	return nil
}

func (c Catalog) Get(name string) (Action, bool) {
	action, ok := c.byName[name]
	return action, ok
}
