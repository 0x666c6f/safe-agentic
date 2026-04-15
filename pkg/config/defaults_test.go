package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Defaults.CPUs != "4" {
		t.Fatalf("CPUs = %q, want 4", cfg.Defaults.CPUs)
	}
	if cfg.Defaults.Memory != "8g" {
		t.Fatalf("Memory = %q, want 8g", cfg.Defaults.Memory)
	}
	if cfg.Defaults.PIDsLimit != 512 {
		t.Fatalf("PIDsLimit = %d, want 512", cfg.Defaults.PIDsLimit)
	}
}

func TestPathsUseSafeAgHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := UserDir(); got != filepath.Join(home, ".safe-ag") {
		t.Fatalf("UserDir() = %q", got)
	}
	if got := ConfigPath(); got != filepath.Join(home, ".safe-ag", "config.toml") {
		t.Fatalf("ConfigPath() = %q", got)
	}
	if got := TemplatesDir(); got != filepath.Join(home, ".safe-ag", "templates") {
		t.Fatalf("TemplatesDir() = %q", got)
	}
	if got := PipelinesDir(); got != filepath.Join(home, ".safe-ag", "pipelines") {
		t.Fatalf("PipelinesDir() = %q", got)
	}
	if got := CronPath(); got != filepath.Join(home, ".safe-ag", "cron.json") {
		t.Fatalf("CronPath() = %q", got)
	}
}

func TestLoadDefaultsMissingFile(t *testing.T) {
	cfg, err := LoadDefaults(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("LoadDefaults() error = %v", err)
	}
	if cfg.Defaults.Memory != "8g" {
		t.Fatalf("Memory = %q, want 8g", cfg.Defaults.Memory)
	}
}

func TestLoadDefaultsMergesSparseToml(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `version = 1

[defaults]
memory = "16g"
reuse_auth = true

[git]
author_name = "Agent Bot"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadDefaults(path)
	if err != nil {
		t.Fatalf("LoadDefaults() error = %v", err)
	}
	if cfg.Defaults.Memory != "16g" {
		t.Fatalf("Memory = %q, want 16g", cfg.Defaults.Memory)
	}
	if cfg.Defaults.CPUs != "4" {
		t.Fatalf("CPUs = %q, want 4", cfg.Defaults.CPUs)
	}
	if !cfg.Defaults.ReuseAuth {
		t.Fatal("ReuseAuth = false, want true")
	}
	if cfg.Git.AuthorName != "Agent Bot" {
		t.Fatalf("AuthorName = %q", cfg.Git.AuthorName)
	}
}

func TestLoadRawConfigRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `version = 1

[defaults]
memory = "16g"
bogus = true
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadRawConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported config keys") {
		t.Fatalf("LoadRawConfig() error = %v, want unsupported config keys", err)
	}
}

func TestSetValueAndSaveRawConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	var raw FileConfig
	if err := SetValue(&raw, "defaults.memory", "16g"); err != nil {
		t.Fatalf("SetValue memory: %v", err)
	}
	if err := SetValue(&raw, "SAFE_AGENTIC_DEFAULT_REUSE_AUTH", "true"); err != nil {
		t.Fatalf("SetValue alias: %v", err)
	}
	if err := SetValue(&raw, "git.author_name", "Agent Bot"); err != nil {
		t.Fatalf("SetValue git: %v", err)
	}
	if err := SaveRawConfig(path, raw); err != nil {
		t.Fatalf("SaveRawConfig() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `memory = "16g"`) {
		t.Fatalf("config missing memory:\n%s", text)
	}
	if !strings.Contains(text, "reuse_auth = true") {
		t.Fatalf("config missing reuse_auth:\n%s", text)
	}
	if !strings.Contains(text, `author_name = "Agent Bot"`) {
		t.Fatalf("config missing author_name:\n%s", text)
	}
}

func TestResetValueDeletesEmptyConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	var raw FileConfig
	if err := SetValue(&raw, "defaults.memory", "16g"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := SaveRawConfig(path, raw); err != nil {
		t.Fatalf("SaveRawConfig: %v", err)
	}
	if err := ResetValue(&raw, "defaults.memory"); err != nil {
		t.Fatalf("ResetValue: %v", err)
	}
	if err := SaveRawConfig(path, raw); err != nil {
		t.Fatalf("SaveRawConfig after reset: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file still exists: %v", err)
	}
}

func TestGetValueSupportsAliases(t *testing.T) {
	cfg := Defaults()
	cfg.Defaults.Memory = "32g"
	got, err := GetValue(cfg, "SAFE_AGENTIC_DEFAULT_MEMORY")
	if err != nil {
		t.Fatalf("GetValue() error = %v", err)
	}
	if got != "32g" {
		t.Fatalf("GetValue() = %q, want 32g", got)
	}
}

func TestSetValueRejectsBadTypes(t *testing.T) {
	var raw FileConfig
	if err := SetValue(&raw, "defaults.pids_limit", "nope"); err == nil {
		t.Fatal("expected integer parse error")
	}
	if err := SetValue(&raw, "defaults.reuse_auth", "nope"); err == nil {
		t.Fatal("expected bool parse error")
	}
}
