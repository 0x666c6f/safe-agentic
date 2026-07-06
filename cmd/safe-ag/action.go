package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	actionpkg "github.com/0x666c6f/safe-agentic/pkg/actions"
	"github.com/0x666c6f/safe-agentic/pkg/docker"
	"github.com/spf13/cobra"
)

var actionFiles []string

var actionCmd = &cobra.Command{
	Use:     "action",
	Short:   "Run configured project or user actions inside an agent",
	GroupID: groupManage,
}

var actionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured actions",
	Args:  cobra.NoArgs,
	RunE:  runActionList,
}

var actionShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show one configured action",
	Args:  cobra.ExactArgs(1),
	RunE:  runActionShow,
}

var actionRunCmd = &cobra.Command{
	Use:   "run <action> [name|--latest]",
	Short: "Run a configured action inside an agent workspace",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runActionRun,
}

func init() {
	actionCmd.PersistentFlags().StringArrayVar(&actionFiles, "file", nil, "Additional actions.toml file; repeatable")
	addLatestFlag(actionRunCmd)
	actionCmd.AddCommand(actionListCmd, actionShowCmd, actionRunCmd)
	rootCmd.AddCommand(actionCmd)
}

func loadActionCatalog() (actionpkg.Catalog, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return actionpkg.Catalog{}, fmt.Errorf("get cwd: %w", err)
	}
	paths := actionpkg.DefaultPaths(cwd)
	paths = append(paths, actionFiles...)
	return actionpkg.LoadFiles(paths)
}

func runActionList(cmd *cobra.Command, args []string) error {
	catalog, err := loadActionCatalog()
	if err != nil {
		return err
	}
	if len(catalog.Actions) == 0 {
		fmt.Println("No actions configured.")
		fmt.Printf("Create %s or .safe-ag/actions.toml.\n", actionpkg.UserPath())
		return nil
	}
	for _, action := range catalog.Actions {
		desc := action.Description
		if desc == "" {
			desc = action.Command
		}
		fmt.Printf("%-20s %s\n", action.Name, desc)
	}
	return nil
}

func runActionShow(cmd *cobra.Command, args []string) error {
	catalog, err := loadActionCatalog()
	if err != nil {
		return err
	}
	action, ok := catalog.Get(args[0])
	if !ok {
		return fmt.Errorf("action %q not found", args[0])
	}
	fmt.Printf("name: %s\n", action.Name)
	if action.Description != "" {
		fmt.Printf("description: %s\n", action.Description)
	}
	if action.CWD != "" {
		fmt.Printf("cwd: %s\n", action.CWD)
	}
	fmt.Printf("command: %s\n", action.Command)
	fmt.Printf("source: %s\n", action.Source)
	return nil
}

func runActionRun(cmd *cobra.Command, args []string) error {
	catalog, err := loadActionCatalog()
	if err != nil {
		return err
	}
	action, ok := catalog.Get(args[0])
	if !ok {
		return fmt.Errorf("action %q not found", args[0])
	}

	targetArgs := []string{}
	if len(args) > 1 {
		targetArgs = append(targetArgs, args[1])
	}

	ctx := context.Background()
	exec := newExecutor()
	name, err := docker.ResolveTarget(ctx, exec, targetFromArgs(cmd, targetArgs))
	if err != nil {
		return err
	}

	command := action.Command
	if action.CWD != "" {
		command = "cd " + shellQuote(cleanActionCWD(action.CWD)) + " && " + command
	}
	out, err := exec.Run(ctx, workspaceExecCommand(name, "bash", "-lc", command)...)
	if err != nil {
		return fmt.Errorf("run action %q in %s: %w", action.Name, name, err)
	}
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	return nil
}

func cleanActionCWD(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "."
	}
	return filepath.Clean(cwd)
}
