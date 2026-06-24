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

	nested := filepath.Join(root, "cmd", "safe-ag")
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
