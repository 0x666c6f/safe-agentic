package docker

import (
	"context"
	"fmt"
	"github.com/0x666c6f/berth/pkg/vmexec"
	"strconv"
	"strings"
)

const ContainerPrefix = "agent"

func ContainerExists(ctx context.Context, exec vmexec.Executor, name string) (bool, error) {
	_, err := exec.Run(ctx, "docker", "inspect", name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func ResolveLatest(ctx context.Context, exec vmexec.Executor) (string, error) {
	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^"+ContainerPrefix+"-",
		"--format", "{{.Names}}",
		"--latest")
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("no berth containers found")
	}
	return name, nil
}

func ResolveTarget(ctx context.Context, exec vmexec.Executor, nameOrPartial string) (string, error) {
	if nameOrPartial == "--latest" || nameOrPartial == "" {
		return ResolveLatest(ctx, exec)
	}
	out, err := exec.Run(ctx, "docker", "ps", "-a",
		"--filter", "name=^"+ContainerPrefix+"-",
		"--format", "{{.Names}}")
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		names = append(names, name)
		if name == nameOrPartial {
			return name, nil
		}
	}
	var matches []string
	for _, name := range names {
		if strings.Contains(name, nameOrPartial) {
			matches = append(matches, name)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("container %q is ambiguous; matches: %s", nameOrPartial, strings.Join(matches, ", "))
	}
	return "", fmt.Errorf("container %q not found", nameOrPartial)
}

func InspectLabel(ctx context.Context, exec vmexec.Executor, name, label string) (string, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", fmt.Sprintf("{{index .Config.Labels %q}}", label), name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func IsRunning(ctx context.Context, exec vmexec.Executor, name string) (bool, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", "{{.State.Running}}", name)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

// ExitCode returns the container's last exit code. A running container reports 0.
func ExitCode(ctx context.Context, exec vmexec.Executor, name string) (int, error) {
	out, err := exec.Run(ctx, "docker", "inspect",
		"--format", "{{.State.ExitCode}}", name)
	if err != nil {
		return 0, err
	}
	raw := strings.TrimSpace(string(out))
	code, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse exit code %q: %w", raw, err)
	}
	return code, nil
}

// TailLogs returns the last n lines of the container's combined stdout+stderr.
// The container name is passed as a positional argument ($1) rather than
// interpolated into the script so it cannot be used for shell injection.
func TailLogs(ctx context.Context, exec vmexec.Executor, name string, n int) (string, error) {
	out, err := exec.Run(ctx, "bash", "-lc",
		"docker logs --tail "+strconv.Itoa(n)+" \"$1\" 2>&1", "bash", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
