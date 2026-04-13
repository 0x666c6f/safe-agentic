package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// CronJob represents a scheduled pipeline/fleet/agent run.
type CronJob struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"` // cron expression: "0 */6 * * *" or shorthand "every 6h"
	Command  string `json:"command"`  // "pipeline pipeline.yaml" or "fleet agents.yaml" or "spawn claude ..."
	Enabled  bool   `json:"enabled"`
	LastRun  string `json:"last_run,omitempty"`
	LastErr  string `json:"last_error,omitempty"`
}

// CronConfig holds all scheduled jobs.
type CronConfig struct {
	Jobs []CronJob `json:"jobs"`
}

var cronCmd = &cobra.Command{
	Use:   "cron",
	Short: "Manage scheduled agent/pipeline runs",
}

var cronAddCmd = &cobra.Command{
	Use:   "add <name> <schedule> <command...>",
	Short: "Add a scheduled job",
	Long: `Add a cron job that runs a safe-ag command on a schedule.

Schedule formats:
  "every 1h"           Run every hour
  "every 6h"           Run every 6 hours
  "every 30m"          Run every 30 minutes
  "daily 09:00"        Run daily at 09:00
  "0 */6 * * *"        Standard cron expression

Command is any safe-ag command (without the 'safe-ag' prefix):
  pipeline pipeline.yaml
  fleet agents.yaml
  spawn claude --ssh --repo git@github.com:org/repo.git --prompt "Run tests"

Examples:
  safe-ag cron add nightly-review "daily 02:00" pipeline pipeline.yaml
  safe-ag cron add hourly-check "every 1h" spawn claude --repo ... --prompt "Check status"`,
	Args: cobra.MinimumNArgs(3),
	RunE: runCronAdd,
}

var cronListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scheduled jobs",
	RunE:  runCronList,
}

var cronRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a scheduled job",
	Args:  cobra.ExactArgs(1),
	RunE:  runCronRemove,
}

var cronEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a disabled job",
	Args:  cobra.ExactArgs(1),
	RunE:  runCronEnable,
}

var cronDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a job without removing it",
	Args:  cobra.ExactArgs(1),
	RunE:  runCronDisable,
}

var cronRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Manually trigger a scheduled job now",
	Args:  cobra.ExactArgs(1),
	RunE:  runCronRun,
}

var cronDaemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Start the cron scheduler daemon (foreground)",
	RunE:  runCronDaemon,
}

func init() {
	cronCmd.AddCommand(cronAddCmd, cronListCmd, cronRemoveCmd, cronEnableCmd, cronDisableCmd, cronRunCmd, cronDaemonCmd)
	rootCmd.AddCommand(cronCmd)
}

func cronConfigPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = home + "/.config"
	}
	return filepath.Join(dir, "safe-agentic", "cron.json")
}

func loadCronConfig() (*CronConfig, error) {
	path := cronConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &CronConfig{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg CronConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveCronConfig(cfg *CronConfig) error {
	path := cronConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func runCronAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	schedule := args[1]
	command := strings.Join(args[2:], " ")

	cfg, err := loadCronConfig()
	if err != nil {
		return err
	}

	// Check for duplicate name
	for _, j := range cfg.Jobs {
		if j.Name == name {
			return fmt.Errorf("job %q already exists. Remove it first with: safe-ag cron remove %s", name, name)
		}
	}

	// Validate schedule
	if _, err := parseSchedule(schedule); err != nil {
		return fmt.Errorf("invalid schedule %q: %w", schedule, err)
	}

	cfg.Jobs = append(cfg.Jobs, CronJob{
		Name:     name,
		Schedule: schedule,
		Command:  command,
		Enabled:  true,
	})

	if err := saveCronConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("Added cron job %q: %s → safe-ag %s\n", name, schedule, command)
	return nil
}

func runCronList(cmd *cobra.Command, args []string) error {
	cfg, err := loadCronConfig()
	if err != nil {
		return err
	}
	if len(cfg.Jobs) == 0 {
		fmt.Println("No cron jobs configured. Add one with: safe-ag cron add <name> <schedule> <command>")
		return nil
	}

	fmt.Printf("%-20s %-15s %-8s %-20s %s\n", "NAME", "SCHEDULE", "ENABLED", "LAST RUN", "COMMAND")
	for _, j := range cfg.Jobs {
		enabled := "yes"
		if !j.Enabled {
			enabled = "no"
		}
		lastRun := "-"
		if j.LastRun != "" {
			lastRun = j.LastRun
		}
		fmt.Printf("%-20s %-15s %-8s %-20s %s\n", j.Name, j.Schedule, enabled, lastRun, j.Command)
	}
	return nil
}

func runCronRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	cfg, err := loadCronConfig()
	if err != nil {
		return err
	}
	var kept []CronJob
	found := false
	for _, j := range cfg.Jobs {
		if j.Name == name {
			found = true
			continue
		}
		kept = append(kept, j)
	}
	if !found {
		return fmt.Errorf("job %q not found", name)
	}
	cfg.Jobs = kept
	if err := saveCronConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("Removed cron job %q\n", name)
	return nil
}

func runCronEnable(cmd *cobra.Command, args []string) error {
	return setCronEnabled(args[0], true)
}

func runCronDisable(cmd *cobra.Command, args []string) error {
	return setCronEnabled(args[0], false)
}

func setCronEnabled(name string, enabled bool) error {
	cfg, err := loadCronConfig()
	if err != nil {
		return err
	}
	for i, j := range cfg.Jobs {
		if j.Name == name {
			cfg.Jobs[i].Enabled = enabled
			if err := saveCronConfig(cfg); err != nil {
				return err
			}
			state := "enabled"
			if !enabled {
				state = "disabled"
			}
			fmt.Printf("Job %q %s\n", name, state)
			return nil
		}
	}
	return fmt.Errorf("job %q not found", name)
}

func runCronRun(cmd *cobra.Command, args []string) error {
	name := args[0]
	cfg, err := loadCronConfig()
	if err != nil {
		return err
	}
	for i, j := range cfg.Jobs {
		if j.Name == name {
			fmt.Printf("Running job %q: safe-ag %s\n", name, j.Command)
			runErr := executeCronJob(j)
			cfg.Jobs[i].LastRun = time.Now().Format(time.RFC3339)
			if runErr != nil {
				cfg.Jobs[i].LastErr = runErr.Error()
			} else {
				cfg.Jobs[i].LastErr = ""
			}
			saveCronConfig(cfg)
			return runErr
		}
	}
	return fmt.Errorf("job %q not found", name)
}

func runCronDaemon(cmd *cobra.Command, args []string) error {
	fmt.Println("safe-ag cron daemon started. Press Ctrl+C to stop.")
	fmt.Println("Checking jobs every 60 seconds...")

	for {
		cfg, err := loadCronConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "load config: %v\n", err)
			time.Sleep(60 * time.Second)
			continue
		}

		now := time.Now()
		for i, j := range cfg.Jobs {
			if !j.Enabled {
				continue
			}
			if shouldRun(j, now) {
				fmt.Printf("[%s] Running job %q: safe-ag %s\n", now.Format("15:04:05"), j.Name, j.Command)
				runErr := executeCronJob(j)
				cfg.Jobs[i].LastRun = now.Format(time.RFC3339)
				if runErr != nil {
					cfg.Jobs[i].LastErr = runErr.Error()
					fmt.Fprintf(os.Stderr, "[%s] Job %q failed: %v\n", now.Format("15:04:05"), j.Name, runErr)
				} else {
					cfg.Jobs[i].LastErr = ""
					fmt.Printf("[%s] Job %q completed\n", now.Format("15:04:05"), j.Name)
				}
				saveCronConfig(cfg)
			}
		}

		time.Sleep(60 * time.Second)
	}
}

func executeCronJob(job CronJob) error {
	parts := strings.Fields(job.Command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	// Execute as subprocess: safe-ag <command parts>
	safeAgBin, err := os.Executable()
	if err != nil {
		safeAgBin = "safe-ag"
	}

	c := exec.Command(safeAgBin, parts...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// shouldRun determines if a job should run based on its schedule and last run time.
func shouldRun(job CronJob, now time.Time) bool {
	interval, err := parseSchedule(job.Schedule)
	if err != nil {
		return false
	}

	if job.LastRun == "" {
		return true // never run before
	}

	lastRun, err := time.Parse(time.RFC3339, job.LastRun)
	if err != nil {
		return true // can't parse last run, run now
	}

	return now.Sub(lastRun) >= interval
}

// parseSchedule parses schedule strings into a duration.
// Supports: "every Xh", "every Xm", "daily HH:MM", basic intervals.
func parseSchedule(schedule string) (time.Duration, error) {
	schedule = strings.TrimSpace(schedule)

	// "every Xh" / "every Xm" / "every Xs"
	if strings.HasPrefix(schedule, "every ") {
		durStr := strings.TrimPrefix(schedule, "every ")
		d, err := time.ParseDuration(durStr)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", durStr)
		}
		if d < time.Minute {
			return 0, fmt.Errorf("minimum interval is 1 minute")
		}
		return d, nil
	}

	// "daily HH:MM" → 24h interval
	if strings.HasPrefix(schedule, "daily") {
		return 24 * time.Hour, nil
	}

	// Standard cron expressions → approximate with intervals
	// "*/5 * * * *" → every 5 minutes
	// "0 */6 * * *" → every 6 hours
	// "0 0 * * *" → daily
	parts := strings.Fields(schedule)
	if len(parts) == 5 {
		// Minute field
		if strings.HasPrefix(parts[0], "*/") {
			var mins int
			fmt.Sscanf(parts[0], "*/%d", &mins)
			if mins > 0 {
				return time.Duration(mins) * time.Minute, nil
			}
		}
		// Hour field
		if strings.HasPrefix(parts[1], "*/") {
			var hrs int
			fmt.Sscanf(parts[1], "*/%d", &hrs)
			if hrs > 0 {
				return time.Duration(hrs) * time.Hour, nil
			}
		}
		// "0 0 * * *" → daily
		if parts[0] == "0" && parts[1] == "0" {
			return 24 * time.Hour, nil
		}
		// Default: hourly
		return time.Hour, nil
	}

	return 0, fmt.Errorf("unrecognized schedule format: %q (use 'every Xh', 'daily HH:MM', or cron expression)", schedule)
}
