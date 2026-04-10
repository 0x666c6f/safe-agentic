package inject

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestEncodeB64Roundtrip(t *testing.T) {
	inputs := []string{
		"hello world",
		"",
		"special chars: !@#$%^&*()",
		"newline\nand\ttab",
		"unicode: 日本語",
	}
	for _, input := range inputs {
		encoded := EncodeB64(input)
		decoded, err := DecodeB64(encoded)
		if err != nil {
			t.Errorf("DecodeB64(EncodeB64(%q)) error: %v", input, err)
			continue
		}
		if decoded != input {
			t.Errorf("roundtrip failed: got %q, want %q", decoded, input)
		}
	}
}

func TestDecodeB64Roundtrip(t *testing.T) {
	original := "safe-agentic config data"
	encoded := base64.StdEncoding.EncodeToString([]byte(original))
	decoded, err := DecodeB64(encoded)
	if err != nil {
		t.Fatalf("DecodeB64 error: %v", err)
	}
	if decoded != original {
		t.Errorf("got %q, want %q", decoded, original)
	}
}

func TestDecodeB64InvalidInput(t *testing.T) {
	_, err := DecodeB64("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64, got nil")
	}
}

func TestReadClaudeConfigWithSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	content := `{"autoUpdates":false,"permissions":{"allow":["Bash"]}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	envs, err := ReadClaudeConfig(dir)
	if err != nil {
		t.Fatalf("ReadClaudeConfig error: %v", err)
	}

	val, ok := envs["SAFE_AGENTIC_CLAUDE_CONFIG_B64"]
	if !ok {
		t.Fatal("expected SAFE_AGENTIC_CLAUDE_CONFIG_B64 key in result")
	}

	decoded, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		t.Fatalf("decode env var value: %v", err)
	}
	if string(decoded) != content {
		t.Errorf("decoded content = %q, want %q", string(decoded), content)
	}
}

func TestReadClaudeConfigMissingDir(t *testing.T) {
	envs, err := ReadClaudeConfig("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("ReadClaudeConfig missing dir returned error: %v", err)
	}
	if len(envs) != 0 {
		t.Errorf("expected empty map for missing dir, got %v", envs)
	}
}

func TestReadAWSCredentialsProfileFound(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	content := "[default]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n\n[my-profile]\naws_access_key_id = AKIAI44QH8DHBEXAMPLE\naws_secret_access_key = je7MtGbClwBF/2Zp9Utk/h3yCo8nvbEXAMPLEKEY\n"
	if err := os.WriteFile(credPath, []byte(content), 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	envs, err := ReadAWSCredentials(credPath, "my-profile")
	if err != nil {
		t.Fatalf("ReadAWSCredentials error: %v", err)
	}

	if envs["AWS_PROFILE"] != "my-profile" {
		t.Errorf("AWS_PROFILE = %q, want %q", envs["AWS_PROFILE"], "my-profile")
	}

	b64val, ok := envs["SAFE_AGENTIC_AWS_CREDS_B64"]
	if !ok {
		t.Fatal("expected SAFE_AGENTIC_AWS_CREDS_B64 key")
	}
	decoded, err := base64.StdEncoding.DecodeString(b64val)
	if err != nil {
		t.Fatalf("decode AWS creds b64: %v", err)
	}
	if string(decoded) != content {
		t.Errorf("decoded AWS creds = %q, want %q", string(decoded), content)
	}
}

func TestReadAWSCredentialsMissingProfile(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	content := "[default]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\n"
	if err := os.WriteFile(credPath, []byte(content), 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	_, err := ReadAWSCredentials(credPath, "nonexistent-profile")
	if err == nil {
		t.Error("expected error for missing profile, got nil")
	}
}

func TestReadAWSCredentialsMissingFile(t *testing.T) {
	_, err := ReadAWSCredentials("/nonexistent/path/credentials", "default")
	if err == nil {
		t.Error("expected error for missing credentials file, got nil")
	}
}

func TestReadAWSCredentialsForwardsRegionEnvVars(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	content := "[my-profile]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\n"
	if err := os.WriteFile(credPath, []byte(content), 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
	t.Setenv("AWS_REGION", "us-west-2")

	envs, err := ReadAWSCredentials(credPath, "my-profile")
	if err != nil {
		t.Fatalf("ReadAWSCredentials error: %v", err)
	}

	if envs["AWS_DEFAULT_REGION"] != "us-east-1" {
		t.Errorf("AWS_DEFAULT_REGION = %q, want %q", envs["AWS_DEFAULT_REGION"], "us-east-1")
	}
	if envs["AWS_REGION"] != "us-west-2" {
		t.Errorf("AWS_REGION = %q, want %q", envs["AWS_REGION"], "us-west-2")
	}
}

func TestEncodeFileB64_ReadsAndEncodesFile(t *testing.T) {
	dir := t.TempDir()
	content := "hello, this is a test file\nwith multiple lines\n"
	path := filepath.Join(dir, "testfile.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	encoded, err := EncodeFileB64(path)
	if err != nil {
		t.Fatalf("EncodeFileB64 error: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if string(decoded) != content {
		t.Errorf("decoded content = %q, want %q", string(decoded), content)
	}
}

func TestEncodeFileB64_MissingFile(t *testing.T) {
	_, err := EncodeFileB64("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestReadCodexConfig_WithConfigToml(t *testing.T) {
	dir := t.TempDir()
	content := `model = "gpt-5.2"
model_reasoning_effort = "xhigh"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	envs, err := ReadCodexConfig(dir)
	if err != nil {
		t.Fatalf("ReadCodexConfig error: %v", err)
	}

	val, ok := envs["SAFE_AGENTIC_CODEX_CONFIG_B64"]
	if !ok {
		t.Fatal("expected SAFE_AGENTIC_CODEX_CONFIG_B64 key in result")
	}

	decoded, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		t.Fatalf("decode env var value: %v", err)
	}
	if string(decoded) != content {
		t.Errorf("decoded content = %q, want %q", string(decoded), content)
	}
}

func TestReadCodexConfig_MissingDir(t *testing.T) {
	envs, err := ReadCodexConfig("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("ReadCodexConfig missing dir returned error: %v", err)
	}
	if len(envs) != 0 {
		t.Errorf("expected empty map for missing dir, got %v", envs)
	}
}

func TestReadClaudeConfig_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denial as root")
	}
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{}`), 0000); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}
	_, err := ReadClaudeConfig(dir)
	if err == nil {
		t.Error("expected error for unreadable settings.json, got nil")
	}
}

func TestReadCodexConfig_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("cannot test permission denial as root")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("model = \"gpt-5.2\"\n"), 0000); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	_, err := ReadCodexConfig(dir)
	if err == nil {
		t.Error("expected error for unreadable config.toml, got nil")
	}
}
