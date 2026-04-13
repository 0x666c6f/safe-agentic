package main

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

func configuredVMName() string {
	if v := os.Getenv("SAFE_AGENTIC_VM_NAME"); v != "" {
		return v
	}
	return "safe-agentic"
}

var (
	copyFileToVM          = copyFileToVMImpl
	syncBuildContextToVM  = syncBuildContextToVMImpl
	findBuildRoot         = findBuildRootImpl
	vmExists              = vmExistsImpl
	runVMBootstrap        = runVMBootstrapImpl
	startVM               = startVMImpl
	installVMSupportFiles = installVMSupportFilesImpl
)

func vmExistsImpl(vmName string) bool {
	out, err := exec.Command("orbctl", "list", "-q").Output()
	if err != nil {
		out, err = exec.Command("orb", "list", "-q").Output()
		if err != nil {
			return false
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == vmName {
			return true
		}
	}
	return false
}

func startVMImpl(vmName string) error {
	start := exec.Command("orb", "start", vmName)
	start.Stdout = os.Stdout
	start.Stderr = os.Stderr
	return start.Run()
}

func runVMBootstrapImpl(vmName string) ([]byte, error) {
	return exec.Command("orb", "run", "-m", vmName, "bash", "/tmp/setup.sh").CombinedOutput()
}

func installVMSupportFilesImpl(vmName, buildRoot string) error {
	seccompSrc := filepath.Join(buildRoot, "config", "seccomp.json")
	if err := copyFileToVM(vmName, seccompSrc, "/tmp/seccomp.json"); err != nil {
		return err
	}
	cmd := exec.Command("orb", "run", "-m", vmName, "sh", "-lc",
		"sudo mkdir -p /etc/safe-agentic && sudo cp /tmp/seccomp.json /etc/safe-agentic/seccomp.json && sudo chmod 0644 /etc/safe-agentic/seccomp.json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install VM support files: %w\n%s", err, string(out))
	}
	return nil
}

func findBuildRootImpl() (string, error) {
	var candidates []string
	seen := map[string]bool{}

	addCandidate := func(dir string) {
		if dir == "" || seen[dir] {
			return
		}
		seen[dir] = true
		candidates = append(candidates, dir)
	}

	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			addCandidate(dir)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		addCandidate(filepath.Dir(exeDir))
		addCandidate(filepath.Dir(filepath.Dir(exeDir)))
	}

	for _, dir := range candidates {
		if fileExists(filepath.Join(dir, "Dockerfile")) &&
			fileExists(filepath.Join(dir, "entrypoint.sh")) &&
			fileExists(filepath.Join(dir, "vm", "setup.sh")) {
			return dir, nil
		}
	}

	return "", fmt.Errorf("build root not found (expected Dockerfile, entrypoint.sh, vm/setup.sh)")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFileToVMImpl(vmName, srcPath, destPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcPath, err)
	}

	destDir := filepath.Dir(destPath)
	cmd := exec.Command("orb", "run", "-m", vmName, "sh", "-lc",
		fmt.Sprintf("mkdir -p %s && cat > %s && chmod +x %s",
			shellQuote(destDir), shellQuote(destPath), shellQuote(destPath)))
	cmd.Stdin = strings.NewReader(string(data))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("copy %s to %s: %w\n%s", srcPath, destPath, err, string(out))
	}
	return nil
}

func syncBuildContextToVMImpl(vmName, root string) error {
	cmd := exec.Command("orb", "run", "-m", vmName, "sh", "-lc",
		"rm -rf /tmp/build-context && mkdir -p /tmp/build-context && tar xf - -C /tmp/build-context")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start context sync: %w", err)
	}

	writeErrCh := make(chan error, 1)
	go func() {
		tw := tar.NewWriter(stdin)
		writeErrCh <- writeBuildContextArchive(tw, root)
		_ = stdin.Close()
	}()

	waitErr := cmd.Wait()
	writeErr := <-writeErrCh
	if writeErr != nil {
		return writeErr
	}
	if waitErr != nil {
		return fmt.Errorf("sync build context: %w", waitErr)
	}
	return nil
}

func writeBuildContextArchive(tw *tar.Writer, root string) error {
	defer tw.Close()

	files, err := buildContextFiles(root)
	if err != nil {
		return err
	}

	for _, rel := range files {
		abs := filepath.Join(root, rel)
		info, err := os.Lstat(abs)
		if err != nil {
			return fmt.Errorf("stat %s: %w", abs, err)
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("header %s: %w", abs, err)
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(abs)
			if err != nil {
				return fmt.Errorf("readlink %s: %w", abs, err)
			}
			hdr.Linkname = target
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write header %s: %w", rel, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(abs)
		if err != nil {
			return fmt.Errorf("open %s: %w", abs, err)
		}
		if _, err := io.Copy(tw, f); err != nil {
			_ = f.Close()
			return fmt.Errorf("copy %s: %w", abs, err)
		}
		_ = f.Close()
	}

	return nil
}

func buildContextFiles(root string) ([]string, error) {
	git := exec.Command("git", "-C", root, "ls-files", "-c", "-z")
	out, err := git.Output()
	if err == nil && len(out) > 0 {
		var files []string
		for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
			if rel == "" {
				continue
			}
			if fileExists(filepath.Join(root, rel)) {
				files = append(files, rel)
			}
		}
		sort.Strings(files)
		return files, nil
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.HasPrefix(rel, ".git") || strings.HasPrefix(rel, "site") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

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
	vmName := configuredVMName()
	buildRoot, err := findBuildRoot()
	if err != nil {
		return err
	}
	// Step 1: Check if orb is installed.
	if _, err := exec.LookPath("orb"); err != nil {
		return fmt.Errorf("orb not found in PATH: install OrbStack from https://orbstack.dev")
	}
	fmt.Println("✓ orb found")

	ctx := context.Background()

	// Step 2: Check if VM exists (orb list gives a line per VM).
	vmExists := vmExists(vmName)

	// Step 3: Create VM if needed.
	if !vmExists {
		fmt.Printf("Creating VM %s (ubuntu:noble)…\n", vmName)
		create := exec.Command("orb", "create", "ubuntu:noble", vmName)
		create.Stdout = cmd.OutOrStdout()
		create.Stderr = cmd.ErrOrStderr()
		if err := create.Run(); err != nil {
			return fmt.Errorf("create VM: %w", err)
		}
		fmt.Println("✓ VM created")

		// Give the VM a moment to boot.
		time.Sleep(3 * time.Second)
	} else {
		fmt.Printf("✓ VM %s already exists\n", vmName)
	}

	orbRunner := newExecutor()

	// Step 4: Copy and run setup script automatically.
	fmt.Println("Bootstrapping VM…")
	if err := copyFileToVM(vmName, filepath.Join(buildRoot, "vm", "setup.sh"), "/tmp/setup.sh"); err != nil {
		return err
	}
	setupOut, err := runVMBootstrap(vmName)
	if err != nil {
		return fmt.Errorf("run VM bootstrap: %w\n%s", err, string(setupOut))
	}
	fmt.Print(string(setupOut))
	if err := installVMSupportFiles(vmName, buildRoot); err != nil {
		return err
	}
	fmt.Println("✓ VM support files installed")

	// Step 5: Verify Docker.
	if _, err := orbRunner.Run(ctx, "docker", "info"); err != nil {
		return fmt.Errorf("docker not available after bootstrap: %w", err)
	}

	// Step 6: Sync build context and build image.
	if err := syncBuildContextToVM(vmName, buildRoot); err != nil {
		return err
	}
	fmt.Println("✓ Build context synced")
	if err := runDockerBuild(ctx, orbRunner); err != nil {
		return err
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
	vmName := configuredVMName()
	buildRoot, err := findBuildRoot()
	if err != nil {
		return err
	}

	if err := syncBuildContextToVM(vmName, buildRoot); err != nil {
		return err
	}
	fmt.Println("✓ Build context synced")
	return runDockerBuild(ctx, orbRunner)
}

func runDockerBuild(ctx context.Context, orbRunner interface {
	Run(context.Context, ...string) ([]byte, error)
}) error {
	buildArgs := []string{"docker", "build", "-t", "safe-agentic:latest", "/tmp/build-context"}

	switch {
	case updateFull:
		buildArgs = []string{"docker", "build", "--no-cache", "-t", "safe-agentic:latest", "/tmp/build-context"}
	case updateQuick:
		cacheBust := fmt.Sprintf("%d", time.Now().Unix())
		buildArgs = []string{"docker", "build", "--build-arg", "CACHEBUST=" + cacheBust, "-t", "safe-agentic:latest", "/tmp/build-context"}
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
	vmName := configuredVMName()
	buildRoot, err := findBuildRoot()
	if err != nil {
		return err
	}
	fmt.Printf("Starting VM %s…\n", vmName)
	if err := startVM(vmName); err != nil {
		return fmt.Errorf("orb start: %w", err)
	}
	fmt.Println("✓ VM started")
	fmt.Println("Re-applying VM hardening…")
	if err := copyFileToVM(vmName, filepath.Join(buildRoot, "vm", "setup.sh"), "/tmp/setup.sh"); err != nil {
		return err
	}
	setupOut, err := runVMBootstrap(vmName)
	if err != nil {
		return fmt.Errorf("run VM bootstrap: %w\n%s", err, string(setupOut))
	}
	fmt.Print(string(setupOut))
	if err := installVMSupportFiles(vmName, buildRoot); err != nil {
		return err
	}
	fmt.Println("✓ VM support files installed")
	return nil
}

func runVMStop(cmd *cobra.Command, args []string) error {
	vmName := configuredVMName()
	fmt.Printf("Stopping VM %s…\n", vmName)
	stop := exec.Command("orb", "stop", vmName)
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
	rootCmd.AddCommand(tuiCmd)
	dashboardCmd.Flags().StringVar(&dashboardBind, "bind", "localhost:8420", "Bind address for web dashboard")
	rootCmd.AddCommand(dashboardCmd)
}

// ─── tui / dashboard ──────────────────────────────────────────────────────

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive TUI dashboard",
	RunE:  runTUI,
}

var dashboardBind string

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Start web dashboard",
	RunE:  runDashboard,
}

func findTUIBinary() (string, error) {
	// Check next to safe-ag binary first
	self, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(self), "safe-ag-tui")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	// Fall back to PATH
	return exec.LookPath("safe-ag-tui")
}

func runTUI(cmd *cobra.Command, args []string) error {
	bin, err := findTUIBinary()
	if err != nil {
		return fmt.Errorf("safe-ag-tui not found. Build with: make build-tui")
	}
	return syscall.Exec(bin, append([]string{bin}, args...), os.Environ())
}

func runDashboard(cmd *cobra.Command, args []string) error {
	bin, err := findTUIBinary()
	if err != nil {
		return fmt.Errorf("safe-ag-tui not found. Build with: make build-tui")
	}
	return syscall.Exec(bin, []string{bin, "--dashboard", "--bind", dashboardBind}, os.Environ())
}

func runDiagnose(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	vmName := configuredVMName()

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
			if strings.Contains(line, vmName) {
				vmExists = true
				break
			}
		}
	}
	check("VM "+vmName+" exists", vmExists, "")

	if !vmExists {
		fmt.Println()
		fmt.Println("Run: safe-ag setup")
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
		fmt.Println("Some checks failed. Run: safe-ag setup")
	}

	return nil
}
