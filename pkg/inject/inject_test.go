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
