package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

const (
	containerPrefix = "agent"
	pollInterval    = 2
)

var vmName = configuredVMName()

func configuredVMName() string {
	if v := os.Getenv("SAFE_AGENTIC_VM_NAME"); v != "" {
		return v
	}
	return "safe-agentic"
}

func main() {
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

func handleHelpMode() bool {
	if len(os.Args) <= 1 || (os.Args[1] != "-h" && os.Args[1] != "--help") {
		return false
	}
	fmt.Println("Usage: safe-ag-tui")
	fmt.Println()
	fmt.Println("Interactive terminal UI for monitoring and managing safe-agentic containers.")
	fmt.Println()
	fmt.Println(helpText())
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
	if _, err := exec.LookPath("container"); err != nil {
		return fmt.Errorf("'container' not found in PATH. Install Apple container first")
	}
	if err := exec.Command("container", "system", "status").Run(); err != nil {
		return fmt.Errorf("Apple container system is stopped. Run 'safe-ag vm start'")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "container", "machine", "list", "--format", "json").Output()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out listing VMs. Check Apple container and run 'safe-ag vm start'")
	}
	if err != nil {
		return fmt.Errorf("failed to list VMs: %w", err)
	}
	var machines []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &machines); err != nil {
		return fmt.Errorf("failed to parse VM list: %w", err)
	}
	for _, machine := range machines {
		if machine.ID == configuredVMName() {
			return nil
		}
	}
	return fmt.Errorf("VM '%s' not found. Run 'safe-ag setup' first", configuredVMName())
}
