package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCronConfigPathPrefersXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/safe-agentic-config")
	got := cronConfigPath()
	want := "/tmp/safe-agentic-config/safe-agentic/cron.json"
	if got != want {
		t.Fatalf("cronConfigPath() = %q, want %q", got, want)
	}
}

func TestLoadSaveCronConfigRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	want := &CronConfig{
		Jobs: []CronJob{{
			Name:     "nightly",
			Schedule: "daily 02:00",
			Command:  "pipeline nightly.yaml",
			Enabled:  true,
			LastRun:  "2026-04-13T00:00:00Z",
		}},
	}
	if err := saveCronConfig(want); err != nil {
		t.Fatalf("saveCronConfig() error = %v", err)
	}

	got, err := loadCronConfig()
	if err != nil {
		t.Fatalf("loadCronConfig() error = %v", err)
	}
	if len(got.Jobs) != 1 || got.Jobs[0].Name != "nightly" || got.Jobs[0].Command != "pipeline nightly.yaml" {
		t.Fatalf("round trip = %#v", got)
	}

	raw, err := os.ReadFile(cronConfigPath())
	if err != nil {
		t.Fatalf("read saved cron config: %v", err)
	}
	var decoded CronConfig
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("saved config is invalid json: %v", err)
	}
}

func TestParseScheduleVariants(t *testing.T) {
	cases := []struct {
		schedule string
		want     time.Duration
		wantErr  string
	}{
		{schedule: "every 90m", want: 90 * time.Minute},
		{schedule: "every 30s", wantErr: "minimum interval is 1 minute"},
		{schedule: "daily 09:00", want: 24 * time.Hour},
		{schedule: "*/5 * * * *", want: 5 * time.Minute},
		{schedule: "0 */6 * * *", want: 6 * time.Hour},
		{schedule: "0 0 * * *", want: 24 * time.Hour},
		{schedule: "15 3 * * *", want: 24 * time.Hour},
		{schedule: "nonsense", wantErr: "unrecognized schedule format"},
	}

	for _, tc := range cases {
		got, err := parseSchedule(tc.schedule)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("parseSchedule(%q) error = %v, want substring %q", tc.schedule, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseSchedule(%q) error = %v", tc.schedule, err)
		}
		if got != tc.want {
			t.Fatalf("parseSchedule(%q) = %v, want %v", tc.schedule, got, tc.want)
		}
	}
}

func TestShouldRun(t *testing.T) {
	now := time.Date(2026, 4, 13, 15, 0, 0, 0, time.UTC)

	if !shouldRun(CronJob{Schedule: "every 1h"}, now) {
		t.Fatal("shouldRun() should be true for never-run job")
	}
	if shouldRun(CronJob{Schedule: "bad schedule", LastRun: now.Format(time.RFC3339)}, now) {
		t.Fatal("shouldRun() should be false for invalid schedule")
	}
	if !shouldRun(CronJob{Schedule: "every 1h", LastRun: "not-a-time"}, now) {
		t.Fatal("shouldRun() should be true for invalid LastRun")
	}
	if shouldRun(CronJob{Schedule: "every 1h", LastRun: now.Add(-30 * time.Minute).Format(time.RFC3339)}, now) {
		t.Fatal("shouldRun() should be false before interval elapses")
	}
	if !shouldRun(CronJob{Schedule: "every 1h", LastRun: now.Add(-2 * time.Hour).Format(time.RFC3339)}, now) {
		t.Fatal("shouldRun() should be true after interval elapses")
	}
}

func TestRunCronLifecycleCommands(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	addOut := captureOutput(func() {
		if err := runCronAdd(cronAddCmd, []string{"nightly", "every 1h", "pipeline", "nightly.yaml"}); err != nil {
			t.Fatalf("runCronAdd() error = %v", err)
		}
	})
	if !strings.Contains(addOut, `Added cron job "nightly"`) {
		t.Fatalf("unexpected add output: %s", addOut)
	}

	if err := runCronAdd(cronAddCmd, []string{"nightly", "every 1h", "pipeline", "nightly.yaml"}); err == nil {
		t.Fatal("duplicate runCronAdd() should fail")
	}

	listOut := captureOutput(func() {
		if err := runCronList(cronListCmd, nil); err != nil {
			t.Fatalf("runCronList() error = %v", err)
		}
	})
	if !strings.Contains(listOut, "nightly") || !strings.Contains(listOut, "pipeline nightly.yaml") {
		t.Fatalf("unexpected list output: %s", listOut)
	}

	disableOut := captureOutput(func() {
		if err := runCronDisable(cronDisableCmd, []string{"nightly"}); err != nil {
			t.Fatalf("runCronDisable() error = %v", err)
		}
	})
	if !strings.Contains(disableOut, `Job "nightly" disabled`) {
		t.Fatalf("unexpected disable output: %s", disableOut)
	}

	enableOut := captureOutput(func() {
		if err := runCronEnable(cronEnableCmd, []string{"nightly"}); err != nil {
			t.Fatalf("runCronEnable() error = %v", err)
		}
	})
	if !strings.Contains(enableOut, `Job "nightly" enabled`) {
		t.Fatalf("unexpected enable output: %s", enableOut)
	}

	removeOut := captureOutput(func() {
		if err := runCronRemove(cronRemoveCmd, []string{"nightly"}); err != nil {
			t.Fatalf("runCronRemove() error = %v", err)
		}
	})
	if !strings.Contains(removeOut, `Removed cron job "nightly"`) {
		t.Fatalf("unexpected remove output: %s", removeOut)
	}

	emptyOut := captureOutput(func() {
		if err := runCronList(cronListCmd, nil); err != nil {
			t.Fatalf("runCronList() empty error = %v", err)
		}
	})
	if !strings.Contains(emptyOut, "No cron jobs configured") {
		t.Fatalf("unexpected empty list output: %s", emptyOut)
	}
}

func TestRunCronRunAndRemoveErrors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := runCronRemove(cronRemoveCmd, []string{"missing"}); err == nil || !strings.Contains(err.Error(), `job "missing" not found`) {
		t.Fatalf("runCronRemove() error = %v", err)
	}
	if err := runCronRun(cronRunCmd, []string{"missing"}); err == nil || !strings.Contains(err.Error(), `job "missing" not found`) {
		t.Fatalf("runCronRun() error = %v", err)
	}
	if err := executeCronJob(CronJob{}); err == nil || !strings.Contains(err.Error(), "empty command") {
		t.Fatalf("executeCronJob() error = %v", err)
	}
}

func TestRunCronAddRejectsInvalidSchedule(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	err := runCronAdd(cronAddCmd, []string{"too-fast", "every 30s", "pipeline", "nightly.yaml"})
	if err == nil || !strings.Contains(err.Error(), `invalid schedule "every 30s"`) {
		t.Fatalf("runCronAdd() error = %v", err)
	}
}

func TestCronConfigPathFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	got := cronConfigPath()
	want := filepath.Join(home, ".config", "safe-agentic", "cron.json")
	if got != want {
		t.Fatalf("cronConfigPath() fallback = %q, want %q", got, want)
	}
}
