package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/0x666c6f/berth/pkg/audit"
	"github.com/0x666c6f/berth/pkg/docker"
	"github.com/0x666c6f/berth/pkg/evidence"
	"github.com/0x666c6f/berth/pkg/vmexec"
)

// populateEvidenceVolume streams a tar of the evidence root into a labeled
// Docker volume. Swappable var so tests can stub it instead of spawning real
// Docker (mirrors syncBuildContextToVM in setup.go).
var populateEvidenceVolume = populateEvidenceVolumeImpl

// ingestEvidence validates hostPath, records a chain-of-custody audit entry,
// and (unless dryRun) populates a labeled Docker volume with its contents.
// Returns the evidence volume name.
func ingestEvidence(ctx context.Context, exec vmexec.Executor, vmName, containerName, hostPath, imageName string, dryRun bool) (string, error) {
	m, err := evidence.Build(hostPath)
	if err != nil {
		return "", fmt.Errorf("build evidence manifest: %w", err)
	}
	volName := containerName + "-evidence"

	// Audit write comes before any Docker work: chain of custody must survive
	// a later populate failure.
	auditLogger := &audit.Logger{Path: audit.DefaultPath()}
	auditLogger.Log("evidence-ingest", containerName, map[string]string{
		"root":  hostPath,
		"count": fmt.Sprint(len(m.Entries)),
		"files": m.String(),
	})

	if dryRun {
		return volName, nil
	}

	if err := docker.CreateLabeledVolume(ctx, exec, volName, "evidence", containerName); err != nil {
		return "", err
	}
	if err := populateEvidenceVolume(vmName, volName, imageName, hostPath); err != nil {
		return "", err
	}
	return volName, nil
}

// populateEvidenceVolumeImpl streams evidence.WriteTar(hostPath) into a
// throwaway container that extracts it into the named volume. Uses os/exec
// directly (not the Executor) because it needs a live stdin pipe; mirrors
// syncBuildContextToVMImpl in setup.go.
func populateEvidenceVolumeImpl(vmName, volName, imageName, hostPath string) error {
	// Every argument must be a single word: the VM relay word-splits a
	// multi-word arg (e.g. a `bash -c "script"` string), so use the tar
	// entrypoint with tokenized flags — the same pattern syncBuildContextToVM
	// relies on. WriteTar emits mode-0444 files (world-readable) and tar
	// creates traversable parent dirs, so no post-extract chmod is needed.
	// Run as root (-u 0): the image defaults to the non-root agent user, but a
	// fresh named volume's root is owned by the container's (userns-remapped)
	// root, so only root can write into it. Extracted files keep mode 0444
	// (world-readable), so the agent container reads them fine over its RO mount.
	cmd := exec.Command("container", "machine", "run", "-i", "-n", vmName, "-u", "root", "--",
		"docker", "run", "--rm", "-i", "-u", "0",
		"-v", volName+":/evidence-dest",
		"--entrypoint", "/bin/tar",
		imageName,
		"xf", "-", "-C", "/evidence-dest", "--no-same-owner",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start evidence populate: %w", err)
	}

	writeErrCh := make(chan error, 1)
	go func() {
		writeErrCh <- evidence.WriteTar(stdin, hostPath)
		_ = stdin.Close()
	}()

	waitErr := cmd.Wait()
	writeErr := <-writeErrCh
	if writeErr != nil {
		return writeErr
	}
	if waitErr != nil {
		return fmt.Errorf("populate evidence volume: %w\n%s", waitErr, stderr.String())
	}
	return nil
}
