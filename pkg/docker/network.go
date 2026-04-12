package docker

import (
	"context"
	"fmt"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/orb"
	"github.com/0x666c6f/safe-agentic/pkg/validate"
)

func ManagedNetworkName(containerName string) string {
	return containerName + "-net"
}

func CreateManagedNetwork(ctx context.Context, exec orb.Executor, containerName string) (string, error) {
	netName := ManagedNetworkName(containerName)
	_, err := exec.Run(ctx, "docker", "network", "create",
		"--driver", "bridge",
		"--label", fmt.Sprintf("%s=%s", labels.App, labels.AppValue),
		netName)
	if err != nil {
		return "", fmt.Errorf("create network %s: %w", netName, err)
	}
	return netName, nil
}

func RemoveManagedNetwork(ctx context.Context, exec orb.Executor, netName string) error {
	_, err := exec.Run(ctx, "docker", "network", "rm", netName)
	if err != nil {
		return fmt.Errorf("remove network %s: %w", netName, err)
	}
	return nil
}

func PrepareNetwork(ctx context.Context, exec orb.Executor, containerName, customNetwork string, dryRun bool) (string, string, error) {
	if customNetwork == "" {
		if dryRun {
			return ManagedNetworkName(containerName), "managed", nil
		}
		name, err := CreateManagedNetwork(ctx, exec, containerName)
		return name, "managed", err
	}
	if customNetwork == "none" {
		return "none", "none", nil
	}
	if err := validate.NetworkName(customNetwork); err != nil {
		return "", "", err
	}
	if !dryRun {
		_, err := exec.Run(ctx, "docker", "network", "inspect", customNetwork)
		if err != nil {
			return "", "", fmt.Errorf("custom network %q does not exist", customNetwork)
		}
	}
	return customNetwork, "custom", nil
}
