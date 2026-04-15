package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/validate"
)

func parseTemplateVars(assignments []string, repos []string, inferRepo bool) (map[string]string, []string, error) {
	vars := make(map[string]string, len(assignments)+1)
	for _, assignment := range assignments {
		key, value, err := splitVarAssignment(assignment)
		if err != nil {
			return nil, nil, err
		}
		if key == "repo" {
			return nil, nil, fmt.Errorf("repo is reserved; use --repo instead of --var repo=...")
		}
		vars[key] = value
	}

	defaultRepos := append([]string{}, repos...)
	if len(defaultRepos) == 0 && inferRepo {
		if inferred, err := inferRepoFromCurrentCheckout(); err == nil && inferred != "" {
			defaultRepos = []string{inferred}
		}
	}
	if len(defaultRepos) > 0 {
		vars["repo"] = defaultRepos[0]
	}
	return vars, defaultRepos, nil
}

func splitVarAssignment(assignment string) (string, string, error) {
	key, value, ok := strings.Cut(assignment, "=")
	if !ok {
		return "", "", fmt.Errorf("invalid --var %q: expected key=value", assignment)
	}
	if err := validate.NameComponent(key, "variable name"); err != nil {
		return "", "", err
	}
	return key, value, nil
}

func interpolateString(value string, vars map[string]string) string {
	for key, repl := range vars {
		value = strings.ReplaceAll(value, "${"+key+"}", repl)
	}
	return value
}

func ensureNoUnresolvedVars(label, value string) error {
	if strings.Contains(value, "${") {
		return fmt.Errorf("%s contains unresolved variables: %s", label, value)
	}
	return nil
}

func inferRepoFromCurrentCheckout() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
