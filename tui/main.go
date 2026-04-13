package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

const (
	vmName          = "safe-agentic"
	containerPrefix = "agent"
	pollInterval    = 2
)

func main() {
	if handleDashboardMode() {
		return
	}

	if handleHelpMode() {
		return
	}

	if err := preflight(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Ensure Ctrl+C always exits, even during loading or when tview doesn't handle it.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		os.Exit(0)
	}()

	app, err := runMainApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	execAfterExit(app)
}

func handleDashboardMode() bool {
	if !wantsDashboard(os.Args[1:]) {
		return false
	}
	if err := preflight(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	log.Fatal(NewDashboard(dashboardBind(os.Args[1:])).Start())
	return true
}

func wantsDashboard(args []string) bool {
	for _, arg := range args {
		if arg == "--dashboard" || arg == "dashboard" {
			return true
		}
	}
	return false
}

func dashboardBind(args []string) string {
	bind := "localhost:8420"
	for i, arg := range args {
		if arg == "--bind" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return bind
}

func handleHelpMode() bool {
	if len(os.Args) <= 1 || (os.Args[1] != "-h" && os.Args[1] != "--help") {
		return false
	}
	fmt.Println("Usage: safe-ag-tui [--dashboard [--bind host:port]]")
	fmt.Println()
	fmt.Println("Interactive terminal UI for monitoring and managing safe-agentic containers.")
	fmt.Println("  --dashboard    Start web dashboard instead of TUI")
	fmt.Println("  --bind         Bind address (default: localhost:8420)")
	fmt.Println()
	fmt.Println("Keybindings: a=attach s=stop l=logs d=describe n=new q=quit")
	os.Exit(0)
	return true
}

func runMainApp() (*App, error) {
	app := NewApp()
	if err := app.Run(); err != nil {
		return nil, err
	}
	return app, nil
}

func execAfterExit(app *App) {
	args := app.ExecAfterArgs()
	if len(args) == 0 {
		return
	}
	bin, err := exec.LookPath(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s not found: %v\n", args[0], err)
		os.Exit(1)
	}
	signal.Reset(syscall.SIGINT, syscall.SIGTERM)
	if err := syscall.Exec(bin, args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: exec failed: %v\n", err)
		os.Exit(1)
	}
}

func preflight() error {
	if _, err := exec.LookPath("orb"); err != nil {
		return fmt.Errorf("'orb' not found in PATH. Install OrbStack first")
	}
	out, err := exec.Command("orb", "list").Output()
	if err != nil {
		return fmt.Errorf("failed to list VMs: %w", err)
	}
	if !strings.Contains(string(out), vmName) {
		return fmt.Errorf("VM '%s' not found. Run 'safe-ag setup' first", vmName)
	}
	return nil
}
