package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ─── setup ─────────────────────────────────────────────────────────────────

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Initialize VM and build Docker image",
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	// Step 1: Check if orb is installed.
	if _, err := exec.LookPath("orb"); err != nil {
		return fmt.Errorf("orb not found in PATH: install OrbStack from https://orbstack.dev")
	}
	fmt.Println("✓ orb found")

	ctx := context.Background()

	// Step 2: Check if VM exists (orb list gives a line per VM).
	vmExists := false
	orbExec := exec.Command("orb", "list")
	if out, err := orbExec.Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "safe-agentic") {
				vmExists = true
				break
			}
		}
	}

	// Step 3: Create VM if needed.
	if !vmExists {
		fmt.Println("Creating VM safe-agentic (ubuntu:noble)…")
		create := exec.Command("orb", "create", "ubuntu:noble", "safe-agentic")
		create.Stdout = cmd.OutOrStdout()
		create.Stderr = cmd.ErrOrStderr()
		if err := create.Run(); err != nil {
			return fmt.Errorf("create VM: %w", err)
		}
		fmt.Println("✓ VM created")

		// Give the VM a moment to boot.
		time.Sleep(3 * time.Second)
	} else {
		fmt.Println("✓ VM safe-agentic already exists")
	}

	orbRunner := newExecutor()

	// Step 4: Copy and run setup script.
	fmt.Println()
	fmt.Println("To complete setup, run the VM bootstrap script manually:")
	fmt.Println()
	fmt.Println("  orb push -m safe-agentic vm/setup.sh /tmp/setup.sh")
	fmt.Println("  orb run -m safe-agentic bash /tmp/setup.sh")
	fmt.Println()

	// Attempt a quick check: is Docker already running in the VM?
	if _, err := orbRunner.Run(ctx, "docker", "info"); err == nil {
		fmt.Println("✓ Docker is already running in VM")

		// Step 5: Build Docker image.
		fmt.Println("Building safe-agentic Docker image…")
		buildOut, buildErr := orbRunner.Run(ctx, "docker", "build", "-t", "safe-agentic:latest", "/tmp/build-context/")
		if buildErr != nil {
			fmt.Println("Could not build image automatically.")
			fmt.Println("Copy build context first:")
			fmt.Println("  orb push -m safe-agentic . /tmp/build-context/")
			fmt.Println("Then run: agent update")
		} else {
			fmt.Print(string(buildOut))
			fmt.Println("✓ Image built")
		}
	} else {
		fmt.Println("Docker not yet available in VM — run the bootstrap script above, then:")
		fmt.Println("  orb push -m safe-agentic . /tmp/build-context/")
		fmt.Println("  agent update")
	}

	return nil
}

// ─── update ────────────────────────────────────────────────────────────────

var (
	updateQuick bool
	updateFull  bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Rebuild Docker image",
	RunE:  runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updateQuick, "quick", false, "Bust only the AI CLI layer")
	updateCmd.Flags().BoolVar(&updateFull, "full", false, "Full rebuild (no cache)")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	orbRunner := newExecutor()

	buildArgs := []string{"docker", "build", "-t", "safe-agentic:latest", "."}

	switch {
	case updateFull:
		buildArgs = []string{"docker", "build", "--no-cache", "-t", "safe-agentic:latest", "."}
	case updateQuick:
		cacheBust := fmt.Sprintf("%d", time.Now().Unix())
		buildArgs = []string{"docker", "build", "--build-arg", "CACHEBUST=" + cacheBust, "-t", "safe-agentic:latest", "."}
	}

	fmt.Println("Building safe-agentic:latest…")
	out, err := orbRunner.Run(ctx, buildArgs...)
	if err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	fmt.Print(string(out))
	fmt.Println("✓ Image updated")
	return nil
}

// ─── vm ────────────────────────────────────────────────────────────────────

var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "Manage the OrbStack VM",
}

var vmSSHCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Open an interactive shell in the VM",
	RunE:  runVMSSH,
}

var vmStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the VM (and re-harden)",
	RunE:  runVMStart,
}

var vmStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the VM",
	RunE:  runVMStop,
}

func init() {
	vmCmd.AddCommand(vmSSHCmd)
	vmCmd.AddCommand(vmStartCmd)
	vmCmd.AddCommand(vmStopCmd)
	rootCmd.AddCommand(vmCmd)
}

func runVMSSH(cmd *cobra.Command, args []string) error {
	orbRunner := newExecutor()
	return orbRunner.RunInteractive()
}

func runVMStart(cmd *cobra.Command, args []string) error {
	fmt.Println("Starting VM safe-agentic…")
	start := exec.Command("orb", "start", "safe-agentic")
	start.Stdout = cmd.OutOrStdout()
	start.Stderr = cmd.ErrOrStderr()
	if err := start.Run(); err != nil {
		return fmt.Errorf("orb start: %w", err)
	}
	fmt.Println("✓ VM started")
	fmt.Println("Tip: run setup script to re-harden: orb run -m safe-agentic bash /tmp/setup.sh")
	return nil
}

func runVMStop(cmd *cobra.Command, args []string) error {
	fmt.Println("Stopping VM safe-agentic…")
	stop := exec.Command("orb", "stop", "safe-agentic")
	stop.Stdout = cmd.OutOrStdout()
	stop.Stderr = cmd.ErrOrStderr()
	if err := stop.Run(); err != nil {
		return fmt.Errorf("orb stop: %w", err)
	}
	fmt.Println("✓ VM stopped")
	return nil
}

// ─── diagnose ──────────────────────────────────────────────────────────────

var diagnoseCmd = &cobra.Command{
	Use:   "diagnose",
	Short: "Check environment health",
	RunE:  runDiagnose,
}

func init() {
	rootCmd.AddCommand(diagnoseCmd)
}

func runDiagnose(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	check := func(label string, ok bool, detail string) {
		icon := "✓"
		if !ok {
			icon = "✗"
		}
		if detail != "" {
			fmt.Printf("  %s %s — %s\n", icon, label, detail)
		} else {
			fmt.Printf("  %s %s\n", icon, label)
		}
	}

	fmt.Println("safe-agentic diagnostics")
	fmt.Println("─────────────────────────")

	// 1. Check orb installed.
	_, orbErr := exec.LookPath("orb")
	check("orb installed", orbErr == nil, "")

	if orbErr != nil {
		fmt.Println()
		fmt.Println("Install OrbStack: https://orbstack.dev")
		return nil
	}

	// 2. Check VM exists.
	vmExists := false
	if out, err := exec.Command("orb", "list").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "safe-agentic") {
				vmExists = true
				break
			}
		}
	}
	check("VM safe-agentic exists", vmExists, "")

	if !vmExists {
		fmt.Println()
		fmt.Println("Run: agent setup")
		return nil
	}

	orbRunner := newExecutor()

	// 3. Check Docker running in VM.
	_, dockerErr := orbRunner.Run(ctx, "docker", "info")
	check("Docker running in VM", dockerErr == nil, "")

	// 4. Check image exists.
	imgOut, imgErr := orbRunner.Run(ctx, "docker", "images", "safe-agentic:latest", "-q")
	imageExists := imgErr == nil && strings.TrimSpace(string(imgOut)) != ""
	imageDetail := ""
	if imageExists {
		imageDetail = strings.TrimSpace(string(imgOut))
	}
	check("Docker image safe-agentic:latest", imageExists, imageDetail)

	fmt.Println()
	if orbErr == nil && vmExists && dockerErr == nil && imageExists {
		fmt.Println("All checks passed. Environment is ready.")
	} else {
		fmt.Println("Some checks failed. Run: agent setup")
	}

	return nil
}
