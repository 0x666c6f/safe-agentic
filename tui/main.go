package main

import (
	"fmt"
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
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Println("Usage: agent tui")
		fmt.Println()
		fmt.Println("Interactive terminal UI for monitoring and managing safe-agentic containers.")
		fmt.Println("Keybindings: a=attach s=stop l=logs d=describe n=new q=quit")
		os.Exit(0)
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

	app := NewApp()
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// If an action scheduled an exec-after-exit (resume, spawn), run it now
	// that tview has fully restored the terminal.
	if args := app.ExecAfterArgs(); len(args) > 0 {
		bin, err := exec.LookPath(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s not found: %v\n", args[0], err)
			os.Exit(1)
		}
		// Stop the signal handler so the child process gets signals directly
		signal.Reset(syscall.SIGINT, syscall.SIGTERM)
		if err := syscall.Exec(bin, args, os.Environ()); err != nil {
			fmt.Fprintf(os.Stderr, "Error: exec failed: %v\n", err)
			os.Exit(1)
		}
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
		return fmt.Errorf("VM '%s' not found. Run 'agent setup' first", vmName)
	}
	return nil
}
