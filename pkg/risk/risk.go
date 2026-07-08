package risk

type Notice struct {
	Flag    string
	Summary string
}

type SpawnInput struct {
	SSH               bool
	ReuseAuth         bool
	ReuseGHAuth       bool
	SeedAuth          bool
	AWSProfile        string
	Docker            bool
	DockerSocket      bool
	AllowSetupScripts bool
	AutoTrust         bool
	NetworkMode       string
	NetworkName       string
}

func SpawnNotices(input SpawnInput) []Notice {
	var notices []Notice
	if input.SSH {
		notices = append(notices, Notice{"--ssh", "forwards signing/auth ability from your SSH agent"})
	}
	if input.ReuseAuth {
		notices = append(notices, Notice{"--reuse-auth", "shares persistent Claude/Codex auth with other sessions"})
	}
	if input.ReuseGHAuth {
		notices = append(notices, Notice{"--reuse-gh-auth", "shares persistent GitHub CLI auth with other sessions"})
	}
	if input.SeedAuth {
		notices = append(notices, Notice{"--seed-auth", "copies host Claude/Codex auth into this session"})
	}
	if input.AWSProfile != "" {
		notices = append(notices, Notice{"--aws " + input.AWSProfile, "injects AWS credentials for this profile"})
	}
	if input.Docker {
		notices = append(notices, Notice{"--docker", "starts a privileged Docker-in-Docker sidecar"})
	}
	if input.DockerSocket {
		notices = append(notices, Notice{"--docker-socket", "mounts the VM Docker daemon socket"})
	}
	if input.AllowSetupScripts {
		notices = append(notices, Notice{"--allow-setup-scripts", "runs repo-controlled berth.json hooks before the agent starts"})
	}
	if input.AutoTrust {
		notices = append(notices, Notice{"--auto-trust", "skips Claude/Codex project trust prompts"})
	}
	if input.NetworkMode == "custom" {
		network := input.NetworkName
		notices = append(notices, Notice{"--network " + network, "uses a custom network outside managed egress guardrails"})
	}
	if input.NetworkMode == "api-only" {
		notices = append(notices, Notice{"--network api-only", "egress restricted to an allowlisted VM proxy; direct internet and DNS are dropped"})
	}
	return notices
}
