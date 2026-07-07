package docker

import (
	"context"
	"github.com/0x666c6f/berth/pkg/vmexec"
	"strings"
	"testing"
)

func TestVolumeExists_Found(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetResponse("docker volume inspect myvolume", "some output")
	found, err := VolumeExists(context.Background(), fake, "myvolume")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected volume to be found")
	}
}

func TestVolumeExists_NotFound(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("docker volume inspect novolume", "no such volume")
	found, err := VolumeExists(context.Background(), fake, "novolume")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected volume to not be found")
	}
}

func TestCreateLabeledVolume_VerifyLabels(t *testing.T) {
	fake := vmexec.NewFake()
	err := CreateLabeledVolume(context.Background(), fake, "myvol", "auth", "agent-claude-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmds := fake.CommandsMatching("docker volume create")
	if len(cmds) == 0 {
		t.Fatal("expected docker volume create command")
	}
	cmdStr := strings.Join(cmds[0], " ")
	if !strings.Contains(cmdStr, "app=berth") {
		t.Errorf("missing app label in: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "berth.type=auth") {
		t.Errorf("missing type label in: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "berth.parent=agent-claude-abc") {
		t.Errorf("missing parent label in: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "myvol") {
		t.Errorf("missing volume name in: %s", cmdStr)
	}
}

func TestCreateLabeledVolume_Error(t *testing.T) {
	fake := vmexec.NewFake()
	fake.SetError("docker volume create", "permission denied")
	err := CreateLabeledVolume(context.Background(), fake, "myvol", "auth", "parent")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "create volume") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAuthVolumeName_Shared(t *testing.T) {
	name := AuthVolumeName("claude", false, "agent-claude-abc")
	if name != "berth-claude-auth" {
		t.Errorf("expected berth-claude-auth, got %s", name)
	}
}

func TestAuthVolumeName_Ephemeral(t *testing.T) {
	name := AuthVolumeName("claude", true, "agent-claude-abc")
	if name != "agent-claude-abc-auth" {
		t.Errorf("expected agent-claude-abc-auth, got %s", name)
	}
}

func TestGHAuthVolumeName(t *testing.T) {
	name := GHAuthVolumeName("claude")
	if name != "berth-claude-gh-auth" {
		t.Errorf("expected berth-claude-gh-auth, got %s", name)
	}
}
