package docker

import (
	"context"
	"fmt"
	"safe-agentic/pkg/orb"
	"strings"
)

const ContainerPrefix = "agent"

func ContainerExists(ctx context.Context, exec orb.Executor, name string) (bool, error) {
	_, err := exec.Run(ctx, "docker", "inspect", name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func ResolveLatest(ctx context.Context, exec orb.Executor) (string, error) {
	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^"+ContainerPrefix+"-",
		"--format", "{{.Names}}",
		"--latest")
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("no safe-agentic containers found")
	}
	return name, nil
}

func ResolveTarget(ctx context.Context, exec orb.Executor, nameOrPartial string) (string, error) {
	if nameOrPartial == "--latest" || nameOrPartial == "" {
		return ResolveLatest(ctx, exec)
	}
	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^"+ContainerPrefix+"-",
		"--format", "{{.Names}}")
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	names := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == nameOrPartial {
			return n, nil
		}
	}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if strings.Contains(n, nameOrPartial) {
			return n, nil
		}
	}
	return "", fmt.Errorf("container %q not found", nameOrPartial)
}

func InspectLabel(ctx context.Context, exec orb.Executor, name, label string) (string, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", fmt.Sprintf("{{index .Config.Labels %q}}", label), name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func IsRunning(ctx context.Context, exec orb.Executor, name string) (bool, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", "{{.State.Running}}", name)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}
