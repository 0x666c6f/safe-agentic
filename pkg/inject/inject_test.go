package inject

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
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

func TestReadClaudeConfigReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "settings.json"), 0700); err != nil {
		t.Fatalf("mkdir settings.json: %v", err)
	}

	_, err := ReadClaudeConfig(dir)
	if err == nil || !strings.Contains(err.Error(), "read claude settings") {
		t.Fatalf("ReadClaudeConfig error = %v", err)
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

func TestExtractClaudeAccessToken(t *testing.T) {
	secret := `{"claudeAiOauth":{"accessToken":"token-123","refreshToken":"refresh-456"}}`
	if got := extractClaudeAccessToken(secret); got != "token-123" {
		t.Fatalf("extractClaudeAccessToken() = %q, want %q", got, "token-123")
	}
	if got := extractClaudeAccessToken(`{"claudeAiOauth":{}}`); got != "" {
		t.Fatalf("extractClaudeAccessToken() = %q, want empty", got)
	}
	if got := extractClaudeAccessToken(`not-json`); got != "" {
		t.Fatalf("extractClaudeAccessToken() = %q, want empty", got)
	}
}

func TestEncodeFileB64Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.txt")
	content := "payload\nwith newline"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	encoded, err := EncodeFileB64(path)
	if err != nil {
		t.Fatalf("EncodeFileB64 error: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if string(decoded) != content {
		t.Fatalf("decoded payload = %q, want %q", string(decoded), content)
	}
}

func TestEncodeFileB64MissingFile(t *testing.T) {
	if _, err := EncodeFileB64("/nonexistent/payload.txt"); err == nil {
		t.Fatal("EncodeFileB64 missing file should fail")
	}
}

func TestReadCodexConfigWithConfigTOML(t *testing.T) {
	dir := t.TempDir()
	content := "model = \"gpt-5\"\napproval = \"never\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	envs, err := ReadCodexConfig(dir)
	if err != nil {
		t.Fatalf("ReadCodexConfig error: %v", err)
	}

	val, ok := envs["SAFE_AGENTIC_CODEX_CONFIG_B64"]
	if !ok {
		t.Fatal("expected SAFE_AGENTIC_CODEX_CONFIG_B64 key")
	}
	decoded, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		t.Fatalf("decode env var: %v", err)
	}
	if string(decoded) != content {
		t.Fatalf("decoded config = %q, want %q", string(decoded), content)
	}
}

func TestReadCodexConfigMissingDir(t *testing.T) {
	envs, err := ReadCodexConfig("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("ReadCodexConfig missing dir returned error: %v", err)
	}
	if len(envs) != 0 {
		t.Fatalf("expected empty map, got %v", envs)
	}
}

func TestReadCodexConfigReadError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "config.toml"), 0700); err != nil {
		t.Fatalf("mkdir config.toml: %v", err)
	}

	_, err := ReadCodexConfig(dir)
	if err == nil {
		t.Fatal("expected ReadCodexConfig error, got nil")
	}
	if !strings.Contains(err.Error(), "read codex config") {
		t.Fatalf("unexpected error: %v", err)
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

func TestReadAWSCredentialsPropagatesRegionEnv(t *testing.T) {
	dir := t.TempDir()
	credPath := filepath.Join(dir, "credentials")
	content := "[my-profile]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\n"
	if err := os.WriteFile(credPath, []byte(content), 0600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}

	t.Setenv("AWS_DEFAULT_REGION", "eu-west-3")
	t.Setenv("AWS_REGION", "eu-central-1")

	envs, err := ReadAWSCredentials(credPath, "my-profile")
	if err != nil {
		t.Fatalf("ReadAWSCredentials error: %v", err)
	}
	if envs["AWS_DEFAULT_REGION"] != "eu-west-3" {
		t.Errorf("AWS_DEFAULT_REGION = %q, want %q", envs["AWS_DEFAULT_REGION"], "eu-west-3")
	}
	if envs["AWS_REGION"] != "eu-central-1" {
		t.Errorf("AWS_REGION = %q, want %q", envs["AWS_REGION"], "eu-central-1")
	}
}
