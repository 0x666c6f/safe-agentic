package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileAndEnforceAllowsListedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.toml")
	body := `[allow]
docker_modes = ["off", "dind"]
networks = ["managed", "none"]
aws_profiles = ["dev"]
ssh = false
reuse_auth = false
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	rule, ok, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if !ok {
		t.Fatal("LoadFile() ok = false, want true")
	}
	err = Enforce([]RuleSet{rule}, SpawnRequest{
		DockerMode: DockerModeDinD,
		Network:    NetworkManaged,
		AWSProfile: "dev",
	})
	if err != nil {
		t.Fatalf("Enforce() error = %v", err)
	}
}

func TestEnforceDeniesDangerousModes(t *testing.T) {
	allowDockerOff := []string{DockerModeOff}
	allowManaged := []string{NetworkManaged}
	allowAWSDev := []string{"dev"}
	disabled := false
	rule := RuleSet{
		Source: "test-rules",
		Allow: AllowRules{
			DockerModes:       &allowDockerOff,
			Networks:          &allowManaged,
			AWSProfiles:       &allowAWSDev,
			SSH:               &disabled,
			ReuseAuth:         &disabled,
			ReuseGHAuth:       &disabled,
			SeedAuth:          &disabled,
			AllowSetupScripts: &disabled,
		},
	}

	tests := []struct {
		name string
		req  SpawnRequest
		want string
	}{
		{
			name: "docker",
			req: SpawnRequest{
				DockerMode: DockerModeDinD,
				Network:    NetworkManaged,
			},
			want: `denies docker mode "dind"`,
		},
		{
			name: "network",
			req: SpawnRequest{
				DockerMode: DockerModeOff,
				Network:    NetworkNone,
			},
			want: `denies network "none"`,
		},
		{
			name: "aws",
			req: SpawnRequest{
				DockerMode: DockerModeOff,
				Network:    NetworkManaged,
				AWSProfile: "prod",
			},
			want: `denies AWS profile "prod"`,
		},
		{
			name: "ssh",
			req: SpawnRequest{
				DockerMode: DockerModeOff,
				Network:    NetworkManaged,
				SSH:        true,
			},
			want: "denies SSH forwarding",
		},
		{
			name: "auth",
			req: SpawnRequest{
				DockerMode: DockerModeOff,
				Network:    NetworkManaged,
				ReuseAuth:  true,
			},
			want: "denies shared agent auth",
		},
		{
			name: "gh-auth",
			req: SpawnRequest{
				DockerMode:  DockerModeOff,
				Network:     NetworkManaged,
				ReuseGHAuth: true,
			},
			want: "denies shared GitHub auth",
		},
		{
			name: "seed-auth",
			req: SpawnRequest{
				DockerMode: DockerModeOff,
				Network:    NetworkManaged,
				SeedAuth:   true,
			},
			want: "denies host auth seeding",
		},
		{
			name: "setup-scripts",
			req: SpawnRequest{
				DockerMode:        DockerModeOff,
				Network:           NetworkManaged,
				AllowSetupScripts: true,
			},
			want: "denies repo setup scripts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Enforce([]RuleSet{rule}, tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Enforce() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.toml")
	body := `[allow]
bogus = true
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	_, _, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported policy keys") {
		t.Fatalf("LoadFile() error = %v, want unsupported policy keys", err)
	}
}

func TestDefaultRulePathsIncludesNearestProjectRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(t.TempDir(), "repo")
	nested := filepath.Join(repo, "a", "b")
	rulesDir := filepath.Join(repo, ".safe-ag")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatalf("mkdir rules dir: %v", err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "rules.toml"), []byte("[allow]\n"), 0o600); err != nil {
		t.Fatalf("write rules: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWD)

	paths := DefaultRulePaths()
	if len(paths) != 2 {
		t.Fatalf("DefaultRulePaths() = %v, want user + project paths", paths)
	}
	if got, want := paths[0], filepath.Join(home, ".safe-ag", "rules.toml"); !sameFilePath(got, want) {
		t.Fatalf("user path = %q, want %q", got, want)
	}
	if got, want := paths[1], filepath.Join(rulesDir, "rules.toml"); !sameFilePath(got, want) {
		t.Fatalf("project path = %q, want %q", got, want)
	}
}

func sameFilePath(a, b string) bool {
	cleanA, errA := filepath.EvalSymlinks(filepath.Dir(a))
	cleanB, errB := filepath.EvalSymlinks(filepath.Dir(b))
	if errA == nil && errB == nil {
		return filepath.Join(cleanA, filepath.Base(a)) == filepath.Join(cleanB, filepath.Base(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
