package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/spf13/cobra"
)

func configuredVMName() string {
	if v := os.Getenv("SAFE_AGENTIC_VM_NAME"); v != "" {
		return v
	}
	return "safe-agentic"
}

var (
	copyFileToVM            = copyFileToVMImpl
	syncBuildContextToVM    = syncBuildContextToVMImpl
	findBuildRoot           = findBuildRootImpl
	vmExists                = vmExistsImpl
	runVMBootstrap          = runVMBootstrapImpl
	startVM                 = startVMImpl
	installVMSupportFiles   = installVMSupportFilesImpl
	configureHostNAT        = configureHostNATImpl
	hostIPForwardingEnabled = hostIPForwardingEnabledImpl
	configureLaunchdSSHAuth = configureLaunchdSSHAuthImpl
)

func vmExistsImpl(vmName string) bool {
	out, err := exec.Command("container", "machine", "list", "--format", "json").Output()
	if err != nil {
		return false
	}
	var machines []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &machines); err == nil {
		for _, machine := range machines {
			if machine.ID == vmName {
				return true
			}
		}
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, vmName) {
			return true
		}
	}
	return false
}

func startVMImpl(vmName string) error {
	start := exec.Command("container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/true")
	start.Stdout = os.Stdout
	start.Stderr = os.Stderr
	return start.Run()
}

func runVMBootstrapImpl(vmName string) ([]byte, error) {
	return exec.Command("container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/sh", "/tmp/setup.sh").CombinedOutput()
}

func installVMSupportFilesImpl(vmName, buildRoot string) error {
	seccompSrc := filepath.Join(buildRoot, "config", "seccomp.json")
	if err := copyFileToVM(vmName, seccompSrc, "/tmp/seccomp.json"); err != nil {
		return err
	}
	commands := [][]string{
		{"container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/mkdir", "-p", "/etc/safe-agentic"},
		{"container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/cp", "/tmp/seccomp.json", "/etc/safe-agentic/seccomp.json"},
		{"container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/chmod", "0644", "/etc/safe-agentic/seccomp.json"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("install VM support files: %w\n%s", err, string(out))
		}
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
	mkdir := exec.Command("container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/mkdir", "-p", destDir)
	if out, err := mkdir.CombinedOutput(); err != nil {
		return fmt.Errorf("create %s in VM: %w\n%s", destDir, err, string(out))
	}
	cmd := exec.Command("container", "machine", "run", "-i", "-n", vmName, "-u", "root", "--", "/usr/bin/tee", destPath)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copy %s to %s: %w\n%s", srcPath, destPath, err, stderr.String())
	}
	chmod := exec.Command("container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/chmod", "+x", destPath)
	if out, err := chmod.CombinedOutput(); err != nil {
		return fmt.Errorf("chmod %s in VM: %w\n%s", destPath, err, string(out))
	}
	return nil
}

func syncBuildContextToVMImpl(vmName, root string) error {
	if out, err := exec.Command("container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/rm", "-rf", "/tmp/build-context").CombinedOutput(); err != nil {
		return fmt.Errorf("clear build context: %w\n%s", err, string(out))
	}
	if out, err := exec.Command("container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/mkdir", "-p", "/tmp/build-context").CombinedOutput(); err != nil {
		return fmt.Errorf("create build context: %w\n%s", err, string(out))
	}
	cmd := exec.Command("container", "machine", "run", "-i", "-n", vmName, "-u", "root", "--", "/bin/tar", "xf", "-", "-C", "/tmp/build-context")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
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
		return fmt.Errorf("sync build context: %w\n%s", waitErr, stderr.String())
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

func ensureContainerSystemRunning(stdout, stderr io.Writer) error {
	if err := configureLaunchdSSHAuth(); err != nil {
		return err
	}
	status := exec.Command("container", "system", "status")
	if err := status.Run(); err == nil {
		return nil
	}
	start := exec.Command("container", "system", "start")
	start.Stdin = strings.NewReader("y\n")
	start.Stdout = stdout
	start.Stderr = stderr
	if err := start.Run(); err != nil {
		return fmt.Errorf("container system start: %w", err)
	}
	return nil
}

func configureLaunchdSSHAuthImpl() error {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" || !filepath.IsAbs(sock) || strings.ContainsAny(sock, "\x00\n\r") {
		return nil
	}
	info, err := os.Lstat(sock)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return nil
	}
	if _, err := exec.LookPath("launchctl"); err != nil {
		return nil
	}
	if err := exec.Command("launchctl", "setenv", "SSH_AUTH_SOCK", sock).Run(); err != nil {
		return fmt.Errorf("set launchd SSH_AUTH_SOCK: %w", err)
	}
	return nil
}

func configureHostNATImpl(stdout, stderr io.Writer) error {
	subnets, err := appleContainerNATSubnets()
	if err != nil {
		return err
	}
	subnets = append(subnets, "172.20.0.0/16")
	iface, err := defaultHostInterface()
	if err != nil {
		return err
	}
	var rules []string
	seen := map[string]bool{}
	for _, subnet := range subnets {
		if seen[subnet] {
			continue
		}
		seen[subnet] = true
		rules = append(rules, fmt.Sprintf("nat on %s from %s to any -> (%s)", iface, subnet, iface))
	}
	script := strings.Join([]string{
		"/usr/sbin/sysctl -w net.inet.ip.forwarding=1 >/dev/null",
		"printf " + shellSingleQuote(strings.Join(rules, "\n")+"\n") + " | /sbin/pfctl -a com.apple/safe-agentic -f -",
		"{ /sbin/pfctl -E >/dev/null 2>&1 || true; }",
	}, " && ")
	if err := exec.Command("sh", "-c", script).Run(); err == nil {
		return nil
	}
	osaCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	osa := exec.CommandContext(osaCtx, "osascript", "-e", "do shell script "+appleScriptQuote(script)+" with administrator privileges")
	osa.Stdout = stdout
	osa.Stderr = stderr
	if err := osa.Run(); err != nil {
		if osaCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("configure Apple container host NAT: admin prompt timed out; run: sudo sh -c %s", shellSingleQuote(script))
		}
		return fmt.Errorf("configure Apple container host NAT: %w", err)
	}
	return nil
}

func appleContainerNATSubnets() ([]string, error) {
	out, err := exec.Command("container", "network", "list", "--format", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("list Apple container networks: %w", err)
	}
	return parseAppleContainerNATSubnets(out)
}

func parseAppleContainerNATSubnets(out []byte) ([]string, error) {
	var networks []struct {
		Configuration struct {
			Mode       string `json:"mode"`
			IPv4Subnet string `json:"ipv4Subnet"`
		} `json:"configuration"`
		Status struct {
			IPv4Subnet string `json:"ipv4Subnet"`
		} `json:"status"`
	}
	if err := json.Unmarshal(out, &networks); err != nil {
		return nil, fmt.Errorf("parse Apple container networks: %w", err)
	}
	seen := map[string]bool{}
	var subnets []string
	for _, network := range networks {
		if network.Configuration.Mode != "nat" {
			continue
		}
		subnet := network.Status.IPv4Subnet
		if subnet == "" {
			subnet = network.Configuration.IPv4Subnet
		}
		if subnet == "" || seen[subnet] {
			continue
		}
		seen[subnet] = true
		subnets = append(subnets, subnet)
	}
	return subnets, nil
}

func defaultHostInterface() (string, error) {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err != nil {
		return "", fmt.Errorf("detect default host interface: %w", err)
	}
	return parseDefaultHostInterface(out)
}

func parseDefaultHostInterface(out []byte) (string, error) {
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "interface:" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("detect default host interface: no interface in route output")
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func appleScriptQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
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
	// Step 1: Check if Apple container is installed and running.
	if _, err := exec.LookPath("container"); err != nil {
		return fmt.Errorf("container not found in PATH: install Apple container from https://github.com/apple/container")
	}
	fmt.Println("✓ container found")
	if err := ensureContainerSystemRunning(cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return err
	}
	fmt.Println("✓ container system running")

	ctx := context.Background()

	// Step 2: Check if Apple container machine exists.
	vmExists := vmExists(vmName)

	// Step 3: Create VM if needed.
	if !vmExists {
		fmt.Printf("Creating VM %s (alpine:3.22)…\n", vmName)
		create := exec.Command("container", "machine", "create", "alpine:3.22", "--name", vmName, "--cpus", "4", "--memory", "8G", "--home-mount", "none")
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
		if err := startVM(vmName); err != nil {
			return fmt.Errorf("start VM: %w", err)
		}
	}
	if err := configureHostNAT(cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return err
	}
	fmt.Println("✓ Apple container host NAT configured")

	vmRunner := newExecutor()

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
	if _, err := vmRunner.Run(ctx, "docker", "info"); err != nil {
		return fmt.Errorf("docker not available after bootstrap: %w", err)
	}

	// Step 6: Sync build context and build image.
	if err := syncBuildContextToVM(vmName, buildRoot); err != nil {
		return err
	}
	fmt.Println("✓ Build context synced")
	if err := runDockerBuild(ctx, vmRunner); err != nil {
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
	vmRunner := newExecutor()
	vmName := configuredVMName()
	buildRoot, err := findBuildRoot()
	if err != nil {
		return err
	}

	if err := syncBuildContextToVM(vmName, buildRoot); err != nil {
		return err
	}
	fmt.Println("✓ Build context synced")
	return runDockerBuild(ctx, vmRunner)
}

func runDockerBuild(ctx context.Context, vmRunner interface {
	Run(context.Context, ...string) ([]byte, error)
}) error {
	buildArgs := []string{"docker", "build", "-t", "safe-agentic:latest", "/tmp/build-context"}

	switch {
	case updateFull:
		buildArgs = []string{"docker", "build", "--no-cache", "-t", "safe-agentic:latest", "/tmp/build-context"}
	case updateQuick:
		cacheBust := fmt.Sprintf("%d", time.Now().Unix())
		buildArgs = []string{"docker", "build", "--build-arg", "CLI_CACHE_BUST=" + cacheBust, "-t", "safe-agentic:latest", "/tmp/build-context"}
	}

	fmt.Println("Building safe-agentic:latest…")
	out, err := vmRunner.Run(ctx, buildArgs...)
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
	Short: "Manage the Apple container machine",
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
	vmRunner := newExecutor()
	return vmRunner.RunInteractive()
}

func runVMStart(cmd *cobra.Command, args []string) error {
	vmName := configuredVMName()
	buildRoot, err := findBuildRoot()
	if err != nil {
		return err
	}
	fmt.Printf("Starting VM %s…\n", vmName)
	if err := startVM(vmName); err != nil {
		return fmt.Errorf("start VM: %w", err)
	}
	fmt.Println("✓ VM started")
	// A reboot resets net.inet.ip.forwarding and flushes the pf NAT anchor, so
	// restore egress before bootstrap tries package/network operations.
	fmt.Println("Restoring host network egress (NAT)…")
	if err := configureHostNAT(cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
		return fmt.Errorf("restore host NAT: %w", err)
	}
	fmt.Println("✓ Host NAT applied")
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
	stop := exec.Command("container", "machine", "stop", vmName)
	stop.Stdout = cmd.OutOrStdout()
	stop.Stderr = cmd.ErrOrStderr()
	if err := stop.Run(); err != nil {
		return fmt.Errorf("stop VM: %w", err)
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
}

// ─── tui ──────────────────────────────────────────────────────────────

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive TUI",
	RunE:  runTUI,
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

	// 1. Check Apple container installed and running.
	_, containerErr := exec.LookPath("container")
	check("container installed", containerErr == nil, "")

	if containerErr != nil {
		fmt.Println()
		fmt.Println("Install Apple container: https://github.com/apple/container")
		return nil
	}

	systemErr := exec.Command("container", "system", "status").Run()
	check("container system running", systemErr == nil, "")

	// 2. Check VM exists.
	vmExists := vmExists(vmName)
	check("VM "+vmName+" exists", vmExists, "")

	if !vmExists {
		fmt.Println()
		fmt.Println("Run: safe-ag setup")
		return nil
	}

	vmRunner := newExecutor()

	// 3. Check Docker running in VM.
	_, dockerErr := vmRunner.Run(ctx, "docker", "info")
	check("Docker running in VM", dockerErr == nil, "")

	// 4. Check image exists.
	imgOut, imgErr := vmRunner.Run(ctx, "docker", "images", "safe-agentic:latest", "-q")
	imageExists := imgErr == nil && strings.TrimSpace(string(imgOut)) != ""
	imageDetail := ""
	if imageExists {
		imageDetail = strings.TrimSpace(string(imgOut))
	}
	check("Docker image safe-agentic:latest", imageExists, imageDetail)

	// 5. Check host egress NAT. A reboot resets net.inet.ip.forwarding to 0 and
	// flushes the pf anchor, leaving the VM unable to reach the internet (clones
	// time out, agents die on startup). This is the most common silent breakage.
	forwarding := hostIPForwardingEnabled()
	egressDetail := ""
	if !forwarding {
		egressDetail = "IP forwarding off — VM has no internet egress; run: safe-ag vm start"
	}
	check("Host egress NAT (ip.forwarding)", forwarding, egressDetail)

	cfg, cfgErr := config.LoadDefaults(config.DefaultsPath())
	if cfgErr != nil {
		fmt.Println()
		fmt.Printf("Spawn defaults\n  ! could not read %s: %v\n", config.DefaultsPath(), cfgErr)
	} else {
		printDiagnoseSpawnDefaults(os.Stdout, cfg, config.DefaultsPath())
	}

	fmt.Println()
	infraOK := containerErr == nil && systemErr == nil && vmExists && dockerErr == nil && imageExists
	switch {
	case infraOK && cfgErr == nil && forwarding:
		fmt.Println("All checks passed. Environment is ready.")
	case infraOK && cfgErr == nil && !forwarding:
		fmt.Println("Environment built but VM has no internet egress. Run: safe-ag vm start")
	case infraOK && cfgErr != nil:
		// executeSpawn aborts on this same config error, so the environment is
		// not actually usable even though the infra checks pass.
		fmt.Printf("Spawn defaults are unreadable (%s); fix or reset with: safe-ag config reset\n", config.DefaultsPath())
	default:
		fmt.Println("Some checks failed. Run: safe-ag setup")
	}

	return nil
}

// hostIPForwardingEnabled reports whether the macOS host is forwarding IP,
// a prerequisite for the pf NAT that gives the VM internet egress.
func hostIPForwardingEnabledImpl() bool {
	out, err := exec.Command("/usr/sbin/sysctl", "-n", "net.inet.ip.forwarding").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}
