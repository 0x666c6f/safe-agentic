package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Version = "dev"

var rootCmd = &cobra.Command{
	Use:   "safe-ag",
	Short: "Isolated environment for running AI coding agents",
	Long:  "Sandboxed AI agent environment with per-agent Docker containers in an OrbStack VM.",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

func init() {
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("safe-agentic v{{.Version}}\n")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
