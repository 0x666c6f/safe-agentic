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

	"github.com/0x666c6f/berth/pkg/config"
	"github.com/0x666c6f/berth/pkg/vmexec"
	"github.com/0x666c6f/berth/pkg/worktrees"
	"github.com/spf13/cobra"
)

func configuredVMName() string {
	if v := os.Getenv("BERTH_VM_NAME"); v != "" {
		return v
	}
	return "berth"
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
	reconcileHomeMount      = reconcileHomeMountImpl
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

// runVMBootstrapImpl runs vm/setup.sh in the VM. setup.sh installs Docker and
// hardens the machine — a multi-minute operation — so it streams its combined
// output live to stdout instead of buffering silently until exit.
func runVMBootstrapImpl(vmName, worktreesDir, homeDir string) ([]byte, error) {
	cmd := exec.Command("container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/sh", "/tmp/setup.sh", worktreesDir, homeDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout
	return nil, cmd.Run()
}

// machineCreateArgs builds the `container machine create` argument vector.
// homeMount is "none" by default (no host sharing — the strongest posture) and
// "rw" only when the worktree mount is explicitly enabled, so vm/setup.sh can
// bind the worktrees root to /worktrees.
func machineCreateArgs(vmName, homeMount string) []string {
	return []string{"machine", "create", "alpine:3.22", "--name", vmName, "--cpus", "4", "--memory", "8G", "--home-mount", homeMount}
}

// worktreesHostDir resolves the host worktrees root, validates it lives under the
// user's home (the only path an Apple container machine can mount into the VM),
// and ensures it exists so the home mount carries it.
func worktreesHostDir() (string, error) {
	dir := config.WorktreesDir()
	if err := validateWorktreesUnderHome(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create worktrees dir %s: %w", dir, err)
	}
	return dir, nil
}

func validateWorktreesUnderHome(dir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	absHome, err := filepath.Abs(home)
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve worktrees dir: %w", err)
	}
	rel, err := filepath.Rel(absHome, absDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("worktrees_dir %s must live under your home directory %s; the Apple container machine can only mount your home into the VM. Set one with: berth config set defaults.worktrees_dir <path-under-home>", absDir, absHome)
	}
	return nil
}

// machineHomeMount reports the home-mount mode ("rw"/"ro"/"none"/"") of a machine.
func machineHomeMount(vmName string) string {
	out, err := exec.Command("container", "machine", "inspect", vmName).Output()
	if err != nil {
		return ""
	}
	var machines []struct {
		HomeMount string `json:"homeMount"`
	}
	if err := json.Unmarshal(out, &machines); err != nil || len(machines) == 0 {
		return ""
	}
	return machines[0].HomeMount
}

// reconcileHomeMountImpl switches an existing machine's home-mount to desired
// ("none" or "rw") when it differs. Apple's container tool only applies the
// change after a restart, so this stops the machine; the caller must (re)start
// it. Returns true when the machine was reconfigured (and therefore stopped).
func reconcileHomeMountImpl(vmName, desired string, stdout, stderr io.Writer) (bool, error) {
	if machineHomeMount(vmName) == desired {
		return false, nil
	}
	fmt.Fprintf(stdout, "Reconfiguring VM %s home-mount=%s (applied on restart)…\n", vmName, desired)
	set := exec.Command("container", "machine", "set", "-n", vmName, "home-mount="+desired)
	set.Stdout = stdout
	set.Stderr = stderr
	if err := set.Run(); err != nil {
		return false, fmt.Errorf("set VM home-mount=%s: %w", desired, err)
	}
	stop := exec.Command("container", "machine", "stop", vmName)
	stop.Stdout = stdout
	stop.Stderr = stderr
	if err := stop.Run(); err != nil {
		return false, fmt.Errorf("stop VM to apply home-mount=%s: %w", desired, err)
	}
	return true, nil
}

// worktreeMountPlan resolves the desired VM posture from config and the setup
// flags. When enabled it validates+creates the worktrees root and returns the
// host home dir (used inside the VM to detach the rest of the home share).
func worktreeMountPlan() (enabled bool, homeMount, worktreesDir, homeDir string, err error) {
	if !config.WorktreesMountEnabled() {
		return false, "none", "", "", nil
	}
	worktreesDir, err = worktreesHostDir()
	if err != nil {
		return false, "none", "", "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, "none", "", "", fmt.Errorf("resolve home directory: %w", err)
	}
	return true, "rw", worktreesDir, home, nil
}

// worktreesSentinelPath is the boot-local file vm/setup.sh writes with the host
// root currently bound at /worktrees, so a re-run (and diagnose) can detect a
// changed defaults.worktrees_dir. Keep in sync with WORKTREES_SENTINEL in
// vm/setup.sh.
const worktreesSentinelPath = "/run/berth-worktrees-source"

// verifyWorktreeMountLive confirms the /worktrees bind is actually mounted inside
// the VM after bootstrap and points at the requested root. vm/setup.sh only warns
// (and exits 0) if the bind fails, the host dir isn't visible, or the root
// changed and could not be rebound, so setup must check from the host side before
// claiming success — otherwise later --worktree spawns fail with no prior signal.
func verifyWorktreeMountLive(ctx context.Context, vmRunner interface {
	Run(context.Context, ...string) ([]byte, error)
}, wantRoot string) error {
	check := "test -d " + worktrees.VMMountPoint + " && mountpoint -q " + worktrees.VMMountPoint
	if _, err := vmRunner.Run(ctx, "sh", "-c", check); err != nil {
		return fmt.Errorf("worktree mount enabled but %s is not mounted in the VM. Check that: the worktrees dir is under your home directory, the machine is home-mount=rw (a restart applies the change), then rerun: berth setup. Diagnose with: berth diagnose", worktrees.VMMountPoint)
	}
	// Confirm the live bind points at the requested root, not a stale one left by a
	// previous defaults.worktrees_dir. An empty sentinel means "unknown" (older
	// bind or tee failure) — don't fail on that alone.
	srcOut, _ := vmRunner.Run(ctx, "sh", "-c", "cat "+worktreesSentinelPath+" 2>/dev/null")
	if cur := strings.TrimSpace(string(srcOut)); cur != "" && cur != wantRoot {
		return fmt.Errorf("%s is bound to %s but you configured %s; the VM must reboot to rebind after changing worktrees_dir: berth vm stop && berth vm start", worktrees.VMMountPoint, cur, wantRoot)
	}
	return nil
}

func installVMSupportFilesImpl(vmName, buildRoot string) error {
	seccompSrc := filepath.Join(buildRoot, "config", "seccomp.json")
	if err := copyFileToVM(vmName, seccompSrc, "/tmp/seccomp.json"); err != nil {
		return err
	}
	commands := [][]string{
		{"container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/mkdir", "-p", "/etc/berth"},
		{"container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/cp", "/tmp/seccomp.json", "/etc/berth/seccomp.json"},
		{"container", "machine", "run", "-n", vmName, "-u", "root", "--", "/bin/chmod", "0644", "/etc/berth/seccomp.json"},
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
	ifaces, err := hostNATInterfaces()
	if err != nil {
		return err
	}
	var rules []string
	seen := map[string]bool{}
	for _, subnet := range subnets {
		for _, iface := range ifaces {
			key := subnet + "\x00" + iface
			if seen[key] {
				continue
			}
			seen[key] = true
			rules = append(rules, fmt.Sprintf("nat on %s from %s to any -> (%s)", iface, subnet, iface))
		}
	}
	script := strings.Join([]string{
		"/usr/sbin/sysctl -w net.inet.ip.forwarding=1 >/dev/null",
		"printf " + shellSingleQuote(strings.Join(rules, "\n")+"\n") + " | /sbin/pfctl -a com.apple/berth -f -",
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

func hostNATInterfaces() ([]string, error) {
	iface, err := defaultHostInterface()
	if err != nil {
		return nil, err
	}
	ifaces := []string{iface}
	if vpnIfaces, err := vpnRouteInterfaces(); err == nil {
		ifaces = append(ifaces, vpnIfaces...)
	}
	seen := map[string]bool{}
	var out []string
	for _, candidate := range ifaces {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	return out, nil
}

func vpnRouteInterfaces() ([]string, error) {
	out, err := exec.Command("netstat", "-rn", "-f", "inet").Output()
	if err != nil {
		return nil, fmt.Errorf("detect VPN route interfaces: %w", err)
	}
	return parseVPNRouteInterfaces(out), nil
}

func parseVPNRouteInterfaces(out []byte) []string {
	seen := map[string]bool{}
	var ifaces []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		iface := fields[len(fields)-1]
		if !(strings.HasPrefix(iface, "utun") || strings.HasPrefix(iface, "ppp") || strings.HasPrefix(iface, "ipsec")) {
			continue
		}
		if seen[iface] {
			continue
		}
		seen[iface] = true
		ifaces = append(ifaces, iface)
	}
	return ifaces
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
	Short: "First-time setup: create the VM, harden it, build the image",
	Long: `Create the Apple container machine, harden it, install Docker, and build the
agent image. Run this once before spawning agents; safe to re-run to reconcile
the VM (it also re-applies host NAT so the VM keeps internet egress).`,
	GroupID: groupSetup,
	RunE:    runSetup,
}

var (
	setupEnableWorktrees  bool
	setupDisableWorktrees bool
)

func init() {
	setupCmd.Flags().BoolVar(&setupEnableWorktrees, "enable-worktrees", false, "Enable the worktree mount (VM home-mount=rw; weakens VM isolation — see docs)")
	setupCmd.Flags().BoolVar(&setupDisableWorktrees, "disable-worktrees", false, "Disable the worktree mount (VM home-mount=none; strongest isolation, --worktree unavailable)")
	rootCmd.AddCommand(setupCmd)
}

// applyWorktreeSetupFlags persists the --enable-worktrees/--disable-worktrees
// choice into config and prints the risk warning when enabling.
func applyWorktreeSetupFlags(stdout io.Writer) error {
	if setupEnableWorktrees && setupDisableWorktrees {
		return fmt.Errorf("--enable-worktrees and --disable-worktrees are mutually exclusive")
	}
	if !setupEnableWorktrees && !setupDisableWorktrees {
		return nil
	}
	raw, err := config.LoadRawConfig(config.ConfigPath())
	if err != nil {
		return err
	}
	val := "false"
	if setupEnableWorktrees {
		val = "true"
	}
	if err := config.SetValue(&raw, "defaults.worktrees_mount", val); err != nil {
		return err
	}
	if err := config.SaveRawConfig(config.ConfigPath(), raw); err != nil {
		return err
	}
	if setupEnableWorktrees {
		printWorktreeMountWarning(stdout)
	} else {
		fmt.Fprintln(stdout, "Worktree mount disabled — VM will use home-mount=none (no host sharing).")
	}
	return nil
}

func printWorktreeMountWarning(w io.Writer) {
	fmt.Fprintln(w, "⚠  Worktree mount ENABLED. The VM will run home-mount=rw, sharing your entire")
	fmt.Fprintln(w, "   home directory with the machine at the virtiofs level. berth binds")
	fmt.Fprintln(w, "   only the worktrees root to /worktrees and detaches/masks the rest, but this")
	fmt.Fprintln(w, "   is a WEAKER boundary than the default (home-mount=none, no host sharing): a")
	fmt.Fprintln(w, "   VM-root compromise or Docker escape could reach your host home. Keep secrets")
	fmt.Fprintln(w, "   and unrelated projects out of the worktrees root.")
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

	// Apply --enable-worktrees/--disable-worktrees (persists config + warns).
	if err := applyWorktreeSetupFlags(cmd.OutOrStdout()); err != nil {
		return err
	}
	// Resolve the desired VM posture. Default: home-mount=none (no host sharing).
	worktreesEnabled, homeMount, worktreesDir, homeDir, err := worktreeMountPlan()
	if err != nil {
		return err
	}

	// Step 2: Check if Apple container machine exists.
	vmExists := vmExists(vmName)

	// Step 3: Create VM if needed.
	if !vmExists {
		fmt.Printf("Creating VM %s (alpine:3.22, home-mount=%s)…\n", vmName, homeMount)
		create := exec.Command("container", append([]string(nil), machineCreateArgs(vmName, homeMount)...)...)
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
		// Reconcile home-mount to the desired posture (migrates in either
		// direction: none↔rw) so the machine matches the worktree-mount setting.
		if _, err := reconcileHomeMount(vmName, homeMount, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
			return err
		}
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
	if _, err := runVMBootstrap(vmName, worktreesDir, homeDir); err != nil {
		return fmt.Errorf("run VM bootstrap: %w", err)
	}
	if err := installVMSupportFiles(vmName, buildRoot); err != nil {
		return err
	}
	fmt.Println("✓ VM support files installed")
	if worktreesEnabled {
		// vm/setup.sh only WARNS (exit 0) if the /worktrees bind fails or the host
		// dir isn't visible, so the user explicitly asked for worktrees but could
		// still land on a machine where --worktree will fail. Verify the mount from
		// the host side and fail the setup if it is not live.
		if err := verifyWorktreeMountLive(ctx, vmRunner, worktreesDir); err != nil {
			return err
		}
		fmt.Printf("✓ Worktrees mounted: %s → %s (VM)\n", worktreesDir, worktrees.VMMountPoint)
	} else {
		fmt.Println("✓ Worktree mount disabled (home-mount=none). Enable with: berth setup --enable-worktrees")
	}

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
	updateQuick    bool
	updateFull     bool
	updateForensic bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Rebuild the agent Docker image",
	Long: `Rebuild the agent image. By default it uses Docker's layer cache; use --quick
to only refresh the AI CLI layer, or --full to rebuild from scratch.`,
	GroupID: groupSetup,
	RunE:    runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updateQuick, "quick", false, "Rebuild only the AI CLI layer (fast; picks up new Claude/Codex versions)")
	updateCmd.Flags().BoolVar(&updateFull, "full", false, "Rebuild every layer from scratch with no cache (slowest, most thorough)")
	updateCmd.Flags().BoolVar(&updateForensic, "forensic", false, "Build the forensic tool image (berth:forensic) from Dockerfile.forensic")
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

func runDockerBuild(ctx context.Context, vmRunner vmexec.Executor) error {
	buildArgs := []string{"docker", "build", "-t", "berth:latest", "/tmp/build-context"}
	image := "berth:latest"

	switch {
	case updateForensic:
		image = "berth:forensic"
		// -f must be absolute: buildkit resolves a relative Dockerfile path
		// against the docker client's cwd (/ in the VM), not the build context,
		// so a bare "Dockerfile.forensic" is looked up as /Dockerfile.forensic.
		buildArgs = []string{"docker", "build", "-f", "/tmp/build-context/Dockerfile.forensic", "-t", image, "/tmp/build-context"}
	case updateFull:
		buildArgs = []string{"docker", "build", "--no-cache", "-t", image, "/tmp/build-context"}
	case updateQuick:
		cacheBust := fmt.Sprintf("%d", time.Now().Unix())
		buildArgs = []string{"docker", "build", "--build-arg", "CLI_CACHE_BUST=" + cacheBust, "-t", image, "/tmp/build-context"}
	}

	// Stream the build log live: it runs for minutes, and on failure the output is
	// the only clue to what broke — buffering it discarded that on error.
	fmt.Printf("Building %s…\n", image)
	if err := vmRunner.RunStreaming(ctx, os.Stdout, buildArgs...); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	fmt.Printf("✓ Image updated (%s)\n", image)
	return nil
}

// ─── vm ────────────────────────────────────────────────────────────────────

var vmCmd = &cobra.Command{
	Use:     "vm",
	Short:   "Manage the Apple container machine (start/stop/ssh)",
	GroupID: groupSetup,
}

var vmSSHCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Open an interactive debug shell inside the VM",
	RunE:  runVMSSH,
}

var vmStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the VM, re-harden it, and re-apply host NAT (restores egress)",
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
	// Resolve the desired posture (default home-mount=none) and reconcile it
	// before booting so the machine matches the worktree-mount setting.
	_, homeMount, worktreesDir, homeDir, err := worktreeMountPlan()
	if err != nil {
		return err
	}
	if _, err := reconcileHomeMount(vmName, homeMount, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
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
	if _, err := runVMBootstrap(vmName, worktreesDir, homeDir); err != nil {
		return fmt.Errorf("run VM bootstrap: %w", err)
	}
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
	Use:     "diagnose",
	Short:   "Health-check the VM, egress, image, and worktree posture",
	GroupID: groupSetup,
	RunE:    runDiagnose,
}

func init() {
	rootCmd.AddCommand(diagnoseCmd)
	rootCmd.AddCommand(tuiCmd)
}

// ─── tui ──────────────────────────────────────────────────────────────

var tuiCmd = &cobra.Command{
	Use:     "tui",
	Short:   "Launch the k9s-style interactive dashboard",
	GroupID: groupSetup,
	RunE:    runTUI,
}

func findTUIBinary() (string, error) {
	// Check next to berth binary first
	self, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(self), "berth-tui")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	// Fall back to PATH
	return exec.LookPath("berth-tui")
}

func runTUI(cmd *cobra.Command, args []string) error {
	bin, err := findTUIBinary()
	if err != nil {
		return fmt.Errorf("berth-tui not found. Build with: make build-tui")
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

	fmt.Println("berth diagnostics")
	fmt.Println("─────────────────────────")

	// 1. Check Apple container installed and running.
	_, containerErr := exec.LookPath("container")
	check("container installed", containerErr == nil, "")

	if containerErr != nil {
		fmt.Println()
		fmt.Println("Install Apple container: https://github.com/apple/container")
		diagnoseSpawnDefaults()
		return nil
	}

	systemErr := exec.Command("container", "system", "status").Run()
	check("container system running", systemErr == nil, "")

	// 2. Check VM exists.
	vmExists := vmExists(vmName)
	check("VM "+vmName+" exists", vmExists, "")

	if !vmExists {
		fmt.Println()
		fmt.Println("Run: berth setup")
		diagnoseSpawnDefaults()
		return nil
	}

	vmRunner := newExecutor()

	// 3. Check Docker running in VM.
	_, dockerErr := vmRunner.Run(ctx, "docker", "info")
	check("Docker running in VM", dockerErr == nil, "")

	// 4. Check image exists.
	imgOut, imgErr := vmRunner.Run(ctx, "docker", "images", "berth:latest", "-q")
	imageExists := imgErr == nil && strings.TrimSpace(string(imgOut)) != ""
	imageDetail := ""
	if imageExists {
		imageDetail = strings.TrimSpace(string(imgOut))
	}
	check("Docker image berth:latest", imageExists, imageDetail)

	// 5. Check host egress NAT. A reboot resets net.inet.ip.forwarding to 0 and
	// flushes the pf anchor, leaving the VM unable to reach the internet (clones
	// time out, agents die on startup). This is the most common silent breakage.
	forwarding := hostIPForwardingEnabled()
	egressDetail := ""
	if !forwarding {
		egressDetail = "IP forwarding off — VM has no internet egress; run: berth vm start"
	}
	check("Host egress NAT (ip.forwarding)", forwarding, egressDetail)

	// 5b. Report api-only egress enforcement posture (bti+ REJECT rule +
	// tinyproxy). Best-effort: a probe error just reports not-active, it
	// doesn't fail diagnose.
	if dockerErr == nil {
		gap := apiOnlyEnforcementGap(ctx, vmRunner)
		detail := ""
		if gap != "" {
			detail = gap + " — run: berth vm start to (re-)provision it"
		}
		check("api-only egress enforcement", gap == "", detail)
	}

	// 6. Report the worktree-mount posture. Disabled (home-mount=none) is the
	// default and the strongest isolation; enabling it trades that for --worktree.
	worktreesEnabled := config.WorktreesMountEnabled()
	homeMount := machineHomeMount(vmName)
	if worktreesEnabled {
		wantRoot := config.WorktreesDir()
		_, mountErr := vmRunner.Run(ctx, "sh", "-c", "test -d "+worktrees.VMMountPoint+" && mountpoint -q "+worktrees.VMMountPoint)
		srcOut, _ := vmRunner.Run(ctx, "sh", "-c", "cat "+worktreesSentinelPath+" 2>/dev/null")
		curRoot := strings.TrimSpace(string(srcOut))
		mounted := homeMount == "rw" && mountErr == nil
		// A non-empty sentinel that disagrees with config means the machine still
		// binds a previous worktrees_dir and needs a reboot to rebind.
		sourceOK := curRoot == "" || curRoot == wantRoot
		mountOK := mounted && sourceOK
		detail := "enabled — home-mount=rw shares host home with the VM (weaker isolation); worktrees root visible at " + worktrees.VMMountPoint
		switch {
		case !mounted:
			detail = fmt.Sprintf("enabled in config but home-mount=%q / %s mount absent — run: berth setup to migrate the machine", homeMount, worktrees.VMMountPoint)
		case !sourceOK:
			detail = fmt.Sprintf("%s is bound to %s but config wants %s — reboot to rebind: berth vm stop && berth vm start", worktrees.VMMountPoint, curRoot, wantRoot)
		}
		check("Worktree mount ENABLED (weakened VM isolation)", mountOK, detail)
	} else {
		detail := "disabled (home-mount=none, strongest isolation); --worktree unavailable — enable with: berth setup --enable-worktrees"
		if homeMount == "rw" {
			detail = "config disabled but VM still home-mount=rw — run: berth setup to restore home-mount=none"
		}
		check("Worktree mount disabled (home-mount=none)", homeMount != "rw", detail)
	}

	cfgErr := diagnoseSpawnDefaults()

	fmt.Println()
	infraOK := containerErr == nil && systemErr == nil && vmExists && dockerErr == nil && imageExists
	switch {
	case infraOK && cfgErr == nil && forwarding:
		fmt.Println("All checks passed. Environment is ready.")
	case infraOK && cfgErr == nil && !forwarding:
		fmt.Println("Environment built but VM has no internet egress. Run: berth vm start")
	case infraOK && cfgErr != nil:
		// executeSpawn aborts on this same config error, so the environment is
		// not actually usable even though the infra checks pass.
		fmt.Printf("Spawn defaults are unreadable (%s); fix or reset with: berth config reset\n", config.DefaultsPath())
	default:
		fmt.Println("Some checks failed. Run: berth setup")
	}

	return nil
}

// diagnoseSpawnDefaults prints the spawn-defaults section and returns the
// config load error. Spawn defaults exist independently of the container
// runtime, so the early-exit diagnose paths (no `container` binary, no VM)
// still surface risky defaults like ssh/reuse_auth enabled.
func diagnoseSpawnDefaults() error {
	cfg, cfgErr := config.LoadDefaults(config.DefaultsPath())
	if cfgErr != nil {
		fmt.Println()
		fmt.Printf("Spawn defaults\n  ! could not read %s: %v\n", config.DefaultsPath(), cfgErr)
		return cfgErr
	}
	printDiagnoseSpawnDefaults(os.Stdout, cfg, config.DefaultsPath())
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
