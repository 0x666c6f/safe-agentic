package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/fleet"
	"github.com/0x666c6f/safe-agentic/pkg/validate"
)

var inferRepoFromCurrent = inferRepoFromCurrentCheckout
var inferPRFromCurrent = inferPRFromCurrentCheckout

func parseTemplateVars(assignments []string, repos []string, inferRepo bool) (map[string]string, []string, error) {
	vars := make(map[string]string, len(assignments))
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
		if inferred, err := inferRepoFromCurrent(); err == nil && inferred != "" {
			defaultRepos = []string{inferred}
		}
	}
	if _, exists := vars["pr"]; !exists {
		if inferred, err := inferPRFromCurrent(); err == nil && inferred != "" {
			vars["pr"] = inferred
		}
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

func inferPRFromCurrentCheckout() (string, error) {
	out, err := exec.Command("gh", "pr", "view", "--json", "number").Output()
	if err != nil {
		return "", err
	}
	var payload struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return "", err
	}
	if payload.Number == 0 {
		return "", fmt.Errorf("no current PR number")
	}
	return fmt.Sprintf("%d", payload.Number), nil
}

func applyInputValues(inputs []fleet.InputSpec, vars map[string]string, repos []string) error {
	if vars["repo"] == "" && len(repos) > 0 {
		vars["repo"] = repos[0]
	}
	for _, input := range inputs {
		if vars[input.Name] == "" && input.Default != "" {
			vars[input.Name] = input.Default
		}
		if input.Required && vars[input.Name] == "" {
			return fmt.Errorf("missing required input %q", input.Name)
		}
	}
	return nil
}
