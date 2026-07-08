// Package evidence computes a sha256 chain-of-custody manifest for a host
// path and packages the same files into a deterministic tar for mounting
// into an agent container.
package evidence

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one regular file under an evidence root.
type Entry struct {
	Path   string // relative to root ("/"-independent: filepath separators), or basename for a single-file root
	SHA256 string // hex-encoded
	Size   int64
}

// Manifest is the chain-of-custody record for an evidence root.
type Manifest struct {
	Root    string
	Entries []Entry
}

// file is a regular file found under root, ready to be hashed or tarred.
type file struct {
	relPath  string // Entry.Path / tar header name
	fullPath string // absolute path on disk
}

// collect walks root and returns every regular file under it, sorted by
// relPath. root must exist and be a regular file or a directory; any other
// node type (symlink, device, socket, ...) at the root is an error. Non-
// regular nodes encountered while walking a directory are skipped.
func collect(root string) ([]file, error) {
	// Stat (not Lstat) so a symlinked root itself is followed to its target;
	// inner symlinks encountered while walking a directory are still skipped
	// below.
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("evidence root %q: %w", root, err)
	}

	var files []file
	switch {
	case info.Mode().IsRegular():
		files = append(files, file{relPath: filepath.Base(root), fullPath: root})
	case info.IsDir():
		walkRoot := root
		if r, err := filepath.EvalSymlinks(root); err == nil {
			walkRoot = r
		}
		err := filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.Type().IsRegular() {
				return nil // skip dirs, symlinks, devices, sockets
			}
			rel, err := filepath.Rel(walkRoot, path)
			if err != nil {
				return err
			}
			files = append(files, file{relPath: rel, fullPath: path})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk evidence root %q: %w", root, err)
		}
	default:
		return nil, fmt.Errorf("evidence root %q: unsupported node type %s", root, info.Mode())
	}

	sort.Slice(files, func(i, j int) bool { return files[i].relPath < files[j].relPath })
	return files, nil
}

// Build computes the chain-of-custody manifest for root.
func Build(root string) (Manifest, error) {
	files, err := collect(root)
	if err != nil {
		return Manifest{}, err
	}

	entries := make([]Entry, 0, len(files))
	for _, f := range files {
		sum, size, err := hashFile(f.fullPath)
		if err != nil {
			return Manifest{}, fmt.Errorf("hash %q: %w", f.fullPath, err)
		}
		entries = append(entries, Entry{Path: f.relPath, SHA256: sum, Size: size})
	}
	return Manifest{Root: root, Entries: entries}, nil
}

func hashFile(path string) (sum string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// WriteTar writes a deterministic tar of the same regular files Build would
// report, each header mode forced to 0444 with uid/gid zeroed.
func WriteTar(w io.Writer, root string) error {
	files, err := collect(root)
	if err != nil {
		return err
	}

	tw := tar.NewWriter(w)
	for _, f := range files {
		info, err := os.Stat(f.fullPath)
		if err != nil {
			return fmt.Errorf("stat %q: %w", f.fullPath, err)
		}
		hdr := &tar.Header{
			Name:    filepath.ToSlash(f.relPath),
			Size:    info.Size(),
			Mode:    0o444,
			Uid:     0,
			Gid:     0,
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar header %q: %w", f.relPath, err)
		}
		if err := copyFile(tw, f.fullPath); err != nil {
			return fmt.Errorf("tar write %q: %w", f.relPath, err)
		}
	}
	return tw.Close()
}

func copyFile(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// String renders one line per entry: "<sha256>  <size>  <path>".
func (m Manifest) String() string {
	var b strings.Builder
	for _, e := range m.Entries {
		fmt.Fprintf(&b, "%s  %d  %s\n", e.SHA256, e.Size, e.Path)
	}
	return b.String()
}
