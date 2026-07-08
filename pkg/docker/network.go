package docker

import (
	"context"
	"crypto/sha1"
	"fmt"
	"github.com/0x666c6f/berth/pkg/labels"
	"github.com/0x666c6f/berth/pkg/validate"
	"github.com/0x666c6f/berth/pkg/vmexec"
)

func ManagedNetworkName(containerName string) string {
	return containerName + "-net"
}

func ManagedBridgeName(containerName string) string {
	sum := sha1.Sum([]byte(containerName))
	return fmt.Sprintf("bt%x", sum[:6])
}

func APIOnlyBridgeName(containerName string) string {
	sum := sha1.Sum([]byte(containerName))
	return fmt.Sprintf("bti%x", sum[:5])
}

func CreateAPIOnlyNetwork(ctx context.Context, exec vmexec.Executor, containerName string) (string, error) {
	netName := ManagedNetworkName(containerName)
	_, err := exec.Run(ctx, "docker", "network", "create",
		"--driver", "bridge",
		"--opt", "com.docker.network.bridge.name="+APIOnlyBridgeName(containerName),
		"--label", fmt.Sprintf("%s=%s", labels.App, labels.AppValue),
		netName)
	if err != nil {
		return "", fmt.Errorf("create api-only network %s: %w", netName, err)
	}
	return netName, nil
}

func CreateManagedNetwork(ctx context.Context, exec vmexec.Executor, containerName string) (string, error) {
	netName := ManagedNetworkName(containerName)
	_, err := exec.Run(ctx, "docker", "network", "create",
		"--driver", "bridge",
		"--opt", "com.docker.network.bridge.name="+ManagedBridgeName(containerName),
		"--label", fmt.Sprintf("%s=%s", labels.App, labels.AppValue),
		netName)
	if err != nil {
		return "", fmt.Errorf("create network %s: %w", netName, err)
	}
	return netName, nil
}

func RemoveManagedNetwork(ctx context.Context, exec vmexec.Executor, netName string) error {
	_, err := exec.Run(ctx, "docker", "network", "rm", netName)
	if err != nil {
		return fmt.Errorf("remove network %s: %w", netName, err)
	}
	return nil
}

func PrepareNetwork(ctx context.Context, exec vmexec.Executor, containerName, customNetwork string, dryRun bool) (string, string, error) {
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
	if customNetwork == "api-only" {
		if dryRun {
			return ManagedNetworkName(containerName), "api-only", nil
		}
		name, err := CreateAPIOnlyNetwork(ctx, exec, containerName)
		return name, "api-only", err
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
