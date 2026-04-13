package labels

import "testing"

func TestLabelKeysUniqueAndNonEmpty(t *testing.T) {
	keys := []string{
		AgentType,
		RepoDisplay,
		SSH,
		AuthType,
		GHAuth,
		NetworkMode,
		DockerMode,
		Resources,
		Prompt,
		Instructions,
		MaxCost,
		OnExit,
		OnCompleteB64,
		OnFailB64,
		NotifyB64,
		Fleet,
		Terminal,
		ForkedFrom,
		ForkLabel,
		AWS,
		App,
		Type,
		Parent,
	}

	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key == "" {
			t.Fatal("label key must not be empty")
		}
		if _, ok := seen[key]; ok {
			t.Fatalf("duplicate label key: %q", key)
		}
		seen[key] = struct{}{}
	}

	if AppValue != "safe-agentic" {
		t.Fatalf("AppValue = %q, want %q", AppValue, "safe-agentic")
	}
	if ContainerFilter() != "name=^agent-" {
		t.Fatalf("ContainerFilter() = %q", ContainerFilter())
	}
}
