package docker

import (
	"context"
	"fmt"
	"safe-agentic/pkg/orb"
	"safe-agentic/pkg/repourl"
	"strings"
	"time"
)

const sshSocketPath = "/run/ssh-agent.sock"
const sshRelaySocket = "/tmp/safe-agentic-ssh-agent.sock"

// AppendSSHMount sets up SSH agent forwarding from the VM into the container.
// With userns-remap, the container's uid maps to an unprivileged VM uid that
// can't read the OrbStack SSH socket (owned user:orbstack 660). We relay via
// socat to a world-accessible socket (mode 666) so the remapped uid works.
func AppendSSHMount(ctx context.Context, exec orb.Executor, cmd *DockerRunCmd) error {
	// Find the SSH agent socket in the VM
	out, err := exec.Run(ctx, "bash", "-c", "echo $SSH_AUTH_SOCK")
	if err != nil {
		return fmt.Errorf("find SSH socket: %w", err)
	}
	vmSocket := strings.TrimSpace(string(out))
	if vmSocket == "" {
		return fmt.Errorf("SSH_AUTH_SOCK not set in VM. Run 'ssh-add' on the host first")
	}

	// Set up socat relay for userns-remap compatibility
	relayScript := fmt.Sprintf(
		"#!/bin/bash\nexec socat UNIX-LISTEN:%s,fork,mode=666 UNIX-CONNECT:%s\n",
		sshRelaySocket, vmSocket)

	// Kill existing relay, create relay script, start as daemon
	setupCmd := fmt.Sprintf(
		"pkill -f 'socat.*safe-agentic-ssh-agent' 2>/dev/null || true; "+
			"rm -f %s; "+
			"printf '%%s' '%s' > /tmp/safe-agentic-ssh-relay.sh; "+
			"chmod +x /tmp/safe-agentic-ssh-relay.sh",
		sshRelaySocket, relayScript)

	exec.Run(ctx, "bash", "-c", setupCmd)
	exec.Run(ctx, "start-stop-daemon", "--start", "--background",
		"--exec", "/tmp/safe-agentic-ssh-relay.sh")

	// Wait for relay socket to appear (up to 1s)
	relayOK := false
	for i := 0; i < 5; i++ {
		_, err := exec.Run(ctx, "test", "-S", sshRelaySocket)
		if err == nil {
			relayOK = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if relayOK {
		cmd.AddFlag("-v", sshRelaySocket+":"+sshSocketPath)
	} else {
		// Fallback to direct mount (may not work with userns-remap)
		cmd.AddFlag("-v", vmSocket+":"+sshSocketPath+":ro")
	}

	cmd.AddEnv("SSH_AUTH_SOCK", sshSocketPath)
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
