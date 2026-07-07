package labels

const (
	AgentType     = "berth.agent-type"
	RepoDisplay   = "berth.repo-display"
	SSH           = "berth.ssh"
	AuthType      = "berth.auth"
	GHAuth        = "berth.gh-auth"
	SeedAuth      = "berth.seed-auth"
	NetworkMode   = "berth.network-mode"
	DockerMode    = "berth.docker"
	Resources     = "berth.resources"
	Prompt        = "berth.prompt"
	Instructions  = "berth.instructions"
	MaxCost       = "berth.max-cost"
	OnExit        = "berth.on-exit"
	OnCompleteB64 = "berth.on-complete-b64"
	OnFailB64     = "berth.on-fail-b64"
	NotifyB64     = "berth.notify-b64"
	Fleet         = "berth.fleet"
	Hierarchy     = "berth.hierarchy"
	Terminal      = "berth.terminal"
	ForkedFrom    = "berth.forked-from"
	ForkLabel     = "berth.fork-label"
	Worktree      = "berth.worktree"
	AWS           = "berth.aws"
	App           = "app"
	Type          = "berth.type"
	Parent        = "berth.parent"
	AppValue      = "berth"
)

func ContainerFilter() string {
	return "name=^agent-"
}
