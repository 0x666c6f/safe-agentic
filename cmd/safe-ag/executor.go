package main

import (
	"context"
	"fmt"
	"os"

	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"

	"github.com/spf13/cobra"
)

// newExecutor creates the executor used by all commands.
// Override in tests with a FakeExecutor.
var newExecutor = func() vmexec.Executor {
	vmName := os.Getenv("SAFE_AGENTIC_VM_NAME")
	if vmName == "" {
		vmName = "safe-agentic"
	}
	return &vmexec.MachineExecutor{VMName: vmName}
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

// splitLatestTarget resolves the container target for commands that take the
// target as the first positional AND additional positionals after it. With
// --latest the first positional is NOT consumed as the target, so the caller's
// remaining positionals shift left. It errors when neither --latest nor a
// positional target is supplied.
func splitLatestTarget(cmd *cobra.Command, args []string) (target string, rest []string, err error) {
	if latest, _ := cmd.Flags().GetBool("latest"); latest {
		return "--latest", args, nil
	}
	if len(args) == 0 {
		return "", nil, fmt.Errorf("provide a container name or --latest")
	}
	return args[0], args[1:], nil
}

// resolveTargetCoded resolves a container target and tags a resolution failure
// (not found / ambiguous / no containers) with the not-found exit code so
// scripts can distinguish it from a generic error.
func resolveTargetCoded(ctx context.Context, exec vmexec.Executor, target string) (string, error) {
	name, err := docker.ResolveTarget(ctx, exec, target)
	if err != nil {
		return "", withExitCode(exitNotFound, err)
	}
	return name, nil
}
