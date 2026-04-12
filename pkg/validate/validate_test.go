package validate

import (
	"testing"
)

func TestNameComponent(t *testing.T) {
	valid := []string{
		"my-agent",
		"agent_1",
		"Agent.Name",
		"a",
		"A123",
		"123numeric",
	}
	for _, v := range valid {
		if err := NameComponent(v, "name"); err != nil {
			t.Errorf("NameComponent(%q) unexpected error: %v", v, err)
		}
	}

	invalid := []string{
		"",
		"-starts-dash",
		".starts-dot",
		"has space",
		"has/slash",
		"has:colon",
		"has@at",
	}
	for _, v := range invalid {
		if err := NameComponent(v, "name"); err == nil {
			t.Errorf("NameComponent(%q) expected error, got nil", v)
		}
	}
}

func TestNetworkName(t *testing.T) {
	valid := []string{
		"my-network",
		"custom_net",
		"none",
	}
	for _, v := range valid {
		if err := NetworkName(v); err != nil {
			t.Errorf("NetworkName(%q) unexpected error: %v", v, err)
		}
	}

	invalid := []string{
		"bridge",
		"host",
		"container:abc",
		"container:anything",
		"",
		"-starts-dash",
		".starts-dot",
	}
	for _, v := range invalid {
		if err := NetworkName(v); err == nil {
			t.Errorf("NetworkName(%q) expected error, got nil", v)
		}
	}
}

func TestPIDsLimit(t *testing.T) {
	valid := []int{512, 64, 1024}
	for _, v := range valid {
		if err := PIDsLimit(v); err != nil {
			t.Errorf("PIDsLimit(%d) unexpected error: %v", v, err)
		}
	}

	invalid := []int{63, 0, -1}
	for _, v := range invalid {
		if err := PIDsLimit(v); err == nil {
			t.Errorf("PIDsLimit(%d) expected error, got nil", v)
		}
	}
}
