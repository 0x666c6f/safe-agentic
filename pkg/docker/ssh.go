package docker

import (
	"context"
	"fmt"
	"safe-agentic/pkg/orb"
	"safe-agentic/pkg/repourl"
	"strings"
)

const sshSocketPath = "/run/ssh-agent.sock"

func AppendSSHMount(ctx context.Context, exec orb.Executor, cmd *DockerRunCmd) error {
	out, err := exec.Run(ctx, "bash", "-c", "echo $SSH_AUTH_SOCK")
	if err != nil {
		return fmt.Errorf("find SSH socket: %w", err)
	}
	vmSocket := strings.TrimSpace(string(out))
	if vmSocket == "" {
		return fmt.Errorf("SSH_AUTH_SOCK not set in VM. Run 'ssh-add' on the host first")
	}
	cmd.AddEnv("SSH_AUTH_SOCK", sshSocketPath)
	cmd.AddFlag("-v", vmSocket+":"+sshSocketPath)
	return nil
}

func AppendSSHMountDryRun(cmd *DockerRunCmd) {
	cmd.AddEnv("SSH_AUTH_SOCK", sshSocketPath)
	cmd.AddFlag("-v", "<SSH_SOCKET>:"+sshSocketPath)
}

func EnsureSSHForRepos(sshEnabled bool, repos []string) error {
	if sshEnabled {
		return nil
	}
	for _, r := range repos {
		if repourl.UsesSSH(r) {
			return fmt.Errorf("repo %q requires SSH but --ssh is not enabled", r)
		}
	}
	return nil
}
