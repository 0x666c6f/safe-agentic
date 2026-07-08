package evidence

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func sha256Hex(t *testing.T, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestBuild_Dir(t *testing.T) {
	root := t.TempDir()
	fileA := []byte("hello world")
	fileB := []byte("second file contents")

	if err := os.WriteFile(filepath.Join(root, "b.txt"), fileB, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), fileA, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	fileC := []byte("nested file")
	if err := os.WriteFile(filepath.Join(root, "sub", "c.txt"), fileC, 0o644); err != nil {
		t.Fatal(err)
	}

	// symlink should be skipped, not error
	if err := os.Symlink(filepath.Join(root, "a.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}

	m, err := Build(root)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.Root != root {
		t.Errorf("Root = %q, want %q", m.Root, root)
	}
	if len(m.Entries) != 3 {
		t.Fatalf("len(Entries) = %d, want 3: %+v", len(m.Entries), m.Entries)
	}

	want := []Entry{
		{Path: "a.txt", SHA256: sha256Hex(t, fileA), Size: int64(len(fileA))},
		{Path: "b.txt", SHA256: sha256Hex(t, fileB), Size: int64(len(fileB))},
		{Path: filepath.Join("sub", "c.txt"), SHA256: sha256Hex(t, fileC), Size: int64(len(fileC))},
	}
	for i, e := range want {
		if m.Entries[i] != e {
			t.Errorf("Entries[%d] = %+v, want %+v", i, m.Entries[i], e)
		}
	}

	// sorted by Path
	for i := 1; i < len(m.Entries); i++ {
		if m.Entries[i-1].Path >= m.Entries[i].Path {
			t.Errorf("Entries not sorted: %q >= %q", m.Entries[i-1].Path, m.Entries[i].Path)
		}
	}
}

func TestBuild_SingleFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "evidence.log")
	data := []byte("single file contents")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Build(path)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(m.Entries))
	}
	want := Entry{Path: "evidence.log", SHA256: sha256Hex(t, data), Size: int64(len(data))}
	if m.Entries[0] != want {
		t.Errorf("Entries[0] = %+v, want %+v", m.Entries[0], want)
	}
}

func TestBuild_SymlinkedDirRoot(t *testing.T) {
	realDir := t.TempDir()
	data := []byte("hello world")
	if err := os.WriteFile(filepath.Join(realDir, "a.txt"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(t.TempDir(), "link-to-samples")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	want, err := Build(realDir)
	if err != nil {
		t.Fatalf("Build(realDir): %v", err)
	}
	got, err := Build(link)
	if err != nil {
		t.Fatalf("Build(link): %v", err)
	}
	if len(got.Entries) != len(want.Entries) {
		t.Fatalf("len(Entries) = %d, want %d", len(got.Entries), len(want.Entries))
	}
	for i := range want.Entries {
		if got.Entries[i].Path != want.Entries[i].Path || got.Entries[i].SHA256 != want.Entries[i].SHA256 {
			t.Errorf("Entries[%d] = %+v, want %+v", i, got.Entries[i], want.Entries[i])
		}
	}
}

func TestBuild_SymlinkedFileRoot(t *testing.T) {
	realDir := t.TempDir()
	data := []byte("single file contents")
	realFile := filepath.Join(realDir, "evidence.log")
	if err := os.WriteFile(realFile, data, 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(t.TempDir(), "link-to-file")
	if err := os.Symlink(realFile, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	m, err := Build(link)
	if err != nil {
		t.Fatalf("Build(link): %v", err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(m.Entries))
	}
	want := Entry{Path: "link-to-file", SHA256: sha256Hex(t, data), Size: int64(len(data))}
	if m.Entries[0] != want {
		t.Errorf("Entries[0] = %+v, want %+v", m.Entries[0], want)
	}
}

func TestBuild_NonexistentPath(t *testing.T) {
	_, err := Build(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("Build: expected error for nonexistent path, got nil")
	}
}

func TestManifest_String(t *testing.T) {
	m := Manifest{
		Root: "/evidence",
		Entries: []Entry{
			{Path: "a.txt", SHA256: "deadbeef", Size: 11},
			{Path: "b.txt", SHA256: "cafef00d", Size: 21},
		},
	}
	want := "deadbeef  11  a.txt\ncafef00d  21  b.txt\n"
	if got := m.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestWriteTar_RoundTrip(t *testing.T) {
	root := t.TempDir()
	fileA := []byte("hello world")
	fileB := []byte("second file")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), fileA, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.txt"), fileB, 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := WriteTar(&buf, root); err != nil {
		t.Fatalf("WriteTar: %v", err)
	}

	tr := tar.NewReader(&buf)
	got := map[string][]byte{}
	modes := map[string]int64{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read: %v", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read tar entry %q: %v", hdr.Name, err)
		}
		got[hdr.Name] = data
		modes[hdr.Name] = hdr.Mode
		if hdr.Uid != 0 || hdr.Gid != 0 {
			t.Errorf("entry %q has uid=%d gid=%d, want 0/0", hdr.Name, hdr.Uid, hdr.Gid)
		}
	}

	wantFiles := map[string][]byte{
		"a.txt":                       fileA,
		filepath.ToSlash("sub/b.txt"): fileB,
	}
	if len(got) != len(wantFiles) {
		t.Fatalf("tar contains %d regular files, want %d: %v", len(got), len(wantFiles), got)
	}
	for name, data := range wantFiles {
		gotData, ok := got[name]
		if !ok {
			t.Errorf("tar missing entry %q", name)
			continue
		}
		if !bytes.Equal(gotData, data) {
			t.Errorf("tar entry %q contents = %q, want %q", name, gotData, data)
		}
		if modes[name] != 0o444 {
			t.Errorf("tar entry %q mode = %o, want 0444", name, modes[name])
		}
	}
}

func TestWriteTar_SingleFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "evidence.log")
	data := []byte("single file contents")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := WriteTar(&buf, path); err != nil {
		t.Fatalf("WriteTar: %v", err)
	}
	tr := tar.NewReader(&buf)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tar read: %v", err)
	}
	if hdr.Name != "evidence.log" {
		t.Errorf("Name = %q, want %q", hdr.Name, "evidence.log")
	}
	if hdr.Mode != 0o444 {
		t.Errorf("Mode = %o, want 0444", hdr.Mode)
	}
}
