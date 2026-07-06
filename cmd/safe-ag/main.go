package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Version = "dev"

// Process exit codes. Code 1 is the generic default; the rest let scripts
// distinguish common failure classes without parsing stderr.
const (
	exitGeneric   = 1 // any unclassified error
	exitInfra     = 2 // VM / infra unavailable (no egress, VM not running)
	exitAgentFail = 3 // the agent itself reported a non-zero exit
	exitNotFound  = 4 // a container or manifest could not be found
)

// codedError wraps an error with a process exit code. RunE handlers wrap the
// errors they return with withExitCode; main maps the wrapper to os.Exit.
type codedError struct {
	code int
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// withExitCode tags err with a process exit code. It returns nil for a nil err
// so it is safe to wrap unconditionally.
func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &codedError{code: code, err: err}
}

// exitCodeFor returns the exit code carried by err, or exitGeneric.
func exitCodeFor(err error) int {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	return exitGeneric
}

var rootCmd = &cobra.Command{
	Use:           "safe-ag",
	Short:         "Isolated environment for running AI coding agents",
	Long:          "Sandboxed AI agent environment with per-agent Docker containers in an Apple container machine.",
	SilenceUsage:  true,
	SilenceErrors: true,
	CompletionOptions: cobra.CompletionOptions{
		// Keep `safe-ag completion bash|zsh|fish|powershell` available for shell
		// setup, but hide it from `--help` so the command map stays uncluttered.
		HiddenDefaultCmd: true,
	},
}

// Command group IDs. Every root subcommand is tagged with one of these so
// `safe-ag --help` renders as a grouped map instead of one flat list. The
// group titles (and their display order) are registered in registerGroups.
const (
	groupSpawn    = "spawn"
	groupManage   = "manage"
	groupObserve  = "observe"
	groupWorkflow = "workflow"
	groupFleet    = "fleet"
	groupSetup    = "setup"
	groupConfig   = "config"
)

// registerGroups adds the command groups to the root command. Groups appear in
// `--help` in the order they are added here.
func registerGroups() {
	rootCmd.AddGroup(
		&cobra.Group{ID: groupSpawn, Title: "Spawn & Run:"},
		&cobra.Group{ID: groupManage, Title: "Manage:"},
		&cobra.Group{ID: groupObserve, Title: "Observe:"},
		&cobra.Group{ID: groupWorkflow, Title: "Workflow:"},
		&cobra.Group{ID: groupFleet, Title: "Fleet & Pipelines:"},
		&cobra.Group{ID: groupSetup, Title: "Setup & Maintenance:"},
		&cobra.Group{ID: groupConfig, Title: "Config:"},
	)
}

func init() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("safe-agentic v{{.Version}}\n")
	registerGroups()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitCodeFor(err))
	}
}
