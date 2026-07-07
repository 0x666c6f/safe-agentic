package svc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// localContainerName mirrors the engine's container naming so we can target a
// just-spawned agent: agent-<type>-<sanitized-name>.
func localContainerName(agent, folder string) (container, name string) {
	base := filepath.Base(filepath.Clean(folder))
	name = strings.Trim(nameSanitize.ReplaceAllString(base, "-"), "-.")
	if name == "" {
		name = "local"
	}
	return "agent-" + agent + "-" + name, name
}

// PickFolder opens the native directory picker (wired to Wails in main).
func (s *AgentService) PickFolder() (string, error) {
	if s.PickDir == nil {
		return "", fmt.Errorf("folder picker unavailable")
	}
	return s.PickDir()
}

// SpawnFromLocal spawns a blank-workspace agent and streams a local folder
// into it, so the agent works on a copy of a laptop directory (no git URL).
func (s *AgentService) SpawnFromLocal(agent, localPath string) (string, error) {
	info, err := os.Stat(localPath)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", localPath)
	}
	container, name := localContainerName(agent, localPath)
	if out, err := s.Spawn(SpawnRequest{Agent: agent, Name: name}); err != nil {
		return out, err
	}
	if err := s.waitRunning(container, 60*time.Second); err != nil {
		return "", err
	}
	if err := s.CloneLocalFolder(container, localPath); err != nil {
		return "", err
	}
	if s.State != nil {
		s.State.ProjectAdd(localPath)
	}
	if s.Poller != nil {
		s.Poller.ForceRefresh()
	}
	return fmt.Sprintf("cloned %s into %s", filepath.Base(filepath.Clean(localPath)), container), nil
}

func (s *AgentService) waitRunning(container string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, err := s.Exec.Run(ctx, "docker", "inspect", "--format", "{{.State.Running}}", container)
		cancel()
		if err == nil && strings.TrimSpace(string(out)) == "true" {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("%s did not start in time", container)
}

// CloneLocalFolder streams a host directory into <container>:/workspace/<base>
// as a gzip tar over `container machine run -i … docker exec -i` (same stdin
// relay the setup build-context sync uses). Skips .git and heavy build dirs.
func (s *AgentService) CloneLocalFolder(container, localPath string) error {
	info, err := os.Stat(localPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("not a directory: %s", localPath)
	}
	// Sanitized single-token dest — the `container machine run` relay splits
	// any arg containing spaces, so no bash -c and no spaces in paths.
	name := strings.Trim(nameSanitize.ReplaceAllString(filepath.Base(filepath.Clean(localPath)), "-"), "-.")
	if name == "" {
		name = "local"
	}
	dest := "/workspace/" + name

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if _, err := s.Exec.Run(ctx, "docker", "exec", container, "mkdir", "-p", dest); err != nil {
		return fmt.Errorf("mkdir %s: %w", dest, err)
	}
	cmd := exec.CommandContext(ctx, "container", "machine", "run", "-i",
		"-n", s.VMName, "-u", "root", "docker", "exec", "-i", container,
		"tar", "-xzf", "-", "-C", dest)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	tarErr := writeDirTar(stdin, localPath)
	stdin.Close()
	if werr := cmd.Wait(); werr != nil {
		return fmt.Errorf("stream folder: %w\n%s", werr, stderr.String())
	}
	return tarErr
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, ".venv": true, "venv": true,
	"target": true, "dist": true, "build": true, "__pycache__": true,
}

// writeDirTar gzip-tars the contents of root into w, skipping heavy build
// dirs so a copy of a working directory doesn't drag gigabytes over the relay.
func writeDirTar(w io.Writer, root string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil || rel == "." {
			return nil
		}
		if fi.IsDir() && skipDirs[fi.Name()] {
			return filepath.SkipDir
		}
		if !fi.Mode().IsRegular() && !fi.IsDir() {
			return nil // skip symlinks/devices/sockets
		}
		hdr, herr := tar.FileInfoHeader(fi, "")
		if herr != nil {
			return herr
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		f, oerr := os.Open(path)
		if oerr != nil {
			return oerr
		}
		defer f.Close()
		_, cerr := io.Copy(tw, f)
		return cerr
	})
	if cerr := tw.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if cerr := gz.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}
