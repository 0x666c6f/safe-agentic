package main

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/0x666c6f/berth/pkg/config"
)

func TestParseAppleContainerNATSubnets(t *testing.T) {
	out := []byte(`[
		{
			"configuration": {"mode": "nat", "ipv4Subnet": "192.168.64.0/24"},
			"status": {"ipv4Subnet": "192.168.64.0/24"}
		},
		{
			"configuration": {"mode": "nat", "ipv4Subnet": "192.168.65.0/24"},
			"status": {}
		},
		{
			"configuration": {"mode": "host", "ipv4Subnet": "192.168.66.0/24"},
			"status": {"ipv4Subnet": "192.168.66.0/24"}
		},
		{
			"configuration": {"mode": "nat", "ipv4Subnet": "192.168.64.0/24"},
			"status": {"ipv4Subnet": "192.168.64.0/24"}
		}
	]`)
	got, err := parseAppleContainerNATSubnets(out)
	if err != nil {
		t.Fatalf("parseAppleContainerNATSubnets returned error: %v", err)
	}
	want := []string{"192.168.64.0/24", "192.168.65.0/24"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subnets = %v, want %v", got, want)
	}
}

func TestParseAppleContainerNATSubnetsInvalidJSON(t *testing.T) {
	_, err := parseAppleContainerNATSubnets([]byte(`not-json`))
	if err == nil || !strings.Contains(err.Error(), "parse Apple container networks") {
		t.Fatalf("error = %v, want parse error", err)
	}
}

func TestParseDefaultHostInterface(t *testing.T) {
	got, err := parseDefaultHostInterface([]byte("   route to: default\ninterface: en0\n"))
	if err != nil {
		t.Fatalf("parseDefaultHostInterface returned error: %v", err)
	}
	if got != "en0" {
		t.Fatalf("interface = %q, want en0", got)
	}
}

func TestParseDefaultHostInterfaceMissing(t *testing.T) {
	_, err := parseDefaultHostInterface([]byte("route to: default\n"))
	if err == nil || !strings.Contains(err.Error(), "no interface") {
		t.Fatalf("error = %v, want missing interface error", err)
	}
}

func TestParseVPNRouteInterfaces(t *testing.T) {
	out := []byte(`Routing tables

Internet:
Destination        Gateway            Flags               Netif Expire
default            192.168.1.1        UGScg                 en0
10/16              172.16.17.129      UGSc                utun6
172.16.17.128/27   link#25            UCS                 utun6
192.168.64         link#32            UC              bridge101
100.64/10          172.16.18.1        UGSc                 ppp0
`)
	got := parseVPNRouteInterfaces(out)
	want := []string{"utun6", "ppp0"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("interfaces = %v, want %v", got, want)
	}
}

func TestSetupQuotingHelpers(t *testing.T) {
	if got, want := shellSingleQuote("a'b"), "'a'\"'\"'b'"; got != want {
		t.Fatalf("shellSingleQuote = %q, want %q", got, want)
	}
	if got, want := appleScriptQuote(`cmd "arg" \ tail`), `"cmd \"arg\" \\ tail"`; got != want {
		t.Fatalf("appleScriptQuote = %q, want %q", got, want)
	}
}

func TestFindBuildRootImplFromCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Dockerfile"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(root, "entrypoint.sh"), "#!/bin/sh\n")
	writeTestFile(t, filepath.Join(root, "vm", "setup.sh"), "#!/bin/sh\n")

	nested := filepath.Join(root, "cmd", "berth")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	got, err := findBuildRootImpl()
	if err != nil {
		t.Fatalf("findBuildRootImpl returned error: %v", err)
	}
	if got != root {
		t.Fatalf("build root = %q, want %q", got, root)
	}
}

func TestBuildContextFilesUsesTrackedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "tracked.txt"), "tracked")
	writeTestFile(t, filepath.Join(root, "untracked.txt"), "untracked")
	runGit(t, root, "init")
	runGit(t, root, "add", "tracked.txt")

	got, err := buildContextFiles(root)
	if err != nil {
		t.Fatalf("buildContextFiles returned error: %v", err)
	}
	want := []string{"tracked.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
}

func TestWriteBuildContextArchiveWalkFallback(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Dockerfile"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(root, "config", "app.conf"), "value=1\n")
	writeTestFile(t, filepath.Join(root, ".git", "ignored"), "ignored")
	writeTestFile(t, filepath.Join(root, "site", "ignored"), "ignored")
	if err := os.Symlink("app.conf", filepath.Join(root, "config", "app-link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	var buf bytes.Buffer
	if err := writeBuildContextArchive(tar.NewWriter(&buf), root); err != nil {
		t.Fatalf("writeBuildContextArchive returned error: %v", err)
	}

	got := map[string]string{}
	links := map[string]string{}
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		switch hdr.Typeflag {
		case tar.TypeReg:
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatalf("read %s: %v", hdr.Name, err)
			}
			got[hdr.Name] = string(data)
		case tar.TypeSymlink:
			links[hdr.Name] = hdr.Linkname
		}
	}

	names := make([]string, 0, len(got)+len(links))
	for name := range got {
		names = append(names, name)
	}
	for name := range links {
		names = append(names, name)
	}
	sort.Strings(names)
	wantNames := []string{"Dockerfile", "config/app-link", "config/app.conf"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("archive names = %v, want %v", names, wantNames)
	}
	if got["config/app.conf"] != "value=1\n" {
		t.Fatalf("config/app.conf content = %q", got["config/app.conf"])
	}
	if links["config/app-link"] != "app.conf" {
		t.Fatalf("config/app-link target = %q", links["config/app-link"])
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
}

func TestMachineCreateArgsHomeMount(t *testing.T) {
	// Default posture: home-mount=none (no host sharing).
	none := strings.Join(machineCreateArgs("berth", "none"), " ")
	if !strings.Contains(none, "--home-mount none") {
		t.Fatalf("machineCreateArgs(none) = %q, want --home-mount none", none)
	}
	for _, want := range []string{"machine create alpine:3.22", "--name berth", "--cpus 4", "--memory 8G"} {
		if !strings.Contains(none, want) {
			t.Fatalf("machineCreateArgs = %q, missing %q", none, want)
		}
	}
	// Opt-in posture: home-mount=rw so vm/setup.sh can bind /worktrees.
	rw := strings.Join(machineCreateArgs("berth", "rw"), " ")
	if !strings.Contains(rw, "--home-mount rw") {
		t.Fatalf("machineCreateArgs(rw) = %q, want --home-mount rw", rw)
	}
}

func TestWorktreeMountPlanDefaultsToNone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No config → worktree mount disabled → home-mount=none, empty paths.
	enabled, homeMount, worktreesDir, homeDir, err := worktreeMountPlan()
	if err != nil {
		t.Fatalf("worktreeMountPlan() error = %v", err)
	}
	if enabled || homeMount != "none" || worktreesDir != "" || homeDir != "" {
		t.Fatalf("default plan = (enabled=%v home=%q wt=%q homeDir=%q), want disabled/none/empty", enabled, homeMount, worktreesDir, homeDir)
	}

	// Enable via config → home-mount=rw with resolved worktrees + home dirs.
	raw, err := config.LoadRawConfig(config.ConfigPath())
	if err != nil {
		t.Fatalf("LoadRawConfig: %v", err)
	}
	if err := config.SetValue(&raw, "defaults.worktrees_mount", "true"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := config.SaveRawConfig(config.ConfigPath(), raw); err != nil {
		t.Fatalf("SaveRawConfig: %v", err)
	}
	enabled, homeMount, worktreesDir, homeDir, err = worktreeMountPlan()
	if err != nil {
		t.Fatalf("worktreeMountPlan(enabled) error = %v", err)
	}
	if !enabled || homeMount != "rw" || worktreesDir == "" || homeDir == "" {
		t.Fatalf("enabled plan = (enabled=%v home=%q wt=%q homeDir=%q), want enabled/rw/non-empty", enabled, homeMount, worktreesDir, homeDir)
	}
}

func TestValidateWorktreesUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := validateWorktreesUnderHome(filepath.Join(home, ".berth", "worktrees")); err != nil {
		t.Fatalf("under-home worktrees dir rejected: %v", err)
	}
	if err := validateWorktreesUnderHome(filepath.Join(t.TempDir(), "outside")); err == nil {
		t.Fatalf("expected rejection for worktrees dir outside home")
	}
}

func TestVMSetupBindsWorktreesBeforeMasking(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "vm", "setup.sh"))
	if err != nil {
		t.Fatalf("read vm/setup.sh: %v", err)
	}
	content := string(script)
	for _, want := range []string{
		`WORKTREES_HOST="${1:-}"`,
		`HOME_MOUNT="${2:-}"`,
		`mount --bind "$WORKTREES_HOST" "$WORKTREES_MOUNT"`,
		`umount -l "$HOME_MOUNT"`, // detach the rest of the shared home
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("vm/setup.sh missing worktrees mount marker %q", want)
		}
	}
	// The bind must precede the tmpfs mask so it survives masking of /Users.
	bindIdx := strings.Index(content, `mount --bind "$WORKTREES_HOST"`)
	maskIdx := strings.Index(content, "size=1k none")
	if bindIdx < 0 || maskIdx < 0 || bindIdx > maskIdx {
		t.Fatalf("worktrees bind (idx %d) must precede the tmpfs mask (idx %d)", bindIdx, maskIdx)
	}
	// The home detach must sit between the bind and the mask.
	detachIdx := strings.Index(content, `umount -l "$HOME_MOUNT"`)
	if detachIdx < bindIdx || detachIdx > maskIdx {
		t.Fatalf("home detach (idx %d) must be between bind (idx %d) and mask (idx %d)", detachIdx, bindIdx, maskIdx)
	}
}

func TestSetupFailsWhenWorktreeMountNotLive(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	installFakeContainerBinary(t)

	// Opt into worktrees (HOME is the tempdir from testSetup).
	raw, err := config.LoadRawConfig(config.ConfigPath())
	if err != nil {
		t.Fatalf("LoadRawConfig: %v", err)
	}
	if err := config.SetValue(&raw, "defaults.worktrees_mount", "true"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := config.SaveRawConfig(config.ConfigPath(), raw); err != nil {
		t.Fatalf("SaveRawConfig: %v", err)
	}

	// Simulate vm/setup.sh's bind having failed: /worktrees is not a mountpoint.
	// vm/setup.sh only warns and exits 0, so setup must catch this from the host.
	fake.SetError("sh -c test -d /worktrees", "not mounted")
	fake.SetResponse("docker info", "Server Version: 24.0\n")
	fake.SetResponse("docker build", "Successfully built abc123\n")

	output := captureOutput(func() {
		err = runSetup(setupCmd, nil)
	})
	if err == nil || !strings.Contains(err.Error(), "is not mounted in the VM") {
		t.Fatalf("runSetup() error = %v, want worktree mount verification failure", err)
	}
	if strings.Contains(output, "Worktrees mounted") {
		t.Fatalf("setup must not claim success when the mount is not live:\n%s", output)
	}
}

func TestSetupVerifiesWorktreeMountWhenLive(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	installFakeContainerBinary(t)

	raw, err := config.LoadRawConfig(config.ConfigPath())
	if err != nil {
		t.Fatalf("LoadRawConfig: %v", err)
	}
	if err := config.SetValue(&raw, "defaults.worktrees_mount", "true"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := config.SaveRawConfig(config.ConfigPath(), raw); err != nil {
		t.Fatalf("SaveRawConfig: %v", err)
	}
	// /worktrees mount check succeeds (fake returns no error by default).
	fake.SetResponse("docker info", "Server Version: 24.0\n")
	fake.SetResponse("docker build", "Successfully built abc123\n")

	output := captureOutput(func() {
		if err := runSetup(setupCmd, nil); err != nil {
			t.Fatalf("runSetup() error = %v", err)
		}
	})
	if !strings.Contains(output, "Worktrees mounted") {
		t.Fatalf("expected worktree success line, got:\n%s", output)
	}
	// The host-side verification must actually have run.
	if len(fake.CommandsMatching("mountpoint -q /worktrees")) == 0 {
		t.Fatal("expected host-side /worktrees mount verification to run")
	}
}

func TestSetupFailsOnStaleWorktreeRootBinding(t *testing.T) {
	fake, cleanup := testSetup(t)
	defer cleanup()
	installFakeContainerBinary(t)

	raw, err := config.LoadRawConfig(config.ConfigPath())
	if err != nil {
		t.Fatalf("LoadRawConfig: %v", err)
	}
	if err := config.SetValue(&raw, "defaults.worktrees_mount", "true"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := config.SaveRawConfig(config.ConfigPath(), raw); err != nil {
		t.Fatalf("SaveRawConfig: %v", err)
	}

	// /worktrees is mounted (mount check passes by default), but the sentinel
	// records a DIFFERENT root than configured — defaults.worktrees_dir changed
	// and the VM still binds the old root. Setup must fail loudly, not claim the
	// new root is mounted.
	fake.SetResponse("sh -c cat /run/berth-worktrees-source", "/some/old/worktrees\n")
	fake.SetResponse("docker info", "Server Version: 24.0\n")
	fake.SetResponse("docker build", "Successfully built abc123\n")

	var runErr error
	output := captureOutput(func() { runErr = runSetup(setupCmd, nil) })
	if runErr == nil || !strings.Contains(runErr.Error(), "is bound to /some/old/worktrees") {
		t.Fatalf("runSetup() error = %v, want stale worktree root binding error", runErr)
	}
	if strings.Contains(output, "Worktrees mounted") {
		t.Fatalf("setup must not claim success on a stale root binding:\n%s", output)
	}
}

func TestVMSetupRebindsOnWorktreesRootChange(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "vm", "setup.sh"))
	if err != nil {
		t.Fatalf("read vm/setup.sh: %v", err)
	}
	content := string(script)
	for _, want := range []string{
		`WORKTREES_SENTINEL=/run/berth-worktrees-source`,
		`cat "$WORKTREES_SENTINEL"`,    // compares the recorded source root
		`umount -l "$WORKTREES_MOUNT"`, // drops the stale bind on mismatch
		`tee "$WORKTREES_SENTINEL"`,    // records the source after (re)bind
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("vm/setup.sh missing worktrees rebind marker %q", want)
		}
	}
	// The stale-bind unmount must be reached before the fresh bind.
	staleUmountIdx := strings.Index(content, `umount -l "$WORKTREES_MOUNT"`)
	bindIdx := strings.Index(content, `mount --bind "$WORKTREES_HOST"`)
	if staleUmountIdx < 0 || bindIdx < 0 || staleUmountIdx > bindIdx {
		t.Fatalf("stale-bind umount (idx %d) must precede the rebind (idx %d)", staleUmountIdx, bindIdx)
	}
}
