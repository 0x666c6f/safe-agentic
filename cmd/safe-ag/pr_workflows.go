package main

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/catalog"
	"github.com/0x666c6f/safe-agentic/pkg/fleet"
	"github.com/spf13/cobra"
)

var (
	prReviewRepos  []string
	prReviewVars   []string
	prReviewDryRun bool
	prFixRepos     []string
	prFixVars      []string
	prFixDryRun    bool
)

var prReviewCmd = &cobra.Command{
	Use:   "pr-review [claude|codex|dual] [pr]",
	Short: "Run a one-shot PR review workflow",
	Long: `Run a one-shot PR review workflow.

If no mode is provided, pr-review defaults to "dual".
If no PR number is provided, the current checkout is inspected with
"gh pr view --json number" and the active PR is used automatically.

Built-in review presets live under reviews/ and can be overridden by
user pipelines in ~/.safe-ag/pipelines/reviews/.`,
	Args:  cobra.RangeArgs(0, 2),
	RunE:  runPRReview,
}

var prFixCmd = &cobra.Command{
	Use:   "pr-fix [pr]",
	Short: "Run the PR fix workflow for the current or specified PR",
	Long: `Run the PR fix workflow for the current or specified PR.

If no PR number is provided, the current checkout is inspected with
"gh pr view --json number" and the active PR is used automatically.

The built-in fix preset can be overridden by a user pipeline at
~/.safe-ag/pipelines/reviews/fix.yaml.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPRFixWorkflow,
}

func init() {
	prReviewCmd.Flags().StringSliceVar(&prReviewRepos, "repo", nil, "Default repo URL; inferred from current checkout when omitted")
	prReviewCmd.Flags().StringSliceVar(&prReviewVars, "var", nil, "Workflow variable assignment (key=value)")
	prReviewCmd.Flags().BoolVar(&prReviewDryRun, "dry-run", false, "Print execution plan without running")
	rootCmd.AddCommand(prReviewCmd)

	prFixCmd.Flags().StringSliceVar(&prFixRepos, "repo", nil, "Default repo URL; inferred from current checkout when omitted")
	prFixCmd.Flags().StringSliceVar(&prFixVars, "var", nil, "Workflow variable assignment (key=value)")
	prFixCmd.Flags().BoolVar(&prFixDryRun, "dry-run", false, "Print execution plan without running")
	rootCmd.AddCommand(prFixCmd)
}

func runPRReview(cmd *cobra.Command, args []string) error {
	mode, prValue, err := parsePRReviewArgs(args)
	if err != nil {
		return err
	}
	assignments := append([]string{}, prReviewVars...)
	if prValue != "" {
		assignments = append(assignments, "pr="+prValue)
	}
	return runReviewPreset(mode, assignments, prReviewRepos, prReviewDryRun)
}

func runPRFixWorkflow(cmd *cobra.Command, args []string) error {
	assignments := append([]string{}, prFixVars...)
	if len(args) == 1 {
		assignments = append(assignments, "pr="+args[0])
	}
	return runReviewPreset("fix", assignments, prFixRepos, prFixDryRun)
}

func runReviewPreset(name string, assignments, repos []string, dryRun bool) error {
	asset, err := catalog.ResolveReviewPreset(name)
	if err != nil {
		return err
	}
	vars, defaultRepos, err := parseTemplateVars(assignments, repos, true)
	if err != nil {
		return err
	}
	parseOpts := fleet.ParseOptions{
		Vars:         vars,
		DefaultRepos: defaultRepos,
	}
	m, err := fleet.ParsePipelineWithOptions(asset.Path, parseOpts)
	if err != nil {
		return err
	}
	if dryRun {
		printPipelineTree(m.Name, m.Stages)
		return nil
	}
	ctx := context.Background()
	exec := newExecutor()
	timestamp := time.Now().Format("20060102-150405")
	return runPipelineManifest(ctx, exec, m, parseOpts, false, timestamp, "", nil)
}

var digitsOnly = regexp.MustCompile(`^\d+$`)

func parsePRReviewArgs(args []string) (string, string, error) {
	mode := "dual"
	prValue := ""
	switch len(args) {
	case 0:
		return mode, prValue, nil
	case 1:
		if isPRReviewMode(args[0]) {
			return args[0], "", nil
		}
		if !digitsOnly.MatchString(args[0]) {
			return "", "", fmt.Errorf("expected PR number or one of claude|codex|dual")
		}
		return mode, args[0], nil
	case 2:
		if !isPRReviewMode(args[0]) {
			return "", "", fmt.Errorf("unknown review mode %q", args[0])
		}
		if !digitsOnly.MatchString(args[1]) {
			return "", "", fmt.Errorf("PR must be numeric")
		}
		return args[0], args[1], nil
	default:
		return "", "", fmt.Errorf("invalid pr-review arguments")
	}
}

func isPRReviewMode(value string) bool {
	switch value {
	case "claude", "codex", "dual":
		return true
	default:
		return false
	}
}
