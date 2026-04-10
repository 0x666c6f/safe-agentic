package docker

import (
	"context"
	"fmt"
	"safe-agentic/pkg/orb"
	"strings"
	"time"
)

const (
	dockerInternalSocketDir  = "/var/run/docker-internal"
	dockerInternalSocketPath = "/var/run/docker-internal/docker.sock"
)

func DinDContainerName(containerName string) string {
	return "safe-agentic-docker-" + containerName
}

func DinDSocketVolume(containerName string) string {
	return containerName + "-docker-sock"
}

func DinDDataVolume(containerName string) string {
	return containerName + "-docker-data"
}

func AppendDinDAccess(cmd *DockerRunCmd, containerName string) {
	socketVolume := DinDSocketVolume(containerName)
	cmd.AddEnv("DOCKER_HOST", "unix://"+dockerInternalSocketPath)
	cmd.AddNamedVolume(socketVolume, dockerInternalSocketDir)
}

func AppendHostDockerSocket(ctx context.Context, exec orb.Executor, cmd *DockerRunCmd) error {
	const hostSocketPath = "/run/docker-host.sock"
	out, err := exec.Run(ctx, "bash", "-c", "stat -c %g /var/run/docker.sock")
	if err != nil {
		return fmt.Errorf("get docker socket GID: %w", err)
	}
	gid := strings.TrimSpace(string(out))
	cmd.AddFlag("--group-add", gid)
	cmd.AddFlag("-v", "/var/run/docker.sock:"+hostSocketPath)
	cmd.AddEnv("DOCKER_HOST", "unix://"+hostSocketPath)
	return nil
}

func StartDinDRuntime(ctx context.Context, exec orb.Executor, containerName, networkName, image string) error {
	dindName := DinDContainerName(containerName)
	socketVol := DinDSocketVolume(containerName)
	dataVol := DinDDataVolume(containerName)
	for _, vol := range []string{socketVol, dataVol} {
		if err := CreateLabeledVolume(ctx, exec, vol, "docker-runtime", containerName); err != nil {
			return err
		}
	}
	args := []string{"docker", "run", "-d",
		"--name", dindName,
		"--privileged",
		"--network", networkName,
		"--tmpfs", "/tmp:rw,nosuid,size=512m",
		"--mount", fmt.Sprintf("type=volume,src=%s,dst=%s", socketVol, dockerInternalSocketDir),
		"--mount", fmt.Sprintf("type=volume,src=%s,dst=/var/lib/docker", dataVol),
		"--label", "app=safe-agentic",
		"--label", "safe-agentic.type=docker-runtime",
		"--label", fmt.Sprintf("safe-agentic.parent=%s", containerName),
		"--entrypoint", "dockerd",
		image,
		"--host", "unix://" + dockerInternalSocketPath,
	}
	if _, err := exec.Run(ctx, args...); err != nil {
		return fmt.Errorf("start DinD sidecar: %w", err)
	}
	return waitForDinD(ctx, exec, dindName)
}

func waitForDinD(ctx context.Context, exec orb.Executor, dindName string) error {
	for i := 0; i < 40; i++ {
		_, err := exec.Run(ctx, "docker", "exec", dindName, "docker", "info")
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("DinD daemon did not start within 20s")
}

func RemoveDinDRuntime(ctx context.Context, exec orb.Executor, containerName string) error {
	dindName := DinDContainerName(containerName)
	exec.Run(ctx, "docker", "rm", "-f", dindName)
	exec.Run(ctx, "docker", "volume", "rm", DinDSocketVolume(containerName))
	exec.Run(ctx, "docker", "volume", "rm", DinDDataVolume(containerName))
	return nil
}

func CleanupAllDinD(ctx context.Context, exec orb.Executor) error {
	// List containers with docker-runtime label, then remove each individually.
	out, _ := exec.Run(ctx, "docker", "ps", "-aq", "--filter", "label=safe-agentic.type=docker-runtime")
	ids := strings.TrimSpace(string(out))
	if ids != "" {
		for _, id := range strings.Split(ids, "\n") {
			id = strings.TrimSpace(id)
			if id != "" {
				exec.Run(ctx, "docker", "rm", "-f", id)
			}
		}
	}
	// List volumes with docker-runtime label, then remove each individually.
	out, _ = exec.Run(ctx, "docker", "volume", "ls", "-q", "--filter", "label=safe-agentic.type=docker-runtime")
	vols := strings.TrimSpace(string(out))
	if vols != "" {
		for _, v := range strings.Split(vols, "\n") {
			v = strings.TrimSpace(v)
			if v != "" {
				exec.Run(ctx, "docker", "volume", "rm", v)
			}
		}
	}
	return nil
}
