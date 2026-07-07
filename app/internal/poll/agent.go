package poll

import "strings"

type Agent struct {
	Name, Type, Repo, SSH, Auth, GHAuth, Docker, NetworkMode string
	Fleet, Hierarchy, Terminal, Status                       string
	Running, Finished                                        bool
	Activity                                                 string // "Working" | "Idle" | "Stopped"
	State, StateReason                                       string // agentstate: blocked/working/done/idle/exited/unknown (set by poller probe)
	CPU, Memory, NetIO, PIDs                                 string
	Prompt, MaxCost                                          string // from docker labels (immutable per container, cached)
}

func PSFormat() string {
	return strings.Join([]string{
		"{{.Names}}",
		`{{.Label "berth.agent-type"}}`,
		`{{.Label "berth.repo-display"}}`,
		`{{.Label "berth.ssh"}}`,
		`{{.Label "berth.auth"}}`,
		`{{.Label "berth.gh-auth"}}`,
		`{{.Label "berth.docker"}}`,
		`{{.Label "berth.network-mode"}}`,
		`{{.Label "berth.fleet"}}`,
		`{{.Label "berth.hierarchy"}}`,
		`{{.Label "berth.terminal"}}`,
		"{{.Status}}",
	}, "\t")
}

func splitFormat(f string) []string { return strings.Split(f, "\t") }

func normalizeContainerStatus(raw string) (string, bool, bool) {
	switch {
	case strings.HasPrefix(raw, "Up"):
		return raw, true, false
	case strings.HasPrefix(raw, "Exited (0)"):
		return raw, false, true
	default:
		return raw, false, false
	}
}

func ParsePS(data []byte) []Agent {
	var agents []Agent
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 12 {
			continue
		}
		status, running, finished := normalizeContainerStatus(parts[11])
		a := Agent{
			Name: parts[0], Type: parts[1], Repo: parts[2], SSH: parts[3],
			Auth: parts[4], GHAuth: parts[5], Docker: parts[6], NetworkMode: parts[7],
			Fleet: parts[8], Hierarchy: parts[9], Terminal: parts[10],
			Status: status, Running: running, Finished: finished,
		}
		a.Activity = "Stopped"
		if running {
			a.Activity = "Idle"
		}
		agents = append(agents, a)
	}
	return agents
}
