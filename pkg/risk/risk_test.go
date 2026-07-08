package risk

import (
	"strings"
	"testing"
)

func TestSpawnNoticesAPIOnly(t *testing.T) {
	notices := SpawnNotices(SpawnInput{NetworkMode: "api-only"})
	found := false
	for _, n := range notices {
		if n.Flag == "--network api-only" {
			found = true
			if !strings.Contains(n.Summary, "allowlisted") {
				t.Fatalf("summary should mention allowlist: %q", n.Summary)
			}
		}
	}
	if !found {
		t.Fatal("expected an api-only notice")
	}
}
