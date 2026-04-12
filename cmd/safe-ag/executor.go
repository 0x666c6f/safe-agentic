package main

import (
	"os"
	"github.com/0x666c6f/safe-agentic/pkg/orb"

	"github.com/spf13/cobra"
)

// newExecutor creates the executor used by all commands.
// Override in tests with a FakeExecutor.
var newExecutor = func() orb.Executor {
	vmName := os.Getenv("SAFE_AGENTIC_VM_NAME")
	if vmName == "" {
		vmName = "safe-agentic"
	}
	return &orb.OrbExecutor{VMName: vmName}
}

// addLatestFlag registers a --latest boolean flag on the given command.
func addLatestFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("latest", false, "Target the most recently started container")
}

// targetFromArgs resolves the container target from either the --latest flag or
// a positional argument. Returns the string to pass to docker.ResolveTarget.
func targetFromArgs(cmd *cobra.Command, args []string) string {
	if latest, _ := cmd.Flags().GetBool("latest"); latest {
		return "--latest"
	}
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
