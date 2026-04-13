package docker

import (
	"context"
	"fmt"
	"github.com/0x666c6f/safe-agentic/pkg/orb"
	"github.com/0x666c6f/safe-agentic/pkg/repourl"
	"strings"
	"time"
)

const sshSocketPath = "/run/ssh-agent.sock"
const sshRelayDir = "/tmp/safe-agentic-ssh-relay"
const sshRelaySocket = sshRelayDir + "/agent.sock"
const sshRelayLock = sshRelayDir + "/relay.lock"
const sshRelayPIDFile = sshRelayDir + "/relay.pid"

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

	// Set up a locked relay for userns-remap compatibility.
	// The socket stays in a private 0700 directory to avoid exposing a global
	// world-traversable relay path inside the VM.
	_, relayExists := exec.Run(ctx, "test", "-S", sshRelaySocket)
	if relayExists != nil {
		lockedScript := fmt.Sprintf(
			"if [ -S %s ]; then exit 0; fi; rm -f %s %s; "+
				"start-stop-daemon --start --background --make-pidfile --pidfile %s "+
				"--exec \"$(command -v socat)\" -- "+
				"UNIX-LISTEN:%s,fork,mode=666 UNIX-CONNECT:%s",
			shellQuote(sshRelaySocket),
			shellQuote(sshRelaySocket),
			shellQuote(sshRelayPIDFile),
			shellQuote(sshRelayPIDFile),
			shellQuote(sshRelaySocket),
			shellQuote(vmSocket),
		)
		setupCmd := fmt.Sprintf(
			"set -e; install -d -m 700 %s; flock %s bash -lc %s",
			shellQuote(sshRelayDir),
			shellQuote(sshRelayLock),
			shellQuote(lockedScript),
		)

		if _, err := exec.Run(ctx, "bash", "-lc", setupCmd); err != nil {
			return fmt.Errorf("start SSH relay: %w", err)
		}
	}

	// Wait for relay socket to appear.
	relayOK := false
	for i := 0; i < 10; i++ {
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

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
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
