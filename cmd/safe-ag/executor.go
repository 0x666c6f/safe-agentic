package main

import "safe-agentic/pkg/orb"

// newExecutor creates the executor used by all commands.
// Override in tests with a FakeExecutor.
var newExecutor = func() orb.Executor {
	return &orb.OrbExecutor{VMName: "safe-agentic"}
}
