package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/0x666c6f/safe-agentic/pkg/labels"
	"github.com/0x666c6f/safe-agentic/pkg/vmexec"
	"github.com/spf13/cobra"
)

var searchLines int
var searchCaseSensitive bool

var searchCmd = &cobra.Command{
	Use:     "search <query> [name|--latest]",
	Short:   "Search agent session logs",
	GroupID: groupObserve,
	Args:    cobra.RangeArgs(1, 2),
	RunE:    runSearch,
}

func init() {
	searchCmd.Flags().IntVar(&searchLines, "lines", 500, "Session lines to scan per agent")
	searchCmd.Flags().BoolVar(&searchCaseSensitive, "case-sensitive", false, "Use case-sensitive matching")
	addLatestFlag(searchCmd)
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()
	exec := newExecutor()

	var names []string
	if len(args) > 1 || latestFlag(cmd) {
		targetArgs := []string{}
		if len(args) > 1 {
			targetArgs = append(targetArgs, args[1])
		}
		name, err := docker.ResolveTarget(ctx, exec, targetFromArgs(cmd, targetArgs))
		if err != nil {
			return err
		}
		names = []string{name}
	} else {
		out, err := exec.Run(ctx, "docker", "ps", "-a", "--filter", "name=^agent-", "--format", "{{.Names}}")
		if err != nil {
			return fmt.Errorf("list containers: %w", err)
		}
		names = splitLines(string(out))
	}
	if len(names) == 0 {
		fmt.Println("No agent containers found.")
		return nil
	}

	matches := 0
	for _, name := range names {
		found, err := searchAgentLog(ctx, exec, name, query, searchLines, searchCaseSensitive)
		if err != nil {
			continue
		}
		for _, line := range found {
			fmt.Printf("%s: %s\n", name, line)
			matches++
		}
	}
	if matches == 0 {
		fmt.Println("No matches.")
	}
	return nil
}

func latestFlag(cmd *cobra.Command) bool {
	latest, _ := cmd.Flags().GetBool("latest")
	return latest
}

func searchAgentLog(ctx context.Context, exec vmexec.Executor, name, query string, lines int, caseSensitive bool) ([]string, error) {
	agentType, _ := docker.InspectLabel(ctx, exec, name, labels.AgentType)
	configDir := "/home/agent/.claude"
	if agentType == "codex" {
		configDir = "/home/agent/.codex"
	}
	repoLabel, _ := docker.InspectLabel(ctx, exec, name, labels.RepoDisplay)
	searchDirs := sessionSearchDirs(configDir, repoLabel)
	running, _ := docker.IsRunning(ctx, exec, name)
	findCmd := fmt.Sprintf(
		"find %s/projects -name '*.jsonl' -not -path '*/subagents/*' -not -name 'history.jsonl' -type f -printf '%%T@ %%p\\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-",
		configDir)
	raw, err := readLatestSessionLog(ctx, exec, name, configDir, searchDirs, findCmd, lines, running)
	if err != nil {
		return nil, err
	}
	return matchRenderedLogLines(raw, query, caseSensitive), nil
}

func matchRenderedLogLines(raw []byte, query string, caseSensitive bool) []string {
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(needle)
	}
	var matches []string
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		rendered := renderLogEntry(line)
		if rendered == "" {
			continue
		}
		haystack := rendered
		if !caseSensitive {
			haystack = strings.ToLower(haystack)
		}
		if strings.Contains(haystack, needle) {
			matches = append(matches, rendered)
		}
	}
	return matches
}
