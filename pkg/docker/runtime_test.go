package docker

import (
	"strings"
	"testing"
)

func TestNewRunCmd_Basics(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	args := cmd.Build()
	cmdStr := strings.Join(args, " ")
	if !strings.Contains(cmdStr, "docker run") {
		t.Errorf("missing docker run: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "--name agent-claude-abc") {
		t.Errorf("missing --name: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "--hostname agent-claude-abc") {
		t.Errorf("missing --hostname: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "--pull never") {
		t.Errorf("missing --pull never: %s", cmdStr)
	}
	if !strings.Contains(cmdStr, "safe-agentic:latest") {
		t.Errorf("missing image: %s", cmdStr)
	}
}

func TestNewRunCmd_Interactive(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	args := cmd.Build()
	if args[2] != "-it" {
		t.Errorf("expected -it for interactive, got %s", args[2])
	}
}

func TestNewRunCmd_Detached(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.Detached = true
	args := cmd.Build()
	if args[2] != "-d" {
		t.Errorf("expected -d for detached, got %s", args[2])
	}
}

func TestAddLabel(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.AddLabel("mykey", "myvalue")
	args := cmd.Build()
	cmdStr := strings.Join(args, " ")
	if !strings.Contains(cmdStr, "--label mykey=myvalue") {
		t.Errorf("missing label: %s", cmdStr)
	}
}

func TestAddLabel_Sorted(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.AddLabel("z-key", "z-value")
	cmd.AddLabel("a-key", "a-value")
	args := cmd.Build()
	cmdStr := strings.Join(args, " ")
	aPos := strings.Index(cmdStr, "a-key")
	zPos := strings.Index(cmdStr, "z-key")
	if aPos > zPos {
		t.Errorf("labels not sorted: a-key should come before z-key")
	}
}

func TestAddEnv(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.AddEnv("MY_VAR", "myvalue")
	args := cmd.Build()
	cmdStr := strings.Join(args, " ")
	if !strings.Contains(cmdStr, "-e MY_VAR=myvalue") {
		t.Errorf("missing env: %s", cmdStr)
	}
}

func TestAddNamedVolume(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.AddNamedVolume("myvol", "/mnt/data")
	args := cmd.Build()
	cmdStr := strings.Join(args, " ")
	if !strings.Contains(cmdStr, "type=volume,src=myvol,dst=/mnt/data") {
		t.Errorf("missing named volume mount: %s", cmdStr)
	}
}

func TestAddEphemeralVolume(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.AddEphemeralVolume("/workspace")
	args := cmd.Build()
	cmdStr := strings.Join(args, " ")
	if !strings.Contains(cmdStr, "type=volume,dst=/workspace") {
		t.Errorf("missing ephemeral volume mount: %s", cmdStr)
	}
}

func TestAddTmpfs(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.AddTmpfs("/tmp", "512m", true, true)
	args := cmd.Build()
	cmdStr := strings.Join(args, " ")
	if !strings.Contains(cmdStr, "--tmpfs /tmp:rw,noexec,nosuid,size=512m") {
		t.Errorf("missing tmpfs: %s", cmdStr)
	}
}

func TestAddTmpfs_NoSize(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.AddTmpfs("/run", "", false, false)
	args := cmd.Build()
	cmdStr := strings.Join(args, " ")
	if !strings.Contains(cmdStr, "--tmpfs /run:rw") {
		t.Errorf("missing tmpfs: %s", cmdStr)
	}
	if strings.Contains(cmdStr, "size=") {
		t.Errorf("should not have size when empty: %s", cmdStr)
	}
}

func TestAddFlag_RejectsUnsafeFlags(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for unsafe flag")
		}
	}()
	cmd.AddFlag("--privileged")
}

func TestAddFlag_RejectsUnsafeNetworkSplitArgs(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for split --network host")
		}
	}()
	cmd.AddFlag("--network", "host")
}

func TestAppendRuntimeHardening(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	AppendRuntimeHardening(cmd, HardeningOpts{
		Network:   "agent-claude-abc-net",
		Memory:    "8g",
		CPUs:      "4",
		PIDsLimit: 512,
	})
	args := cmd.Build()
	cmdStr := strings.Join(args, " ")

	checks := []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--security-opt=seccomp=/etc/safe-agentic/seccomp.json",
		"--read-only",
		"--network agent-claude-abc-net",
		"--memory 8g",
		"--cpus 4",
		"--pids-limit 512",
		"--ulimit nofile=65536:65536",
		"--tmpfs /tmp:rw,noexec,nosuid,size=512m",
		"--tmpfs /var/tmp:rw,noexec,nosuid,size=256m",
		"--tmpfs /run:rw,noexec,nosuid,size=16m",
		"--tmpfs /dev/shm:rw,noexec,nosuid,size=64m",
		"--tmpfs /home/agent/.config:rw,noexec,nosuid,size=32m",
		"--tmpfs /home/agent/.ssh:rw,noexec,nosuid,size=1m",
		"type=volume,dst=/workspace",
	}
	for _, check := range checks {
		if !strings.Contains(cmdStr, check) {
			t.Errorf("missing %q in: %s", check, cmdStr)
		}
	}
	// Ensure no --privileged flag
	if strings.Contains(cmdStr, "--privileged") {
		t.Errorf("should not contain --privileged: %s", cmdStr)
	}
}

func TestAppendRuntimeHardening_CustomSeccomp(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	AppendRuntimeHardening(cmd, HardeningOpts{SeccompPath: "/custom/seccomp.json"})
	cmdStr := strings.Join(cmd.Build(), " ")
	if !strings.Contains(cmdStr, "seccomp=/custom/seccomp.json") {
		t.Errorf("missing custom seccomp: %s", cmdStr)
	}
}

func TestAppendRuntimeHardening_NoOptional(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	AppendRuntimeHardening(cmd, HardeningOpts{})
	cmdStr := strings.Join(cmd.Build(), " ")
	// Should not have network/memory/cpus/pids when not set
	if strings.Contains(cmdStr, "--network ") {
		t.Errorf("should not have --network when empty: %s", cmdStr)
	}
	if strings.Contains(cmdStr, "--memory ") {
		t.Errorf("should not have --memory when empty: %s", cmdStr)
	}
	if strings.Contains(cmdStr, "--cpus ") {
		t.Errorf("should not have --cpus when empty: %s", cmdStr)
	}
	if strings.Contains(cmdStr, "--pids-limit ") {
		t.Errorf("should not have --pids-limit when 0: %s", cmdStr)
	}
}

func TestAppendCacheMounts(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	AppendCacheMounts(cmd)
	cmdStr := strings.Join(cmd.Build(), " ")
	caches := []string{
		"/home/agent/.npm",
		"/home/agent/.cache/pip",
		"/home/agent/go",
		"/home/agent/.terraform.d/plugin-cache",
	}
	for _, c := range caches {
		if !strings.Contains(cmdStr, c) {
			t.Errorf("missing cache mount %s in: %s", c, cmdStr)
		}
	}
}

func TestRender(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.AddEnv("MY_VAR", "value with spaces")
	rendered := cmd.Render()
	// The entire env arg "MY_VAR=value with spaces" contains spaces, so it gets quoted
	if !strings.Contains(rendered, "\"MY_VAR=value with spaces\"") {
		t.Errorf("expected quoted env entry in: %s", rendered)
	}
}

func TestBuild_ImageIsLast(t *testing.T) {
	cmd := NewRunCmd("agent-claude-abc", "safe-agentic:latest")
	cmd.AddFlag("--rm")
	args := cmd.Build()
	if args[len(args)-1] != "safe-agentic:latest" {
		t.Errorf("image should be last arg, got %s", args[len(args)-1])
	}
}
