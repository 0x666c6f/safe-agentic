package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/0x666c6f/safe-agentic/pkg/inject"

	"github.com/spf13/cobra"
)

// ─── item 3: --latest is registered on every advertised command ───────────────

func TestAdvertisedCommandsAcceptLatest(t *testing.T) {
	cmds := map[string]*cobra.Command{
		"diff":               diffCmd,
		"pr":                 prCmd,
		"review":             reviewCmd,
		"retry":              retryCmd,
		"checkpoint create":  checkpointCreateCmd,
		"checkpoint list":    checkpointListCmd,
		"checkpoint restore": checkpointRestoreCmd,
		"todo add":           todoAddCmd,
		"todo list":          todoListCmd,
		"todo check":         todoCheckCmd,
		"todo uncheck":       todoUncheckCmd,
		"aws-refresh":        awsRefreshCmd,
	}
	for name, c := range cmds {
		if c.Flags().Lookup("latest") == nil {
			t.Errorf("%s: --latest flag is not registered", name)
		}
	}
}

// splitLatestTarget must consume the first positional as the target only when
// --latest is absent, and otherwise leave every positional for the caller.
func TestSplitLatestTarget(t *testing.T) {
	c := &cobra.Command{}
	addLatestFlag(c)

	target, rest, err := splitLatestTarget(c, []string{"agent-x", "label"})
	if err != nil || target != "agent-x" || len(rest) != 1 || rest[0] != "label" {
		t.Fatalf("positional target: got (%q, %v, %v)", target, rest, err)
	}

	if _, _, err := splitLatestTarget(c, nil); err == nil {
		t.Fatal("expected an error when neither --latest nor a name is given")
	}

	if err := c.Flags().Set("latest", "true"); err != nil {
		t.Fatal(err)
	}
	target, rest, err = splitLatestTarget(c, []string{"label"})
	if err != nil || target != "--latest" || len(rest) != 1 || rest[0] != "label" {
		t.Fatalf("--latest target: got (%q, %v, %v)", target, rest, err)
	}
}

// ─── item 5: exit codes ───────────────────────────────────────────────────────

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{errors.New("plain"), exitGeneric},
		{withExitCode(exitInfra, errors.New("vm")), exitInfra},
		{withExitCode(exitAgentFail, errors.New("agent")), exitAgentFail},
		{withExitCode(exitNotFound, errors.New("missing")), exitNotFound},
		{fmt.Errorf("wrap: %w", withExitCode(exitNotFound, errors.New("missing"))), exitNotFound},
	}
	for _, c := range cases {
		if got := exitCodeFor(c.err); got != c.want {
			t.Errorf("exitCodeFor(%v) = %d, want %d", c.err, got, c.want)
		}
	}
	if withExitCode(0, nil) != nil {
		t.Error("withExitCode(_, nil) must be nil")
	}
}

func TestResolveTargetCodedTagsNotFound(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	fake.SetResponse("docker ps -a --filter name=^agent-", "")

	_, err := resolveTargetCoded(context.Background(), fake, "nope")
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if got := exitCodeFor(err); got != exitNotFound {
		t.Errorf("exit code = %d, want %d (%v)", got, exitNotFound, err)
	}
}

// ─── item 2: confirmation gates ───────────────────────────────────────────────

func TestConfirmDestructiveNonTTY(t *testing.T) {
	// Test stdin is never a terminal, so without --yes it must refuse.
	if _, err := confirmDestructive("Remove everything", false); err == nil {
		t.Fatal("expected a refusal on non-terminal stdin without --yes")
	} else if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error should mention --yes: %v", err)
	}
	ok, err := confirmDestructive("Remove everything", true)
	if err != nil || !ok {
		t.Fatalf("--yes must approve silently: ok=%v err=%v", ok, err)
	}
}

func TestStopAllRequiresConfirmation(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	stopAll = true
	defer func() { stopAll = false }()

	err := runStop(stopCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("stop --all without --yes should fail asking for --yes, got %v", err)
	}
	if got := len(fake.CommandsMatching("docker stop")); got != 0 {
		t.Errorf("stop must not run before confirmation, ran %d docker stop", got)
	}
}

func TestCleanupRequiresConfirmation(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()

	err := runCleanup(cleanupCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("cleanup without --yes should fail asking for --yes, got %v", err)
	}
	if got := len(fake.CommandsMatching("docker network prune")); got != 0 {
		t.Errorf("cleanup must not prune before confirmation, ran %d prune", got)
	}
}

// ─── item 6: config keys ──────────────────────────────────────────────────────

func TestConfigKeys(t *testing.T) {
	_, cleanup := testSetup(t)
	defer cleanup()

	out := captureOutput(func() {
		if err := runConfigKeys(configKeysCmd, nil); err != nil {
			t.Fatalf("runConfigKeys: %v", err)
		}
	})

	if !strings.Contains(out, "KEY") || !strings.Contains(out, "CURRENT") || !strings.Contains(out, "DEFAULT") {
		t.Errorf("missing header row: %q", out)
	}
	if !strings.Contains(out, "defaults.cpus") || !strings.Contains(out, "git.author_name") {
		t.Errorf("missing canonical keys: %q", out)
	}
	// The compiled-in default for cpus is 4, so it appears as a value.
	if !strings.Contains(out, "4") {
		t.Errorf("expected the cpus default (4): %q", out)
	}
	// Env-var aliases must not leak in as duplicate rows.
	if strings.Contains(out, "SAFE_AGENTIC_DEFAULT") || strings.Contains(out, "GIT_AUTHOR_NAME") {
		t.Errorf("env aliases leaked into keys output: %q", out)
	}
}

// ─── item 1: notify dispatch on container completion ──────────────────────────

func TestDispatchContainerNotify(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	name := "agent-claude-x"
	fake.SetResponse("docker inspect --format", inject.EncodeB64("terminal")+"\n")

	success := captureOutput(func() {
		dispatchContainerNotify(context.Background(), fake, name, 0)
	})
	if !strings.Contains(success, name) || !strings.Contains(success, "finished") {
		t.Errorf("success notify missing content: %q", success)
	}

	failure := captureOutput(func() {
		dispatchContainerNotify(context.Background(), fake, name, 1)
	})
	if !strings.Contains(failure, "code 1") {
		t.Errorf("failure notify should mention the exit code: %q", failure)
	}
}

func TestDispatchContainerNotifyNoLabelIsNoop(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	// No notify label configured -> InspectLabel returns "" -> nothing dispatched.
	out := captureOutput(func() {
		dispatchContainerNotify(context.Background(), fake, "agent-claude-y", 0)
	})
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected no output without a notify label, got %q", out)
	}
}
