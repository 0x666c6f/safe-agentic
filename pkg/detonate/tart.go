package detonate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var _ Runner = (*TartRunner)(nil)

// cmdRunner is a thin exec.Command wrapper so TartRunner's command building
// is swappable in tests without ever shelling out. execCmdRunner is the
// real implementation; tests use a spy.
type cmdRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execCmdRunner struct{}

func (execCmdRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, out.String())
	}
	return out.Bytes(), nil
}

// TartRunner is the real Runner backed by the `tart` CLI (cirruslabs/tart,
// Apple Virtualization.framework) plus `hdiutil` for sample staging.
//
// Network isolation is operator-provisioned, not invented here: real
// containment (no route to the internet) depends on the operator having
// already built a bridge/softnet interface with no uplink. TartRunner only
// enforces that ConfigureIsolatedNet was told about that exact,
// operator-named gateway (AllowedIsolatedGateway) and that Run() re-checks
// the resulting NetAttachment through ValidateIsolated before ever invoking
// `tart run`.
type TartRunner struct {
	cmd cmdRunner

	// WorkDir roots this runner's per-run host-side staging: sample
	// images and the artifacts directory shared into the guest.
	WorkDir string

	// AllowedIsolatedGateway is the name of the single operator-verified,
	// no-uplink network interface ConfigureIsolatedNet will accept. Left
	// empty (the zero value), every ConfigureIsolatedNet call fails
	// closed — isolation must be explicitly provisioned, never assumed.
	AllowedIsolatedGateway string

	mu   sync.Mutex
	nets map[string]NetAttachment
	// gateways captures, per run, the exact gateway string ConfigureIsolatedNet
	// validated against AllowedIsolatedGateway. Run must build its command
	// from this captured value — never from the mutable AllowedIsolatedGateway
	// field — so the value the guard checked and the value the command uses
	// can never diverge, even if AllowedIsolatedGateway changes afterward.
	gateways map[string]string
}

func NewTartRunner(workDir string) *TartRunner {
	return &TartRunner{
		cmd:      execCmdRunner{},
		WorkDir:  workDir,
		nets:     make(map[string]NetAttachment),
		gateways: make(map[string]string),
	}
}

// --- pure arg builders (unit-tested without exec) ---

func buildTartListArgs() []string {
	return []string{"list", "--format", "json"}
}

func buildTartCloneArgs(golden, run string) []string {
	return []string{"clone", golden, run}
}

func buildTartDeleteArgs(run string) []string {
	return []string{"delete", run}
}

// buildTartRunArgs builds the `tart run` argv for an isolated detonation:
// no graphics (headless), attached only to the operator-provisioned
// isolated bridge, with the host artifacts directory shared in read-write
// so the guest can drop results without any extraction step being needed.
func buildTartRunArgs(run, isolatedGateway, artifactsDir string) []string {
	return []string{"run", run, "--no-graphics", "--net-bridged=" + isolatedGateway, "--dir=out:" + artifactsDir}
}

// buildHdiutilCreateArgs builds a read-only (UDRO) disk image containing
// only the staged sample, so the sample can never be written back to.
func buildHdiutilCreateArgs(srcDir, volName, outPath string) []string {
	return []string{"create", "-srcfolder", srcDir, "-volname", volName, "-format", "UDRO", "-ov", "-o", outPath}
}

// tartListEntry is the subset of `tart list --format json` fields this
// package needs.
type tartListEntry struct {
	Name  string `json:"Name"`
	State string `json:"State"`
}

func parseTartList(output []byte) ([]tartListEntry, error) {
	var entries []tartListEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("parsing tart list output: %w", err)
	}
	return entries, nil
}

// --- Runner impl ---

func (r *TartRunner) GoldenExists(ctx context.Context, golden string) (bool, error) {
	out, err := r.cmd.Run(ctx, "tart", buildTartListArgs()...)
	if err != nil {
		return false, err
	}
	entries, err := parseTartList(out)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Name == golden {
			return true, nil
		}
	}
	return false, nil
}

func (r *TartRunner) Clone(ctx context.Context, golden, run string) error {
	_, err := r.cmd.Run(ctx, "tart", buildTartCloneArgs(golden, run)...)
	return err
}

func (r *TartRunner) ConfigureIsolatedNet(_ context.Context, run, gw string) (NetAttachment, error) {
	if r.AllowedIsolatedGateway == "" || gw != r.AllowedIsolatedGateway {
		// operator-provisioned: real isolation (no route to a real
		// uplink) is a network-level guarantee this code cannot verify
		// on its own. Fail closed until the operator sets
		// AllowedIsolatedGateway to their verified no-uplink interface.
		return NetAttachment{}, fmt.Errorf("operator-provisioned: no verified isolated gateway configured for %q (got %q) — set TartRunner.AllowedIsolatedGateway to a pre-provisioned no-uplink bridge/softnet interface first", run, gw)
	}
	n := NetAttachment{Mode: "isolated", HasUplink: false}
	r.mu.Lock()
	r.nets[run] = n
	r.gateways[run] = gw
	r.mu.Unlock()
	return n, nil
}

func (r *TartRunner) Run(ctx context.Context, run string, timeout time.Duration) error {
	r.mu.Lock()
	n, ok := r.nets[run]
	gw := r.gateways[run]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("containment violation: no network attachment configured for run %q; call ConfigureIsolatedNet first", run)
	}
	// CRITICAL: re-validate here, not just at ConfigureIsolatedNet time —
	// this is the last gate before an egress-capable command runs, and it
	// must hold even if ConfigureIsolatedNet is refactored later.
	if err := ValidateIsolated(n); err != nil {
		return err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	artifactsDir := filepath.Join(r.WorkDir, run, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o700); err != nil {
		return fmt.Errorf("preparing artifacts dir: %w", err)
	}
	// Use the gateway captured at ConfigureIsolatedNet time (gw), not
	// r.AllowedIsolatedGateway, so the value ValidateIsolated just guarded
	// is the exact value that reaches the command.
	_, err := r.cmd.Run(runCtx, "tart", buildTartRunArgs(run, gw, artifactsDir)...)
	return err
}

// InjectOffline builds a read-only disk image of the sample with hdiutil
// (a real, nameable command) and refuses to proceed unless the guest is
// confirmed powered off.
//
// operator-provisioned: attaching that built image as an extra disk to an
// existing Tart guest is NOT wired up here. Tart's exact mechanism/flag for
// attaching an additional disk to an already-cloned VM is version-dependent
// and not something this code will guess at — inventing a flag that turns
// out wrong would silently do nothing (or worse) while claiming success.
// Confirm the correct incantation against the operator's installed Tart
// version and extend this method before relying on it; until then it fails
// closed after building the image.
func (r *TartRunner) InjectOffline(ctx context.Context, run, samplePath string) error {
	off, err := r.PoweredOff(ctx, run)
	if err != nil {
		return err
	}
	if !off {
		return fmt.Errorf("inject refused: run %q is not powered off", run)
	}

	runDir := filepath.Join(r.WorkDir, run)
	stageDir := filepath.Join(runDir, "sample-src")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return fmt.Errorf("preparing sample staging dir: %w", err)
	}
	data, err := os.ReadFile(samplePath)
	if err != nil {
		return fmt.Errorf("reading sample: %w", err)
	}
	staged := filepath.Join(stageDir, filepath.Base(samplePath))
	if err := os.WriteFile(staged, data, 0o400); err != nil {
		return fmt.Errorf("staging sample: %w", err)
	}

	outDMG := filepath.Join(runDir, "sample.dmg")
	if _, err := r.cmd.Run(ctx, "hdiutil", buildHdiutilCreateArgs(stageDir, run+"-sample", outDMG)...); err != nil {
		return fmt.Errorf("building read-only sample image: %w", err)
	}

	return fmt.Errorf("operator-provisioned: built read-only sample image at %s but the guest-attach step is not configured — verify your Tart version's extra-disk mechanism and extend TartRunner.InjectOffline before use", outDMG)
}

func (r *TartRunner) Collect(ctx context.Context, run, destDir string) ([]string, error) {
	off, err := r.PoweredOff(ctx, run)
	if err != nil {
		return nil, err
	}
	if !off {
		return nil, fmt.Errorf("collect refused: run %q is not powered off", run)
	}

	srcDir := filepath.Join(r.WorkDir, run, "artifacts")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("reading artifacts dir: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return nil, fmt.Errorf("preparing dest dir: %w", err)
	}

	var collected []string
	for _, e := range entries {
		// srcDir is the guest-writable --dir=out: mount: a sample can plant
		// a symlink here (e.g. named report.json) pointing at any host path.
		// e.Type() reflects the raw directory entry, not a followed stat, so
		// it flags a symlink without ever resolving it. Skip anything that
		// isn't a plain regular file — symlinks, dirs, devices, sockets,
		// pipes — so the os.ReadFile below can never become a guest-to-host
		// file-read primitive. e.Name() is always a bare filename (no path
		// separators), so the Join below can't escape srcDir either.
		if !e.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			return collected, fmt.Errorf("reading artifact %s: %w", e.Name(), err)
		}
		dst := filepath.Join(destDir, e.Name())
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return collected, fmt.Errorf("writing artifact %s: %w", e.Name(), err)
		}
		collected = append(collected, dst)
	}
	return collected, nil
}

func (r *TartRunner) PoweredOff(ctx context.Context, run string) (bool, error) {
	out, err := r.cmd.Run(ctx, "tart", buildTartListArgs()...)
	if err != nil {
		return false, err
	}
	entries, err := parseTartList(out)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Name == run {
			return strings.EqualFold(e.State, "stopped"), nil
		}
	}
	return false, fmt.Errorf("run %q not found in tart list", run)
}

func (r *TartRunner) Destroy(ctx context.Context, run string) error {
	_, err := r.cmd.Run(ctx, "tart", buildTartDeleteArgs(run)...)
	r.mu.Lock()
	delete(r.nets, run)
	delete(r.gateways, run)
	r.mu.Unlock()
	return err
}
