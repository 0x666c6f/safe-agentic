package docker

import (
	"context"
	"fmt"
	"github.com/0x666c6f/berth/pkg/repourl"
	"github.com/0x666c6f/berth/pkg/vmexec"
	"regexp"
	"strings"
	"time"
)

const sshSocketPath = "/run/ssh-agent.sock"
const sshRelayDir = "/tmp/berth-ssh-relay"
const sshRelaySocket = sshRelayDir + "/agent.sock"
const sshRelayLock = sshRelayDir + "/relay.lock"
const sshRelayPIDFile = sshRelayDir + "/relay.pid"

var vmSocketPathPattern = regexp.MustCompile(`^/[\w./-]+$`)

// AppendSSHMount sets up SSH agent forwarding from the VM into the container.
// With userns-remap, the container's uid maps to an unprivileged VM uid that
// may not be able to read the Apple container machine host SSH socket. We relay
// via socat to a socket the remapped uid can access.
func AppendSSHMount(ctx context.Context, exec vmexec.Executor, cmd *DockerRunCmd) error {
	// Find the SSH agent socket in the VM
	out, err := exec.Run(ctx, "bash", "-c", "echo $SSH_AUTH_SOCK")
	if err != nil {
		return fmt.Errorf("find SSH socket: %w", err)
	}
	vmSocket := strings.TrimSpace(string(out))
	if vmSocket == "" {
		return fmt.Errorf("SSH_AUTH_SOCK not set in VM. Run 'ssh-add' on the host first")
	}
	if !vmSocketPathPattern.MatchString(vmSocket) {
		return fmt.Errorf("SSH_AUTH_SOCK has unsafe value %q", vmSocket)
	}
	if err := checkSSHAgent(ctx, exec, vmSocket); err != nil {
		return fmt.Errorf("VM SSH agent has no identities; ensure 1Password SSH agent is enabled, run `launchctl setenv SSH_AUTH_SOCK \"$SSH_AUTH_SOCK\"`, restart Apple container services, then run `berth vm start`: %w", err)
	}

	// Set up a locked relay for userns-remap compatibility.
	// The socket stays in a private 0700 directory to avoid exposing a global
	// world-traversable relay path inside the VM.
	_, relayExists := exec.Run(ctx, "test", "-S", sshRelaySocket)
	if relayExists == nil {
		if err := checkSSHAgent(ctx, exec, sshRelaySocket); err != nil {
			_, _ = exec.Run(ctx, "bash", "-lc", fmt.Sprintf("rm -f %s %s", shellQuote(sshRelaySocket), shellQuote(sshRelayPIDFile)))
			relayExists = fmt.Errorf("stale SSH relay: %w", err)
		}
	}
	if relayExists != nil {
		lockedScript := fmt.Sprintf(
			"if [ -S %s ]; then exit 0; fi; rm -f %s %s; "+
				"socat_path=\"$(command -v socat)\"; "+
				"nohup \"$socat_path\" UNIX-LISTEN:%s,fork,mode=666 UNIX-CONNECT:%s >/dev/null 2>&1 & "+
				"echo $! > %s",
			shellQuote(sshRelaySocket),
			shellQuote(sshRelaySocket),
			shellQuote(sshRelayPIDFile),
			shellQuote(sshRelaySocket),
			shellQuote(vmSocket),
			shellQuote(sshRelayPIDFile),
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

func checkSSHAgent(ctx context.Context, exec vmexec.Executor, socket string) error {
	checkCmd := fmt.Sprintf("SSH_AUTH_SOCK=%s ssh-add -l >/dev/null", shellQuote(socket))
	_, err := exec.Run(ctx, "bash", "-lc", checkCmd)
	return err
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
