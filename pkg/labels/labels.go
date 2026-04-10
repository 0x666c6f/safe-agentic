package labels

const (
	AgentType     = "safe-agentic.agent-type"
	RepoDisplay   = "safe-agentic.repo-display"
	SSH           = "safe-agentic.ssh"
	AuthType      = "safe-agentic.auth"
	GHAuth        = "safe-agentic.gh-auth"
	NetworkMode   = "safe-agentic.network-mode"
	DockerMode    = "safe-agentic.docker"
	Resources     = "safe-agentic.resources"
	Prompt        = "safe-agentic.prompt"
	Instructions  = "safe-agentic.instructions"
	MaxCost       = "safe-agentic.max-cost"
	OnExit        = "safe-agentic.on-exit"
	OnCompleteB64 = "safe-agentic.on-complete-b64"
	OnFailB64     = "safe-agentic.on-fail-b64"
	NotifyB64     = "safe-agentic.notify-b64"
	Fleet         = "safe-agentic.fleet"
	Terminal      = "safe-agentic.terminal"
	ForkedFrom    = "safe-agentic.forked-from"
	ForkLabel     = "safe-agentic.fork-label"
	AWS           = "safe-agentic.aws"
	App           = "app"
	Type          = "safe-agentic.type"
	Parent        = "safe-agentic.parent"
	AppValue      = "safe-agentic"
)

func ContainerFilter() string {
	return "name=^agent-"
}
