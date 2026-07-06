package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/0x666c6f/safe-agentic/pkg/docker"
	rc "github.com/0x666c6f/safe-agentic/pkg/reviewcomments"
	"github.com/spf13/cobra"
)

var reviewCommentsPath string
var reviewCommentsAll bool

var reviewCommentsCmd = &cobra.Command{
	Use:     "review-comments",
	Short:   "Manage local file/line review comments for agents",
	GroupID: groupWorkflow,
}

var reviewCommentsListCmd = &cobra.Command{
	Use:   "list [name|--latest]",
	Short: "List review comments",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReviewCommentsList,
}

var reviewCommentsAddCmd = &cobra.Command{
	Use:   "add [name|--latest] <file> <line> <body>",
	Short: "Add a review comment",
	Args:  cobra.RangeArgs(3, 4),
	RunE:  runReviewCommentsAdd,
}

var reviewCommentsResolveCmd = &cobra.Command{
	Use:   "resolve <id>",
	Short: "Mark a review comment resolved",
	Args:  cobra.ExactArgs(1),
	RunE:  runReviewCommentsResolve,
}

var reviewCommentsClearCmd = &cobra.Command{
	Use:   "clear <name|--latest>",
	Short: "Clear review comments for one agent",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runReviewCommentsClear,
}

func init() {
	reviewCommentsCmd.PersistentFlags().StringVar(&reviewCommentsPath, "file", "", "Review comments storage file")
	reviewCommentsListCmd.Flags().BoolVar(&reviewCommentsAll, "all", false, "Include resolved comments")
	addLatestFlag(reviewCommentsListCmd)
	addLatestFlag(reviewCommentsAddCmd)
	addLatestFlag(reviewCommentsClearCmd)
	reviewCommentsCmd.AddCommand(reviewCommentsListCmd, reviewCommentsAddCmd, reviewCommentsResolveCmd, reviewCommentsClearCmd)
	rootCmd.AddCommand(reviewCommentsCmd)
}

func reviewCommentStore() rc.Store {
	path := reviewCommentsPath
	if strings.TrimSpace(path) == "" {
		path = rc.DefaultPath()
	}
	return rc.Store{Path: path}
}

func runReviewCommentsList(cmd *cobra.Command, args []string) error {
	agent := ""
	if len(args) > 0 || latestFlag(cmd) {
		name, err := resolveReviewCommentTarget(cmd, args)
		if err != nil {
			return err
		}
		agent = name
	}
	comments, err := reviewCommentStore().List(rc.Filter{Agent: agent, IncludeResolved: reviewCommentsAll})
	if err != nil {
		return err
	}
	if len(comments) == 0 {
		fmt.Println("No review comments.")
		return nil
	}
	for _, comment := range comments {
		status := "open"
		if comment.Resolved {
			status = "resolved"
		}
		fmt.Printf("%s\t%s\t%s:%d\t%s\t%s\n", comment.ID, comment.Agent, comment.File, comment.Line, status, oneLine(comment.Body))
	}
	return nil
}

func runReviewCommentsAdd(cmd *cobra.Command, args []string) error {
	targetArgs := args[:0]
	fields := args
	if len(args) == 4 {
		targetArgs = []string{args[0]}
		fields = args[1:]
	}
	name, err := resolveReviewCommentTarget(cmd, targetArgs)
	if err != nil {
		return err
	}
	line, err := strconv.Atoi(fields[1])
	if err != nil {
		return fmt.Errorf("invalid line %q: %w", fields[1], err)
	}
	comment, err := reviewCommentStore().Add(rc.Comment{
		Agent: name,
		File:  fields[0],
		Line:  line,
		Body:  fields[2],
	})
	if err != nil {
		return err
	}
	fmt.Printf("Added %s for %s %s:%d\n", comment.ID, comment.Agent, comment.File, comment.Line)
	return nil
}

func runReviewCommentsResolve(cmd *cobra.Command, args []string) error {
	comment, err := reviewCommentStore().Resolve(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("Resolved %s\n", comment.ID)
	return nil
}

func runReviewCommentsClear(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && !latestFlag(cmd) {
		return fmt.Errorf("agent or --latest is required")
	}
	name, err := resolveReviewCommentTarget(cmd, args)
	if err != nil {
		return err
	}
	removed, err := reviewCommentStore().ClearAgent(name)
	if err != nil {
		return err
	}
	fmt.Printf("Cleared %d review comments for %s\n", removed, name)
	return nil
}

func resolveReviewCommentTarget(cmd *cobra.Command, args []string) (string, error) {
	ctx := context.Background()
	exec := newExecutor()
	return docker.ResolveTarget(ctx, exec, targetFromArgs(cmd, args))
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}
