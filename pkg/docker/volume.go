package docker

import (
	"context"
	"fmt"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/orb"
)

func VolumeExists(ctx context.Context, exec orb.Executor, name string) (bool, error) {
	_, err := exec.Run(ctx, "docker", "volume", "inspect", name)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func CreateLabeledVolume(ctx context.Context, exec orb.Executor, name, volumeType, parentName string) error {
	_, err := exec.Run(ctx, "docker", "volume", "create",
		"--label", fmt.Sprintf("%s=%s", labels.App, labels.AppValue),
		"--label", fmt.Sprintf("%s=%s", labels.Type, volumeType),
		"--label", fmt.Sprintf("%s=%s", labels.Parent, parentName),
		name)
	if err != nil {
		return fmt.Errorf("create volume %s: %w", name, err)
	}
	return nil
}

func AuthVolumeName(agentType string, ephemeral bool, containerName string) string {
	if ephemeral {
		return containerName + "-auth"
	}
	return "safe-agentic-" + agentType + "-auth"
}

func GHAuthVolumeName(agentType string) string {
	return "safe-agentic-" + agentType + "-gh-auth"
}
