package events

import (
	"testing"
)

func TestParseNotifyTargetsTerminalOnly(t *testing.T) {
	targets := ParseNotifyTargets("terminal")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Kind != "terminal" {
		t.Errorf("Kind: expected %q, got %q", "terminal", targets[0].Kind)
	}
	if targets[0].Value != "" {
		t.Errorf("Value: expected empty, got %q", targets[0].Value)
	}
}

func TestParseNotifyTargetsTerminalAndSlack(t *testing.T) {
	targets := ParseNotifyTargets("terminal,slack:https://hooks.slack.com/foo")
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	if targets[0].Kind != "terminal" {
		t.Errorf("targets[0].Kind: expected %q, got %q", "terminal", targets[0].Kind)
	}
	if targets[0].Value != "" {
		t.Errorf("targets[0].Value: expected empty, got %q", targets[0].Value)
	}

	if targets[1].Kind != "slack" {
		t.Errorf("targets[1].Kind: expected %q, got %q", "slack", targets[1].Kind)
	}
	if targets[1].Value != "https://hooks.slack.com/foo" {
		t.Errorf("targets[1].Value: expected %q, got %q", "https://hooks.slack.com/foo", targets[1].Value)
	}
}

func TestParseNotifyTargetsCommand(t *testing.T) {
	targets := ParseNotifyTargets("command:/usr/local/bin/notify.sh")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Kind != "command" {
		t.Errorf("Kind: expected %q, got %q", "command", targets[0].Kind)
	}
	if targets[0].Value != "/usr/local/bin/notify.sh" {
		t.Errorf("Value: expected %q, got %q", "/usr/local/bin/notify.sh", targets[0].Value)
	}
}

func TestParseNotifyTargetsEmpty(t *testing.T) {
	targets := ParseNotifyTargets("")
	if len(targets) != 0 {
		t.Errorf("expected 0 targets for empty string, got %d", len(targets))
	}
}

func TestParseNotifyTargetsWhitespace(t *testing.T) {
	targets := ParseNotifyTargets("terminal, slack:url")
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[1].Kind != "slack" {
		t.Errorf("targets[1].Kind: expected %q, got %q", "slack", targets[1].Kind)
	}
}

func TestParseNotifyTargetsMultiple(t *testing.T) {
	targets := ParseNotifyTargets("terminal,slack:url,command:script")
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	if targets[2].Kind != "command" {
		t.Errorf("targets[2].Kind: expected %q, got %q", "command", targets[2].Kind)
	}
	if targets[2].Value != "script" {
		t.Errorf("targets[2].Value: expected %q, got %q", "script", targets[2].Value)
	}
}
